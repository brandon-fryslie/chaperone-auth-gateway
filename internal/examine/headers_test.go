package examine

import "testing"

func TestIsAuthRelevant(t *testing.T) {
	tests := []struct {
		name       string
		headerName string
		want       bool
	}{
		// Headers that should NOT be considered auth-relevant
		{
			name:       "content-type is not auth",
			headerName: "Content-Type",
			want:       false,
		},
		{
			name:       "accept is not auth",
			headerName: "Accept",
			want:       false,
		},
		{
			name:       "user-agent is not auth",
			headerName: "User-Agent",
			want:       false,
		},
		{
			name:       "host is not auth",
			headerName: "Host",
			want:       false,
		},
		// Case insensitive matching
		{
			name:       "content-type lowercase is not auth",
			headerName: "content-type",
			want:       false,
		},
		{
			name:       "CONTENT-TYPE uppercase is not auth",
			headerName: "CONTENT-TYPE",
			want:       false,
		},
		// Glob pattern matching - x-stainless-*
		{
			name:       "x-stainless-arch matches pattern",
			headerName: "X-Stainless-Arch",
			want:       false,
		},
		{
			name:       "x-stainless-os matches pattern",
			headerName: "X-Stainless-Os",
			want:       false,
		},
		{
			name:       "x-stainless-runtime matches pattern",
			headerName: "X-Stainless-Runtime",
			want:       false,
		},
		{
			name:       "x-stainless- with any suffix matches",
			headerName: "X-Stainless-Whatever",
			want:       false,
		},
		// Case insensitive glob matching
		{
			name:       "lowercase x-stainless-arch matches",
			headerName: "x-stainless-arch",
			want:       false,
		},
		{
			name:       "UPPERCASE X-STAINLESS-OS matches",
			headerName: "X-STAINLESS-OS",
			want:       false,
		},
		// Headers that SHOULD be considered auth-relevant
		{
			name:       "authorization is auth-relevant",
			headerName: "Authorization",
			want:       true,
		},
		{
			name:       "x-api-key is auth-relevant",
			headerName: "X-API-Key",
			want:       true,
		},
		{
			name:       "api-key is auth-relevant",
			headerName: "API-Key",
			want:       true,
		},
		{
			name:       "cookie is auth-relevant",
			headerName: "Cookie",
			want:       true,
		},
		{
			name:       "custom-auth-header is auth-relevant",
			headerName: "X-Custom-Auth",
			want:       true,
		},
		// Edge case: headers that start with x-stainless but don't match pattern
		{
			name:       "x-stainles (no trailing s) is auth-relevant",
			headerName: "X-Stainles",
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsAuthRelevant(tt.headerName)
			if got != tt.want {
				t.Errorf("IsAuthRelevant(%q) = %v, want %v", tt.headerName, got, tt.want)
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
