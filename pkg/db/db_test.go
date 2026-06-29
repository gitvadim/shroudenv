package db

import (
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestDatabaseOperations(t *testing.T) {
	// Setup a temporary DB file
	tmpDir, err := os.MkdirTemp("", "shroudenv_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "db.json")

	// 1. Load empty database (non-existent file)
	db, err := LoadDatabase(dbPath)
	if err != nil {
		t.Fatalf("LoadDatabase failed: %v", err)
	}

	if len(db.Salt) != 32 { // 16 bytes encoded to hex = 32 chars
		t.Errorf("expected 32-char hex salt, got %q", db.Salt)
	}

	if len(db.Projects) != 0 {
		t.Errorf("expected 0 projects initially, got %d", len(db.Projects))
	}

	// 2. Create project
	err = db.CreateProject("MyProject")
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	if db.GetProject("MyProject") == nil {
		t.Errorf("project MyProject not found")
	}

	// Case insensitivity check
	if db.GetProject("myproject") == nil {
		t.Errorf("project lookup should be case insensitive")
	}

	// 3. Create environment
	err = db.CreateEnvironment("MyProject", "development")
	if err != nil {
		t.Fatalf("CreateEnvironment failed: %v", err)
	}

	p := db.GetProject("MyProject")
	env := p.GetEnvironment("development")
	if env == nil {
		t.Errorf("environment development not found")
	}

	if p.GetEnvironment("DEVELOPMENT") == nil {
		t.Errorf("environment lookup should be case insensitive")
	}

	// 4. Save and Reload
	err = SaveDatabase(dbPath, db)
	if err != nil {
		t.Fatalf("SaveDatabase failed: %v", err)
	}

	db2, err := LoadDatabase(dbPath)
	if err != nil {
		t.Fatalf("reloading database failed: %v", err)
	}

	if db2.Salt != db.Salt {
		t.Errorf("salt mismatch after reload. Got %q, want %q", db2.Salt, db.Salt)
	}

	if len(db2.Projects) != 1 || db2.Projects[0].Name != "MyProject" {
		t.Errorf("project structure lost or modified on reload")
	}
}

func TestEncryptDecryptSecrets(t *testing.T) {
	db := &Database{
		Salt:     "somesalt",
		Projects: []Project{},
	}
	db.CreateProject("proj")
	db.CreateEnvironment("proj", "dev")

	p := db.GetProject("proj")
	env := p.GetEnvironment("dev")

	// Generate 32-byte key
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	// Get secrets initially
	sec, err := env.GetSecrets(key)
	if err != nil {
		t.Fatalf("GetSecrets failed: %v", err)
	}
	if len(sec) != 0 {
		t.Errorf("expected empty secrets map, got %v", sec)
	}

	// Set secrets
	newSec := map[string]string{
		"API_KEY": "secret123",
		"PORT":    "8080",
	}

	err = env.SetSecrets(newSec, key)
	if err != nil {
		t.Fatalf("SetSecrets failed: %v", err)
	}

	if env.Secrets == nil || env.Secrets.Ciphertext == "" {
		t.Fatalf("secrets field empty after encryption")
	}

	// Retrieve secrets
	decryptedSec, err := env.GetSecrets(key)
	if err != nil {
		t.Fatalf("failed to decrypt secrets: %v", err)
	}

	if decryptedSec["API_KEY"] != "secret123" || decryptedSec["PORT"] != "8080" {
		t.Errorf("decrypted secrets mismatch. Got %v", decryptedSec)
	}

	// Attempt decrypt with bad key
	badKey := make([]byte, 32)
	copy(badKey, key)
	badKey[0] ^= 0xAB

	_, err = env.GetSecrets(badKey)
	if err == nil {
		t.Errorf("expected decryption to fail with incorrect key")
	}
}

func TestLockIdempotency(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "shroudenv_lock_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "db.json")

	// Acquire exclusive lock
	lock, err := LockExclusive(dbPath)
	if err != nil {
		t.Fatalf("failed to acquire exclusive lock: %v", err)
	}

	// First unlock should succeed
	err = lock.Unlock()
	if err != nil {
		t.Errorf("expected first unlock to succeed, got: %v", err)
	}

	// Second unlock (double-unlock) should not error out or panic
	err = lock.Unlock()
	if err != nil {
		t.Errorf("expected second unlock to be a no-op and succeed, got: %v", err)
	}
}

