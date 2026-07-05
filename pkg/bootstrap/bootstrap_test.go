package bootstrap

import (
	"encoding/base64"
	"encoding/hex"
	"regexp"
	"strings"
	"testing"
)

func TestParseConfig(t *testing.T) {
	validYAML := `
version: "1"
project: "test-app"
default_environment: "development"
variables:
  - name: PORT
    description: "Server port"
    type: integer
    default: 3000
    validation:
      min: 1024
      max: 65535
  - name: DB_PASSWORD
    generator:
      type: random_string
      length: 16
      charset: alphanumeric
`
	config, err := ParseConfig([]byte(validYAML))
	if err != nil {
		t.Fatalf("unexpected error parsing valid config: %v", err)
	}

	if config.Version != "1" {
		t.Errorf("expected version to be '1', got %q", config.Version)
	}
	if config.Project != "test-app" {
		t.Errorf("expected project to be 'test-app', got %q", config.Project)
	}
	if len(config.Variables) != 2 {
		t.Errorf("expected 2 variables, got %d", len(config.Variables))
	}

	// Invalid version
	invalidYAML1 := `
project: "test-app"
variables:
  - name: PORT
`
	if _, err := ParseConfig([]byte(invalidYAML1)); err == nil {
		t.Error("expected error for missing version, got nil")
	}

	// Duplicate names
	invalidYAML2 := `
version: "1"
project: "test-app"
variables:
  - name: PORT
  - name: port
`
	if _, err := ParseConfig([]byte(invalidYAML2)); err == nil {
		t.Error("expected error for duplicate variable names, got nil")
	}

	// Bounds check on string type
	invalidYAML3 := `
version: "1"
project: "test-app"
variables:
  - name: STR
    type: string
    validation:
      min: 10
`
	if _, err := ParseConfig([]byte(invalidYAML3)); err == nil {
		t.Error("expected error for min validation on string type, got nil")
	}
}

func TestGetPromptableVariables(t *testing.T) {
	yamlData := `
version: "1"
project: "test-app"
variables:
  - name: VAR1
    type: string
  - name: VAR2
    generator:
      type: uuid
  - name: VAR3
    type: integer
`
	config, err := ParseConfig([]byte(yamlData))
	if err != nil {
		t.Fatalf("unexpected parsing error: %v", err)
	}

	promptable := GetPromptableVariables(config)
	if len(promptable) != 2 {
		t.Fatalf("expected 2 promptable variables, got %d", len(promptable))
	}

	if promptable[0].Name != "VAR1" || promptable[1].Name != "VAR3" {
		t.Errorf("incorrect promptable variables returned: %+v", promptable)
	}
}

func TestGenerators(t *testing.T) {
	// 1. random_string alphanumeric
	sVal, err := Generate(GeneratorSchema{Type: "random_string", Length: 12, Charset: "alphanumeric"})
	if err != nil {
		t.Fatalf("random_string error: %v", err)
	}
	if len(sVal) != 12 {
		t.Errorf("expected string length 12, got %d", len(sVal))
	}
	for _, char := range sVal {
		if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", char) {
			t.Errorf("unexpected char in alphanumeric random string: %c", char)
		}
	}

	// 2. random_string custom charset
	cVal, err := Generate(GeneratorSchema{Type: "random_string", Length: 5, Charset: "abc!"})
	if err != nil {
		t.Fatalf("custom random_string error: %v", err)
	}
	if len(cVal) != 5 {
		t.Errorf("expected length 5, got %d", len(cVal))
	}
	for _, char := range cVal {
		if !strings.ContainsRune("abc!", char) {
			t.Errorf("unexpected char in custom random string: %c", char)
		}
	}

	// 3. random_bytes hex
	hexVal, err := Generate(GeneratorSchema{Type: "random_bytes", Length: 10, Encoding: "hex"})
	if err != nil {
		t.Fatalf("random_bytes hex error: %v", err)
	}
	decoded, err := hex.DecodeString(hexVal)
	if err != nil {
		t.Errorf("random_bytes hex is not valid hex: %v", err)
	}
	if len(decoded) != 10 {
		t.Errorf("expected decoded random_bytes length 10, got %d", len(decoded))
	}

	// 4. random_bytes base64
	b64Val, err := Generate(GeneratorSchema{Type: "random_bytes", Length: 12, Encoding: "base64"})
	if err != nil {
		t.Fatalf("random_bytes base64 error: %v", err)
	}
	decodedB64, err := base64.StdEncoding.DecodeString(b64Val)
	if err != nil {
		t.Errorf("random_bytes base64 is not valid base64: %v", err)
	}
	if len(decodedB64) != 12 {
		t.Errorf("expected decoded random_bytes length 12, got %d", len(decodedB64))
	}

	// 5. uuid
	uuidVal, err := Generate(GeneratorSchema{Type: "uuid"})
	if err != nil {
		t.Fatalf("uuid error: %v", err)
	}
	uuidRegex := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !uuidRegex.MatchString(uuidVal) {
		t.Errorf("generated uuid %q is not a valid UUID v4", uuidVal)
	}
}

