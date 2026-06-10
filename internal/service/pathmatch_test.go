package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The vocabulary: exact path or "<prefix>/*". Everything else is rejected at
// parse — never silently mismatched at request time.
func TestParsePathPattern_Vocabulary(t *testing.T) {
	valid := []string{"/", "/*", "/v1", "/v1/chat", "/v1/*", "/v1/chat/*"}
	for _, p := range valid {
		t.Run("accepts "+p, func(t *testing.T) {
			_, err := parsePathPattern(p)
			assert.NoError(t, err)
		})
	}

	invalid := []string{
		"",               // not rooted
		"v1/chat",        // not rooted
		"/v1/**",         // segment glob
		"/**",            // segment glob
		"/v1/*/messages", // mid-pattern wildcard
		"/v1/cha?",       // glob metachar
		"/v1/[ab]",       // glob metachar
		"/v1/",           // non-canonical: trailing slash (write /v1)
		"//v1/*",         // non-canonical: duplicate slash
		"/v1//chat",      // non-canonical: duplicate slash
		"/v1/../admin",   // non-canonical: dot-segment
		"/v1/../admin/*", // non-canonical: dot-segment
		"//*",            // non-canonical: write /*
	}
	for _, p := range invalid {
		t.Run("rejects "+p, func(t *testing.T) {
			_, err := parsePathPattern(p)
			assert.Error(t, err)
		})
	}
}

// The one documented match policy: request paths are normalized (dot-segments
// resolved, duplicate slashes collapsed, trailing slash dropped) and compared
// case-insensitively. "<prefix>/*" matches strictly below the prefix at any
// depth; it never matches the prefix itself or a textual sibling.
func TestPathPattern_Matches(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		// Subtree wildcard
		{"/v1/*", "/v1/chat", true},
		{"/v1/*", "/v1/chat/completions", true}, // any depth
		{"/v1/*", "/v1", false},                 // the prefix itself is not below it
		{"/v1/*", "/v1/", false},                // normalizes to the prefix itself
		{"/v1/*", "/v10/chat", false},           // textual sibling must not prefix-match
		{"/v1/*", "/v2/chat", false},

		// Normalization closes the classic bypasses
		{"/v1/*", "/v1/../admin", false},  // escapes the subtree → judged as /admin
		{"/v1/*", "/v1/../v1/chat", true}, // resolves back inside → judged as /v1/chat
		{"/v1/*", "//v1//chat", true},     // duplicate slashes collapse
		{"/v1/*", "/v1/./chat", true},     // single dot-segment resolves
		{"/v1/*", "/v1/chat/", true},      // trailing slash drops

		// Case-insensitive comparison (documented policy)
		{"/v1/*", "/V1/Chat", true},
		{"/HEALTH", "/health", true},

		// Exact patterns
		{"/health", "/health", true},
		{"/health", "/health/", true},
		{"/health", "/health/live", false},
		{"/health", "/healthz", false},

		// Root and match-all
		{"/", "/", true},
		{"/", "", true}, // empty normalizes to root
		{"/", "/x", false},
		{"/*", "/", true},
		{"/*", "/anything/at/all", true},
	}
	for _, c := range cases {
		t.Run(c.pattern+" vs "+c.path, func(t *testing.T) {
			pp, err := parsePathPattern(c.pattern)
			require.NoError(t, err)
			assert.Equal(t, c.want, pp.matches(c.path))
		})
	}
}

func TestValidatePathPatterns(t *testing.T) {
	assert.NoError(t, ValidatePathPatterns(nil))
	assert.NoError(t, ValidatePathPatterns([]string{"/v1/*", "/health"}))

	err := ValidatePathPatterns([]string{"/v1/*", "/v1/**"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "/v1/**") // names the offending pattern
}

// Containment (grant narrowing) is decided over the same folded, normalized
// representation matching uses — a grant that narrows can never out-match its
// bound under any spelling of a request path.
func TestPathPattern_ContainsFoldsCase(t *testing.T) {
	assert.NoError(t, Narrows(
		&Policy{AllowedPaths: []string{"/V1/Chat"}},
		&Policy{AllowedPaths: []string{"/v1/*"}},
	))
}
