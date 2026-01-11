package examine

import (
	"path/filepath"
	"strings"
)

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
// Headers that are definitionally non-auth (content negotiation, caching directives, etc.)
// return false to reduce noise in examine mode output.
//
// Matching is case-insensitive and supports glob patterns (* wildcard).
func IsAuthRelevant(name string) bool {
	nameLower := strings.ToLower(name)

	for _, pattern := range noAuthHeaderPatterns {
		if matchPattern(pattern, nameLower) {
			return false
		}
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
