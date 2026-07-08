package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"shroudenv/pkg/keyring"
)

func init() {
	keyring.MockInit()
}

func TestSecretCommands(t *testing.T) {
	// 1. Setup temp database path (N-01: prevents testutil from compiling to production)
	tmpDB, err := os.CreateTemp("", "shroudenv-test-secret-db-*.json")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpDBPath := tmpDB.Name()
	tmpDB.Close()
	os.Remove(tmpDBPath) // delete so it starts fresh
	defer os.Remove(tmpDBPath)

	os.Setenv("SHROUDENV_DB_PATH", tmpDBPath)
	defer os.Unsetenv("SHROUDENV_DB_PATH")

	// 2. Initialize the database salt and files
	initCmd := RootCmd
	initCmd.SetArgs([]string{"init", "--force"})
	if err := initCmd.Execute(); err != nil {
		t.Fatalf("init command failed: %v", err)
	}

	// 3. Create a project and environment
	projCmd := RootCmd
	projCmd.SetArgs([]string{"project", "create", "-p", "testproject"})
	if err := projCmd.Execute(); err != nil {
		t.Fatalf("project create failed: %v", err)
	}

	envCmd := RootCmd
	envCmd.SetArgs([]string{"env", "create", "-p", "testproject", "-e", "dev"})
	if err := envCmd.Execute(); err != nil {
		t.Fatalf("env create failed: %v", err)
	}

	// 4. Set a short secret and a long secret
	setCmd1 := RootCmd
	setCmd1.SetArgs([]string{"secret", "set", "-p", "testproject", "-e", "dev", "SHORT_KEY", "123"})
	if err := setCmd1.Execute(); err != nil {
		t.Fatalf("failed to set short secret: %v", err)
	}

	setCmd2 := RootCmd
	setCmd2.SetArgs([]string{"secret", "set", "-p", "testproject", "-e", "dev", "LONG_KEY", "supersecretpassword"})
	if err := setCmd2.Execute(); err != nil {
		t.Fatalf("failed to set long secret: %v", err)
	}

	// 5. Run secret list and check that values are masked
	// We want to capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	listCmd := RootCmd
	listCmd.SetArgs([]string{"secret", "list", "-p", "testproject", "-e", "dev"})
	err = listCmd.Execute()
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("secret list failed: %v", err)
	}

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	// Assertions for list command output
	if !strings.Contains(output, "SHORT_KEY=****") {
		t.Errorf("expected SHORT_KEY to be masked as ****, got output: %q", output)
	}
	if !strings.Contains(output, "LONG_KEY=sup...********") {
		t.Errorf("expected LONG_KEY to be masked as sup...********, got output: %q", output)
	}
	if strings.Contains(output, "supersecretpassword") {
		t.Errorf("plaintext secret value was leaked in list command output: %q", output)
	}

	// 6. Run secret inspect for both keys
	// Capture stdout for SHORT_KEY
	r1, w1, _ := os.Pipe()
	os.Stdout = w1

	inspectCmd1 := RootCmd
	inspectCmd1.SetArgs([]string{"secret", "inspect", "-p", "testproject", "-e", "dev", "SHORT_KEY"})
	err = inspectCmd1.Execute()
	w1.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("inspect SHORT_KEY failed: %v", err)
	}

	var buf1 bytes.Buffer
	_, _ = io.Copy(&buf1, r1)
	val1 := strings.TrimSpace(buf1.String())
	if val1 != "123" {
		t.Errorf("expected inspected value to be '123', got %q", val1)
	}

	// Capture stdout for LONG_KEY
	r2, w2, _ := os.Pipe()
	os.Stdout = w2

	inspectCmd2 := RootCmd
	inspectCmd2.SetArgs([]string{"secret", "inspect", "-p", "testproject", "-e", "dev", "LONG_KEY"})
	err = inspectCmd2.Execute()
	w2.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("inspect LONG_KEY failed: %v", err)
	}

	var buf2 bytes.Buffer
	_, _ = io.Copy(&buf2, r2)
	val2 := strings.TrimSpace(buf2.String())
	if val2 != "supersecretpassword" {
		t.Errorf("expected inspected value to be 'supersecretpassword', got %q", val2)
	}

	// 7. Test non-existent key inspection
	inspectCmd3 := RootCmd
	inspectCmd3.SetArgs([]string{"secret", "inspect", "-p", "testproject", "-e", "dev", "NON_EXISTENT"})
	err = inspectCmd3.Execute()
	if err == nil {
		t.Error("expected inspection of non-existent key to fail, got nil")
	}
}
