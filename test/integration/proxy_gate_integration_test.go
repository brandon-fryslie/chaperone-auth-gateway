package integration

// Integration proof for the inject-mode proxy access gate
// (chaperone-security-3at.2): the credential-injecting proxy is never
// reachable without per-run proxy credentials.
//
// ANTI-GAMING MEASURES:
// 1. The daemon is assembled exactly as `chaperone inject` assembles it
//    (orchestrate.Setup + orchestrate.CreateProxy), not from test-only wiring.
// 2. Real TLS upstream, real CONNECT, real MITM handshakes.
// 3. The upstream counts every request it receives — "no credential injected"
//    is proven by the upstream never being reached, not by log inspection.
// 4. The positive control (valid credential → injection) proves the gate is
//    the only thing that stood between the attacker and the credential.

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bmf/chaperone/internal/config"
	"github.com/bmf/chaperone/internal/mitm"
	"github.com/bmf/chaperone/internal/orchestrate"
	"github.com/bmf/chaperone/internal/proxy"
	"github.com/bmf/chaperone/internal/shutdown"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gateDaemon is an inject-mode daemon plus the observability the gate tests
// assert against: how many requests actually reached the upstream, and what
// Authorization value the last one carried.
type gateDaemon struct {
	proxyServer  *proxy.Server
	upstreamURL  string
	upstreamHits *atomic.Int64
	lastAuth     *atomic.Value
	clientCAs    *x509.CertPool
}

func startGateDaemon(t *testing.T) *gateDaemon {
	t.Helper()
	ctx := context.Background()

	var hits atomic.Int64
	var lastAuth atomic.Value
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		lastAuth.Store(r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	t.Cleanup(upstream.Close)
	upstreamHost, _, err := net.SplitHostPort(strings.TrimPrefix(upstream.URL, "https://"))
	require.NoError(t, err)

	t.Setenv("GATE_TEST_CREDENTIAL", "gate-test-real-credential")

	tempDir := t.TempDir()
	caKeyPath := filepath.Join(tempDir, "ca-key.pem")
	caCertPath := filepath.Join(tempDir, "ca-cert.pem")
	ca, err := mitm.GenerateCA()
	require.NoError(t, err)
	require.NoError(t, mitm.StoreCA(ca, caKeyPath, caCertPath))

	upstreamCAPath := filepath.Join(tempDir, "upstream-ca.pem")
	upstreamPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: upstream.Certificate().Raw})
	require.NoError(t, os.WriteFile(upstreamCAPath, upstreamPEM, 0o600))

	cfg := &config.Config{
		Server: config.ServerConfig{
			Address:        "127.0.0.1",
			Port:           findAvailablePort(t),
			UpstreamCAFile: upstreamCAPath,
		},
		Services: map[string]config.ServiceConfig{
			"gate-test": {
				HostPattern:    upstreamHost,
				AuthStrategy:   "bearer",
				CredentialRef:  "env:GATE_TEST_CREDENTIAL",
				AllowedMethods: []string{"GET"},
				AllowedPaths:   []string{"/*"},
				MaxBodyBytes:   1024 * 1024,
			},
		},
		Logging: config.LoggingConfig{Level: "error", Format: "json", Output: "stdout"},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	shutdownMgr := shutdown.NewManager(logger)
	t.Cleanup(func() { _ = shutdownMgr.Shutdown(5 * time.Second) })

	result, err := orchestrate.Setup(ctx, orchestrate.SetupConfig{
		Config:     cfg,
		CAKeyPath:  caKeyPath,
		CACertPath: caCertPath,
	}, ca, logger)
	require.NoError(t, err)

	proxyServer, err := orchestrate.CreateProxy(ctx, cfg, logger, shutdownMgr, result, testProxySecret, nil)
	require.NoError(t, err)
	require.NoError(t, proxyServer.Start())

	caCertPEM, err := os.ReadFile(caCertPath)
	require.NoError(t, err)
	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM(caCertPEM))

	return &gateDaemon{
		proxyServer:  proxyServer,
		upstreamURL:  upstream.URL,
		upstreamHits: &hits,
		lastAuth:     &lastAuth,
		clientCAs:    pool,
	}
}

