package examine

import "testing"

func TestIsAuthRelevant(t *testing.T) {
	tests := []struct {
		name        string
		headerName  string
		headerValue string
		want        bool
	}{
		// Headers that should NOT be considered auth-relevant
		{
			name:        "content-type is not auth",
			headerName:  "Content-Type",
			headerValue: "application/json",
			want:        false,
		},
		{
			name:        "accept is not auth",
			headerName:  "Accept",
			headerValue: "application/json",
			want:        false,
		},
		{
			name:        "user-agent is not auth",
			headerName:  "User-Agent",
			headerValue: "Mozilla/5.0",
			want:        false,
		},
		{
			name:        "host is not auth",
			headerName:  "Host",
			headerValue: "example.com",
			want:        false,
		},
		// Case insensitive matching
		{
			name:        "content-type lowercase is not auth",
			headerName:  "content-type",
			headerValue: "text/html",
			want:        false,
		},
		{
			name:        "CONTENT-TYPE uppercase is not auth",
			headerName:  "CONTENT-TYPE",
			headerValue: "application/json",
			want:        false,
		},
		// Glob pattern matching - x-stainless-*
		{
			name:        "x-stainless-arch matches pattern",
			headerName:  "X-Stainless-Arch",
			headerValue: "arm64",
			want:        false,
		},
		{
			name:        "x-stainless-os matches pattern",
			headerName:  "X-Stainless-Os",
			headerValue: "linux",
			want:        false,
		},
		{
			name:        "x-stainless-runtime matches pattern",
			headerName:  "X-Stainless-Runtime",
			headerValue: "nodejs",
			want:        false,
		},
		{
			name:        "x-stainless- with any suffix matches",
			headerName:  "X-Stainless-Whatever",
			headerValue: "some-value",
			want:        false,
		},
		// Case insensitive glob matching
		{
			name:        "lowercase x-stainless-arch matches",
			headerName:  "x-stainless-arch",
			headerValue: "x86_64",
			want:        false,
		},
		{
			name:        "UPPERCASE X-STAINLESS-OS matches",
			headerName:  "X-STAINLESS-OS",
			headerValue: "windows",
			want:        false,
		},
		// Headers that SHOULD be considered auth-relevant (always-include list)
		{
			name:        "authorization with short value is auth-relevant",
			headerName:  "Authorization",
			headerValue: "Bearer sk-proj-abc123",
			want:        true,
		},
		{
			name:        "x-api-key is auth-relevant",
			headerName:  "X-API-Key",
			headerValue: "secret-key-123456",
			want:        true,
		},
		{
			name:        "api-key is auth-relevant",
			headerName:  "API-Key",
			headerValue: "my-secret-token",
			want:        true,
		},
		// Value heuristics - minimum length (20 chars)
		{
			name:        "short value filtered out (< 20 chars)",
			headerName:  "X-Request-ID",
			headerValue: "abc",
			want:        false,
		},
		{
			name:        "19 char value filtered out",
			headerName:  "X-Custom-Header",
			headerValue: "1234567890123456789",
			want:        false,
		},
		{
			name:        "20 char value kept",
			headerName:  "X-Custom-Header",
			headerValue: "12345678901234567890",
			want:        true,
		},
		{
			name:        "metadata header filtered if < 20 chars",
			headerName:  "Anthropic-Version",
			headerValue: "2023-06-01",
			want:        false,
		},
		{
			name:        "long version number kept if >= 20 chars",
			headerName:  "X-Version",
			headerValue: "v1.2.3.4.5.6.7.8.9.10",
			want:        true,
		},
		// Known auth schemes
		{
			name:        "Bearer scheme is auth-relevant",
			headerName:  "X-Custom-Auth",
			headerValue: "Bearer token123",
			want:        true,
		},
		{
			name:        "Basic scheme is auth-relevant",
			headerName:  "X-Custom-Auth",
			headerValue: "Basic dXNlcjpwYXNz",
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsAuthRelevant(tt.headerName, tt.headerValue)
			if got != tt.want {
				t.Errorf("IsAuthRelevant(%q, %q) = %v, want %v", tt.headerName, tt.headerValue, got, tt.want)
			}
		})
	}
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		input   string
		want    bool
	}{
		{
			name:    "exact match",
			pattern: "content-type",
			input:   "content-type",
			want:    true,
		},
		{
			name:    "no match",
			pattern: "content-type",
			input:   "authorization",
			want:    false,
		},
		{
			name:    "glob * matches anything",
			pattern: "x-stainless-*",
			input:   "x-stainless-arch",
			want:    true,
		},
		{
			name:    "glob * matches empty",
			pattern: "x-stainless-*",
			input:   "x-stainless-",
			want:    true,
		},
		{
			name:    "glob * matches multiple segments",
			pattern: "x-stainless-*",
			input:   "x-stainless-something-else",
			want:    true,
		},
		{
			name:    "glob * doesn't match without prefix",
			pattern: "x-stainless-*",
			input:   "x-stainles-arch",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchPattern(tt.pattern, tt.input)
			if got != tt.want {
				t.Errorf("matchPattern(%q, %q) = %v, want %v", tt.pattern, tt.input, got, tt.want)
			}
		})
	}
}
