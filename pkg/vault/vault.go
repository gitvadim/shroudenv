package vault

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"

	"shroudenv/pkg/keyring"
	"golang.org/x/crypto/scrypt"
	"golang.org/x/term"
)

var (
	serviceName = "shroudenv"
	accountName = "master-key"
)

// GenerateRandomKey generates a cryptographically secure 32-byte key.
func GenerateRandomKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	return key, nil
}

// SetMasterKey stores the master key in the OS keyring.
func SetMasterKey(key []byte) error {
	if len(key) != 32 {
		return errors.New("master key must be exactly 32 bytes")
	}
	hexKey := hex.EncodeToString(key)
	return keyring.Set(serviceName, accountName, hexKey)
}

// GetMasterKeyFromKeyring retrieves the master key from the OS keyring/vault.
// It returns an error if the key is not found or is invalid.
func GetMasterKeyFromKeyring() ([]byte, error) {
	hexKey, err := keyring.Get(serviceName, accountName)
	if err != nil {
		return nil, err
	}
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode master key from keyring: %w", err)
	}
	if len(key) != 32 {
		return nil, errors.New("master key in keyring must be exactly 32 bytes")
	}
	return key, nil
}

// GetMasterKey attempts to retrieve the master key in this order:
// 1. OS Keyring/Vault
// 2. Interactive terminal prompt using scrypt and the provided salt.
func GetMasterKey(salt []byte) ([]byte, error) {
	// 1. Try OS Keyring
	if key, err := GetMasterKeyFromKeyring(); err == nil {
		return key, nil
	}

	// 2. Try Interactive Prompt
	// Check if stdin is a terminal
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil, errors.New("master key not found and stdin is not a terminal (cannot prompt for password)")
	}

	if len(salt) == 0 {
		return nil, errors.New("cannot derive master key from password without database salt")
	}

	fmt.Print("Enter shroudenv Master Password: ")
	bytePassword, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println() // Print newline after password entry
	if err != nil {
		return nil, fmt.Errorf("failed to read password: %w", err)
	}

	if len(bytePassword) == 0 {
		return nil, errors.New("password cannot be empty")
	}

	// Derive key using scrypt
	key, err := scrypt.Key(bytePassword, salt, 32768, 8, 1, 32)
	if err != nil {
		return nil, fmt.Errorf("failed to derive key using scrypt: %w", err)
	}

	return key, nil
}

