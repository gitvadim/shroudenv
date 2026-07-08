package cmd

import (
	"fmt"

	"shroudenv/pkg/db"

	"github.com/spf13/cobra"
)

var (
	secretProjFlag string
	secretEnvFlag  string
)

var secretCmd = &cobra.Command{
	Use:   "secret",
	Short: "Manage secrets",
	Long:  `Set, list, and inspect secrets in a project environment.`,
}

var secretSetCmd = &cobra.Command{
	Use:   "set -p <project> -e <env> <key> <value>",
	Short: "Set a secret",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		secretKey := args[0]
		secretValue := args[1]

		database, path, key, lock, err := LoadDBAndKeyExclusive()
		if err != nil {
			return err
		}
		defer lock.Unlock()

		p := database.GetProject(secretProjFlag)
		if p == nil {
			return fmt.Errorf("project %q not found", secretProjFlag)
		}

		e := p.GetEnvironment(secretEnvFlag)
		if e == nil {
			return fmt.Errorf("environment %q not found in project %q", secretEnvFlag, secretProjFlag)
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

		fmt.Printf("Secret %q set successfully in %s/%s.\n", secretKey, secretProjFlag, secretEnvFlag)
		return nil
	},
}

var secretListCmd = &cobra.Command{
	Use:   "list -p <project> -e <env>",
	Short: "List all secrets (masked)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Enforce TTY check first
		if err := EnforceTTY(); err != nil {
			return err
		}

		database, _, key, lock, err := LoadDBAndKeyShared()
		if err != nil {
			return err
		}
		defer lock.Unlock()

		p := database.GetProject(secretProjFlag)
		if p == nil {
			return fmt.Errorf("project %q not found", secretProjFlag)
		}

		e := p.GetEnvironment(secretEnvFlag)
		if e == nil {
			return fmt.Errorf("environment %q not found in project %q", secretEnvFlag, secretProjFlag)
		}

		secrets, err := e.GetSecrets(key)
		if err != nil {
			return fmt.Errorf("failed to decrypt secrets: %w", err)
		}

		if len(secrets) == 0 {
			fmt.Printf("No secrets found in %s/%s.\n", secretProjFlag, secretEnvFlag)
			return nil
		}

		fmt.Printf("Secrets in %s/%s:\n", secretProjFlag, secretEnvFlag)
		for k, v := range secrets {
			fmt.Printf("%s=%s\n", k, maskSecretValue(v))
		}
		return nil
	},
}

var secretInspectCmd = &cobra.Command{
	Use:   "inspect -p <project> -e <env> <key>",
	Short: "Inspect a secret value in plaintext",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Enforce TTY check first
		if err := EnforceTTY(); err != nil {
			return err
		}

		secretKey := args[0]

		database, _, key, lock, err := LoadDBAndKeyShared()
		if err != nil {
			return err
		}
		defer lock.Unlock()

		p := database.GetProject(secretProjFlag)
		if p == nil {
			return fmt.Errorf("project %q not found", secretProjFlag)
		}

		e := p.GetEnvironment(secretEnvFlag)
		if e == nil {
			return fmt.Errorf("environment %q not found in project %q", secretEnvFlag, secretProjFlag)
		}

		secrets, err := e.GetSecrets(key)
		if err != nil {
			return fmt.Errorf("failed to decrypt secrets: %w", err)
		}

		val, exists := secrets[secretKey]
		if !exists {
			return fmt.Errorf("secret key %q not found in %s/%s", secretKey, secretProjFlag, secretEnvFlag)
		}

		fmt.Println(val)
		return nil
	},
}

func maskSecretValue(val string) string {
	if len(val) <= 4 {
		return "****"
	}
	return val[:3] + "...********"
}

func init() {
	secretCmd.PersistentFlags().StringVarP(&secretProjFlag, "project", "p", "", "Project name")
	secretCmd.PersistentFlags().StringVarP(&secretEnvFlag, "env", "e", "", "Environment name")

	secretCmd.AddCommand(secretSetCmd)
	secretCmd.AddCommand(secretListCmd)
	secretCmd.AddCommand(secretInspectCmd)

	_ = secretSetCmd.MarkFlagRequired("project")
	_ = secretSetCmd.MarkFlagRequired("env")
	_ = secretListCmd.MarkFlagRequired("project")
	_ = secretListCmd.MarkFlagRequired("env")
	_ = secretInspectCmd.MarkFlagRequired("project")
	_ = secretInspectCmd.MarkFlagRequired("env")

	RootCmd.AddCommand(secretCmd)
}
