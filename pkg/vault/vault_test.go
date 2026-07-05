package vault

import (
	"bytes"
	"encoding/hex"
	"os"
	"testing"

	"shroudenv/pkg/keyring"
	"golang.org/x/crypto/scrypt"
)

func init() {
	// Sandbox the keyring service/account name for all tests to protect host OS credentials.
	serviceName = "shroudenv-test-sandbox"
	accountName = "master-key-test-sandbox"
	keyring.MockInit()
}

func TestGenerateRandomKey(t *testing.T) {
	key, err := GenerateRandomKey()
	if err != nil {
		t.Fatalf("failed to generate random key: %v", err)
	}
	if len(key) != 32 {
		t.Errorf("expected 32-byte key, got %d bytes", len(key))
	}
}

func TestGetMasterKeyFromEnv(t *testing.T) {
	// Ensure no env variable is leaking in
	if oldVal, exists := os.LookupEnv("SHROUDENV_MASTER_KEY"); exists {
		os.Unsetenv("SHROUDENV_MASTER_KEY")
		defer os.Setenv("SHROUDENV_MASTER_KEY", oldVal)
	}

	// 1. Test 64-character hex string env var
	expectedKey := make([]byte, 32)
	for i := range expectedKey {
		expectedKey[i] = byte(i)
	}
	hexStr := hex.EncodeToString(expectedKey)

	os.Setenv("SHROUDENV_MASTER_KEY", hexStr)
	defer os.Unsetenv("SHROUDENV_MASTER_KEY")

	// We pass nonInteractive=true to avoid terminal prompt fallback
	key, err := GetMasterKey(nil, true)
	if err != nil {
		t.Fatalf("failed to get master key from hex env: %v", err)
	}

	if !bytes.Equal(key, expectedKey) {
		t.Errorf("key from hex env does not match. Got %x, want %x", key, expectedKey)
	}

	// 2. Test raw string env var
	rawPassphrase := "my-super-secret-passphrase"
	os.Setenv("SHROUDENV_MASTER_KEY", rawPassphrase)

	salt := []byte("test-salt-1234567")
	expectedDerivedKey, err := scrypt.Key([]byte(rawPassphrase), salt, 32768, 8, 1, 32)
	if err != nil {
		t.Fatalf("failed to derive scrypt key: %v", err)
	}

	key2, err := GetMasterKey(salt, true)
	if err != nil {
		t.Fatalf("failed to get master key from raw env: %v", err)
	}

	if !bytes.Equal(key2, expectedDerivedKey) {
		t.Errorf("key from raw env does not match. Got %x, want %x", key2, expectedDerivedKey)
	}

	// 3. Test error when salt is missing
	_, err = GetMasterKey(nil, true)
	if err == nil {
		t.Errorf("expected error when salt is missing for raw env var")
	}
}

func TestGetMasterKeyFromKeyring(t *testing.T) {
	// Clear env variable if present to force keyring path
	if oldVal, exists := os.LookupEnv("SHROUDENV_MASTER_KEY"); exists {
		os.Unsetenv("SHROUDENV_MASTER_KEY")
		defer os.Setenv("SHROUDENV_MASTER_KEY", oldVal)
	}

	// Make sure keyring starts clean
	_ = keyring.Delete(serviceName, accountName)
	defer keyring.Delete(serviceName, accountName)

	testKey := make([]byte, 32)
	for i := range testKey {
		testKey[i] = byte(i + 15)
	}

	// Store key
	err := SetMasterKey(testKey)
	if err != nil {
		t.Fatalf("failed to set master key in keyring: %v", err)
	}

	// Retrieve key
	gotKey, err := GetMasterKey(nil, true)
	if err != nil {
		t.Fatalf("failed to get master key from keyring: %v", err)
	}

	if !bytes.Equal(gotKey, testKey) {
		t.Errorf("got key from keyring %x, want %x", gotKey, testKey)
	}
}

func TestGetMasterKeyNonInteractiveError(t *testing.T) {
	// Temporarily clear env var if present
	if oldVal, exists := os.LookupEnv("SHROUDENV_MASTER_KEY"); exists {
		os.Unsetenv("SHROUDENV_MASTER_KEY")
		defer os.Setenv("SHROUDENV_MASTER_KEY", oldVal)
	}

	// Make sure keyring is empty
	_ = keyring.Delete(serviceName, accountName)

	// Since we are running in non-interactive mode and keyring is empty/env is cleared, this must fail
	_, err := GetMasterKey(nil, true)
	if err == nil {
		t.Errorf("expected non-interactive fallback to fail when keyring and env are empty")
	}
}

