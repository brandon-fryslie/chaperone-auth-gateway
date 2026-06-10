package integration

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bmf/chaperone/internal/config"
	"github.com/bmf/chaperone/internal/mitm"
	"github.com/bmf/chaperone/internal/proxy"
	"github.com/bmf/chaperone/internal/service"
	"github.com/bmf/chaperone/internal/shutdown"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests prove the proxy verifies UPSTREAM server certificates on the
// outbound leg of MITM'd connections — the leg that carries the injected
// credential. Without verification, an on-path attacker presenting any cert
// for a configured host would receive the real credential while the client
// sees nothing wrong (the proxy still presents a cert the client trusts).
//
// ANTI-GAMING MEASURES:
// 1. Real TLS handshakes against real upstream servers (no mocks)
// 2. The security assertion is the upstream's request counter: zero requests
//    arriving means zero credentials forwarded — there is no way to fake this
// 3. A positive control proves verification-on still injects for legitimate
//    upstreams, so the negative cases can't pass via a broken pipeline

// startInjectingProxy starts a credential-injecting MITM proxy for
// upstreamHost whose outbound trust is anchored to upstreamCAs (nil = system
// roots) and returns an HTTP client routed through it that trusts the proxy's
// own MITM CA.
func startInjectingProxy(t *testing.T, upstreamHost string, upstreamCAs *x509.CertPool, credentialRef string) *http.Client {
	t.Helper()

	proxyCA, err := mitm.GenerateCA()
	require.NoError(t, err, "proxy MITM CA generation should succeed")

	cfg := &config.Config{
		Server: config.ServerConfig{
			Address: "127.0.0.1",
			Port:    findAvailablePort(t),
		},
		Logging: config.LoggingConfig{
			Level:  "info",
			Format: "json",
			Output: "stdout",
		},
	}

	registry := service.NewRegistry()
	svc := &service.Service{
		HostPattern:     upstreamHost,
		AuthStrategyRef: "bearer",
		CredentialRef:   credentialRef,
		Policy: &service.Policy{
			AllowedMethods: []string{"GET"},
			AllowedPaths:   []string{"/*"},
			MaxBodyBytes:   1024 * 1024,
		},
	}
	require.NoError(t, registry.Register(svc))

	certCache := mitm.NewCertCache(proxyCA, nil)
	logger := slog.Default()
	shutdownMgr := shutdown.NewManager(logger)
	secretRegistry, authRegistry := setupAuthRegistries()

	proxyServer := proxy.NewWithMITM(cfg, logger, shutdownMgr, registry, certCache, &proxy.MITMOptions{
		SecretRegistry: secretRegistry,
		AuthRegistry:   authRegistry,
		UpstreamCAs:    upstreamCAs,
	})
	require.NoError(t, proxyServer.Start())
	t.Cleanup(func() { _ = proxyServer.Stop(context.Background()) })

	mitmPool := x509.NewCertPool()
	mitmPool.AddCert(proxyCA.Certificate())

	proxyURL, dialer := proxy.GetProxyURL(cfg)
	return &http.Client{
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(proxyURL),
			DialContext:     dialer,
			TLSClientConfig: &tls.Config{RootCAs: mitmPool},
		},
		Timeout: 10 * time.Second,
	}
}

// startUpstreamWithCert serves TLS with a leaf certificate minted for
// certHostname and signed by ca — regardless of the address it listens on.
// Presenting a cert whose identity doesn't match the dialed address is
// exactly what an on-path attacker does.
func startUpstreamWithCert(t *testing.T, ca *mitm.CA, certHostname string, handler http.Handler) *httptest.Server {
	t.Helper()

	cert, err := mitm.NewCertCache(ca, nil).GetCertificate(certHostname)
	require.NoError(t, err, "leaf certificate generation should succeed")

	upstream := httptest.NewUnstartedServer(handler)
	upstream.TLS = &tls.Config{Certificates: []tls.Certificate{*cert}}
	upstream.StartTLS()
	t.Cleanup(upstream.Close)
	return upstream
}

