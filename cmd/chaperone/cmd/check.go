package cmd

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/bmf/chaperone/internal/config"
	"github.com/spf13/cobra"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check security configuration and show recommendations",
	Long: `Assess your Chaperone security posture.

Shows the current security configuration status for each protection layer
and provides recommendations for improvements.

This command is informational only - it always exits with code 0.`,
	RunE: runCheck,
}

func init() {
	rootCmd.AddCommand(checkCmd)
}

// Status icons
const (
	iconOK   = "✅"
	iconWarn = "⚠️ "
	iconInfo = "ℹ️ "
)

func runCheck(cmd *cobra.Command, args []string) error {
	// Try to load config (optional)
	var cfg *config.Config
	configPath := cfgFile
	if configPath == "" {
		// Try default paths
		if home, err := os.UserHomeDir(); err == nil {
			defaultPath := filepath.Join(home, ".config", "chaperone", "chaperone.toml")
			if _, err := os.Stat(defaultPath); err == nil {
				configPath = defaultPath
			}
		}
		if configPath == "" {
			// Try current directory
			if _, err := os.Stat("chaperone.toml"); err == nil {
				configPath = "chaperone.toml"
			}
		}
	}

	if configPath != "" {
		if loaded, err := config.Load(configPath); err == nil {
			cfg = loaded
		}
	}

	// Print header
	fmt.Println("Chaperone Security Check")
	fmt.Println("========================")
	fmt.Println()

	recommendations := []string{}

	// Layer 1: Credential Isolation
	fmt.Println("Layer 1: Credential Isolation")
	fmt.Printf("  %s Single-service mode: Each instance handles one credential\n", iconOK)
	fmt.Println()

	// Layer 2: Process Authentication
	fmt.Println("Layer 2: Process Authentication")
	placeholderStatus := checkPlaceholders(cfg)
	if placeholderStatus.allConfigured {
		fmt.Printf("  %s Placeholder auth: All services have placeholders configured\n", iconOK)
	} else if len(placeholderStatus.missing) > 0 {
		fmt.Printf("  %sPlaceholder auth: Not configured for: %s\n", iconInfo,
			joinMax(placeholderStatus.missing, 3))
		recommendations = append(recommendations,
			"Configure placeholder tokens for services without them")
	} else {
		fmt.Printf("  %sPlaceholder auth: No services configured\n", iconInfo)
	}
	fmt.Println()

	// Layer 3: User/Permission Isolation
	fmt.Println("Layer 3: User/Permission Isolation")
	if isDedicatedUser() {
		fmt.Printf("  %s Dedicated user: Running as 'chaperone' user\n", iconOK)
	} else {
		currentUser := getCurrentUsername()
		fmt.Printf("  %sRunning as '%s' (consider dedicated 'chaperone' user)\n", iconWarn, currentUser)
		recommendations = append(recommendations,
			"Run as dedicated 'chaperone' user for credential file isolation")
	}

	// Check if using Unix socket mode
	if cfg != nil && cfg.Server.Socket != "" {
		fmt.Printf("  %s Unix socket: Using socket at %s\n", iconOK, cfg.Server.Socket)
	} else {
		fmt.Printf("  %sUsing TCP port (consider Unix socket for better isolation)\n", iconWarn)
		recommendations = append(recommendations,
			"Use Unix socket mode for better permission isolation: --socket /path/to/chaperone.sock")
	}
	fmt.Println()

	// Layer 4: Network Hardening
	fmt.Println("Layer 4: Network Hardening")
	fmt.Printf("  %s Upstream TLS: Certificate validation enabled\n", iconOK)
	fmt.Printf("  %s System proxy: Ignored for upstream connections\n", iconOK)
	fmt.Printf("  %sCA bundle: Using system CAs (bundled CA not yet supported)\n", iconWarn)
	fmt.Println()

	// Layer 5: Runtime Protection
	fmt.Println("Layer 5: Runtime Protection")
	if cfg != nil && cfg.Audit.Enabled {
		path := cfg.Audit.Path
		if path == "" || path == "stdout" {
			path = "stdout"
		}
		fmt.Printf("  %s Audit logging: Enabled (%s)\n", iconOK, path)
	} else {
		fmt.Printf("  %sAudit logging: Not enabled\n", iconInfo)
		recommendations = append(recommendations,
			"Enable audit logging in config: [audit] enabled = true")
	}
	fmt.Println()

	// Recommendations
	if len(recommendations) > 0 {
		fmt.Println("Recommendations:")
		for _, rec := range recommendations {
			fmt.Printf("  • %s\n", rec)
		}
		fmt.Println()
	}

	fmt.Println("See SECURITY.md for the full security model.")

	return nil
}

type placeholderCheck struct {
	allConfigured bool
	missing       []string
}

func checkPlaceholders(cfg *config.Config) placeholderCheck {
	if cfg == nil || len(cfg.Services) == 0 {
		return placeholderCheck{allConfigured: false, missing: nil}
	}

	var missing []string
	for name, svc := range cfg.Services {
		if svc.Placeholder == "" {
			missing = append(missing, name)
		}
	}

	return placeholderCheck{
		allConfigured: len(missing) == 0,
		missing:       missing,
	}
}

func isDedicatedUser() bool {
	current, err := user.Current()
	if err != nil {
		return false
	}
	return current.Username == "chaperone"
}

func getCurrentUsername() string {
	current, err := user.Current()
	if err != nil {
		return "unknown"
	}
	return current.Username
}

func joinMax(items []string, max int) string {
	if len(items) <= max {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:max], ", ") + fmt.Sprintf(" (+%d more)", len(items)-max)
}