func TestValidators(t *testing.T) {
	// Type validations
	intVar := VariableSchema{Name: "INT_VAR", Type: "integer"}
	if err := Validate("3000", intVar); err != nil {
		t.Errorf("expected 3000 to be valid integer: %v", err)
	}
	if err := Validate("abc", intVar); err == nil {
		t.Error("expected abc to be invalid integer")
	}

	floatVar := VariableSchema{Name: "FLOAT_VAR", Type: "float"}
	if err := Validate("3.14", floatVar); err != nil {
		t.Errorf("expected 3.14 to be valid float: %v", err)
	}

	boolVar := VariableSchema{Name: "BOOL_VAR", Type: "boolean"}
	for _, val := range []string{"true", "yes", "Y", "1", "false", "no", "N", "0"} {
		if err := Validate(val, boolVar); err != nil {
			t.Errorf("expected %q to be valid boolean: %v", val, err)
		}
	}
	if err := Validate("maybe", boolVar); err == nil {
		t.Error("expected 'maybe' to be invalid boolean")
	}

	// Bounds validations
	minVal := 10.0
	maxVal := 100.0
	boundsVar := VariableSchema{
		Name: "BOUNDS_VAR",
		Type: "integer",
		Validation: &ValidationSchema{
			Min: &minVal,
			Max: &maxVal,
		},
	}
	if err := Validate("50", boundsVar); err != nil {
		t.Errorf("expected 50 to be in bounds [10, 100]: %v", err)
	}
	if err := Validate("9", boundsVar); err == nil {
		t.Error("expected 9 to be out of bounds (too low)")
	}
	if err := Validate("101", boundsVar); err == nil {
		t.Error("expected 101 to be out of bounds (too high)")
	}

	// Enum validation
	enumVar := VariableSchema{
		Name: "ENUM_VAR",
		Type: "string",
		Validation: &ValidationSchema{
			Enum: []string{"dev", "prod"},
		},
	}
	if err := Validate("dev", enumVar); err != nil {
		t.Errorf("expected 'dev' to be valid enum option: %v", err)
	}
	if err := Validate("staging", enumVar); err == nil {
		t.Error("expected 'staging' to be invalid enum option")
	}

	// Pattern validation with Custom Error override
	patternVar := VariableSchema{
		Name: "PATTERN_VAR",
		Type: "string",
		Validation: &ValidationSchema{
			Pattern:      "^sk_test_[a-zA-Z0-9]+$",
			ErrorMessage: "custom pattern error message",
		},
	}
	if err := Validate("sk_test_12345", patternVar); err != nil {
		t.Errorf("expected sk_test_12345 to be valid: %v", err)
	}
	patternErr := Validate("pk_live_12345", patternVar)
	if patternErr == nil {
		t.Error("expected pk_live_12345 to be invalid pattern")
	} else if patternErr.Error() != "custom pattern error message" {
		t.Errorf("expected custom error message override, got %q", patternErr.Error())
	}
}

