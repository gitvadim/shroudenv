package server

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"

	"shroudenv/pkg/db"
)

type Server struct {
	dbPath    string
	masterKey []byte
	token     string
	staticFS  embed.FS
	mu        sync.RWMutex
}


func NewServer(dbPath string, masterKey []byte, token string, staticFS embed.FS) *Server {
	return &Server{
		dbPath:    dbPath,
		masterKey: masterKey,
		token:     token,
		staticFS:  staticFS,
	}
}

// HostHeaderMiddleware validates the incoming Host header to protect against DNS rebinding.
func (s *Server) HostHeaderMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		// Allow localhost and 127.0.0.1
		if host != "localhost" && host != "127.0.0.1" {
			http.Error(w, "Forbidden - DNS Rebinding Protection", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// TokenAuthMiddleware enforces API token validation.
func (s *Server) TokenAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Bypass token auth for non-API requests (static files)
		if !strings.HasPrefix(r.URL.Path, "/api") {
			next.ServeHTTP(w, r)
			return
		}

		tokenHeader := r.Header.Get("Authorization")
		var reqToken string
		if len(tokenHeader) > 7 && strings.HasPrefix(tokenHeader, "Bearer ") {
			reqToken = tokenHeader[7:]
		} else {
			reqToken = r.URL.Query().Get("token")
		}

		if reqToken != s.token {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// CorsMiddleware injects CORS headers for local development.
func (s *Server) CorsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		// Only allow specific local Vite dev server origin
		if origin == "http://localhost:5173" || origin == "http://127.0.0.1:5173" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		}
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// SecurityHeadersMiddleware injects security-hardening headers.
func (s *Server) SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; font-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; img-src 'self' data:;")
		next.ServeHTTP(w, r)
	})
}

// Handler returns the HTTP handler for the server.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// API Endpoints
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/projects", s.handleListProjects)
	mux.HandleFunc("POST /api/projects", s.handleCreateProject)
	mux.HandleFunc("POST /api/projects/{project}/envs", s.handleCreateEnvironment)
	mux.HandleFunc("GET /api/projects/{project}/envs/{env}/secrets", s.handleGetSecrets)
	mux.HandleFunc("POST /api/projects/{project}/envs/{env}/secrets", s.handleSetSecrets)

	// Static assets handler
	subFS, err := fs.Sub(s.staticFS, "frontend/dist")
	if err != nil {
		// Fallback if sub fails (should not happen if files exist)
		mux.Handle("/", http.NotFoundHandler())
	} else {
		fileServer := http.FileServer(http.FS(subFS))
		mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			// Serve embedded static files
			// If path is a subdirectory or not found, let FileServer handle it (or serve index.html for SPA)
			fileServer.ServeHTTP(w, r)
		})
	}

	// Apply middlewares
	var handler http.Handler = mux
	handler = s.CorsMiddleware(handler)
	handler = s.TokenAuthMiddleware(handler)
	handler = s.HostHeaderMiddleware(handler)
	handler = s.SecurityHeadersMiddleware(handler)

	return handler
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	flock, err := db.LockShared(s.dbPath)
	if err != nil {
		http.Error(w, "failed to lock database: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer flock.Unlock()

	database, err := db.LoadDatabase(s.dbPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	res := map[string]interface{}{
		"db_path":        s.dbPath,
		"projects_count": len(database.Projects),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	flock, err := db.LockShared(s.dbPath)
	if err != nil {
		http.Error(w, "failed to lock database: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer flock.Unlock()

	database, err := db.LoadDatabase(s.dbPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type EnvRes struct {
		Name       string `json:"name"`
		HasSecrets bool   `json:"has_secrets"`
	}
	type ProjRes struct {
		Name         string   `json:"name"`
		Environments []EnvRes `json:"environments"`
	}

	projects := make([]ProjRes, 0, len(database.Projects))
	for _, p := range database.Projects {
		envs := make([]EnvRes, 0, len(p.Environments))
		for _, e := range p.Environments {
			envs = append(envs, EnvRes{
				Name:       e.Name,
				HasSecrets: e.Secrets != nil && e.Secrets.Ciphertext != "",
			})
		}
		projects = append(projects, ProjRes{
			Name:         p.Name,
			Environments: envs,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(projects)
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	flock, err := db.LockExclusive(s.dbPath)
	if err != nil {
		http.Error(w, "failed to lock database: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer flock.Unlock()

	database, err := db.LoadDatabase(s.dbPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := database.CreateProject(body.Name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := db.SaveDatabase(s.dbPath, database); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"message": "Project created successfully"})
}

func (s *Server) handleCreateEnvironment(w http.ResponseWriter, r *http.Request) {
	projectName := r.PathValue("project")
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	flock, err := db.LockExclusive(s.dbPath)
	if err != nil {
		http.Error(w, "failed to lock database: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer flock.Unlock()

	database, err := db.LoadDatabase(s.dbPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := database.CreateEnvironment(projectName, body.Name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := db.SaveDatabase(s.dbPath, database); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"message": "Environment created successfully"})
}

func (s *Server) handleGetSecrets(w http.ResponseWriter, r *http.Request) {
	projectName := r.PathValue("project")
	envName := r.PathValue("env")

	s.mu.RLock()
	defer s.mu.RUnlock()

	flock, err := db.LockShared(s.dbPath)
	if err != nil {
		http.Error(w, "failed to lock database: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer flock.Unlock()

	database, err := db.LoadDatabase(s.dbPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	p := database.GetProject(projectName)
	if p == nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	e := p.GetEnvironment(envName)
	if e == nil {
		http.Error(w, "environment not found", http.StatusNotFound)
		return
	}

	secrets, err := e.GetSecrets(s.masterKey)
	if err != nil {
		http.Error(w, "failed to decrypt secrets: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(secrets)
}

func (s *Server) handleSetSecrets(w http.ResponseWriter, r *http.Request) {
	projectName := r.PathValue("project")
	envName := r.PathValue("env")

	var body struct {
		Secrets map[string]string `json:"secrets"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	flock, err := db.LockExclusive(s.dbPath)
	if err != nil {
		http.Error(w, "failed to lock database: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer flock.Unlock()

	database, err := db.LoadDatabase(s.dbPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	p := database.GetProject(projectName)
	if p == nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	e := p.GetEnvironment(envName)
	if e == nil {
		http.Error(w, "environment not found", http.StatusNotFound)
		return
	}

	if err := e.SetSecrets(body.Secrets, s.masterKey); err != nil {
		http.Error(w, "failed to encrypt secrets: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := db.SaveDatabase(s.dbPath, database); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Secrets updated successfully"})
}
