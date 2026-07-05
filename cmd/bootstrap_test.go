package cmd

import (
	"os"
	"strings"
	"testing"

	"shroudenv/pkg/keyring"
)

func init() {
	keyring.MockInit()
}

func TestBootstrapCmd_NoConfigFile(t *testing.T) {
	os.Setenv("SHROUDENV_MASTER_KEY", strings.Repeat("a", 64))
	defer os.Unsetenv("SHROUDENV_MASTER_KEY")

	cmd := RootCmd
	cmd.SetArgs([]string{"bootstrap", "-f", "non_existent_file_xyz.yaml", "--non-interactive"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for non-existent config file, got nil")
	}
	if !strings.Contains(err.Error(), "scaffolding configuration file not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBootstrapCmd_SuccessNonInteractive(t *testing.T) {
	// 1. Setup temp database path and mock master key env var (N-01: prevents testutil from compiling to production)
	tmpDB, err := os.CreateTemp("", "shroudenv-test-db-*.json")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpDBPath := tmpDB.Name()
	tmpDB.Close()
	os.Remove(tmpDBPath) // delete so it starts fresh/non-existent
	defer os.Remove(tmpDBPath)

	os.Setenv("SHROUDENV_DB_PATH", tmpDBPath)
	os.Setenv("SHROUDENV_MASTER_KEY", strings.Repeat("a", 64)) // 64 char hex string
	defer os.Unsetenv("SHROUDENV_DB_PATH")
	defer os.Unsetenv("SHROUDENV_MASTER_KEY")

	// 2. Initialize the database salt and files
	initCmd := RootCmd
	initCmd.SetArgs([]string{"init", "--force"})
	if err := initCmd.Execute(); err != nil {
		t.Fatalf("init command failed: %v", err)
	}

	// 3. Create a temporary .shroudenv.yaml file
	yamlContent := `
version: "1"
project: "cli-test-app"
default_environment: "development"
variables:
  - name: API_KEY
    generator:
      type: uuid
  - name: PORT
    type: integer
    default: 8080
  - name: HOST
    type: string
    fallback: "CLI_TEST_HOST"
    default: "localhost"
`
	tmpYaml, err := os.CreateTemp("", "shroudenv-test-config-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp config: %v", err)
	}
	defer os.Remove(tmpYaml.Name())
	if _, err := tmpYaml.Write([]byte(yamlContent)); err != nil {
		t.Fatalf("failed to write config yaml: %v", err)
	}
	tmpYaml.Close()

	// Set fallback environment variable
	os.Setenv("CLI_TEST_HOST", "my-test-host")
	defer os.Unsetenv("CLI_TEST_HOST")

	// 4. Run bootstrap in non-interactive mode
	bootstrapCmd := RootCmd
	bootstrapCmd.SetArgs([]string{"bootstrap", "-f", tmpYaml.Name(), "--non-interactive"})
	if err := bootstrapCmd.Execute(); err != nil {
		t.Fatalf("bootstrap command failed: %v", err)
	}

	// 5. Verify results in DB
	database, _, key, lock, err := LoadDBAndKeyShared()
	if err != nil {
		t.Fatalf("failed to load db: %v", err)
	}
	defer lock.Unlock()

	p := database.GetProject("cli-test-app")
	if p == nil {
		t.Fatal("expected project 'cli-test-app' to be created")
	}
	e := p.GetEnvironment("development")
	if e == nil {
		t.Fatal("expected environment 'development' to be created")
	}

	secrets, err := e.GetSecrets(key)
	if err != nil {
		t.Fatalf("failed to decrypt secrets: %v", err)
	}

	if secrets["PORT"] != "8080" {
		t.Errorf("expected PORT to be '8080', got %q", secrets["PORT"])
	}
	if secrets["HOST"] != "my-test-host" {
		t.Errorf("expected HOST to be 'my-test-host', got %q", secrets["HOST"])
	}
	if len(secrets["API_KEY"]) == 0 {
		t.Error("expected generated API_KEY, got empty string")
	}
}

func TestBootstrapCmd_EnvironmentAlreadyExistsGuardrail(t *testing.T) {
	// 1. Setup temp database and master key (N-01: prevents testutil from compiling to production)
	tmpDB, err := os.CreateTemp("", "shroudenv-test-db-*.json")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpDBPath := tmpDB.Name()
	tmpDB.Close()
	os.Remove(tmpDBPath) // delete so it starts fresh/non-existent
	defer os.Remove(tmpDBPath)

	os.Setenv("SHROUDENV_DB_PATH", tmpDBPath)
	os.Setenv("SHROUDENV_MASTER_KEY", strings.Repeat("a", 64))
	defer os.Unsetenv("SHROUDENV_DB_PATH")
	defer os.Unsetenv("SHROUDENV_MASTER_KEY")

	// 2. Initialize db
	initCmd := RootCmd
	initCmd.SetArgs([]string{"init", "--force"})
	if err := initCmd.Execute(); err != nil {
		t.Fatalf("init command failed: %v", err)
	}

	// 3. Create mock yaml
	yamlContent := `
version: "1"
project: "cli-test-app"
variables:
  - name: PORT
    type: integer
    default: 8080
`
	tmpYaml, err := os.CreateTemp("", "shroudenv-test-config-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp config: %v", err)
	}
	defer os.Remove(tmpYaml.Name())
	if _, err := tmpYaml.Write([]byte(yamlContent)); err != nil {
		t.Fatalf("failed to write config yaml: %v", err)
	}
	tmpYaml.Close()

	// 4. Run first bootstrap command (should succeed)
	bootstrapCmd1 := RootCmd
	bootstrapCmd1.SetArgs([]string{"bootstrap", "-f", tmpYaml.Name(), "-e", "staging", "--non-interactive"})
	if err := bootstrapCmd1.Execute(); err != nil {
		t.Fatalf("first bootstrap execution failed: %v", err)
	}

	// 5. Run second bootstrap command (should fail because environment staging already exists)
	bootstrapCmd2 := RootCmd
	bootstrapCmd2.SetArgs([]string{"bootstrap", "-f", tmpYaml.Name(), "-e", "staging", "--non-interactive"})
	err = bootstrapCmd2.Execute()
	if err == nil {
		t.Fatal("expected second bootstrap execution to fail, but it succeeded")
	}

	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected error message to contain 'already exists', got: %v", err)
	}
}
