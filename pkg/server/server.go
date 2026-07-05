package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	"shroudenv/pkg/bootstrap"
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
	mux.HandleFunc("POST /api/bootstrap/parse", s.handleBootstrapParse)
	mux.HandleFunc("POST /api/bootstrap/validate", s.handleBootstrapValidate)
	mux.HandleFunc("POST /api/projects/{project}/envs/{env}/bootstrap", s.handleBootstrapCommit)

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

type validationJSON struct {
	Min          *float64 `json:"min,omitempty"`
	Max          *float64 `json:"max,omitempty"`
	Enum         []string `json:"enum,omitempty"`
	Pattern      string   `json:"pattern,omitempty"`
	ErrorMessage string   `json:"error_message,omitempty"`
}

type resolvedVariableJSON struct {
	Name             string          `json:"name"`
	Description      string          `json:"description,omitempty"`
	Type             string          `json:"type,omitempty"`
	Prompt           string          `json:"prompt,omitempty"`
	Sensitive        bool            `json:"sensitive,omitempty"`
	Optional         bool            `json:"optional,omitempty"`
	Validation       *validationJSON `json:"validation,omitempty"`
	PreResolvedValue string          `json:"pre_resolved_value"`
	IsGenerated      bool            `json:"is_generated"`
}

func (s *Server) handleBootstrapParse(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Yaml string `json:"yaml"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	config, err := bootstrap.ParseConfig([]byte(body.Yaml))
	if err != nil {
		http.Error(w, "failed to parse config: "+err.Error(), http.StatusBadRequest)
		return
	}

	// 1. Generate secrets for generator variables
	resolved, err := bootstrap.PreResolve(config)
	if err != nil {
		http.Error(w, "failed to pre-resolve generators: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Read OS environment variables for fallback checks
	osEnv := make(map[string]string)
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			osEnv[parts[0]] = parts[1]
		}
	}

	// Map domain objects to transport-specific serialization JSON structs
	jsonVars := make([]resolvedVariableJSON, 0, len(config.Variables))
	for _, v := range config.Variables {
		var validationPtr *validationJSON
		if v.Validation != nil {
			validationPtr = &validationJSON{
				Min:          v.Validation.Min,
				Max:          v.Validation.Max,
				Enum:         v.Validation.Enum,
				Pattern:      v.Validation.Pattern,
				ErrorMessage: v.Validation.ErrorMessage,
			}
		}

		var preResolvedValue string
		var isGenerated bool

		if v.Generator != nil {
			preResolvedValue = resolved[v.Name]
			isGenerated = true
		} else {
			isGenerated = false
			var val string
			// Check fallback
			if v.Fallback != "" {
				if envVal, exists := osEnv[v.Fallback]; exists && envVal != "" {
					val = envVal
				}
			}
			// Check default
			if val == "" && v.Default != nil {
				defaultStr := fmt.Sprintf("%v", v.Default)
				interpolated, err := bootstrap.Interpolate(defaultStr, resolved)
				if err == nil {
					val = interpolated
				}
			}
			preResolvedValue = val
			// Add to resolved map so subsequent variables can interpolate it if needed
			resolved[v.Name] = val
		}

		jsonVars = append(jsonVars, resolvedVariableJSON{
			Name:             v.Name,
			Description:      v.Description,
			Type:             v.Type,
			Prompt:           v.Prompt,
			Sensitive:        v.Sensitive,
			Optional:         v.Optional,
			Validation:       validationPtr,
			PreResolvedValue: preResolvedValue,
			IsGenerated:      isGenerated,
		})
	}

	res := map[string]interface{}{
		"project":             config.Project,
		"default_environment": config.DefaultEnvironment,
		"variables":           jsonVars,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func (s *Server) handleBootstrapCommit(w http.ResponseWriter, r *http.Request) {
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
	if p != nil {
		e := p.GetEnvironment(envName)
		if e != nil {
			// Decrypt existing secrets to check if they are empty
			existingSecrets, err := e.GetSecrets(s.masterKey)
			if err != nil {
				http.Error(w, "failed to decrypt existing secrets: "+err.Error(), http.StatusInternalServerError)
				return
			}
			if len(existingSecrets) > 0 {
				http.Error(w, "cannot bootstrap environment: environment is not empty", http.StatusBadRequest)
				return
			}
		}
	}

	// Auto-create project if missing
	if p == nil {
		if err := database.CreateProject(projectName); err != nil {
			http.Error(w, "failed to create project: "+err.Error(), http.StatusInternalServerError)
			return
		}
		p = database.GetProject(projectName)
	}

	// Auto-create environment if missing
	e := p.GetEnvironment(envName)
	if e == nil {
		if err := database.CreateEnvironment(projectName, envName); err != nil {
			http.Error(w, "failed to create environment: "+err.Error(), http.StatusInternalServerError)
			return
		}
		e = p.GetEnvironment(envName)
	}

	// F-03: explicit nil-guard after environment creation
	if e == nil {
		http.Error(w, "internal error: environment not found after creation", http.StatusInternalServerError)
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
	json.NewEncoder(w).Encode(map[string]string{"message": "Environment bootstrapped successfully"})
}

func (s *Server) handleBootstrapValidate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Yaml   string            `json:"yaml"`
		Inputs map[string]string `json:"inputs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	config, err := bootstrap.ParseConfig([]byte(body.Yaml))
	if err != nil {
		http.Error(w, "failed to parse config: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Read OS environment variables for fallback checks
	osEnv := make(map[string]string)
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			osEnv[parts[0]] = parts[1]
		}
	}

	// Run validation using bootstrap.Resolve
	_, err = bootstrap.Resolve(config, body.Inputs, osEnv)

	errorsMap := make(map[string]string)
	valid := true

	if err != nil {
		valid = false
		if valErr, ok := err.(*bootstrap.ValidationErrorMap); ok {
			for k, vErr := range valErr.Errors {
				errorsMap[k] = vErr.Error()
			}
		} else {
			// Other general resolution error (e.g. interpolation error)
			errorsMap["_general"] = err.Error()
		}
	}

	res := map[string]interface{}{
		"valid":  valid,
		"errors": errorsMap,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}
