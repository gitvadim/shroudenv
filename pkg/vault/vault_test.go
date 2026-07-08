package vault

import (
	"bytes"
	"testing"

	"shroudenv/pkg/keyring"
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

func TestGetMasterKeyFromKeyring(t *testing.T) {
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

	// Retrieve key using GetMasterKeyFromKeyring
	gotKey, err := GetMasterKeyFromKeyring()
	if err != nil {
		t.Fatalf("failed to get master key from keyring: %v", err)
	}

	if !bytes.Equal(gotKey, testKey) {
		t.Errorf("got key from keyring %x, want %x", gotKey, testKey)
	}

	// Retrieve key using GetMasterKey
	gotKey2, err := GetMasterKey(nil)
	if err != nil {
		t.Fatalf("failed to get master key: %v", err)
	}

	if !bytes.Equal(gotKey2, testKey) {
		t.Errorf("got key %x, want %x", gotKey2, testKey)
	}
}

