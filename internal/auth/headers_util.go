package auth

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"github.com/bmf/chaperone/internal/log"
)

// findHeaderVariants finds all headers with names matching targetName (case-insensitive).
// Returns a sorted list of actual header names found in the request.
// This is used to detect existing headers that might have different capitalizations
// than what we're trying to inject.
func findHeaderVariants(r *http.Request, targetName string) []string {
	var found []string
	targetLower := strings.ToLower(targetName)

	for name := range r.Header {
		if strings.ToLower(name) == targetLower {
			found = append(found, name)
		}
	}

	sort.Strings(found)
	return found
}

// setHeaderPreservingCapitalization sets a header, preserving existing capitalization if present.
// This handles the case where a client has already set a header with a different capitalization
// (e.g., "x-api-key" vs "X-API-Key" vs "X-Api-Key").
//
// Behavior:
// - If no existing header found: sets header with targetName capitalization
// - If one existing header found: replaces it using its capitalization, logs warning
// - If multiple existing headers found: uses first (alphabetically), removes others, logs warning
//
// Returns true if an existing header was found and replaced.
func setHeaderPreservingCapitalization(ctx context.Context, r *http.Request, targetName, value string) bool {
	variants := findHeaderVariants(r, targetName)

	if len(variants) == 0 {
		// No existing header, set with target capitalization
		r.Header.Set(targetName, value)
		return false
	}

	// Existing header(s) found - warn the user (every time)
	logger := log.FromContext(ctx)
	if len(variants) > 1 {
		logger.Warn("⚠️  Multiple header capitalization variants found in request - using first and removing others",
			"target_header", targetName,
			"found_variants", variants,
			"action", "using '"+variants[0]+"' and removing others",
		)
	} else {
		logger.Warn("⚠️  Existing header found with different capitalization - replacing with matching capitalization",
			"target_header", targetName,
			"existing_capitalization", variants[0],
			"action", "replacing existing header value",
		)
	}

	// Use first variant's capitalization (alphabetically first if multiple)
	preservedName := variants[0]

	// Delete all variants
	for _, name := range variants {
		delete(r.Header, name)
	}

	// Set using preserved capitalization
	r.Header.Set(preservedName, value)

	return true
}
