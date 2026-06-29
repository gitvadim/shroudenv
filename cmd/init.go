package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"shroudenv/pkg/db"
	"shroudenv/pkg/vault"

	"github.com/spf13/cobra"
)

var forceFlag bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize shroudenv storage and master key",
	Long:  `Generates a new master key, stores it securely in the OS keyring, and sets up the database file.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := GetDBPath()
		if err != nil {
			return err
		}

		fmt.Printf("Initializing shroudenv at %s...\n", path)

		// Check if database file exists
		dbExists := true
		if _, err := os.Stat(path); os.IsNotExist(err) {
			dbExists = false
		}

		// Check if we can already load a key from keyring
		existingKey, err := vault.GetMasterKey(nil, true)
		hasKey := (err == nil && len(existingKey) == 32)

		if hasKey && dbExists && !forceFlag {
			fmt.Println("Already initialized! Master key exists in OS Keyring.")
			return nil
		}

		// Prepare database
		var database *db.Database
		if forceFlag || !dbExists {
			// Generate fresh salt and database structure
			saltBytes := make([]byte, 16)
			if _, err := io.ReadFull(rand.Reader, saltBytes); err != nil {
				return err
			}
			database = &db.Database{
				Salt:     hex.EncodeToString(saltBytes),
				Projects: []db.Project{},
			}
		} else {
			// Database exists, key might not. Just load the existing database.
			database, err = db.LoadDatabase(path)
			if err != nil {
				return fmt.Errorf("failed to load database: %w", err)
			}
		}

		// Generate/store key if missing or forced
		if !hasKey || forceFlag {
			key, err := vault.GenerateRandomKey()
			if err != nil {
				return fmt.Errorf("failed to generate master key: %w", err)
			}

			err = vault.SetMasterKey(key)
			if err != nil {
				fmt.Fprintf(os.Stderr, "WARNING: Failed to store master key in OS vault: %v\n", err)
				fmt.Fprintf(os.Stderr, "You will need to supply SHROUDENV_MASTER_KEY environment variable or enter password interactively.\n")
				fmt.Printf("Generated Master Key (Hex): %s\n", hex.EncodeToString(key))
			} else {
				fmt.Println("Master key securely stored in OS Keyring.")
			}
		} else {
			fmt.Println("Using existing master key from OS Keyring.")
		}

		// Lock exclusively and save the database to disk
		lock, err := db.LockExclusive(path)
		if err != nil {
			return fmt.Errorf("failed to lock database for initialization: %w", err)
		}
		defer lock.Unlock()

		err = db.SaveDatabase(path, database)
		if err != nil {
			return fmt.Errorf("failed to save database: %w", err)
		}

		fmt.Println("Initialization complete.")
		return nil
	},
}

func init() {
	initCmd.Flags().BoolVarP(&forceFlag, "force", "f", false, "Force re-initialization (overwrites existing key and database)")
	RootCmd.AddCommand(initCmd)
}
