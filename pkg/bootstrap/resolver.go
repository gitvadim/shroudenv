package bootstrap

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var interpolationRegex = regexp.MustCompile(`\$\{([a-zA-Z0-9_]+)\}`)

// ValidationErrorMap wraps multiple validation errors mapped by variable name.
type ValidationErrorMap struct {
	Errors map[string]error
}

func (e *ValidationErrorMap) Error() string {
	var sb strings.Builder
	sb.WriteString("validation failed for variables:")
	keys := make([]string, 0, len(e.Errors))
	for k := range e.Errors {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, name := range keys {
		sb.WriteString(fmt.Sprintf("\n  - %s: %v", name, e.Errors[name]))
	}
	return sb.String()
}

// Interpolate substitutes ${VAR_NAME} placeholders in the value string
// with values from the resolved map. Returns an error if a referenced variable
// is not present in the resolved map.
func Interpolate(value string, resolved map[string]string) (string, error) {
	var err error
	res := interpolationRegex.ReplaceAllStringFunc(value, func(match string) string {
		varName := match[2 : len(match)-1] // strip "${" and "}"
		val, exists := resolved[varName]
		if !exists {
			err = fmt.Errorf("variable %q referenced by interpolation but not yet resolved", varName)
			return match
		}
		return val
	})
	if err != nil {
		return "", err
	}
	return res, nil
}

// PreResolve runs all generator-based variables in sequential order to produce a base map
// of generated secrets. Consuming surfaces can use this base map to pre-interpolate defaults
// for interactive variables.
func PreResolve(config *BootstrapConfig) (map[string]string, error) {
	resolved := make(map[string]string)

	for _, v := range config.Variables {
		if v.Generator != nil {
			generatedVal, err := Generate(*v.Generator)
			if err != nil {
				return nil, fmt.Errorf("failed to pre-resolve generator for variable %s: %w", v.Name, err)
			}
			// Validate the generated value (sanity check)
			if err := Validate(generatedVal, v); err != nil {
				return nil, fmt.Errorf("pre-resolved value for generator %s failed validation: %w", v.Name, err)
			}
			resolved[v.Name] = generatedVal
		}
	}

	return resolved, nil
}

// Resolve resolves the entire configuration using the provided userInputs map and env map.
// If any variable fails validation or required variables are missing, it returns a ValidationErrorMap.
func Resolve(config *BootstrapConfig, userInputs map[string]string, env map[string]string) (map[string]string, error) {
	resolved := make(map[string]string)
	validationErrors := make(map[string]error)

	for _, v := range config.Variables {
		var val string
		// 1. If generator is present, execute it
		if v.Generator != nil {
			generatedVal, err := Generate(*v.Generator)
			if err != nil {
				return nil, fmt.Errorf("failed to generate value for variable %s: %w", v.Name, err)
			}
			val = generatedVal
		} else {
			// 2. Check user inputs first
			inputVal, ok := userInputs[v.Name]
			if ok {
				val = inputVal
			} else {
				// Check fallback from shell environment
				if v.Fallback != "" {
					if envVal, exists := env[v.Fallback]; exists && envVal != "" {
						val = envVal
					}
				}
				// Check default value (interpolate if needed)
				if val == "" && v.Default != nil {
					defaultStr := fmt.Sprintf("%v", v.Default)
					interpolated, err := Interpolate(defaultStr, resolved)
					if err != nil {
						validationErrors[v.Name] = fmt.Errorf("failed to interpolate default value: %w", err)
						continue
					}
					val = interpolated
				}
			}
		}

		// 3. Handle missing values
		if strings.TrimSpace(val) == "" {
			if v.Optional {
				continue // Skip optional variables if not provided
			}
			validationErrors[v.Name] = errors.New("required variable is missing")
			continue
		}

		// 4. Validate the value
		if err := Validate(val, v); err != nil {
			validationErrors[v.Name] = err
			// Store it in resolved map anyway so that subsequent interpolation does not fail,
			// allowing us to perform validation checks on other variables down the chain.
			resolved[v.Name] = val
		} else {
			resolved[v.Name] = val
		}
	}

	if len(validationErrors) > 0 {
		return nil, &ValidationErrorMap{Errors: validationErrors}
	}

	return resolved, nil
}
