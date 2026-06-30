package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"
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

		runErr := runSubprocess(subCmd)

		if runErr != nil {
			var exitErr *exec.ExitError
			if errors.As(runErr, &exitErr) {
				os.Exit(exitErr.ExitCode())
			}
			return fmt.Errorf("subcommand execution failed: %w", runErr)
		}

		return nil
	},
}

func runSubprocess(subCmd *exec.Cmd) error {
	// Capture the initial terminal state so we can restore it if the child process
	// or cmd.exe leaves the terminal in a raw/corrupted state on termination.
	var oldState *term.State
	if term.IsTerminal(int(os.Stdin.Fd())) {
		if state, err := term.GetState(int(os.Stdin.Fd())); err == nil {
			oldState = state
		}
	}

	sigChan := make(chan os.Signal, 2) // buffer size 2 to avoid blocking
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	done := make(chan struct{})
	defer close(done)

	go func() {
		select {
		case sig := <-sigChan:
			// Restore the terminal state immediately on the first signal.
			// This ensures that if the child process (or cmd.exe) left the terminal
			// in raw mode, we restore it to cooked mode immediately. This allows cmd.exe's
			// "Terminate batch job (Y/N)?" prompt to receive input and behave correctly.
			if oldState != nil {
				term.Restore(int(os.Stdin.Fd()), oldState)
			}

			// Forward SIGTERM to the child process.
			if sig == syscall.SIGTERM && subCmd.Process != nil {
				if err := subCmd.Process.Signal(syscall.SIGTERM); err != nil {
					_ = subCmd.Process.Kill()
				}
			}

			// If another signal is received, or if the user presses Ctrl+C/sends SIGTERM again,
			// force exit immediately as an escape hatch.
			select {
			case <-sigChan:
				signal.Stop(sigChan)
				if subCmd.Process != nil {
					_ = subCmd.Process.Kill()
				}
				os.Exit(130) // 130 is the standard exit code for SIGINT termination
			case <-done:
				return
			}
		case <-done:
			return
		}
	}()

	runErr := subCmd.Run()

	signal.Stop(sigChan)
	if oldState != nil {
		term.Restore(int(os.Stdin.Fd()), oldState)
	}

	return runErr
}

func init() {
	injectCmd.Flags().StringVarP(&projectNameFlag, "project", "p", "", "Project name")
	injectCmd.Flags().StringVarP(&envNameFlag, "env", "e", "", "Environment name")
	injectCmd.MarkFlagRequired("project")
	injectCmd.MarkFlagRequired("env")

	RootCmd.AddCommand(injectCmd)
}
