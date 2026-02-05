package proxy

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/elazarl/goproxy"
)

const (
	// ProxyAuthUser is the fixed username for proxy authentication.
	// The entropy is in the password.
	ProxyAuthUser = "chaperone"

	// ProxyAuthRealm is the realm for proxy authentication challenges.
	ProxyAuthRealm = "chaperone-proxy"
)

// GenerateProxySecret generates a cryptographically random 32-byte secret
// encoded as URL-safe base64 (no padding).
func GenerateProxySecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// proxyAuthHandler returns a goproxy request handler that validates
// Proxy-Authorization header. Returns 407 if missing or invalid.
func proxyAuthHandler(secret string) func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	return func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		if !validateProxyAuth(r, secret) {
			return r, &http.Response{
				StatusCode: http.StatusProxyAuthRequired,
				Status:     "407 Proxy Authentication Required",
				Proto:      "HTTP/1.1",
				ProtoMajor: 1,
				ProtoMinor: 1,
				Header: http.Header{
					"Proxy-Authenticate": []string{`Basic realm="` + ProxyAuthRealm + `"`},
					"Content-Length":     []string{"0"},
				},
				Body:          http.NoBody,
				ContentLength: 0,
				Request:       r,
			}
		}
		// Remove Proxy-Authorization header before forwarding
		r.Header.Del("Proxy-Authorization")
		return r, nil
	}
}

// validateProxyAuth checks the Proxy-Authorization header against the secret.
func validateProxyAuth(r *http.Request, secret string) bool {
	auth := r.Header.Get("Proxy-Authorization")
	if auth == "" {
		return false
	}

	// Must be Basic auth
	if !strings.HasPrefix(auth, "Basic ") {
		return false
	}

	// Decode credentials
	encoded := strings.TrimPrefix(auth, "Basic ")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return false
	}

	// Parse user:password
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return false
	}

	// Constant-time comparison of password
	return subtle.ConstantTimeCompare([]byte(parts[1]), []byte(secret)) == 1
}

// proxyAuthConnectHandler wraps a connect handler with proxy auth validation.
// Returns 407 if Proxy-Authorization is missing or invalid.
func proxyAuthConnectHandler(secret string, next func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string)) func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
	return func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
		if !validateProxyAuth(ctx.Req, secret) {
			// Return 407 for CONNECT requests
			ctx.Resp = &http.Response{
				StatusCode: http.StatusProxyAuthRequired,
				Status:     "407 Proxy Authentication Required",
				Proto:      "HTTP/1.1",
				ProtoMajor: 1,
				ProtoMinor: 1,
				Header: http.Header{
					"Proxy-Authenticate": []string{`Basic realm="` + ProxyAuthRealm + `"`},
					"Content-Length":     []string{"0"},
				},
				Body:          http.NoBody,
				ContentLength: 0,
				Request:       ctx.Req,
			}
			return goproxy.RejectConnect, host
		}
		// Remove Proxy-Authorization header before forwarding
		ctx.Req.Header.Del("Proxy-Authorization")
		return next(host, ctx)
	}
}
