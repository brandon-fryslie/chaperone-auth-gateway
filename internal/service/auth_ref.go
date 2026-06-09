package service

import "strings"

// headerPrefix is the canonical scheme for header auth strategy refs ("header:X-API-Key").
const headerPrefix = "header:"

// CanonicalAuthRef folds the two accepted spellings of an auth strategy into one
// canonical reference, so callers can compare strategy identity by string equality.
//
// Accepted inputs:
//   - combined: authStrategy="header:x-api-key" (headerName ignored)
//   - separate: authStrategy="header", headerName="x-api-key"
//   - simple:   authStrategy="bearer"
//
// This is the [LAW:one-source-of-truth] for "are these two auth strategies the
// same"; both the config→Service bridge and the grant enforcer route through it.
func CanonicalAuthRef(authStrategy, headerName string) string {
	if strings.HasPrefix(authStrategy, headerPrefix) {
		return authStrategy
	}
	if authStrategy == "header" && headerName != "" {
		return headerPrefix + headerName
	}
	return authStrategy
}

// HeaderNameFromRef returns the header name carried by a canonical header strategy
// ref ("header:X-API-Key" → "X-API-Key", true) or ("", false) for non-header strategies.
func HeaderNameFromRef(ref string) (string, bool) {
	if name, ok := strings.CutPrefix(ref, headerPrefix); ok {
		return name, true
	}
	return "", false
}
