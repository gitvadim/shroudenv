package bootstrap

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type RuleValidator interface {
	Validate(value string, variable VariableSchema) error
}

type TypeValidator struct{}

func (v *TypeValidator) Validate(value string, variable VariableSchema) error {
	vType := strings.ToLower(strings.TrimSpace(variable.Type))
	trimmed := strings.TrimSpace(value)

	switch vType {
	case "", "string":
		return nil
	case "integer":
		if _, err := strconv.ParseInt(trimmed, 10, 64); err != nil {
			return fmt.Errorf("invalid integer: %q", value)
		}
		return nil
	case "float":
		if _, err := strconv.ParseFloat(trimmed, 64); err != nil {
			return fmt.Errorf("invalid float: %q", value)
		}
		return nil
	case "boolean":
		valLower := strings.ToLower(trimmed)
		if valLower == "true" || valLower == "1" || valLower == "yes" || valLower == "y" ||
			valLower == "false" || valLower == "0" || valLower == "no" || valLower == "n" {
			return nil
		}
		return fmt.Errorf("invalid boolean: %q (allowed: true, false, yes, no, y, n, 1, 0)", value)
	default:
		return fmt.Errorf("unsupported type %q", variable.Type)
	}
}

type MinMaxValidator struct{}

func (v *MinMaxValidator) Validate(value string, variable VariableSchema) error {
	if variable.Validation == nil {
		return nil
	}
	if variable.Validation.Min == nil && variable.Validation.Max == nil {
		return nil
	}

	trimmed := strings.TrimSpace(value)
	valFloat, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return fmt.Errorf("cannot perform range validation: %q is not a number", value)
	}

	if variable.Validation.Min != nil && valFloat < *variable.Validation.Min {
		return fmt.Errorf("value must be at least %v", *variable.Validation.Min)
	}
	if variable.Validation.Max != nil && valFloat > *variable.Validation.Max {
		return fmt.Errorf("value must be at most %v", *variable.Validation.Max)
	}
	return nil
}

type EnumValidator struct{}

func (v *EnumValidator) Validate(value string, variable VariableSchema) error {
	if variable.Validation == nil || len(variable.Validation.Enum) == 0 {
		return nil
	}

	found := false
	for _, opt := range variable.Validation.Enum {
		if value == opt {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("value %q must be one of: %s", value, strings.Join(variable.Validation.Enum, ", "))
	}
	return nil
}

type PatternValidator struct{}

func (v *PatternValidator) Validate(value string, variable VariableSchema) error {
	if variable.Validation == nil || variable.Validation.Pattern == "" {
		return nil
	}

	rx, err := regexp.Compile(variable.Validation.Pattern)
	if err != nil {
		return fmt.Errorf("failed to compile validation pattern: %w", err)
	}

	if !rx.MatchString(value) {
		// Associate ErrorMessage only with the PatternValidator where it makes semantic sense
		if variable.Validation != nil && variable.Validation.ErrorMessage != "" {
			return errors.New(variable.Validation.ErrorMessage)
		}
		return fmt.Errorf("value %q does not match required pattern", value)
	}
	return nil
}

// Validate checks a resolved value against all active validation rules.
func Validate(value string, variable VariableSchema) error {
	// If the variable is optional and value is empty, validation is bypassed
	if variable.Optional && strings.TrimSpace(value) == "" {
		return nil
	}

	validators := []RuleValidator{
		&TypeValidator{},
		&MinMaxValidator{},
		&EnumValidator{},
		&PatternValidator{},
	}

	for _, validator := range validators {
		if err := validator.Validate(value, variable); err != nil {
			return err
		}
	}

	return nil
}
