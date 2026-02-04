package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bmf/chaperone/internal/defaults"
	"github.com/spf13/cobra"
)

var (
	// rootCmd represents the base command when called without any subcommands
	rootCmd = &cobra.Command{
		Use:   "chaperone",
		Short: "A transparent local HTTPS proxy that injects API credentials",
		Long: `Chaperone is a transparent local HTTPS proxy that automatically
injects API credentials into requests on behalf of applications.

Applications never see or handle API keys - Chaperone injects them transparently.

For more information, visit: https://github.com/bmf/chaperone`,
		Version: defaults.Version,
	}

	// cfgFile holds the path to the config file (set via --config flag)
	cfgFile string

	// logFormat holds the log output format (text, json, logfmt)
	logFormat string
)

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). Only setup here should be initialization.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Set up completion command
	rootCmd.InitDefaultCompletionCmd()

	// Persistent config flag available to all subcommands
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "",
		"config file (default ~/.config/chaperone/chaperone.toml)")

	// Persistent log format flag available to all subcommands
	rootCmd.PersistentFlags().StringVar(&logFormat, "log-format", "text",
		"log output format: text (default, human-readable), json (structured), logfmt (key=value)")
}

// getConfigPath resolves the config file path using the following precedence:
// 1. Explicit -c/--config flag
// 2. ~/.config/chaperone/chaperone.toml
// 3. ./chaperone.toml (current directory)
// Returns an error if no config file is found.
func getConfigPath() (string, error) {
	// 1. Check explicit flag
	if cfgFile != "" {
		return cfgFile, nil
	}

	// 2. Check default path ~/.config/chaperone/chaperone.toml
	homeDir, err := os.UserHomeDir()
	if err == nil {
		defaultPath := filepath.Join(homeDir, ".config", "chaperone", "chaperone.toml")
		if _, err := os.Stat(defaultPath); err == nil {
			return defaultPath, nil
		}
	}

	// 3. Check current directory for chaperone.toml
	if _, err := os.Stat("chaperone.toml"); err == nil {
		return "chaperone.toml", nil
	}

	return "", fmt.Errorf("no config file found (checked -c flag, ~/.config/chaperone/chaperone.toml, ./chaperone.toml)")
}

// getCAPath returns the CA directory and file paths.
// Uses ~/.config/chaperone/ as the default location.
func getCAPath() (dir, keyPath, certPath string, err error) {
	// Get user home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", "", "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	// Build CA paths using ~/.config/chaperone
	dir = filepath.Join(homeDir, ".config", "chaperone")
	keyPath = filepath.Join(dir, "ca-key.pem")
	certPath = filepath.Join(dir, "ca-cert.pem")

	return dir, keyPath, certPath, nil
}