// clientVia builds an HTTP client routing through the daemon's proxy with the
// given proxy URL (credentialed or not).
func (d *gateDaemon) clientVia(t *testing.T, proxyURLStr string) *http.Client {
	t.Helper()
	u, err := url.Parse(proxyURLStr)
	require.NoError(t, err)
	return &http.Client{
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(u),
			TLSClientConfig: &tls.Config{RootCAs: d.clientCAs},
		},
		Timeout: 10 * time.Second,
	}
}

func TestInjectModeProxyGate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	d := startGateDaemon(t)
	plainURL := "http://" + d.proxyServer.Addr()

	t.Run("missing proxy credential gets 407 and no injection", func(t *testing.T) {
		client := d.clientVia(t, plainURL)
		//nolint:bodyclose // the request never succeeds, there is no body
		_, err := client.Get(d.upstreamURL + "/api/data")
		require.Error(t, err, "CONNECT without proxy credential must be refused")
		assert.Contains(t, err.Error(), "Proxy Authentication Required",
			"refusal must be the 407 proxy-auth challenge")
		assert.Equal(t, int64(0), d.upstreamHits.Load(),
			"no request may reach the upstream — no credential was injected")
	})

	t.Run("invalid proxy credential gets 407 and no injection", func(t *testing.T) {
		client := d.clientVia(t, "http://"+proxy.ProxyAuthUser+":wrong-secret@"+d.proxyServer.Addr())
		//nolint:bodyclose // the request never succeeds, there is no body
		_, err := client.Get(d.upstreamURL + "/api/data")
		require.Error(t, err, "CONNECT with a wrong proxy credential must be refused")
		assert.Contains(t, err.Error(), "Proxy Authentication Required")
		assert.Equal(t, int64(0), d.upstreamHits.Load(),
			"no request may reach the upstream — no credential was injected")
	})

	t.Run("direct absolute-form request cannot smuggle past the CONNECT gate", func(t *testing.T) {
		// An attacker that skips CONNECT and writes an absolute-form request
		// directly at the listener must hit the request-path gate. The https
		// scheme in the request line is attacker-chosen input and must not be
		// trusted as evidence of an authenticated tunnel.
		conn, err := net.Dial("tcp", d.proxyServer.Addr())
		require.NoError(t, err)
		defer conn.Close()

		u, err := url.Parse(d.upstreamURL)
		require.NoError(t, err)
		fmt.Fprintf(conn, "GET https://%s/api/data HTTP/1.1\r\nHost: %s\r\n\r\n", u.Host, u.Host)

		resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusProxyAuthRequired, resp.StatusCode)
		assert.Equal(t, int64(0), d.upstreamHits.Load(),
			"no request may reach the upstream — no credential was injected")
	})

	t.Run("valid proxy credential passes the gate and injection works", func(t *testing.T) {
		// Positive control: with the per-run credential the same request
		// succeeds and the real credential is injected — proving the gate was
		// the only thing between the unauthenticated client and the secret.
		client := d.clientVia(t, d.proxyServer.ProxyURL())
		resp, err := client.Get(d.upstreamURL + "/api/data")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, int64(1), d.upstreamHits.Load())
		assert.Equal(t, "Bearer gate-test-real-credential", d.lastAuth.Load(),
			"authenticated traffic must still get the credential injected")
	})
}

// TestInjectingProxyRequiresSecret proves "injecting proxy with no gate" is
// not constructible: every constructor of an intercepting proxy refuses an
// empty proxy access secret with a loud error.
func TestInjectingProxyRequiresSecret(t *testing.T) {
	cfg := &config.Config{
		Server:  config.ServerConfig{Address: "127.0.0.1", Port: 0},
		Logging: config.LoggingConfig{Level: "error", Format: "json", Output: "stdout"},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	_, err := proxy.NewWithMITM(cfg, logger, nil, nil, nil, "")
	require.Error(t, err, "NewWithMITM must refuse an empty proxy secret")
	assert.Contains(t, err.Error(), "proxy access secret")

	_, err = proxy.NewExamineProxy(cfg, logger, nil, nil, nil, nil, nil, "")
	require.Error(t, err, "NewExamineProxy must refuse an empty proxy secret")
	assert.Contains(t, err.Error(), "proxy access secret")
}
