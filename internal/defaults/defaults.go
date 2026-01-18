// Package defaults provides shared default values and constants used throughout
// the application. This package exists to avoid magic numbers scattered throughout
// the codebase and to ensure consistency in default values.
package defaults

const (
	// DefaultMaxBodyBytes is the maximum body size for proxied requests (10MB).
	DefaultMaxBodyBytes = 10 * 1024 * 1024

	// DefaultExamineBodyBytes is the maximum body size logged in examine mode (4KB).
	DefaultExamineBodyBytes = 4096

	// Version is the application version string.
	// This should be overridden at build time for release builds.
	Version = "0.1.0"
)
