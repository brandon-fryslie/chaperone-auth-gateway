package examine

import (
	"path/filepath"
	"strings"
)

// alwaysIncludeHeaders are header names that should always be logged if present,
// regardless of value heuristics. Case-insensitive matching.
var alwaysIncludeHeaders = []string{
	"authorization",
	"x-api-key",
	"x-auth-token",
	"x-access-token",
	"x-secret-token",
	"api-key",
	"api-token",
	"access-token",
	"x-token",
	"x-key",
}

// knownAuthSchemes are auth scheme prefixes that indicate an authentication header.
// If a header value starts with one of these, it's considered auth-relevant.
// Case-insensitive matching.
var knownAuthSchemes = []string{
	"Bearer ",
	"Basic ",
	"Digest ",
	"AWS4-HMAC-SHA256 ",
	"AWS4-HMAC-SHA512 ",
	"Hawk ",
	"DPoP ",
	"VAPID ",
	"HOBA ",
	"Mutual ",
	"scram-sha-1 ",
	"scram-sha-256 ",
}

// noAuthHeaderPatterns are patterns that match headers that never contain authentication.
// Patterns are case-insensitive and support glob matching (* wildcard).
// Requests will log all headers EXCEPT those matching these patterns to reduce noise.
var noAuthHeaderPatterns = []string{
	// Content negotiation
	"accept",
	"accept-charset",
	"accept-encoding",
	"accept-language",

	// Content description
	"content-type",
	"content-length",
	"content-encoding",
	"content-language",

	// Caching
	"cache-control",
	"pragma",
	"expires",
	"if-match",
	"if-modified-since",
	"if-none-match",
	"if-unmodified-since",
	"etag",
	"last-modified",
	"age",

	// Connection management
	"connection",
	"keep-alive",
	"upgrade",
	"transfer-encoding",
	"te",
	"trailer",

	// Request context
	"host", // Already shown in URL
	"referer",
	"origin",
	"from",

	// User agent and client hints
	"user-agent",
	"sec-ch-ua",
	"sec-ch-ua-mobile",
	"sec-ch-ua-platform",
	"sec-fetch-dest",
	"sec-fetch-mode",
	"sec-fetch-site",
	"sec-fetch-user",
	"upgrade-insecure-requests",

	// CORS
	"access-control-request-method",
	"access-control-request-headers",
	"access-control-allow-origin",
	"access-control-allow-methods",
	"access-control-allow-headers",
	"access-control-max-age",
	"access-control-allow-credentials",
	"access-control-expose-headers",

	// Other non-auth
	"date",
	"vary",
	"via",
	"warning",
	"dnt",
	"x-requested-with",
	"range",
	"if-range",

	// WebSocket
	"sec-websocket-key",
	"sec-websocket-version",
	"sec-websocket-extensions",
	"sec-websocket-protocol",

	// Vendor-specific non-auth headers (glob patterns)
	"x-stainless-*",
}

// IsAuthRelevant returns true if the header could potentially contain authentication data.
// It checks three criteria:
// 1. If the header name is in the always-include list, return true
// 2. If the value starts with a known auth scheme, return true
// 3. Otherwise, apply exclusion patterns and value heuristics
//
// Header name matching is case-insensitive.
func IsAuthRelevant(name, value string) bool {
	nameLower := strings.ToLower(name)

	// Check if header name is in always-include list
	for _, header := range alwaysIncludeHeaders {
		if nameLower == header {
			return true
		}
	}

	// Check if value starts with a known auth scheme
	for _, scheme := range knownAuthSchemes {
		if strings.HasPrefix(value, scheme) {
			return true
		}
		// Also check case-insensitively for schemes
		if strings.HasPrefix(strings.ToLower(value), strings.ToLower(scheme)) {
			return true
		}
	}

	// Check exclusion patterns
	for _, pattern := range noAuthHeaderPatterns {
		if matchPattern(pattern, nameLower) {
			return false
		}
	}

	// Apply value heuristic: skip if value is less than 20 characters
	// Real credentials (API keys, tokens, etc.) are substantially longer than this
	if len(value) < 20 {
		return false
	}

	return true
}

// matchPattern performs case-insensitive glob matching.
// Supports * wildcard for matching any sequence of characters.
func matchPattern(pattern, name string) bool {
	// filepath.Match performs glob matching with * wildcard support
	// Both pattern and name are already lowercase at this point
	matched, _ := filepath.Match(pattern, name)
	return matched
}
