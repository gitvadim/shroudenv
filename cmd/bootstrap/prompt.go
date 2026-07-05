package bootstrap

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/term"
)

type TerminalInputReader struct {
	reader *bufio.Reader
	once   sync.Once
}

func (r *TerminalInputReader) getReader() *bufio.Reader {
	r.once.Do(func() {
		r.reader = bufio.NewReader(os.Stdin)
	})
	return r.reader
}

// ReadInput reads standard line input from standard input.
func (r *TerminalInputReader) ReadInput(prompt string) (string, error) {
	fmt.Print(prompt)
	reader := r.getReader()
	text, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(text, "\r\n"), nil
}

// ReadSensitiveInput reads terminal input without echoing (password masking).
// Safe against keyboard interrupts (restores echo status on SIGINT/SIGTERM).
func (r *TerminalInputReader) ReadSensitiveInput(prompt string) (string, error) {
	fd := int(os.Stdin.Fd())
	var oldState *term.State
	var err error
	if term.IsTerminal(fd) {
		oldState, err = term.GetState(fd)
		if err != nil {
			return "", err
		}
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	doneChan := make(chan struct{})
	defer close(doneChan)

	go func() {
		select {
		case <-sigChan:
			if oldState != nil {
				_ = term.Restore(fd, oldState)
			}
			fmt.Println()
			// Re-raise SIGINT so the shell sees exit code 130 naturally
			signal.Reset(os.Interrupt, syscall.SIGTERM)
			proc, _ := os.FindProcess(os.Getpid())
			if proc != nil {
				_ = proc.Signal(os.Interrupt)
			}
		case <-doneChan:
			return
		}
	}()

	fmt.Print(prompt)
	passwordBytes, err := term.ReadPassword(fd)
	fmt.Println() // Print newline after password entry
	if err != nil {
		return "", err
	}
	return string(passwordBytes), nil
}
