package proxy

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateProxySecret(t *testing.T) {
	secret, err := GenerateProxySecret()
	require.NoError(t, err)

	// Verify length: 32 bytes = 43 base64 chars (without padding)
	assert.Len(t, secret, 43, "base64 encoded 32 bytes should be 43 chars")

	// Verify uniqueness
	secret2, err := GenerateProxySecret()
	require.NoError(t, err)
	assert.NotEqual(t, secret, secret2, "secrets should be unique")
}

func TestValidateProxyAuth(t *testing.T) {
	secret := "test-secret-12345"

	tests := []struct {
		name     string
		header   string
		expected bool
	}{
		{
			name:     "valid credentials",
			header:   "Basic " + base64.StdEncoding.EncodeToString([]byte(ProxyAuthUser+":"+secret)),
			expected: true,
		},
		{
			name:     "missing header",
			header:   "",
			expected: false,
		},
		{
			name:     "wrong password",
			header:   "Basic " + base64.StdEncoding.EncodeToString([]byte(ProxyAuthUser+":wrong-password")),
			expected: false,
		},
		{
			name:     "different username still works (only password matters)",
			header:   "Basic " + base64.StdEncoding.EncodeToString([]byte("otheruser:"+secret)),
			expected: true,
		},
		{
			name:     "not basic auth",
			header:   "Bearer " + secret,
			expected: false,
		},
		{
			name:     "invalid base64",
			header:   "Basic not-valid-base64!!!",
			expected: false,
		},
		{
			name:     "no colon in credentials",
			header:   "Basic " + base64.StdEncoding.EncodeToString([]byte("no-colon")),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
			if tt.header != "" {
				req.Header.Set("Proxy-Authorization", tt.header)
			}

			result := validateProxyAuth(req, secret)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProxyAuthConstants(t *testing.T) {
	assert.Equal(t, "chaperone", ProxyAuthUser)
	assert.Equal(t, "chaperone-proxy", ProxyAuthRealm)
}
