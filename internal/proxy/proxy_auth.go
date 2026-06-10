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

// proxyAuthChallenge builds the 407 challenge returned to unauthenticated
// clients on both the CONNECT path and the request path. [LAW:one-source-of-truth]
func proxyAuthChallenge(r *http.Request) *http.Response {
	return &http.Response{
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

// proxyAuthHandler returns a goproxy request handler that gates the request
// path. Two legitimate ways in, both explicit:
//   - the request was decrypted from a CONNECT tunnel the connect gate already
//     authenticated (clients send Proxy-Authorization only on the CONNECT, never
//     on tunneled requests — that temporal fact is carried as the tunnel marker,
//     not re-derived from absent headers) [LAW:no-ambient-temporal-coupling]
//   - a direct proxy-form request presenting valid Proxy-Authorization itself
//
// Everything else gets 407 before any downstream handler can inject.
func proxyAuthHandler(secret string) func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	return func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		if meta, ok := ctx.UserData.(*requestMetadata); ok && meta.proxyAuthenticated {
			return r, nil
		}
		if !validateProxyAuth(r, secret) {
			return r, proxyAuthChallenge(r)
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
// Returns 407 if Proxy-Authorization is missing or invalid. On success it
// stamps the connection's metadata as proxy-authenticated; goproxy hands that
// same metadata to every request decrypted from this tunnel, which is how the
// request-path gate recognizes tunneled requests as already authenticated.
func proxyAuthConnectHandler(secret string, next func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string)) func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
	return func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
		if !validateProxyAuth(ctx.Req, secret) {
			ctx.Resp = proxyAuthChallenge(ctx.Req)
			return goproxy.RejectConnect, host
		}
		// Remove Proxy-Authorization header before forwarding
		ctx.Req.Header.Del("Proxy-Authorization")

		// The wrapped handler may install fresh metadata, so stamp after it runs.
		action, nextHost := next(host, ctx)
		if meta, ok := ctx.UserData.(*requestMetadata); ok {
			meta.proxyAuthenticated = true
		} else {
			ctx.UserData = &requestMetadata{proxyAuthenticated: true}
		}
		return action, nextHost
	}
}
