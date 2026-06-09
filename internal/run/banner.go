package run

import (
	"fmt"
	"os"
)

// ANSI color codes for terminal output.
// These are shared across run and examine modes for consistent formatting.
const (
	Reset   = "\033[0m"
	Bold    = "\033[1m"
	Cyan    = "\033[36m"
	Blue    = "\033[34m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Magenta = "\033[35m"
)

// RunBannerConfig contains configuration for the run mode startup banner.
type RunBannerConfig struct {
	Service string // Service name being run
	Command string // Formatted command string (command + args)
	LogPath string // Path to log file
}

// ExamineBannerConfig contains configuration for the examine mode startup banner.
type ExamineBannerConfig struct {
	Command    string // Formatted command string
	LogPath    string // Path to log file
	HAREnabled bool   // Whether HAR recording is enabled
	HARPath    string // Path to HAR file (if enabled)
}

// PrintRunBanner prints the run mode startup banner to stderr before the child
// process starts. Banner output is best-effort terminal decoration.
func PrintRunBanner(cfg RunBannerConfig) {
	fmt.Fprintf(os.Stderr, "\n%s=== Chaperone Run Mode ===%s\n\n", Cyan+Bold, Reset)
	fmt.Fprintf(os.Stderr, "%sService:%s  %s\n", Blue+Bold, Reset, cfg.Service)
	fmt.Fprintf(os.Stderr, "%sCommand:%s  %s\n", Blue+Bold, Reset, cfg.Command)
	fmt.Fprintf(os.Stderr, "%sLog file:%s %s\n\n", Blue+Bold, Reset, cfg.LogPath)
}

// PrintExamineBanner prints the examine mode startup banner to stderr, including
// HAR recording details when enabled. Banner output is best-effort terminal decoration.
func PrintExamineBanner(cfg ExamineBannerConfig) {
	fmt.Fprintf(os.Stderr, "\n%s=== Chaperone Examine Mode ===%s\n\n", Cyan+Bold, Reset)
	fmt.Fprintf(os.Stderr, "%sCommand:%s %s\n", Blue+Bold, Reset, cfg.Command)
	fmt.Fprintf(os.Stderr, "%sLog file:%s %s\n", Blue+Bold, Reset, cfg.LogPath)
	fmt.Fprintf(os.Stderr, "%sTo print logs from latest run:%s tail -F /tmp/chaperone-examine.latest.log\n\n", Blue+Bold, Reset)

	if cfg.HAREnabled {
		fmt.Fprintf(os.Stderr, "%sHAR Recording:%s %sENABLED%s\n", Blue+Bold, Reset, Green+Bold, Reset)
		fmt.Fprintf(os.Stderr, "%sHAR file:%s %s\n\n", Blue+Bold, Reset, cfg.HARPath)
	}
}
