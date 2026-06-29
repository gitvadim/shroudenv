package crypto

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	// Generate random 32-byte key
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	originalText := []byte("hello secret world! 12345")

	iv, ciphertext, err := Encrypt(originalText, key)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	if iv == "" || ciphertext == "" {
		t.Errorf("empty IV or ciphertext returned")
	}

	decrypted, err := Decrypt(iv, ciphertext, key)
	if err != nil {
		t.Fatalf("decryption failed: %v", err)
	}

	if !bytes.Equal(originalText, decrypted) {
		t.Errorf("decrypted text does not match original. Got %q, want %q", string(decrypted), string(originalText))
	}
}

func TestEncryptDecryptInvalidKey(t *testing.T) {
	key := make([]byte, 16) // invalid size
	_, _, err := Encrypt([]byte("data"), key)
	if err == nil {
		t.Errorf("expected error for invalid key size, got nil")
	}

	_, err = Decrypt("iv", "cipher", key)
	if err == nil {
		t.Errorf("expected error for invalid key size, got nil")
	}
}

func TestDecryptCorruptedData(t *testing.T) {
	key := make([]byte, 32)
	iv, ciphertext, err := Encrypt([]byte("data"), key)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	// Corrupt key
	badKey := make([]byte, 32)
	copy(badKey, key)
	badKey[0] ^= 0xFF

	_, err = Decrypt(iv, ciphertext, badKey)
	if err == nil {
		t.Errorf("expected decryption to fail with wrong key, got nil")
	}

	// Corrupt ciphertext
	corruptCiphertext := []byte(ciphertext)
	if len(corruptCiphertext) > 0 {
		corruptCiphertext[0] = 'a' // Change one hex character
		if corruptCiphertext[0] == ciphertext[0] {
			corruptCiphertext[0] = 'b'
		}
	}
	_, err = Decrypt(iv, string(corruptCiphertext), key)
	if err == nil {
		t.Errorf("expected decryption to fail with corrupted ciphertext, got nil")
	}
}
