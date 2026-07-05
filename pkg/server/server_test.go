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

func TestServerBootstrap(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "shroudenv_server_bootstrap_test")
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

	// 1. Test handleBootstrapParse with valid yaml
	yamlInput := `
version: "1"
project: "testproj"
default_environment: "development"
variables:
  - name: PORT
    type: integer
    default: 8080
  - name: API_KEY
    generator:
      type: random_string
      length: 16
`
	reqParseBody := map[string]string{"yaml": yamlInput}
	reqParseBytes, _ := json.Marshal(reqParseBody)
	reqParse := httptest.NewRequest("POST", "/api/bootstrap/parse", strings.NewReader(string(reqParseBytes)))
	reqParse.Host = "localhost"
	reqParse.Header.Set("Authorization", "Bearer "+token)
	reqParse.Header.Set("Content-Type", "application/json")

	recParse := httptest.NewRecorder()
	handler.ServeHTTP(recParse, reqParse)
	if recParse.Code != http.StatusOK {
		t.Fatalf("parse failed: code %d, body: %s", recParse.Code, recParse.Body.String())
	}

	var parseResult map[string]interface{}
	if err := json.Unmarshal(recParse.Body.Bytes(), &parseResult); err != nil {
		t.Fatalf("failed to unmarshal parse result: %v", err)
	}

	if parseResult["project"] != "testproj" {
		t.Errorf("expected project 'testproj', got %v", parseResult["project"])
	}

	vars := parseResult["variables"].([]interface{})
	if len(vars) != 2 {
		t.Errorf("expected 2 variables, got %d", len(vars))
	}

	// 2. Test handleBootstrapCommit on empty env
	reqCommitBody := `{"secrets":{"PORT":"9000","API_KEY":"generatedsecret123"}}`
	reqCommit := httptest.NewRequest("POST", "/api/projects/testproj/envs/development/bootstrap", strings.NewReader(reqCommitBody))
	reqCommit.Host = "localhost"
	reqCommit.Header.Set("Authorization", "Bearer "+token)
	reqCommit.Header.Set("Content-Type", "application/json")

	recCommit := httptest.NewRecorder()
	handler.ServeHTTP(recCommit, reqCommit)
	if recCommit.Code != http.StatusOK {
		t.Fatalf("commit failed on empty env: code %d, body: %s", recCommit.Code, recCommit.Body.String())
	}

	// Verify secrets were saved
	reqGet := httptest.NewRequest("GET", "/api/projects/testproj/envs/development/secrets", nil)
	reqGet.Host = "localhost"
	reqGet.Header.Set("Authorization", "Bearer "+token)
	recGet := httptest.NewRecorder()
	handler.ServeHTTP(recGet, reqGet)
	if recGet.Code != http.StatusOK {
		t.Fatalf("failed to get secrets after bootstrap: code %d", recGet.Code)
	}
	var secrets map[string]string
	json.Unmarshal(recGet.Body.Bytes(), &secrets)
	if secrets["PORT"] != "9000" || secrets["API_KEY"] != "generatedsecret123" {
		t.Errorf("unexpected secrets: %v", secrets)
	}

	// 3. Test handleBootstrapCommit on non-empty env (should fail)
	reqCommitFail := httptest.NewRequest("POST", "/api/projects/testproj/envs/development/bootstrap", strings.NewReader(reqCommitBody))
	reqCommitFail.Host = "localhost"
	reqCommitFail.Header.Set("Authorization", "Bearer "+token)
	reqCommitFail.Header.Set("Content-Type", "application/json")

	recCommitFail := httptest.NewRecorder()
	handler.ServeHTTP(recCommitFail, reqCommitFail)
	if recCommitFail.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request when bootstrapping non-empty env, got %d. Body: %s", recCommitFail.Code, recCommitFail.Body.String())
	}

	// 4. Test handleBootstrapValidate
	validateYaml := `
version: "1"
project: "valproj"
variables:
  - name: DB_HOST
    type: string
  - name: PORT
    type: integer
    validation:
      min: 1024
`
	t.Run("Validate missing required", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"yaml":   validateYaml,
			"inputs": map[string]string{"PORT": "2000"},
		}
		bytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/bootstrap/validate", strings.NewReader(string(bytes)))
		req.Host = "localhost"
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var res struct {
			Valid  bool              `json:"valid"`
			Errors map[string]string `json:"errors"`
		}
		json.Unmarshal(rec.Body.Bytes(), &res)
		if res.Valid {
			t.Error("expected invalid for missing DB_HOST")
		}
		if res.Errors["DB_HOST"] == "" {
			t.Error("expected error message for DB_HOST")
		}
	})

	t.Run("Validate type mismatch", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"yaml":   validateYaml,
			"inputs": map[string]string{"DB_HOST": "localhost", "PORT": "abc"},
		}
		bytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/bootstrap/validate", strings.NewReader(string(bytes)))
		req.Host = "localhost"
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		var res struct {
			Valid  bool              `json:"valid"`
			Errors map[string]string `json:"errors"`
		}
		json.Unmarshal(rec.Body.Bytes(), &res)
		if res.Valid {
			t.Error("expected invalid for non-integer PORT")
		}
		if !strings.Contains(res.Errors["PORT"], "invalid integer") {
			t.Errorf("expected invalid integer error, got: %s", res.Errors["PORT"])
		}
	})

	t.Run("Validate range violation", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"yaml":   validateYaml,
			"inputs": map[string]string{"DB_HOST": "localhost", "PORT": "80"},
		}
		bytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/bootstrap/validate", strings.NewReader(string(bytes)))
		req.Host = "localhost"
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		var res struct {
			Valid  bool              `json:"valid"`
			Errors map[string]string `json:"errors"`
		}
		json.Unmarshal(rec.Body.Bytes(), &res)
		if res.Valid {
			t.Error("expected invalid for PORT < 1024")
		}
		if !strings.Contains(res.Errors["PORT"], "value must be at least") {
			t.Errorf("expected range validation error, got: %s", res.Errors["PORT"])
		}
	})

	t.Run("Validate empty required", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"yaml":   validateYaml,
			"inputs": map[string]string{"DB_HOST": "   ", "PORT": "2000"},
		}
		bytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/bootstrap/validate", strings.NewReader(string(bytes)))
		req.Host = "localhost"
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		var res struct {
			Valid  bool              `json:"valid"`
			Errors map[string]string `json:"errors"`
		}
		json.Unmarshal(rec.Body.Bytes(), &res)
		if res.Valid {
			t.Error("expected invalid for whitespace DB_HOST")
		}
		if !strings.Contains(res.Errors["DB_HOST"], "required variable is missing") {
			t.Errorf("expected required error for DB_HOST, got: %s", res.Errors["DB_HOST"])
		}
	})

	t.Run("Validate fully valid", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"yaml":   validateYaml,
			"inputs": map[string]string{"DB_HOST": "localhost", "PORT": "2000"},
		}
		bytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/bootstrap/validate", strings.NewReader(string(bytes)))
		req.Host = "localhost"
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		var res struct {
			Valid  bool              `json:"valid"`
			Errors map[string]string `json:"errors"`
		}
		json.Unmarshal(rec.Body.Bytes(), &res)
		if !res.Valid {
			t.Errorf("expected valid, got errors: %v", res.Errors)
		}
	})
}
