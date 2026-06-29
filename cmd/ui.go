package cmd

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"shroudenv/pkg/db"
	"shroudenv/pkg/server"

	"github.com/spf13/cobra"
)

// FrontendFS is populated by main.go to avoid import cycle
var FrontendFS embed.FS

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Start shroudenv local server and GUI dashboard",
	Long:  `Launches the HTTP API server locally, serves the embedded frontend dashboard, and opens your default browser.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load DB and get Key (fails if not initialized)
		database, dbPath, key, err := LoadDBAndKey()
		if err != nil {
			return err
		}

		// Generate API token
		tokenBytes := make([]byte, 16)
		if _, err := rand.Read(tokenBytes); err != nil {
			return fmt.Errorf("failed to generate secure token: %w", err)
		}
		token := hex.EncodeToString(tokenBytes)

		// Find an available port starting from 4554
		port := 4554
		var listener net.Listener
		for {
			addr := fmt.Sprintf("127.0.0.1:%d", port)
			listener, err = net.Listen("tcp", addr)
			if err == nil {
				break
			}
			port++
			if port > 4600 {
				return fmt.Errorf("could not find an open port between 4554 and 4600: %w", err)
			}
		}

		// Start local server
		srv := server.NewServer(dbPath, key, token, FrontendFS)
		httpServer := &http.Server{
			Handler:        srv.Handler(),
			ReadTimeout:    5 * time.Second,
			WriteTimeout:   10 * time.Second,
			IdleTimeout:    120 * time.Second,
		}

		url := fmt.Sprintf("http://localhost:%d/?token=%s", port, token)
		fmt.Println("==========================================================")
		fmt.Printf(" shroudenv local server is running!\n")
		fmt.Printf(" Database: %s\n", dbPath)
		fmt.Printf(" Status:   Key unlocked via %s\n", getVaultStatus(database))
		fmt.Printf(" Server:   http://localhost:%d\n", port)
		fmt.Printf(" Token:    %s\n", token)
		fmt.Println("==========================================================")
		fmt.Printf("Opening dashboard: %s\n", url)
		fmt.Println("Press Ctrl+C to stop the server.")

		// Open browser
		openBrowser(url)

		// Serve requests
		return httpServer.Serve(listener)
	},
}

func getVaultStatus(d *db.Database) string {
	// A helper to show where the key came from, or environment variable, etc.
	if os.Getenv("SHROUDENV_MASTER_KEY") != "" {
		return "Environment Variable"
	}
	return "OS Keyring/Vault"
}

func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		// Safest command invocation on Windows for launching URLs
		err = exec.Command("cmd", "/c", "start", "", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open browser automatically: %v\n", err)
	}
}

func init() {
	RootCmd.AddCommand(uiCmd)
}
