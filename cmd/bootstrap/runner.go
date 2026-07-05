package bootstrap

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	pkgbootstrap "shroudenv/pkg/bootstrap"
	"shroudenv/pkg/db"
)

type InputReader interface {
	ReadInput(prompt string) (string, error)
	ReadSensitiveInput(prompt string) (string, error)
}

type BootstrapRunner struct {
	EnvVars        map[string]string
	Stdout         io.Writer
	Stderr         io.Writer
	InputReader    InputReader
	DBPath         string
	MasterKey      []byte
	DryRun         bool
	NonInteractive bool
}

// Run executes the bootstrapping configuration logic.
func (r *BootstrapRunner) Run(configPath string, envOverride string) error {
	// F-14: Clear master key in memory immediately when Run exits
	defer func() {
		clear(r.MasterKey)
	}()

	// 1. Read configuration file directly (F-01: prevents TOCTOU race)
	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("scaffolding configuration file not found at %s. Please specify it using -f flag", configPath)
		}
		return fmt.Errorf("failed to read configuration file: %w", err)
	}

	config, err := pkgbootstrap.ParseConfig(configBytes)
	if err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// F-07: Filter environment variables to mitigate variable exfiltration (trust boundary)
	filteredEnv := make(map[string]string)
	for _, v := range config.Variables {
		if v.Fallback != "" {
			if val, exists := r.EnvVars[v.Fallback]; exists {
				filteredEnv[v.Fallback] = val
			}
		}
	}
	r.EnvVars = filteredEnv

	// 2. Resolve target environment name
	envName := envOverride
	if envName == "" {
		envName = config.DefaultEnvironment
	}
	if envName == "" {
		envName = "development"
	}

	fmt.Fprintf(r.Stdout, "🔍 Found %s for project %q\n", filepath.Base(configPath), config.Project)
	fmt.Fprintf(r.Stdout, "🌱 Bootstrapping environment %q in secure vault...\n\n", envName)

	resolved := make(map[string]string)
	totalVars := len(config.Variables)

	// 3. Resolve base configuration (pre-resolve or resolve)
	if r.NonInteractive {
		// Non-interactive flow
		resolved, err = pkgbootstrap.Resolve(config, nil, r.EnvVars)
		if err != nil {
			if valErr, ok := err.(*pkgbootstrap.ValidationErrorMap); ok {
				fmt.Fprintln(r.Stderr, "❌ Validation failed in non-interactive mode:")
				for name, e := range valErr.Errors {
					fmt.Fprintf(r.Stderr, "  - %s: %v\n", name, e)
				}
				return fmt.Errorf("scaffolding aborted due to validation errors")
			}
			return err
		}

		// Print resolved secrets
		for i, v := range config.Variables {
			if v.Generator != nil {
				fmt.Fprintf(r.Stdout, "[%d/%d] Generative secret: %s\n", i+1, totalVars, v.Name)
				r.describeGeneratorResult(v, resolved[v.Name])
			} else {
				if _, exists := resolved[v.Name]; exists {
					fmt.Fprintf(r.Stdout, "[%d/%d] Resolved: %s\n", i+1, totalVars, v.Name)
				}
			}
		}
	} else {
		// Interactive prompting flow
		preResolved, err := pkgbootstrap.PreResolve(config)
		if err != nil {
			return err
		}
		for k, v := range preResolved {
			resolved[k] = v
		}

		// 4. Prompt user for variables
		for i, v := range config.Variables {
			if v.Generator != nil {
				fmt.Fprintf(r.Stdout, "[%d/%d] Generative secret: %s\n", i+1, totalVars, v.Name)
				r.describeGeneratorResult(v, resolved[v.Name])
				fmt.Fprintln(r.Stdout)
				continue
			}

			fmt.Fprintf(r.Stdout, "[%d/%d] %s (%s)\n", i+1, totalVars, v.Name, v.Description)

			// Determine default value
			var defaultValue string
			if v.Fallback != "" {
				if envVal, exists := r.EnvVars[v.Fallback]; exists && envVal != "" {
					fmt.Fprintf(r.Stdout, "  ↳ Found shell environment fallback $%s: %q\n", v.Fallback, envVal)
					defaultValue = envVal
				}
			}
			if defaultValue == "" && v.Default != nil {
				defaultStr := fmt.Sprintf("%v", v.Default)
				interpolated, err := pkgbootstrap.Interpolate(defaultStr, resolved)
				if err == nil {
					defaultValue = interpolated
				}
			}

			// Format prompt details
			promptText := v.Prompt
			if promptText == "" {
				promptText = "Enter value"
			}

			var promptDetails []string
			if defaultValue != "" {
				if v.Sensitive {
					promptDetails = append(promptDetails, "default: <masked>")
				} else {
					promptDetails = append(promptDetails, fmt.Sprintf("default: %s", defaultValue))
				}
			} else if v.Optional {
				promptDetails = append(promptDetails, "optional, Enter to skip")
			}
			if v.Validation != nil && len(v.Validation.Enum) > 0 {
				promptDetails = append(promptDetails, fmt.Sprintf("options: %s", strings.Join(v.Validation.Enum, ", ")))
			}

			fullPrompt := "  ↳ " + promptText
			if len(promptDetails) > 0 {
				fullPrompt = fmt.Sprintf("%s [%s]", fullPrompt, strings.Join(promptDetails, ", "))
			}
			fullPrompt = fullPrompt + ": "

			// Prompt loop with validation
			for {
				var userInput string
				var promptErr error

				if v.Sensitive {
					userInput, promptErr = r.InputReader.ReadSensitiveInput(fullPrompt)
				} else {
					userInput, promptErr = r.InputReader.ReadInput(fullPrompt)
				}

				if promptErr != nil {
					return fmt.Errorf("failed to read user input: %w", promptErr)
				}

				if userInput == "" {
					userInput = defaultValue
				}

				if userInput == "" && v.Optional {
					break
				}

				// F-04: Non-optional, non-default interactive empty-value check
				if userInput == "" && !v.Optional {
					fmt.Fprintln(r.Stdout, "  ❌ This field is required.")
					continue
				}

				if valErr := pkgbootstrap.Validate(userInput, v); valErr != nil {
					fmt.Fprintf(r.Stdout, "  ❌ %v\n", valErr)
					continue
				}

				resolved[v.Name] = userInput
				break
			}
			fmt.Fprintln(r.Stdout)
		}
	}

	// 5. Commit Secrets
	if r.DryRun {
		fmt.Fprintln(r.Stdout, "💾 [Dry-run] Simulating saving of secrets...")
		fmt.Fprintf(r.Stdout, "✨ Dry-run complete. Resolved %d secrets. No changes were saved to the database.\n", len(resolved))
		return nil
	}

	// Acquire exclusive write lock on the database file
	lock, err := db.LockExclusive(r.DBPath)
	if err != nil {
		return fmt.Errorf("failed to lock database: %w", err)
	}
	defer lock.Unlock()

	// Reload database to get the absolute latest state
	database, err := db.LoadDatabase(r.DBPath)
	if err != nil {
		return fmt.Errorf("failed to reload database: %w", err)
	}

	// Safety Guardrail: verify target environment doesn't already exist
	p := database.GetProject(config.Project)
	if p != nil && p.GetEnvironment(envName) != nil {
		return fmt.Errorf("environment %q already exists in project %q; shroudenv bootstrap cannot overwrite an existing environment", envName, config.Project)
	}

	// Auto-create project if missing
	if p == nil {
		if err := database.CreateProject(config.Project); err != nil {
			return fmt.Errorf("failed to create project %q: %w", config.Project, err)
		}
		p = database.GetProject(config.Project)
	}

	// Auto-create environment
	if err := database.CreateEnvironment(config.Project, envName); err != nil {
		return fmt.Errorf("failed to create environment %q in project %q: %w", envName, config.Project, err)
	}

	// Get environment and set secrets
	e := p.GetEnvironment(envName)
	if e == nil {
		// F-03: explicit nil-guard after environment creation
		return fmt.Errorf("internal error: environment %q not found after creation", envName)
	}
	if err := e.SetSecrets(resolved, r.MasterKey); err != nil {
		return fmt.Errorf("failed to encrypt and set secrets: %w", err)
	}

	// Write database back to disk
	if err := db.SaveDatabase(r.DBPath, database); err != nil {
		return fmt.Errorf("failed to save database: %w", err)
	}

	fmt.Fprintf(r.Stdout, "💾 Saving %d secrets securely to project %q, environment %q...\n", len(resolved), config.Project, envName)
	fmt.Fprintln(r.Stdout, "✨ Success! Environment bootstrapped successfully.")
	fmt.Fprintf(r.Stdout, "💡 Try running your app using: shroudenv inject -p %s -e %s -- <command>\n", config.Project, envName)

	return nil
}

func (r *BootstrapRunner) describeGeneratorResult(v pkgbootstrap.VariableSchema, val string) {
	gType := strings.ToLower(strings.TrimSpace(v.Generator.Type))
	switch gType {
	case "random_string":
		fmt.Fprintf(r.Stdout, "  ↳ Generated secure alphanumeric string (%d chars)\n", len(val))
	case "random_bytes":
		encoding := strings.ToLower(v.Generator.Encoding)
		if encoding == "" {
			encoding = "hex"
		}
		fmt.Fprintf(r.Stdout, "  ↳ Generated secure random bytes as %s (%d chars)\n", encoding, len(val))
	case "uuid":
		fmt.Fprintf(r.Stdout, "  ↳ Generated secure UUID v4 (%d chars)\n", len(val))
	}
}
