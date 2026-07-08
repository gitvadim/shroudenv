package cmd

import (
	"os"
	"strings"

	cli "shroudenv/cmd/bootstrap"

	"github.com/spf13/cobra"
)

var (
	bootstrapEnvFlag    string
	bootstrapFileFlag   string
	bootstrapDryRunFlag bool
)

var bootstrapCmd = &cobra.Command{
	Use:   "bootstrap",
	Short: "Bootstrap environment scaffolding from a config file",
	Long:  `Reads .shroudenv.yaml config file and interactively sets up environment variables in your secure shroudenv database.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Enforce TTY check first (F-06)
		if err := EnforceTTY(); err != nil {
			return err
		}

		// 1. Derive keyring master key and load database path
		_, dbPath, key, err := LoadDBAndKey()
		if err != nil {
			return err
		}

		// 2. Prepare environment variables map
		envVars := make(map[string]string)
		for _, env := range os.Environ() {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				envVars[parts[0]] = parts[1]
			}
		}

		// 3. Instantiate and execute the runner
		runner := &cli.BootstrapRunner{
			EnvVars:        envVars,
			Stdout:         os.Stdout,
			Stderr:         os.Stderr,
			InputReader:    &cli.TerminalInputReader{},
			DBPath:         dbPath,
			MasterKey:      key,
			DryRun:         bootstrapDryRunFlag,
		}

		return runner.Run(bootstrapFileFlag, bootstrapEnvFlag)
	},
}

func init() {
	bootstrapCmd.Flags().StringVarP(&bootstrapEnvFlag, "env", "e", "", "Target environment name to bootstrap")
	bootstrapCmd.Flags().StringVarP(&bootstrapFileFlag, "file", "f", ".shroudenv.yaml", "Path to scaffolding configuration file")
	bootstrapCmd.Flags().BoolVar(&bootstrapDryRunFlag, "dry-run", false, "Simulate the setup process without committing to database")
	RootCmd.AddCommand(bootstrapCmd)
}
