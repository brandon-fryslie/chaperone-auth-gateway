package init

import (
	"regexp"
	"strconv"
	"strings"
)

// GeneralizePathPatterns converts concrete paths into generalized patterns.
// Examples:
//   - /users/123 → /users/*
//   - /api/v1/chat/completions → /api/v1/chat/completions/*
//   - /items/abc-def-123/details → /items/*/details/*
func GeneralizePathPatterns(paths []string) []string {
	if len(paths) == 0 {
		return []string{"/*"} // Default: allow all paths
	}

	// Pattern to match numeric IDs, UUIDs, and alphanumeric IDs
	numericPattern := regexp.MustCompile(`/\d+(/|$)`)
	uuidPattern := regexp.MustCompile(`/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}(/|$)`)
	// Only match alphanumeric sequences that look like IDs:
	// - Contains hyphens or underscores (abc-def, foo_bar)
	// - Or is very long (20+ chars) without hyphens/underscores
	alphanumericWithSeparatorsPattern := regexp.MustCompile(`/[a-zA-Z0-9]*[-_][a-zA-Z0-9_-]{7,}(/|$)`)
	longAlphanumericPattern := regexp.MustCompile(`/[a-zA-Z0-9]{20,}(/|$)`)

	generalized := make(map[string]bool)

	for _, path := range paths {
		// Apply patterns in order
		result := path

		// Replace UUIDs first (most specific)
		result = uuidPattern.ReplaceAllString(result, "/*/")

		// Replace numeric IDs
		result = numericPattern.ReplaceAllString(result, "/*/")

		// Replace alphanumeric IDs with separators (abc-def-123)
		result = alphanumericWithSeparatorsPattern.ReplaceAllString(result, "/*/")

		// Replace very long alphanumeric strings (likely hashes/IDs)
		result = longAlphanumericPattern.ReplaceAllString(result, "/*/")

		// Clean up duplicate wildcards
		result = strings.ReplaceAll(result, "/*/*", "/*")

		// Ensure trailing wildcard
		result = strings.TrimSuffix(result, "/")
		if !strings.HasSuffix(result, "/*") {
			result += "/*"
		}

		generalized[result] = true
	}

	// Convert to slice
	patterns := make([]string, 0, len(generalized))
	for pattern := range generalized {
		patterns = append(patterns, pattern)
	}

	return patterns
}

// InferMaxBodyBytes returns a reasonable max body size based on observed values.
// Adds 20% headroom to account for variance in request sizes.
func InferMaxBodyBytes(observedMax int64) int64 {
	if observedMax == 0 {
		return 1048576 // 1MB default if no bodies observed
	}

	// Add 20% headroom
	withHeadroom := int64(float64(observedMax) * 1.2)

	// Round up to nearest MB for cleaner config
	const mb = 1048576
	roundedMB := (withHeadroom + mb - 1) / mb
	return roundedMB * mb
}

// FormatBodySize returns a human-readable size string.
func FormatBodySize(bytes int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)

	switch {
	case bytes >= gb:
		return strconv.FormatFloat(float64(bytes)/float64(gb), 'f', 2, 64) + " GB"
	case bytes >= mb:
		return strconv.FormatFloat(float64(bytes)/float64(mb), 'f', 2, 64) + " MB"
	case bytes >= kb:
		return strconv.FormatFloat(float64(bytes)/float64(kb), 'f', 2, 64) + " KB"
	default:
		return strconv.FormatInt(bytes, 10) + " bytes"
	}
}
