package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
)

// Encrypt encrypts the plaintext using AES-256-GCM with the provided 32-byte key.
// It returns hex-encoded IV and hex-encoded ciphertext (which includes the appended tag).
func Encrypt(plaintext []byte, key []byte) (string, string, error) {
	if len(key) != 32 {
		return "", "", errors.New("key must be exactly 32 bytes (256-bit)")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", "", err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", err
	}

	// Generate a unique 12-byte IV
	iv := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", "", err
	}

	// Seal appends the tag to the ciphertext automatically.
	// We pass nil as dst so it allocates a new slice.
	ciphertextWithTag := aesGCM.Seal(nil, iv, plaintext, nil)

	return hex.EncodeToString(iv), hex.EncodeToString(ciphertextWithTag), nil
}

// Decrypt decrypts the hex-encoded ciphertext (including tag) and hex-encoded IV
// using AES-256-GCM and the provided 32-byte key.
func Decrypt(ivHex string, ciphertextHex string, key []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, errors.New("key must be exactly 32 bytes (256-bit)")
	}

	iv, err := hex.DecodeString(ivHex)
	if err != nil {
		return nil, errors.New("failed to decode IV hex: " + err.Error())
	}

	ciphertextWithTag, err := hex.DecodeString(ciphertextHex)
	if err != nil {
		return nil, errors.New("failed to decode ciphertext hex: " + err.Error())
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	if len(iv) != aesGCM.NonceSize() {
		return nil, errors.New("incorrect IV size")
	}

	plaintext, err := aesGCM.Open(nil, iv, ciphertextWithTag, nil)
	if err != nil {
		return nil, errors.New("failed to decrypt or authenticate (corrupted key or data)")
	}

	return plaintext, nil
}
