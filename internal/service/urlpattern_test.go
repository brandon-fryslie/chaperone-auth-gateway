package service

import (
	"testing"
)

func mustParseURLPattern(t *testing.T, pattern string) *URLPattern {
	t.Helper()
	p, err := ParseURLPattern(pattern)
	if err != nil {
		t.Fatalf("ParseURLPattern(%q) failed: %v", pattern, err)
	}
	return p
}

func TestURLPattern_HostMatching(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		host     string
		expected bool
	}{
		// Exact host matching
		{"exact match", "example.com", "example.com", true},
		{"exact match with path requires path", "example.com/api", "example.com", false}, // Path in pattern requires path match
		{"no match different host", "example.com", "other.com", false},

		// Implicit wildcard - bare domain matches all subdomains
		{"bare domain matches itself", "example.com", "example.com", true},
		{"bare domain matches subdomain", "example.com", "api.example.com", true},
		{"bare domain matches deep subdomain", "example.com", "foo.bar.example.com", true},
		{"bare domain no match wrong domain", "example.com", "example.org", false},
		{"bare domain no match partial", "example.com", "notexample.com", false},

		// Explicit wildcard patterns
		{"wildcard matches subdomain", "*.example.com", "api.example.com", true},
		{"wildcard matches deep subdomain", "*.example.com", "foo.bar.example.com", true},
		{"wildcard matches bare domain", "*.example.com", "example.com", true},
		{"wildcard no match wrong domain", "*.example.com", "example.org", false},

		// Case insensitivity
		{"case insensitive host", "Example.COM", "example.com", true},
		{"case insensitive pattern", "example.com", "EXAMPLE.COM", true},
		{"case insensitive wildcard", "*.Example.COM", "api.example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := mustParseURLPattern(t, tt.pattern)
			result := p.Matches(tt.host, "")
			if result != tt.expected {
				t.Errorf("URLPattern(%q).Matches(%q, '') = %v, want %v",
					tt.pattern, tt.host, result, tt.expected)
			}
		})
	}
}

// Path matching goes through the shared pathPattern vocabulary (exact path or
// "<prefix>/*" subtree) under the one documented policy: request paths are
// normalized (dot-segments, duplicate slashes, trailing slash) and compared
// case-insensitively — identical to allowed_paths matching.
func TestURLPattern_PathMatching(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		host     string
		path     string
		expected bool
	}{
		// No path = match all paths
		{"no path matches root", "example.com", "example.com", "/", true},
		{"no path matches any path", "example.com", "example.com", "/api/users", true},

		// Exact path matching
		{"exact path match", "example.com/api", "example.com", "/api", true},
		{"exact path no match", "example.com/api", "example.com", "/users", false},
		{"exact path deep", "example.com/api/users", "example.com", "/api/users", true},

		// Subtree wildcard: matches every path BELOW the prefix, at any depth
		{"subtree matches one level", "example.com/api/*", "example.com", "/api/users", true},
		{"subtree matches deep path", "example.com/api/*", "example.com", "/api/users/123", true},
		{"subtree no match prefix itself", "example.com/api/*", "example.com", "/api", false},
		{"subtree no match different prefix", "example.com/api/*", "example.com", "/users/123", false},
		{"subtree no match sibling prefix", "example.com/api/*", "example.com", "/api2/users", false},
		{"root subtree matches everything", "example.com/*", "example.com", "/anything/at/all", true},

		// Normalization: the matcher judges the path an RFC-3986 router resolves
		{"dot-segment escape does not match", "example.com/api/*", "example.com", "/api/../admin", false},
		{"dot-segment within subtree matches", "example.com/api/*", "example.com", "/api/x/../users", true},
		{"duplicate slashes collapse", "example.com/api/*", "example.com", "//api//users", true},
		{"trailing slash drops", "example.com/api/users", "example.com", "/api/users/", true},

		// Case-insensitive: a drop rule cannot be bypassed by case tricks
		{"case variant still dropped", "example.com/admin/*", "example.com", "/Admin/keys", true},
		{"pattern case folded too", "example.com/Admin/*", "example.com", "/admin/keys", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := mustParseURLPattern(t, tt.pattern)
			result := p.Matches(tt.host, tt.path)
			if result != tt.expected {
				t.Errorf("URLPattern(%q).Matches(%q, %q) = %v, want %v",
					tt.pattern, tt.host, tt.path, result, tt.expected)
			}
		})
	}
}

// A drop rule outside the vocabulary must not construct: drop is a deny
// control, and a rule that silently failed to match would fail open.
func TestURLPattern_RejectsUnsupportedPatterns(t *testing.T) {
	for _, pattern := range []string{
		"example.com/**",            // segment globs are not part of the vocabulary
		"example.com/**/index.html", // ditto
		"example.com/api/*/users",   // mid-pattern wildcard
		"example.com/*/api/*",       // ditto
		"example.com/api/?",         // glob metachar
		"example.com/api/",          // non-canonical: trailing slash
		"example.com//api",          // non-canonical: duplicate slash
		"example.com/api/../admin",  // non-canonical: dot-segment
	} {
		t.Run(pattern, func(t *testing.T) {
			if _, err := ParseURLPattern(pattern); err == nil {
				t.Errorf("ParseURLPattern(%q) succeeded, want error", pattern)
			}
		})
	}
}

func TestURLPattern_CombinedMatching(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		host     string
		path     string
		expected bool
	}{
		// Combined host and path patterns
		{"wildcard host with path", "*.example.com/api", "api.example.com", "/api", true},
		{"wildcard host with path no match host", "*.example.com/api", "other.com", "/api", false},
		{"wildcard host with path no match path", "*.example.com/api", "api.example.com", "/users", false},

		{"wildcard host with subtree path", "*.example.com/api/*", "api.example.com", "/api/users", true},

		// Real-world examples
		{"anthropic block all", "anthropic.com", "anthropic.com", "/api/chat", true},
		{"anthropic block subdomain", "anthropic.com", "api.anthropic.com", "/v1/messages", true},
		{"datadog block all", "datadoghq.com", "app.datadoghq.com", "/api/events", true},
		{"specific API path", "api.example.com/v1/*", "api.example.com", "/v1/users", true},
		{"specific API path no match version", "api.example.com/v1/*", "api.example.com", "/v2/users", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := mustParseURLPattern(t, tt.pattern)
			result := p.Matches(tt.host, tt.path)
			if result != tt.expected {
				t.Errorf("URLPattern(%q).Matches(%q, %q) = %v, want %v",
					tt.pattern, tt.host, tt.path, result, tt.expected)
			}
		})
	}
}

func TestURLPattern_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		host     string
		path     string
		expected bool
	}{
		{"empty path normalizes to root", "example.com/api", "example.com", "", false},
		{"empty path matches exact root", "example.com/", "example.com", "", true},
		{"path without leading slash", "example.com/api", "example.com", "api", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := mustParseURLPattern(t, tt.pattern)
			result := p.Matches(tt.host, tt.path)
			if result != tt.expected {
				t.Errorf("URLPattern(%q).Matches(%q, %q) = %v, want %v",
					tt.pattern, tt.host, tt.path, result, tt.expected)
			}
		})
	}
}
