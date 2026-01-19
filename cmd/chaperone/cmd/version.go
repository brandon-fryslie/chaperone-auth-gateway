package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// Version is set at build time using -ldflags
var Version = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  "Print the version of the chaperone binary.",
	Run: func(cmd *cobra.Command, args []string) {
		v := Version
		// If version is still "dev", try to read from VERSION file
		if v == "dev" {
			if versionFromFile := readVersionFile(); versionFromFile != "" {
				v = versionFromFile
			}
		}
		fmt.Printf("chaperone version %s\n", v)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

// readVersionFile attempts to read version from VERSION file in project root
func readVersionFile() string {
	// Try to find VERSION file relative to binary location
	exe, err := os.Executable()
	if err != nil {
		return ""
	}

	// Check common locations
	locations := []string{
		filepath.Join(filepath.Dir(exe), "VERSION"),
		filepath.Join(filepath.Dir(exe), "..", "VERSION"),
		"VERSION",
	}

	for _, loc := range locations {
		if data, err := os.ReadFile(loc); err == nil {
			return strings.TrimSpace(string(data))
		}
	}

	return ""
}
