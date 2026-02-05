package run

import (
	"fmt"
	"io"
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

// PrintRunBanner prints the startup banner for run mode to the given writer.
// Typically called with os.Stderr to display before starting the child process.
func PrintRunBanner(w io.Writer, cfg RunBannerConfig) {
	fmt.Fprintf(w, "\n%s=== Chaperone Run Mode ===%s\n\n", Cyan+Bold, Reset)
	fmt.Fprintf(w, "%sService:%s  %s\n", Blue+Bold, Reset, cfg.Service)
	fmt.Fprintf(w, "%sCommand:%s  %s\n", Blue+Bold, Reset, cfg.Command)
	fmt.Fprintf(w, "%sLog file:%s %s\n\n", Blue+Bold, Reset, cfg.LogPath)
}

// PrintExamineBanner prints the startup banner for examine mode to the given writer.
// Includes optional HAR recording information if enabled.
func PrintExamineBanner(w io.Writer, cfg ExamineBannerConfig) {
	fmt.Fprintf(w, "\n%s=== Chaperone Examine Mode ===%s\n\n", Cyan+Bold, Reset)
	fmt.Fprintf(w, "%sCommand:%s %s\n", Blue+Bold, Reset, cfg.Command)
	fmt.Fprintf(w, "%sLog file:%s %s\n", Blue+Bold, Reset, cfg.LogPath)
	fmt.Fprintf(w, "%sTo print logs from latest run:%s tail -F /tmp/chaperone-examine.latest.log\n\n", Blue+Bold, Reset)

	if cfg.HAREnabled {
		fmt.Fprintf(w, "%sHAR Recording:%s %sENABLED%s\n", Blue+Bold, Reset, Green+Bold, Reset)
		fmt.Fprintf(w, "%sHAR file:%s %s\n\n", Blue+Bold, Reset, cfg.HARPath)
	}
}
