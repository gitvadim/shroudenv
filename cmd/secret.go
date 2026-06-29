package cmd

import (
	"fmt"

	"shroudenv/pkg/db"

	"github.com/spf13/cobra"
)

var secretCmd = &cobra.Command{
	Use:   "secret",
	Short: "Manage secrets",
	Long:  `Set and list secrets in a project environment.`,
}

var secretSetCmd = &cobra.Command{
	Use:   "set <project> <env> <key> <value>",
	Short: "Set a secret",
	Args:  cobra.ExactArgs(4),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectName := args[0]
		envName := args[1]
		secretKey := args[2]
		secretValue := args[3]

		database, path, key, lock, err := LoadDBAndKeyExclusive()
		if err != nil {
			return err
		}
		defer lock.Unlock()

		p := database.GetProject(projectName)
		if p == nil {
			return fmt.Errorf("project %q not found", projectName)
		}

		e := p.GetEnvironment(envName)
		if e == nil {
			return fmt.Errorf("environment %q not found in project %q", envName, projectName)
		}

		secrets, err := e.GetSecrets(key)
		if err != nil {
			return fmt.Errorf("failed to get existing secrets: %w", err)
		}

		secrets[secretKey] = secretValue

		err = e.SetSecrets(secrets, key)
		if err != nil {
			return fmt.Errorf("failed to encrypt secrets: %w", err)
		}

		err = db.SaveDatabase(path, database)
		if err != nil {
			return fmt.Errorf("failed to save database: %w", err)
		}

		fmt.Printf("Secret %q set successfully in %s/%s.\n", secretKey, projectName, envName)
		return nil
	},
}

var secretListCmd = &cobra.Command{
	Use:   "list <project> <env>",
	Short: "List all secrets (decrypted)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Enforce TTY check first
		if err := EnforceTTY(); err != nil {
			return err
		}

		projectName := args[0]
		envName := args[1]

		database, _, key, lock, err := LoadDBAndKeyShared()
		if err != nil {
			return err
		}
		defer lock.Unlock()

		p := database.GetProject(projectName)
		if p == nil {
			return fmt.Errorf("project %q not found", projectName)
		}

		e := p.GetEnvironment(envName)
		if e == nil {
			return fmt.Errorf("environment %q not found in project %q", envName, projectName)
		}

		secrets, err := e.GetSecrets(key)
		if err != nil {
			return fmt.Errorf("failed to decrypt secrets: %w", err)
		}

		if len(secrets) == 0 {
			fmt.Printf("No secrets found in %s/%s.\n", projectName, envName)
			return nil
		}

		fmt.Printf("Secrets in %s/%s:\n", projectName, envName)
		for k, v := range secrets {
			fmt.Printf("%s=%s\n", k, v)
		}
		return nil
	},
}

func init() {
	secretCmd.AddCommand(secretSetCmd)
	secretCmd.AddCommand(secretListCmd)
	RootCmd.AddCommand(secretCmd)
}
