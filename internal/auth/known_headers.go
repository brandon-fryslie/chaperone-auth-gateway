package auth

import "strings"

// KnownAuthHeaders is the canonical list of headers commonly used for authentication.
// These are automatically stripped from requests for security (internal/proxy/handlers.go)
// and used for detection heuristics (internal/init/heuristics.go).
//
// SINGLE SOURCE OF TRUTH: This is the only definition of known auth headers.
// All other packages must import from here.
var KnownAuthHeaders = []string{
	"authorization",
	"x-api-key",
	"x-auth-token",
	"api-key",
	"apikey",
	"x-access-token",
	"x-token",
	"token",
	"bearer",
	"x-session-token",
	"x-csrf-token",
	"x-xsrf-token",
}

// IsKnownAuthHeader returns true if the header is a known auth header (case-insensitive).
func IsKnownAuthHeader(header string) bool {
	headerLower := strings.ToLower(header)
	for _, known := range KnownAuthHeaders {
		if headerLower == known {
			return true
		}
	}
	return false
}