func TestInterpolation(t *testing.T) {
	resolved := map[string]string{
		"PORT":        "8080",
		"DB_PASSWORD": "secret_password",
	}

	// Simple substitution
	res, err := Interpolate("postgres://postgres:${DB_PASSWORD}@localhost:${PORT}/db", resolved)
	if err != nil {
		t.Fatalf("unexpected interpolation error: %v", err)
	}
	expected := "postgres://postgres:secret_password@localhost:8080/db"
	if res != expected {
		t.Errorf("expected %q, got %q", expected, res)
	}

	// Missing reference
	_, err = Interpolate("postgres://postgres:${DB_PASSWORD}@${HOST}:${PORT}/db", resolved)
	if err == nil {
		t.Error("expected error for missing reference 'HOST', got nil")
	}
}

func TestPreResolveAndResolve(t *testing.T) {
	yamlData := `
version: "1"
project: "test-app"
variables:
  - name: DB_PASSWORD
    generator:
      type: random_string
      length: 8
  - name: PORT
    type: integer
    default: 3000
  - name: DATABASE_URL
    type: string
    default: "postgres://postgres:${DB_PASSWORD}@localhost:${PORT}/db"
  - name: HOST
    type: string
    fallback: "ENV_HOST"
    default: "localhost"
  - name: STRIPE_KEY
    type: string
    optional: true
`
	config, err := ParseConfig([]byte(yamlData))
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	// 1. Test PreResolve (should only run generators)
	preResolved, err := PreResolve(config)
	if err != nil {
		t.Fatalf("PreResolve error: %v", err)
	}
	if len(preResolved) != 1 {
		t.Errorf("expected 1 pre-resolved variable, got %d", len(preResolved))
	}
	dbPass, exists := preResolved["DB_PASSWORD"]
	if !exists || len(dbPass) != 8 {
		t.Errorf("DB_PASSWORD wasn't properly generated in PreResolve: %q", dbPass)
	}

	// 2. Test Resolve with prompt inputs
	userInputs := map[string]string{
		"PORT": "8080",
	}
	env := map[string]string{
		"ENV_HOST": "prod-machine",
	}

	finalSecrets, err := Resolve(config, userInputs, env)
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}

	// PORT should use user input
	if finalSecrets["PORT"] != "8080" {
		t.Errorf("expected PORT to be '8080', got %q", finalSecrets["PORT"])
	}

	// HOST should use fallback ENV_HOST
	if finalSecrets["HOST"] != "prod-machine" {
		t.Errorf("expected HOST to use fallback 'prod-machine', got %q", finalSecrets["HOST"])
	}

	// DATABASE_URL should be fully interpolated
	expectedDBUrl := "postgres://postgres:" + finalSecrets["DB_PASSWORD"] + "@localhost:8080/db"
	if finalSecrets["DATABASE_URL"] != expectedDBUrl {
		t.Errorf("expected DATABASE_URL to be %q, got %q", expectedDBUrl, finalSecrets["DATABASE_URL"])
	}

	// STRIPE_KEY is optional and wasn't provided, should be skipped (not present in map)
	if _, exists := finalSecrets["STRIPE_KEY"]; exists {
		t.Error("optional STRIPE_KEY should not be present in final secrets map")
	}

	// 3. Test Resolve Validation Failure
	invalidInputs := map[string]string{
		"PORT": "abc", // invalid integer
	}
	_, err = Resolve(config, invalidInputs, nil)
	if err == nil {
		t.Error("expected validation error for PORT='abc', got nil")
	} else {
		valErr, ok := err.(*ValidationErrorMap)
		if !ok {
			t.Errorf("expected error to be ValidationErrorMap, got %T", err)
		} else if _, exists := valErr.Errors["PORT"]; !exists {
			t.Errorf("expected PORT in validation error map: %v", valErr.Errors)
		}
	}

	// 4. Test Resolve Validation Failure for empty/whitespace required variable
	invalidInputs2 := map[string]string{
		"PORT": "8080",
		"HOST": "   ", // whitespace only
	}
	_, err = Resolve(config, invalidInputs2, nil)
	if err == nil {
		t.Error("expected validation error for HOST='   ' (whitespace only), got nil")
	} else {
		valErr, ok := err.(*ValidationErrorMap)
		if !ok {
			t.Errorf("expected error to be ValidationErrorMap, got %T", err)
		} else if _, exists := valErr.Errors["HOST"]; !exists {
			t.Errorf("expected HOST in validation error map: %v", valErr.Errors)
		}
	}
}
