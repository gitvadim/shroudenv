package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var (
	projectNameFlag string
	envNameFlag     string
)

var injectCmd = &cobra.Command{
	Use:   "inject -p <project> -e <env> -- <command>",
	Short: "Inject secrets into a subprocess environment",
	Long: `Decrypts the secrets for the specified project environment,
injects them into the current process environment, and spawns the given subcommand.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Enforce TTY check first
		if err := EnforceTTY(); err != nil {
			return err
		}

		if projectNameFlag == "" || envNameFlag == "" {
			return errors.New("both --project (-p) and --env (-e) flags are required")
		}

		database, _, key, lock, err := LoadDBAndKeyShared()
		if err != nil {
			return err
		}
		defer lock.Unlock()

		p := database.GetProject(projectNameFlag)
		if p == nil {
			return fmt.Errorf("project %q not found", projectNameFlag)
		}

		e := p.GetEnvironment(envNameFlag)
		if e == nil {
			return fmt.Errorf("environment %q not found in project %q", envNameFlag, projectNameFlag)
		}

		secrets, err := e.GetSecrets(key)
		if err != nil {
			return fmt.Errorf("failed to decrypt secrets: %w", err)
		}

		lock.Unlock() // Release lock early before running subprocess

		// Prepare environment variables to inject
		currentEnv := os.Environ()
		mergedEnv := make([]string, len(currentEnv), len(currentEnv)+len(secrets))
		copy(mergedEnv, currentEnv)

		for k, v := range secrets {
			mergedEnv = append(mergedEnv, fmt.Sprintf("%s=%s", k, v))
		}

		// Set up subprocess
		subCommandName := args[0]
		subCommandArgs := args[1:]

		subCmd := exec.Command(subCommandName, subCommandArgs...)
		subCmd.Env = mergedEnv
		subCmd.Stdin = os.Stdin
		subCmd.Stdout = os.Stdout
		subCmd.Stderr = os.Stderr

		if err := subCmd.Run(); err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				os.Exit(exitErr.ExitCode())
			}
			return fmt.Errorf("subcommand execution failed: %w", err)
		}

		return nil
	},
}

func init() {
	injectCmd.Flags().StringVarP(&projectNameFlag, "project", "p", "", "Project name")
	injectCmd.Flags().StringVarP(&envNameFlag, "env", "e", "", "Environment name")
	injectCmd.MarkFlagRequired("project")
	injectCmd.MarkFlagRequired("env")

	RootCmd.AddCommand(injectCmd)
}
