package proxy

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elazarl/goproxy"
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

// TestProxyAuthHandlerAcceptancePaths pins the request-path gate's contract:
// exactly two ways in — the connection's authenticated-tunnel stamp (requests
// decrypted from a gated CONNECT never carry Proxy-Authorization) or a valid
// per-request credential. Everything else is challenged with 407.
func TestProxyAuthHandlerAcceptancePaths(t *testing.T) {
	secret := "test-secret-12345"
	handler := proxyAuthHandler(secret)
	validHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte(ProxyAuthUser+":"+secret))

	t.Run("no credential and no tunnel stamp is challenged", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
		_, resp := handler(req, &goproxy.ProxyCtx{})
		require.NotNil(t, resp)
		assert.Equal(t, http.StatusProxyAuthRequired, resp.StatusCode)
	})

	t.Run("attacker-supplied metadata without the stamp is still challenged", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
		ctx := &goproxy.ProxyCtx{UserData: &requestMetadata{requestID: "req-x"}}
		_, resp := handler(req, ctx)
		require.NotNil(t, resp)
		assert.Equal(t, http.StatusProxyAuthRequired, resp.StatusCode)
	})

	t.Run("authenticated tunnel stamp passes without a header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
		ctx := &goproxy.ProxyCtx{UserData: &requestMetadata{proxyAuthenticated: true}}
		_, resp := handler(req, ctx)
		assert.Nil(t, resp)
	})

	t.Run("valid per-request credential passes and is stripped", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		req.Header.Set("Proxy-Authorization", validHeader)
		out, resp := handler(req, &goproxy.ProxyCtx{})
		assert.Nil(t, resp)
		assert.Empty(t, out.Header.Get("Proxy-Authorization"),
			"the proxy credential must never travel upstream")
	})
}

// TestProxyAuthConnectHandlerStampsTunnel pins the connect gate's contract:
// a rejected CONNECT never reaches the wrapped handler, and an accepted one
// leaves the connection metadata stamped for the request-path gate.
func TestProxyAuthConnectHandlerStampsTunnel(t *testing.T) {
	secret := "test-secret-12345"
	validHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte(ProxyAuthUser+":"+secret))

	nextCalled := false
	next := func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
		nextCalled = true
		// Mirror connectHandler: the wrapped handler installs fresh metadata.
		ctx.UserData = &requestMetadata{requestID: "req-y"}
		return goproxy.OkConnect, host
	}
	handler := proxyAuthConnectHandler(secret, next)

	t.Run("unauthenticated CONNECT is rejected before the wrapped handler", func(t *testing.T) {
		nextCalled = false
		ctx := &goproxy.ProxyCtx{Req: httptest.NewRequest(http.MethodConnect, "https://example.com:443", nil)}
		action, _ := handler("example.com:443", ctx)
		assert.Equal(t, goproxy.RejectConnect, action)
		assert.False(t, nextCalled)
		require.NotNil(t, ctx.Resp)
		assert.Equal(t, http.StatusProxyAuthRequired, ctx.Resp.StatusCode)
	})

	t.Run("authenticated CONNECT stamps the tunnel metadata", func(t *testing.T) {
		nextCalled = false
		req := httptest.NewRequest(http.MethodConnect, "https://example.com:443", nil)
		req.Header.Set("Proxy-Authorization", validHeader)
		ctx := &goproxy.ProxyCtx{Req: req}
		_, _ = handler("example.com:443", ctx)
		assert.True(t, nextCalled)
		meta, ok := ctx.UserData.(*requestMetadata)
		require.True(t, ok)
		assert.True(t, meta.proxyAuthenticated)
		assert.Equal(t, "req-y", meta.requestID,
			"the stamp augments the wrapped handler's metadata, it does not replace it")
	})
}
