package service

import (
	"testing"
)

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
			p := NewURLPattern(tt.pattern)
			result := p.Matches(tt.host, "")
			if result != tt.expected {
				t.Errorf("URLPattern(%q).Matches(%q, '') = %v, want %v",
					tt.pattern, tt.host, result, tt.expected)
			}
		})
	}
}

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
		{"exact path with trailing", "example.com/api/users", "example.com", "/api/users", true},

		// Single-level wildcard (*)
		{"single wildcard matches one level", "example.com/api/*", "example.com", "/api/users", true},
		{"single wildcard matches different segment", "example.com/api/*", "example.com", "/api/posts", true},
		{"single wildcard no match deep path", "example.com/api/*", "example.com", "/api/users/123", false},
		{"single wildcard no match different prefix", "example.com/api/*", "example.com", "/users/123", false},
		{"wildcard in middle", "example.com/api/*/users", "example.com", "/api/v1/users", true},
		{"wildcard in middle no match", "example.com/api/*/users", "example.com", "/api/v1/posts", false},

		// Multi-level wildcard (**)
		{"double wildcard at end", "example.com/api/**", "example.com", "/api/users", true},
		{"double wildcard matches deep", "example.com/api/**", "example.com", "/api/users/123/posts", true},
		{"double wildcard matches immediate", "example.com/api/**", "example.com", "/api", true},
		{"double wildcard no match wrong prefix", "example.com/api/**", "example.com", "/users", false},

		// Double wildcard with suffix
		{"double wildcard with suffix", "example.com/**/index.html", "example.com", "/index.html", true},
		{"double wildcard with suffix deep", "example.com/**/index.html", "example.com", "/api/index.html", true},
		{"double wildcard with suffix very deep", "example.com/**/index.html", "example.com", "/api/v1/docs/index.html", true},
		{"double wildcard with suffix no match", "example.com/**/index.html", "example.com", "/api/other.html", false},

		// Double wildcard in middle
		{"double wildcard in middle", "example.com/api/**/users", "example.com", "/api/users", true},
		{"double wildcard in middle with intermediate", "example.com/api/**/users", "example.com", "/api/v1/users", true},
		{"double wildcard in middle deep", "example.com/api/**/users", "example.com", "/api/v1/internal/users", true},
		{"double wildcard in middle no match suffix", "example.com/api/**/users", "example.com", "/api/v1/posts", false},

		// Edge cases
		{"pattern with multiple wildcards", "example.com/*/api/*", "example.com", "/v1/api/users", true},
		{"pattern with multiple wildcards no match", "example.com/*/api/*", "example.com", "/v1/users", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewURLPattern(tt.pattern)
			result := p.Matches(tt.host, tt.path)
			if result != tt.expected {
				t.Errorf("URLPattern(%q).Matches(%q, %q) = %v, want %v",
					tt.pattern, tt.host, tt.path, result, tt.expected)
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

		{"wildcard host with wildcard path", "*.example.com/api/*", "api.example.com", "/api/users", true},
		{"wildcard host with double wildcard path", "*.example.com/**/index.html", "docs.example.com", "/guide/index.html", true},

		// Real-world examples
		{"anthropic block all", "anthropic.com", "anthropic.com", "/api/chat", true},
		{"anthropic block subdomain", "anthropic.com", "api.anthropic.com", "/v1/messages", true},
		{"datadog block all", "datadoghq.com", "app.datadoghq.com", "/api/events", true},
		{"specific API path", "api.example.com/v1/*", "api.example.com", "/v1/users", true},
		{"specific API path no match version", "api.example.com/v1/*", "api.example.com", "/v2/users", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewURLPattern(tt.pattern)
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
		// Empty and edge cases
		{"empty path in URL", "example.com/api", "example.com", "", false},
		{"path without leading slash", "example.com/api", "example.com", "api", true}, // Should normalize
		{"pattern without leading slash", "example.com/api", "example.com", "/api", true},

		// Trailing slashes (path.Clean handles these)
		{"trailing slash in pattern", "example.com/api/", "example.com", "/api", true},
		{"trailing slash in path", "example.com/api", "example.com", "/api/", true},

		// Double slashes (path.Clean handles these)
		{"double slash in pattern", "example.com//api", "example.com", "/api", true},
		{"double slash in path", "example.com/api", "example.com", "//api", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewURLPattern(tt.pattern)
			result := p.Matches(tt.host, tt.path)
			if result != tt.expected {
				t.Errorf("URLPattern(%q).Matches(%q, %q) = %v, want %v",
					tt.pattern, tt.host, tt.path, result, tt.expected)
			}
		})
	}
}
