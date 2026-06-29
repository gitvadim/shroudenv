package cmd

import (
	"fmt"

	"shroudenv/pkg/db"

	"github.com/spf13/cobra"
)

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Manage environments",
	Long:  `Create and list environments inside a project.`,
}

var envCreateCmd = &cobra.Command{
	Use:   "create <project> <env>",
	Short: "Create a new environment in a project",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectName := args[0]
		envName := args[1]

		database, path, _, lock, err := LoadDBAndKeyExclusive()
		if err != nil {
			return err
		}
		defer lock.Unlock()

		err = database.CreateEnvironment(projectName, envName)
		if err != nil {
			return err
		}

		err = db.SaveDatabase(path, database)
		if err != nil {
			return fmt.Errorf("failed to save database: %w", err)
		}

		fmt.Printf("Environment %q created in project %q successfully.\n", envName, projectName)
		return nil
	},
}

var envListCmd = &cobra.Command{
	Use:   "list <project>",
	Short: "List all environments in a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectName := args[0]

		database, _, _, lock, err := LoadDBAndKeyShared()
		if err != nil {
			return err
		}
		defer lock.Unlock()


		p := database.GetProject(projectName)
		if p == nil {
			return fmt.Errorf("project %q not found", projectName)
		}

		if len(p.Environments) == 0 {
			fmt.Printf("No environments found in project %q.\n", projectName)
			return nil
		}

		fmt.Printf("Environments in project %q:\n", projectName)
		for _, e := range p.Environments {
			hasSecrets := "no secrets"
			if e.Secrets != nil && e.Secrets.Ciphertext != "" {
				hasSecrets = "has secrets"
			}
			fmt.Printf("- %s (%s)\n", e.Name, hasSecrets)
		}
		return nil
	},
}

func init() {
	envCmd.AddCommand(envCreateCmd)
	envCmd.AddCommand(envListCmd)
	RootCmd.AddCommand(envCmd)
}
