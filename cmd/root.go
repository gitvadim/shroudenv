package cmd

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"shroudenv/pkg/db"
	"shroudenv/pkg/vault"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	dbPathFlag         string
	nonInteractiveFlag bool
)

var Version = "dev"

var RootCmd = &cobra.Command{
	Use:     "shroudenv",
	Short:   "shroudenv is a secure, local-first secret management tool",
	Long:    `shroudenv is an offline/local-first CLI and GUI tool for developers to securely manage environment secrets.`,
	Version: Version,
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	RootCmd.PersistentFlags().StringVarP(&dbPathFlag, "db-path", "d", "", "Path to the local db.json file")
	RootCmd.PersistentFlags().BoolVar(&nonInteractiveFlag, "non-interactive", false, "Run in non-interactive mode (no prompt fallbacks)")
	// Support --ci as an alias for non-interactive
	RootCmd.PersistentFlags().BoolVar(&nonInteractiveFlag, "ci", false, "Run in non-interactive mode (no prompt fallbacks)")
	_ = RootCmd.PersistentFlags().MarkHidden("ci")
}

// GetDBPath resolves the database file path.
func GetDBPath() (string, error) {
	if dbPathFlag != "" {
		return dbPathFlag, nil
	}

	if envPath := os.Getenv("SHROUDENV_DB_PATH"); envPath != "" {
		return envPath, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	return filepath.Join(homeDir, ".shroudenv", "db.json"), nil
}

// GetNonInteractive returns whether the non-interactive/CI override is active.
func GetNonInteractive() bool {
	if os.Getenv("SHROUDENV_NON_INTERACTIVE") == "true" {
		return true
	}
	return nonInteractiveFlag
}

// LoadDBAndKey loads the database and retrieves the master key.
func LoadDBAndKey() (*db.Database, string, []byte, error) {
	path, err := GetDBPath()
	if err != nil {
		return nil, "", nil, err
	}

	database, err := db.LoadDatabase(path)
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to load database: %w", err)
	}

	saltBytes, err := hex.DecodeString(database.Salt)
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to decode db salt: %w", err)
	}

	key, err := vault.GetMasterKey(saltBytes, GetNonInteractive())
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to get master key: %w", err)
	}

	return database, path, key, nil
}

// LoadDBAndKeyShared loads the database with a shared lock.
func LoadDBAndKeyShared() (*db.Database, string, []byte, *db.LockFile, error) {
	path, err := GetDBPath()
	if err != nil {
		return nil, "", nil, nil, err
	}

	// 1. Initial quick load without lock to read the salt for master key derivation
	initDB, err := db.LoadDatabase(path)
	if err != nil {
		return nil, "", nil, nil, fmt.Errorf("failed to load database: %w", err)
	}

	saltBytes, err := hex.DecodeString(initDB.Salt)
	if err != nil {
		return nil, "", nil, nil, fmt.Errorf("failed to decode db salt: %w", err)
	}

	key, err := vault.GetMasterKey(saltBytes, GetNonInteractive())
	if err != nil {
		return nil, "", nil, nil, fmt.Errorf("failed to get master key: %w", err)
	}

	// 2. Now acquire the shared lock
	lock, err := db.LockShared(path)
	if err != nil {
		return nil, "", nil, nil, fmt.Errorf("failed to lock database: %w", err)
	}

	// 3. Reload the database to get the absolute latest state
	database, err := db.LoadDatabase(path)
	if err != nil {
		lock.Unlock()
		return nil, "", nil, nil, fmt.Errorf("failed to reload database: %w", err)
	}

	return database, path, key, lock, nil
}

// LoadDBAndKeyExclusive loads the database with an exclusive lock.
func LoadDBAndKeyExclusive() (*db.Database, string, []byte, *db.LockFile, error) {
	path, err := GetDBPath()
	if err != nil {
		return nil, "", nil, nil, err
	}

	// 1. Initial quick load without lock to read the salt for master key derivation
	initDB, err := db.LoadDatabase(path)
	if err != nil {
		return nil, "", nil, nil, fmt.Errorf("failed to load database: %w", err)
	}

	saltBytes, err := hex.DecodeString(initDB.Salt)
	if err != nil {
		return nil, "", nil, nil, fmt.Errorf("failed to decode db salt: %w", err)
	}

	key, err := vault.GetMasterKey(saltBytes, GetNonInteractive())
	if err != nil {
		return nil, "", nil, nil, fmt.Errorf("failed to get master key: %w", err)
	}

	// 2. Now acquire the exclusive lock
	lock, err := db.LockExclusive(path)
	if err != nil {
		return nil, "", nil, nil, fmt.Errorf("failed to lock database: %w", err)
	}

	// 3. Reload the database to get the absolute latest state
	database, err := db.LoadDatabase(path)
	if err != nil {
		lock.Unlock()
		return nil, "", nil, nil, fmt.Errorf("failed to reload database: %w", err)
	}

	return database, path, key, lock, nil
}


// EnforceTTY checks if the execution is running in an interactive terminal.
// It fails-securely if non-interactive mode is detected and no bypass flag/env is set.
func EnforceTTY() error {
	if GetNonInteractive() {
		return nil
	}
	// Verify if standard output is a TTY
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return fmt.Errorf("fail-secure: non-interactive TTY context detected (stdout is redirected or piped). " +
			"Use --non-interactive/--ci flag or set SHROUDENV_NON_INTERACTIVE=true env var to permit access.")
	}
	return nil
}

