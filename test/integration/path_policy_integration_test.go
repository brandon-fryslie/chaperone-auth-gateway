package integration

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/bmf/chaperone/internal/auth"
	"github.com/bmf/chaperone/internal/config"
	"github.com/bmf/chaperone/internal/mitm"
	"github.com/bmf/chaperone/internal/proxy"
	"github.com/bmf/chaperone/internal/secrets"
	"github.com/bmf/chaperone/internal/service"
	"github.com/bmf/chaperone/internal/shutdown"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPathAllowlistNormalizationAndCase is the completion gate for the unified
// path matcher (chaperone-security-3at.7): over real TLS and the full MITM
// pipeline it proves that the allowlist judges the path an upstream router
// resolves — dot-segments (plain and percent-encoded), duplicate slashes, and
// trailing slashes cannot reclassify a request, case variants classify with
// their canonical spelling, and a denied request never reaches the upstream
// (so the credential never rides on it).
func TestPathAllowlistNormalizationAndCase(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Upstream records what actually arrives — the ground truth the proxy's
	// classification is judged against.
	type seenRequest struct {
		path string
		auth string
	}
	var seen []seenRequest
	upstreamServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, seenRequest{path: r.URL.Path, auth: r.Header.Get("Authorization")})
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("upstream response"))
	}))
	defer upstreamServer.Close()
	upstreamURL, err := url.Parse(upstreamServer.URL)
	require.NoError(t, err)
	upstreamHost := upstreamURL.Hostname()

	tempDir := t.TempDir()
	ca, err := mitm.GenerateCA()
	require.NoError(t, err)
	caKeyPath := filepath.Join(tempDir, "ca-key.pem")
	caCertPath := filepath.Join(tempDir, "ca-cert.pem")
	require.NoError(t, mitm.StoreCA(ca, caKeyPath, caCertPath))

	certCache := mitm.NewCertCache(ca, slog.Default())
	serviceRegistry := service.NewRegistry()
	require.NoError(t, serviceRegistry.Register(&service.Service{
		HostPattern:     upstreamHost,
		AuthStrategyRef: "bearer",
		CredentialRef:   "env:PATH_POLICY_TEST_SECRET",
		Policy: &service.Policy{
			AllowedPaths: []string{"/v1/*", "/health"},
		},
	}))

	secretRegistry := secrets.NewRegistry()
	secretRegistry.Register("env", secrets.NewEnvProvider())
	os.Setenv("PATH_POLICY_TEST_SECRET", "path-policy-token")
	defer os.Unsetenv("PATH_POLICY_TEST_SECRET")

	authRegistry := auth.NewRegistry()
	authRegistry.Register("bearer", &auth.BearerStrategy{})

	cfg := &config.Config{
		Server:  config.ServerConfig{Address: "127.0.0.1", Port: findAvailablePort(t)},
		Logging: config.LoggingConfig{Level: "error"},
	}
	shutdownMgr := shutdown.NewManager(slog.Default())
	proxyServer := newGatedMITMProxy(t, cfg, slog.Default(), shutdownMgr, serviceRegistry, certCache,
		&proxy.MITMOptions{
			UpstreamCAs:    trustUpstreams(upstreamServer),
			SecretRegistry: secretRegistry,
			AuthRegistry:   authRegistry,
		})

	require.NoError(t, proxyServer.Start())
	defer proxyServer.Stop(context.Background())

	proxyURL, dialer := gatedProxyURL(t, proxyServer)

	caCertPool := x509.NewCertPool()
	caCert, err := os.ReadFile(caCertPath)
	require.NoError(t, err)
	caCertPool.AppendCertsFromPEM(caCert)

	client := &http.Client{
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(proxyURL),
			DialContext:     dialer,
			TLSClientConfig: &tls.Config{RootCAs: caCertPool},
		},
	}

	cases := []struct {
		name    string
		path    string
		allowed bool
	}{
		// The classic bypass: a path that is textually "inside /v1" but
		// resolves outside it must be denied BEFORE injection.
		{"dot-segment escape denied", "/v1/../admin", false},
		{"percent-encoded dot-segment escape denied", "/v1/%2e%2e/admin", false},

		// Variants an upstream router unifies with an allowed path classify
		// with their canonical spelling.
		{"case variant of allowed subtree injected", "/V1/chat", true},
		{"duplicate slashes collapse to allowed path", "//v1//chat", true},
		{"dot-segment resolving inside subtree allowed", "/v1/x/../chat", true},
		{"trailing slash on exact entry allowed", "/health/", true},

		// Boundary semantics of "<prefix>/*": strictly below the prefix.
		{"subtree root itself denied", "/v1", false},
		{"textual sibling prefix denied", "/v1extra/chat", false},
		{"sibling subtree denied", "/v2/chat", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seen = nil

			resp, err := client.Get(upstreamServer.URL + tc.path)
			require.NoError(t, err)
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			if tc.allowed {
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				require.Len(t, seen, 1, "allowed request must reach the upstream")
				assert.Equal(t, "Bearer path-policy-token", seen[0].auth,
					"allowed request must carry the injected credential")
			} else {
				assert.Equal(t, http.StatusForbidden, resp.StatusCode)
				assert.Contains(t, string(body), "not allowed")
				assert.Empty(t, seen,
					"denied request must never reach the upstream — the credential must not ride on it")
			}
		})
	}
}
