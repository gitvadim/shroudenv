package bootstrap

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"shroudenv/pkg/db"

	"shroudenv/pkg/keyring"
)

func init() {
	keyring.MockInit()
}

type MockInputReader struct {
	Inputs []string
	Index  int
}

func (r *MockInputReader) ReadInput(prompt string) (string, error) {
	if r.Index >= len(r.Inputs) {
		return "", errors.New("no more inputs")
	}
	val := r.Inputs[r.Index]
	r.Index++
	return val, nil
}

func (r *MockInputReader) ReadSensitiveInput(prompt string) (string, error) {
	return r.ReadInput(prompt)
}

func TestBootstrapRunner_InteractiveSuccess(t *testing.T) {
	yamlContent := `
version: "1"
project: "runner-test-app"
default_environment: "development"
variables:
  - name: PORT
    type: integer
    default: 3000
  - name: DB_PASSWORD
    generator:
      type: random_string
      length: 8
`
	tmpFile, err := os.CreateTemp("", "shroudenv-test-config-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.Write([]byte(yamlContent)); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	tmpFile.Close()

	// Inline temp DB file path setup (N-01: prevents testutil from compiling to production)
	tmpDB, err := os.CreateTemp("", "shroudenv-test-db-*.json")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpDBPath := tmpDB.Name()
	tmpDB.Close()
	os.Remove(tmpDBPath) // delete so it starts fresh/non-existent
	defer os.Remove(tmpDBPath)

	// Initialize DB structure with salt
	database, err := db.LoadDatabase(tmpDBPath)
	if err != nil {
		t.Fatalf("failed to load db: %v", err)
	}
	err = db.SaveDatabase(tmpDBPath, database)
	if err != nil {
		t.Fatalf("failed to save db: %v", err)
	}

	var stdout, stderr bytes.Buffer
	mockReader := &MockInputReader{Inputs: []string{"8080"}}
	mockKey := make([]byte, 32) // 32-byte key

	runner := &BootstrapRunner{
		EnvVars:        make(map[string]string),
		Stdout:         &stdout,
		Stderr:         &stderr,
		InputReader:    mockReader,
		DBPath:         tmpDBPath,
		MasterKey:      mockKey,
		DryRun:         false,
		NonInteractive: false,
	}

	err = runner.Run(tmpFile.Name(), "")
	if err != nil {
		t.Fatalf("unexpected error from runner.Run: %v", err)
	}

	// Verify database was written
	reloaded, err := db.LoadDatabase(tmpDBPath)
	if err != nil {
		t.Fatalf("failed to reload db: %v", err)
	}

	p := reloaded.GetProject("runner-test-app")
	if p == nil {
		t.Fatal("expected project runner-test-app to exist")
	}

	e := p.GetEnvironment("development")
	if e == nil {
		t.Fatal("expected env development to exist")
	}

	secrets, err := e.GetSecrets(mockKey)
	if err != nil {
		t.Fatalf("failed to decrypt secrets: %v", err)
	}

	if secrets["PORT"] != "8080" {
		t.Errorf("expected PORT to be 8080, got %q", secrets["PORT"])
	}
	if len(secrets["DB_PASSWORD"]) != 8 {
		t.Errorf("expected DB_PASSWORD length 8, got %q", secrets["DB_PASSWORD"])
	}
}

func TestBootstrapRunner_NonInteractiveValidationFailure(t *testing.T) {
	yamlContent := `
version: "1"
project: "runner-test-app"
variables:
  - name: PORT
    type: integer
    default: 99
    validation:
      min: 1024
`
	tmpFile, err := os.CreateTemp("", "shroudenv-test-config-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.Write([]byte(yamlContent)); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	tmpFile.Close()

	// Inline temp DB file path setup (N-01: prevents testutil from compiling to production)
	tmpDB, err := os.CreateTemp("", "shroudenv-test-db-*.json")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpDBPath := tmpDB.Name()
	tmpDB.Close()
	os.Remove(tmpDBPath) // delete so it starts fresh/non-existent
	defer os.Remove(tmpDBPath)

	// Initialize DB structure with salt
	database, err := db.LoadDatabase(tmpDBPath)
	if err != nil {
		t.Fatalf("failed to load db: %v", err)
	}
	err = db.SaveDatabase(tmpDBPath, database)
	if err != nil {
		t.Fatalf("failed to save db: %v", err)
	}

	var stdout, stderr bytes.Buffer
	mockKey := make([]byte, 32)

	runner := &BootstrapRunner{
		Stdout:         &stdout,
		Stderr:         &stderr,
		DBPath:         tmpDBPath,
		MasterKey:      mockKey,
		NonInteractive: true,
	}

	err = runner.Run(tmpFile.Name(), "development")
	if err == nil {
		t.Fatal("expected error from non-interactive run with validation failure, got nil")
	}

	if !strings.Contains(err.Error(), "validation failed") && !strings.Contains(err.Error(), "scaffolding aborted") {
		t.Errorf("expected error message to contain validation failure, got %v", err)
	}

	// Verify database was NOT written (no project created)
	reloaded, err := db.LoadDatabase(tmpDBPath)
	if err != nil {
		t.Fatalf("failed to reload db: %v", err)
	}
	if len(reloaded.Projects) != 0 {
		t.Error("database should not have been saved on validation failure")
	}
}