// countingHandler records how many requests actually reached the upstream and
// the Authorization header of the last one. The counter is the security
// gauge: zero requests means zero injected credentials crossed the wire.
func countingHandler() (*atomic.Int64, *atomic.Value, http.Handler) {
	var hits atomic.Int64
	var lastAuth atomic.Value
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		lastAuth.Store(r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	return &hits, &lastAuth, handler
}

// TestUpstreamUntrustedCertIsRefusedNoCredentialForwarded validates the
// DEFAULT posture: an upstream presenting a certificate not anchored in the
// system root store (here: httptest's self-signed cert) is refused before any
// request — and therefore any credential — is sent.
func TestUpstreamUntrustedCertIsRefusedNoCredentialForwarded(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	hits, _, handler := countingHandler()
	upstream := httptest.NewTLSServer(handler)
	defer upstream.Close()
	upstreamURL, _ := url.Parse(upstream.URL)

	t.Setenv("TEST_UPSTREAM_TLS_SECRET", "credential-that-must-not-leak")

	// nil UpstreamCAs = system roots: the default production trust posture
	client := startInjectingProxy(t, upstreamURL.Hostname(), nil, "env:TEST_UPSTREAM_TLS_SECRET")

	resp, err := client.Get(upstream.URL + "/v1/data")
	if err == nil {
		defer resp.Body.Close()
		assert.NotEqual(t, http.StatusOK, resp.StatusCode,
			"request to an untrusted upstream must not succeed")
	}

	assert.Equal(t, int64(0), hits.Load(),
		"SECURITY: no request may reach an upstream presenting an untrusted certificate — a forwarded request carries the real credential")
}

// TestUpstreamWrongHostnameCertIsRefused validates hostname verification, not
// just chain verification: the upstream's chain is anchored in the proxy's
// pinned trust pool, but the leaf identifies a different host than the one
// dialed — the exact shape of a credential-harvesting redirect.
func TestUpstreamWrongHostnameCertIsRefused(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	upstreamCA, err := mitm.GenerateCA()
	require.NoError(t, err)

	hits, _, handler := countingHandler()
	upstream := startUpstreamWithCert(t, upstreamCA, "wrong.example.com", handler)
	upstreamURL, _ := url.Parse(upstream.URL)

	t.Setenv("TEST_UPSTREAM_TLS_SECRET", "credential-that-must-not-leak")

	trustedPool := x509.NewCertPool()
	trustedPool.AddCert(upstreamCA.Certificate())
	client := startInjectingProxy(t, upstreamURL.Hostname(), trustedPool, "env:TEST_UPSTREAM_TLS_SECRET")

	resp, err := client.Get(upstream.URL + "/v1/data")
	if err == nil {
		defer resp.Body.Close()
		assert.NotEqual(t, http.StatusOK, resp.StatusCode,
			"request to an upstream with a wrong-hostname certificate must not succeed")
	}

	assert.Equal(t, int64(0), hits.Load(),
		"SECURITY: a trusted chain is not enough — the certificate must identify the host actually dialed")
}

// TestUpstreamTrustedCertIsVerifiedAndInjected is the positive control: with
// verification ON, a legitimate upstream (chain anchored in the pinned pool,
// leaf valid for the dialed address) still receives the injected credential.
// This proves the negative cases above fail on trust, not on a broken pipeline.
func TestUpstreamTrustedCertIsVerifiedAndInjected(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	upstreamCA, err := mitm.GenerateCA()
	require.NoError(t, err)

	hits, lastAuth, handler := countingHandler()
	upstream := startUpstreamWithCert(t, upstreamCA, "127.0.0.1", handler)
	upstreamURL, _ := url.Parse(upstream.URL)

	t.Setenv("TEST_UPSTREAM_TLS_SECRET", "expected-injected-credential")

	trustedPool := x509.NewCertPool()
	trustedPool.AddCert(upstreamCA.Certificate())
	client := startInjectingProxy(t, upstreamURL.Hostname(), trustedPool, "env:TEST_UPSTREAM_TLS_SECRET")

	resp, err := client.Get(upstream.URL + "/v1/data")
	require.NoError(t, err, "request to a verified upstream should succeed")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int64(1), hits.Load(), "exactly one request should reach the upstream")
	assert.Equal(t, "Bearer expected-injected-credential", lastAuth.Load(),
		"verification on must not break credential injection for legitimate upstreams")
}
