package server

import (
	"embed"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"shroudenv/pkg/db"
)

// Dummy embed for testing
//go:embed server.go
var testFS embed.FS

func TestServerMiddlewares(t *testing.T) {
	// Setup a temporary DB file
	tmpDir, err := os.MkdirTemp("", "shroudenv_server_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "db.json")

	// Init DB
	database, _ := db.LoadDatabase(dbPath)
	db.SaveDatabase(dbPath, database)

	key := make([]byte, 32)
	token := "secret-test-token"

	srv := NewServer(dbPath, key, token, testFS)
	handler := srv.Handler()

	t.Run("Valid Host and Token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/status", nil)
		req.Host = "localhost:4554"
		req.Header.Set("Authorization", "Bearer "+token)

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d. Body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("Security Headers and CORS", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/status", nil)
		req.Host = "localhost:4554"
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Origin", "http://localhost:5173")

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		// Verify security headers
		if rec.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("expected Referrer-Policy 'no-referrer', got '%s'", rec.Header().Get("Referrer-Policy"))
		}
		if rec.Header().Get("X-Frame-Options") != "DENY" {
			t.Errorf("expected X-Frame-Options 'DENY', got '%s'", rec.Header().Get("X-Frame-Options"))
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("expected X-Content-Type-Options 'nosniff', got '%s'", rec.Header().Get("X-Content-Type-Options"))
		}
		if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "default-src 'self'") {
			t.Errorf("expected CSP default-src 'self', got '%s'", rec.Header().Get("Content-Security-Policy"))
		}

		// Verify CORS for allowed origin
		if rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
			t.Errorf("expected Access-Control-Allow-Origin 'http://localhost:5173', got '%s'", rec.Header().Get("Access-Control-Allow-Origin"))
		}

		// Verify CORS is blocked (not set) for untrusted origin
		reqBlocked := httptest.NewRequest("GET", "/api/status", nil)
		reqBlocked.Host = "localhost:4554"
		reqBlocked.Header.Set("Authorization", "Bearer "+token)
		reqBlocked.Header.Set("Origin", "http://evil-attacker.com")

		recBlocked := httptest.NewRecorder()
		handler.ServeHTTP(recBlocked, reqBlocked)

		if recBlocked.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("expected empty Access-Control-Allow-Origin for unauthorized origin, got '%s'", recBlocked.Header().Get("Access-Control-Allow-Origin"))
		}
	})

	t.Run("Invalid Host DNS Rebinding Block", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/status", nil)
		req.Host = "evil-domain.com"
		req.Header.Set("Authorization", "Bearer "+token)

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden, got %d", rec.Code)
		}
	})

	t.Run("Invalid Token Block", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/status", nil)
		req.Host = "localhost"
		req.Header.Set("Authorization", "Bearer wrong-token")

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", rec.Code)
		}
	})

	t.Run("Allow Static Assets Bypass Token Auth", func(t *testing.T) {
		// Static assets shouldn't need a token, though they are subject to Host header check
		req := httptest.NewRequest("GET", "/index.html", nil)
		req.Host = "127.0.0.1:4554"

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		// It will return 404 (not found) since index.html is not in the "frontend/dist" root of the testFS,
		// but it should NOT return 401 Unauthorized.
		if rec.Code == http.StatusUnauthorized {
			t.Errorf("expected non-401 for static file request, got %d", rec.Code)
		}
	})
}

func TestServerCRUD(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "shroudenv_server_crud_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "db.json")
	database, _ := db.LoadDatabase(dbPath)
	db.SaveDatabase(dbPath, database)

	key := make([]byte, 32)
	token := "testtoken"
	srv := NewServer(dbPath, key, token, testFS)
	handler := srv.Handler()

	// 1. Create project
	reqBody := `{"name":"ProjectX"}`
	req := httptest.NewRequest("POST", "/api/projects", strings.NewReader(reqBody))
	req.Host = "localhost"
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("failed to create project: %d, body: %s", rec.Code, rec.Body.String())
	}

	// 2. Create Env
	reqBodyEnv := `{"name":"prod"}`
	reqEnv := httptest.NewRequest("POST", "/api/projects/ProjectX/envs", strings.NewReader(reqBodyEnv))
	reqEnv.Host = "localhost"
	reqEnv.Header.Set("Authorization", "Bearer "+token)
	reqEnv.Header.Set("Content-Type", "application/json")

	recEnv := httptest.NewRecorder()
	handler.ServeHTTP(recEnv, reqEnv)
	if recEnv.Code != http.StatusCreated {
		t.Fatalf("failed to create env: %d, body: %s", recEnv.Code, recEnv.Body.String())
	}

	// 3. Set secrets
	reqBodySecrets := `{"secrets":{"DB_PASS":"super-secret-123"}}`
	reqSec := httptest.NewRequest("POST", "/api/projects/ProjectX/envs/prod/secrets", strings.NewReader(reqBodySecrets))
	reqSec.Host = "localhost"
	reqSec.Header.Set("Authorization", "Bearer "+token)
	reqSec.Header.Set("Content-Type", "application/json")

	recSec := httptest.NewRecorder()
	handler.ServeHTTP(recSec, reqSec)
	if recSec.Code != http.StatusOK {
		t.Fatalf("failed to set secrets: %d, body: %s", recSec.Code, recSec.Body.String())
	}

	// 4. Get secrets
	reqGetSec := httptest.NewRequest("GET", "/api/projects/ProjectX/envs/prod/secrets", nil)
	reqGetSec.Host = "localhost"
	reqGetSec.Header.Set("Authorization", "Bearer "+token)

	recGetSec := httptest.NewRecorder()
	handler.ServeHTTP(recGetSec, reqGetSec)
	if recGetSec.Code != http.StatusOK {
		t.Fatalf("failed to get secrets: %d", recGetSec.Code)
	}

	var secrets map[string]string
	json.Unmarshal(recGetSec.Body.Bytes(), &secrets)
	if secrets["DB_PASS"] != "super-secret-123" {
		t.Errorf("expected DB_PASS to be 'super-secret-123', got %s", secrets["DB_PASS"])
	}
}
