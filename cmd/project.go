package cmd

import (
	"fmt"

	"shroudenv/pkg/db"

	"github.com/spf13/cobra"
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage projects",
	Long:  `Create and list projects in the shroudenv database.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Default to listing projects if no subcommand is provided
		return listProjects()
	},
}

var projectCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectName := args[0]

		database, path, _, lock, err := LoadDBAndKeyExclusive()
		if err != nil {
			return err
		}
		defer lock.Unlock()

		err = database.CreateProject(projectName)
		if err != nil {
			return err
		}

		err = db.SaveDatabase(path, database)
		if err != nil {
			return fmt.Errorf("failed to save database: %w", err)
		}

		fmt.Printf("Project %q created successfully.\n", projectName)
		return nil
	},
}

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all projects",
	RunE: func(cmd *cobra.Command, args []string) error {
		return listProjects()
	},
}

func listProjects() error {
	database, _, _, lock, err := LoadDBAndKeyShared()
	if err != nil {
		return err
	}
	defer lock.Unlock()

	if len(database.Projects) == 0 {
		fmt.Println("No projects found.")
		return nil
	}

	fmt.Println("Projects:")
	for _, p := range database.Projects {
		fmt.Printf("- %s (%d environments)\n", p.Name, len(p.Environments))
	}
	return nil
}

func init() {
	projectCmd.AddCommand(projectCreateCmd)
	projectCmd.AddCommand(projectListCmd)
	RootCmd.AddCommand(projectCmd)
}
