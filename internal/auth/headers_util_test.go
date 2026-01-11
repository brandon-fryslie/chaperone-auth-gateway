package auth

import (
	"context"
	"net/http"
	"testing"
)

func TestFindHeaderVariants(t *testing.T) {
	// Note: Go's HTTP library canonicalizes header names using http.CanonicalHeaderKey
	// This means headers with different capitalizations are actually stored with the same key.
	// For example, "authorization", "Authorization", and "AUTHORIZATION" all become "Authorization".
	// This test verifies that our function correctly finds headers using case-insensitive matching.

	tests := []struct {
		name       string
		headers    map[string][]string
		targetName string
		want       []string
	}{
		{
			name:       "no matching headers",
			headers:    map[string][]string{"Content-Type": {"application/json"}},
			targetName: "Authorization",
			want:       []string{},
		},
		{
			name: "single matching header - exact case",
			headers: map[string][]string{
				"Authorization": {"Bearer token"},
			},
			targetName: "Authorization",
			want:       []string{"Authorization"},
		},
		{
			name: "single matching header - different case (canonicalized by Go)",
			headers: map[string][]string{
				"authorization": {"Bearer token"}, // Will be canonicalized to "Authorization"
			},
			targetName: "Authorization",
			want:       []string{"Authorization"}, // Go canonicalizes to "Authorization"
		},
		{
			name: "finding with different target case",
			headers: map[string][]string{
				"Authorization": {"Bearer token"},
			},
			targetName: "authorization",           // Target is lowercase
			want:       []string{"Authorization"}, // But we still find the canonical form
		},
		{
			name: "x-api-key canonical form",
			headers: map[string][]string{
				"x-api-key": {"key1"}, // Will be canonicalized to "X-Api-Key"
			},
			targetName: "x-api-key",
			want:       []string{"X-Api-Key"}, // Go's canonical form
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "http://example.com", nil)
			for name, values := range tt.headers {
				for _, value := range values {
					req.Header.Add(name, value)
				}
			}

			got := findHeaderVariants(req, tt.targetName)

			if len(got) != len(tt.want) {
				t.Errorf("findHeaderVariants() got %d results, want %d. Got: %v, Want: %v", len(got), len(tt.want), got, tt.want)
				return
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("findHeaderVariants()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSetHeaderPreservingCapitalization(t *testing.T) {
	// Note: Go canonicalizes HTTP header names, so this test reflects that behavior.
	// The main value of this function is detecting when a client has already set
	// a header (even with different capitalization) and warning about it.

	tests := []struct {
		name            string
		existingHeaders map[string][]string
		targetName      string
		value           string
		wantReplaced    bool
		wantHeaderName  string // The header name that should be present after the operation
		wantValue       string
	}{
		{
			name:            "no existing header - sets with target capitalization",
			existingHeaders: map[string][]string{},
			targetName:      "Authorization",
			value:           "Bearer token",
			wantReplaced:    false,
			wantHeaderName:  "Authorization",
			wantValue:       "Bearer token",
		},
		{
			name: "existing header - replaces and warns",
			existingHeaders: map[string][]string{
				"Authorization": {"Bearer old"},
			},
			targetName:     "Authorization",
			value:          "Bearer new",
			wantReplaced:   true,
			wantHeaderName: "Authorization",
			wantValue:      "Bearer new",
		},
		{
			name: "existing header with different case - Go canonicalizes",
			existingHeaders: map[string][]string{
				"authorization": {"Bearer old"}, // Go canonicalizes to "Authorization"
			},
			targetName:     "Authorization",
			value:          "Bearer new",
			wantReplaced:   true,
			wantHeaderName: "Authorization", // Go's canonical form
			wantValue:      "Bearer new",
		},
		{
			name: "x-api-key - Go canonicalizes to X-Api-Key",
			existingHeaders: map[string][]string{
				"x-api-key": {"old-key"}, // Go canonicalizes to "X-Api-Key"
			},
			targetName:     "x-api-key",
			value:          "new-key",
			wantReplaced:   true,
			wantHeaderName: "X-Api-Key", // Go's canonical form
			wantValue:      "new-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "http://example.com", nil)
			for name, values := range tt.existingHeaders {
				for _, value := range values {
					req.Header.Add(name, value)
				}
			}

			ctx := context.Background()
			replaced := setHeaderPreservingCapitalization(ctx, req, tt.targetName, tt.value)

			if replaced != tt.wantReplaced {
				t.Errorf("setHeaderPreservingCapitalization() replaced = %v, want %v", replaced, tt.wantReplaced)
			}

			// Check that the header exists with the expected name
			gotValue := req.Header.Get(tt.wantHeaderName)
			if gotValue != tt.wantValue {
				t.Errorf("Header %q = %q, want %q", tt.wantHeaderName, gotValue, tt.wantValue)
			}

			// After the operation, there should only be one variant
			variants := findHeaderVariants(req, tt.targetName)
			if len(variants) != 1 {
				t.Errorf("After operation, found %d header variants, want 1: %v", len(variants), variants)
			}
		})
	}
}
