package bootstrap

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type GeneratorSchema struct {
	Type     string `yaml:"type"`
	Length   int    `yaml:"length,omitempty"`
	Charset  string `yaml:"charset,omitempty"`
	Encoding string `yaml:"encoding,omitempty"`
}

type ValidationSchema struct {
	Min          *float64 `yaml:"min,omitempty"`
	Max          *float64 `yaml:"max,omitempty"`
	Enum         []string `yaml:"enum,omitempty"`
	Pattern      string   `yaml:"pattern,omitempty"`
	ErrorMessage string   `yaml:"error_message,omitempty"`
}

type VariableSchema struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description,omitempty"`
	Type        string            `yaml:"type,omitempty"` // defaults to string
	Default     interface{}       `yaml:"default,omitempty"`
	Prompt      string            `yaml:"prompt,omitempty"`
	Sensitive   bool              `yaml:"sensitive,omitempty"`
	Optional    bool              `yaml:"optional,omitempty"`
	Fallback    string            `yaml:"fallback,omitempty"`
	Validation  *ValidationSchema `yaml:"validation,omitempty"`
	Generator   *GeneratorSchema  `yaml:"generator,omitempty"`
}

type BootstrapConfig struct {
	Version            string           `yaml:"version"`
	Project            string           `yaml:"project"`
	DefaultEnvironment string           `yaml:"default_environment,omitempty"`
	Variables          []VariableSchema `yaml:"variables"`
}

var varNameRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// ParseConfig deserializes and validates a .shroudenv.yaml content.
func ParseConfig(data []byte) (*BootstrapConfig, error) {
	var config BootstrapConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	if strings.TrimSpace(config.Version) == "" {
		return nil, errors.New("config version is required")
	}
	if strings.TrimSpace(config.Project) == "" {
		return nil, errors.New("project name is required")
	}

	if len(config.Variables) == 0 {
		return nil, errors.New("at least one variable must be defined")
	}

	seenNames := make(map[string]bool)

	for i, v := range config.Variables {
		if strings.TrimSpace(v.Name) == "" {
			return nil, fmt.Errorf("variable at index %d has no name", i)
		}
		if !varNameRegex.MatchString(v.Name) {
			return nil, fmt.Errorf("variable name %q is invalid: must start with a letter/underscore and contain only alphanumeric chars and underscores", v.Name)
		}

		nameLower := strings.ToLower(v.Name)
		if seenNames[nameLower] {
			return nil, fmt.Errorf("duplicate variable name %q", v.Name)
		}
		seenNames[nameLower] = true

		// Validate Type
		vType := strings.ToLower(strings.TrimSpace(v.Type))
		if vType != "" && vType != "string" && vType != "integer" && vType != "boolean" && vType != "float" {
			return nil, fmt.Errorf("variable %q has invalid type %q: must be string, integer, boolean, or float", v.Name, v.Type)
		}

		// Validate Generator
		if v.Generator != nil {
			gType := strings.ToLower(strings.TrimSpace(v.Generator.Type))
			if gType != "random_string" && gType != "random_bytes" && gType != "uuid" {
				return nil, fmt.Errorf("variable %q has invalid generator type %q: must be random_string, random_bytes, or uuid", v.Name, v.Generator.Type)
			}
			if gType == "random_string" {
				// Custom charset: all non-empty values other than known presets are allowed as-is.
			}
			if gType == "random_bytes" {
				encoding := strings.ToLower(v.Generator.Encoding)
				if encoding != "" && encoding != "hex" && encoding != "base64" {
					return nil, fmt.Errorf("variable %q generator encoding %q is invalid: must be hex or base64", v.Name, v.Generator.Encoding)
				}
			}
		}

		// Validate Validation Rules
		if v.Validation != nil {
			if v.Validation.Pattern != "" {
				if _, err := regexp.Compile(v.Validation.Pattern); err != nil {
					return nil, fmt.Errorf("variable %q has invalid regex pattern %q: %w", v.Name, v.Validation.Pattern, err)
				}
			}
			// If min/max are defined, check that the type is integer or float (if type is specified)
			if (v.Validation.Min != nil || v.Validation.Max != nil) && vType != "" && vType != "integer" && vType != "float" {
				return nil, fmt.Errorf("variable %q has min/max validation but type is %q: bounds are only supported for integer and float types", v.Name, v.Type)
			}
		}
	}

	return &config, nil
}

// GetPromptableVariables returns variables that do not have a generator.
func GetPromptableVariables(config *BootstrapConfig) []VariableSchema {
	var promptable []VariableSchema
	for _, v := range config.Variables {
		if v.Generator == nil {
			promptable = append(promptable, v)
		}
	}
	return promptable
}
