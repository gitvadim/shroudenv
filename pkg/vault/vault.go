package vault

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/zalando/go-keyring"
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

// GetMasterKey attempts to retrieve the master key in this order:
// 1. SHROUDENV_MASTER_KEY environment variable
// 2. OS Keyring
// 3. Interactive terminal prompt (unless nonInteractive is true) using scrypt and the provided salt.
func GetMasterKey(salt []byte, nonInteractive bool) ([]byte, error) {
	// 1. Try Environment Variable
	if envVal := os.Getenv("SHROUDENV_MASTER_KEY"); envVal != "" {
		// If it's a 64-char hex string, decode it
		if len(envVal) == 64 {
			if key, err := hex.DecodeString(envVal); err == nil && len(key) == 32 {
				return key, nil
			}
		}
		// Otherwise, derive a 32-byte key from the environment variable using scrypt and salt
		if len(salt) == 0 {
			return nil, errors.New("cannot derive master key from password without database salt")
		}
		key, err := scrypt.Key([]byte(envVal), salt, 32768, 8, 1, 32)
		if err != nil {
			return nil, fmt.Errorf("failed to derive key using scrypt: %w", err)
		}
		return key, nil
	}

	// 2. Try OS Keyring
	if hexKey, err := keyring.Get(serviceName, accountName); err == nil {
		if key, err := hex.DecodeString(hexKey); err == nil && len(key) == 32 {
			return key, nil
		}
	}

	// 3. Try Interactive Prompt
	if nonInteractive {
		return nil, errors.New("master key not found (keyring empty, env var absent, and running in non-interactive mode)")
	}

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

