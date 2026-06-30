package cmd

import (
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"testing"
	"time"
)

// TestHelperProcess is the helper function that we run as a subprocess.
// We use the GO_WANT_HELPER_PROCESS flag to distinguish this execution path.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	// This is the subprocess spawned by the helper process.
	// We want it to be a long-running process so we can test signal forwarding.
	var subCmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// Ping localhost 20 times (approx 20 seconds)
		subCmd = exec.Command("ping", "-n", "20", "127.0.0.1")
	} else {
		// Sleep for 20 seconds
		subCmd = exec.Command("sleep", "20")
	}

	err := runSubprocess(subCmd)
	if err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestRunSubprocess_NormalExit(t *testing.T) {
	// Test that runSubprocess exits normally when the child command exits normally.
	var subCmd *exec.Cmd
	if runtime.GOOS == "windows" {
		subCmd = exec.Command("cmd", "/c", "echo hello")
	} else {
		subCmd = exec.Command("echo", "hello")
	}

	err := runSubprocess(subCmd)
	if err != nil {
		t.Fatalf("runSubprocess failed: %v", err)
	}
}

func TestRunSubprocess_SignalForwarding(t *testing.T) {
	// Skip on Windows because Windows does not support sending unix signals (like SIGTERM) to subprocesses.
	if runtime.GOOS == "windows" {
		t.Skip("Skipping Unix signal forwarding test on Windows")
	}

	// We spawn the test binary itself, running the TestHelperProcess function.
	helperCmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess")
	helperCmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")

	if err := helperCmd.Start(); err != nil {
		t.Fatalf("Failed to start helper process: %v", err)
	}

	// Give the helper process a moment to start and set up signal handlers.
	time.Sleep(200 * time.Millisecond)

	// Send SIGTERM to the helper process.
	// Our runSubprocess function in the helper process should intercept this SIGTERM
	// and forward it to its child (the sleep command).
	if err := helperCmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("Failed to send SIGTERM: %v", err)
	}

	// Wait for the helper process to finish.
	doneChan := make(chan error, 1)
	go func() {
		doneChan <- helperCmd.Wait()
	}()

	select {
	case err := <-doneChan:
		if err == nil {
			// Clean exit is successful
		} else {
			// An exit error is expected since the helper exited with code 1 after child termination.
			t.Logf("Helper process exited with error as expected: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout: Helper process did not exit after receiving SIGTERM")
	}
}
