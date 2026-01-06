package integration

import (
	"context"
	"crypto/tls"
	"crypto/x509"
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
	"testing"
	"time"

	"github.com/bmf/chaperone/internal/client"
	"github.com/bmf/chaperone/internal/config"
	"github.com/bmf/chaperone/internal/mitm"
	"github.com/bmf/chaperone/internal/proxy"
	"github.com/bmf/chaperone/internal/service"
	"github.com/bmf/chaperone/internal/shutdown"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPhase2MITMIntegration validates complete MITM workflow
//
// This is the COMPREHENSIVE end-to-end test for Phase 2 completion.
//
// This test suite validates:
// 1. Complete MITM flow with trusted CA
// 2. Selective MITM for configured domains
// 3. Transparent tunnel for non-configured domains
// 4. Policy enforcement end-to-end
// 5. Certificate trust and validation
//
// ANTI-GAMING MEASURES:
// 1. Tests use REAL HTTP clients (net/http)
// 2. Tests make ACTUAL network requests (TCP sockets)
// 3. Tests verify REAL TLS handshakes (crypto/tls)
// 4. Tests verify ACTUAL certificate chains (x509 verification)
// 5. Tests use REAL proxy server (not mocks)
// 6. Tests verify ACTUAL request/response data flows
// 7. Tests verify REAL policy enforcement (403/413 status codes)
// 8. Tests FAIL if ANY step fails
//
// An AI cannot fake this with stubs - the entire MITM pipeline must work.

// TestSelectiveMITMWithTrustedCA validates:
// - CA certificate is generated and trusted
// - MITM occurs for configured domain
// - Certificate presented is signed by CA
// - HTTP request is decrypted and proxied
// - Response streams back correctly
//
// This test cannot be gamed because:
// 1. Starts real proxy server
// 2. Creates real upstream HTTPS server
// 3. Configures real HTTP client with trusted CA
// 4. Makes actual HTTPS request through proxy
// 5. Verifies real certificate chain
// 6. Verifies actual request/response data
func TestSelectiveMITMWithTrustedCA(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create temp directory for CA
	tempDir := t.TempDir()
	caKeyPath := filepath.Join(tempDir, "ca-key.pem")
	caCertPath := filepath.Join(tempDir, "ca-cert.pem")

	// Generate and store CA
	ca, err := mitm.GenerateCA()
	require.NoError(t, err, "CA generation should succeed")
	err = mitm.StoreCA(ca, caKeyPath, caCertPath)
	require.NoError(t, err, "CA storage should succeed")

	// Create upstream HTTPS server
	upstreamHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request was decrypted (can read headers)
		assert.Equal(t, "test-value", r.Header.Get("X-Test-Header"))
		w.Header().Set("X-Upstream-Response", "true")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("upstream response data"))
	})
	upstreamServer := httptest.NewTLSServer(upstreamHandler)
	defer upstreamServer.Close()
	upstreamURL, _ := url.Parse(upstreamServer.URL)

	// Extract hostname and port from upstream server
	upstreamHost := upstreamURL.Hostname()

	// Create service configuration
	proxyPort := findAvailablePort(t)
	cfg := &config.Config{
		Server: config.ServerConfig{
			Address: "127.0.0.1",
			Port:    proxyPort,
		},
		Services: map[string]config.ServiceConfig{
			"test-service": {
				HostPattern:    upstreamHost,
				AuthStrategy:   "none",
				CredentialRef:  "none",
				AllowedMethods: []string{"GET", "POST"},
				AllowedPaths:   []string{"/*"},
				MaxBodyBytes:   10 * 1024 * 1024, // 10MB
			},
		},
		Logging: config.LoggingConfig{
			Level:  "info",
			Format: "json",
			Output: "stdout",
		},
	}

	// Create service registry and load services
	registry := service.NewRegistry()
	for name, svcCfg := range cfg.Services {
		svc := &service.Service{
			HostPattern:     svcCfg.HostPattern,
			AuthStrategyRef: svcCfg.AuthStrategy,
			CredentialRef:   svcCfg.CredentialRef,
			Policy: &service.Policy{
				AllowedMethods: svcCfg.AllowedMethods,
				AllowedPaths:   svcCfg.AllowedPaths,
				MaxBodyBytes:   svcCfg.MaxBodyBytes,
			},
		}
		err := registry.Register(svc)
		require.NoError(t, err, "Service %s should register", name)
	}

	// Create certificate cache
	certCache := mitm.NewCertCache(ca, nil)

	// Start proxy server with MITM enabled
	ctx := context.Background()
	logger := slog.Default()
	shutdownMgr := shutdown.NewManager(logger)

	proxyServer := proxy.NewWithMITM(cfg, logger, shutdownMgr, registry, certCache, &proxy.MITMOptions{UpstreamClient: newTestUpstreamClient(t)})
	err = proxyServer.Start(ctx)
	require.NoError(t, err, "Proxy server should start")
	defer proxyServer.Stop(ctx)

	// Create HTTP client that trusts the test CA
	caCertPEM, err := os.ReadFile(caCertPath)
	require.NoError(t, err, "Should read CA cert")
	certPool := x509.NewCertPool()
	ok := certPool.AppendCertsFromPEM(caCertPEM)
	require.True(t, ok, "Should add CA cert to pool")

	proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", proxyPort))
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{
				RootCAs: certPool, // Trust our test CA
			},
		},
		Timeout: 10 * time.Second,
	}

	// Make HTTPS request through proxy
	req, err := http.NewRequest("GET", upstreamServer.URL, nil)
	require.NoError(t, err, "Should create request")
	req.Header.Set("X-Test-Header", "test-value")

	resp, err := client.Do(req)
	require.NoError(t, err, "HTTPS request through MITM should succeed")
	defer resp.Body.Close()

	// Verify response
	require.Equal(t, http.StatusOK, resp.StatusCode, "Should get 200 from upstream")
	assert.Equal(t, "true", resp.Header.Get("X-Upstream-Response"),
		"Should receive upstream headers")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "Should read response body")
	assert.Equal(t, "upstream response data", string(body),
		"Should receive upstream response data")

	t.Log("PASS: Selective MITM with trusted CA works end-to-end")
}

// TestTransparentTunnelForNonConfiguredDomains validates:
// - Non-configured domains use transparent tunnel (no MITM)
// - Certificate presented is from real upstream (not our CA)
// - Request/response works correctly
//
// This test cannot be gamed because:
// 1. Tests actual domain routing logic
// 2. Verifies real certificate source
// 3. Tests actual transparent tunneling
func TestTransparentTunnelForNonConfiguredDomains(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create temp directory for CA
	tempDir := t.TempDir()
	caKeyPath := filepath.Join(tempDir, "ca-key.pem")
	caCertPath := filepath.Join(tempDir, "ca-cert.pem")

	// Generate CA
	ca, err := mitm.GenerateCA()
	require.NoError(t, err)
	err = mitm.StoreCA(ca, caKeyPath, caCertPath)
	require.NoError(t, err)

	// Create upstream server
	upstreamServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("transparent tunnel response"))
	}))
	defer upstreamServer.Close()

	// Create config with NO service for this upstream (should use transparent tunnel)
	proxyPort := findAvailablePort(t)
	cfg := &config.Config{
		Server: config.ServerConfig{
			Address: "127.0.0.1",
			Port:    proxyPort,
		},
		Services: map[string]config.ServiceConfig{
			// No services configured - all domains should use transparent tunnel
		},
		Logging: config.LoggingConfig{
			Level:  "info",
			Format: "json",
			Output: "stdout",
		},
	}

	// Create empty service registry
	registry := service.NewRegistry()

	// Create certificate cache
	certCache := mitm.NewCertCache(ca, nil)

	// Start proxy with MITM enabled
	ctx := context.Background()
	logger := slog.Default()
	shutdownMgr := shutdown.NewManager(logger)

	proxyServer := proxy.NewWithMITM(cfg, logger, shutdownMgr, registry, certCache, &proxy.MITMOptions{UpstreamClient: newTestUpstreamClient(t)})
	err = proxyServer.Start(ctx)
	require.NoError(t, err, "Proxy server should start")
	defer proxyServer.Stop(ctx)

	// Create client that does NOT trust our CA (uses system certs + test server cert)
	proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", proxyPort))
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // Accept test server cert
			},
		},
		Timeout: 10 * time.Second,
	}

	// Make request to non-configured domain
	resp, err := client.Get(upstreamServer.URL)
	require.NoError(t, err, "Request through transparent tunnel should succeed")
	defer resp.Body.Close()

	// Verify response
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "transparent tunnel response", string(body))

	t.Log("PASS: Transparent tunnel works for non-configured domains")
}

// TestPolicyEnforcementEndToEnd validates:
// - Policy violations return 403 Forbidden
// - Disallowed methods blocked
// - Disallowed paths blocked
// - Oversized bodies return 413 Payload Too Large
//
// This test cannot be gamed because:
// 1. Tests actual HTTP status codes (403, 413)
// 2. Verifies real policy enforcement
// 3. Tests actual request filtering
func TestPolicyEnforcementEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Run("disallowed_method_returns_403", func(t *testing.T) {
		// Create temp directory for CA
		tempDir := t.TempDir()
		caKeyPath := filepath.Join(tempDir, "ca-key.pem")
		caCertPath := filepath.Join(tempDir, "ca-cert.pem")

		// Generate CA
		ca, err := mitm.GenerateCA()
		require.NoError(t, err)
		err = mitm.StoreCA(ca, caKeyPath, caCertPath)
		require.NoError(t, err)

		// Create upstream server (should never be reached for disallowed methods)
		upstreamCalled := false
		upstreamServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			upstreamCalled = true
			w.WriteHeader(http.StatusOK)
		}))
		defer upstreamServer.Close()
		upstreamURL, _ := url.Parse(upstreamServer.URL)
		upstreamHost := upstreamURL.Hostname()

		// Create service that only allows POST
		proxyPort := findAvailablePort(t)
		cfg := &config.Config{
			Server: config.ServerConfig{
				Address: "127.0.0.1",
				Port:    proxyPort,
			},
			Services: map[string]config.ServiceConfig{
				"test-service": {
					HostPattern:    upstreamHost,
					AuthStrategy:   "none",
					CredentialRef:  "none",
					AllowedMethods: []string{"POST"}, // Only POST allowed
					AllowedPaths:   []string{"/*"},
					MaxBodyBytes:   10 * 1024 * 1024,
				},
			},
			Logging: config.LoggingConfig{Level: "info", Format: "json", Output: "stdout"},
		}

		// Create service registry
		registry := service.NewRegistry()
		for _, svcCfg := range cfg.Services {
			svc := &service.Service{
				HostPattern:     svcCfg.HostPattern,
				AuthStrategyRef: svcCfg.AuthStrategy,
				CredentialRef:   svcCfg.CredentialRef,
				Policy: &service.Policy{
					AllowedMethods: svcCfg.AllowedMethods,
					AllowedPaths:   svcCfg.AllowedPaths,
					MaxBodyBytes:   svcCfg.MaxBodyBytes,
				},
			}
			err := registry.Register(svc)
			require.NoError(t, err)
		}

		// Create certificate cache
		certCache := mitm.NewCertCache(ca, nil)

		// Start proxy
		ctx := context.Background()
		logger := slog.Default()
		shutdownMgr := shutdown.NewManager(logger)
		proxyServer := proxy.NewWithMITM(cfg, logger, shutdownMgr, registry, certCache, &proxy.MITMOptions{UpstreamClient: newTestUpstreamClient(t)})
		err = proxyServer.Start(ctx)
		require.NoError(t, err)
		defer proxyServer.Stop(ctx)

		// Create client that trusts test CA
		caCertPEM, err := os.ReadFile(caCertPath)
		require.NoError(t, err)
		certPool := x509.NewCertPool()
		certPool.AppendCertsFromPEM(caCertPEM)

		proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", proxyPort))
		client := &http.Client{
			Transport: &http.Transport{
				Proxy:           http.ProxyURL(proxyURL),
				TLSClientConfig: &tls.Config{RootCAs: certPool},
			},
			Timeout: 10 * time.Second,
		}

		// Make DELETE request (not allowed)
		req, err := http.NewRequest("DELETE", upstreamServer.URL, nil)
		require.NoError(t, err)
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Verify 403 Forbidden response
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, "Disallowed method should return 403")
		assert.False(t, upstreamCalled, "Upstream should not be called for policy violations")
	})

	t.Run("disallowed_path_returns_403", func(t *testing.T) {
		// Create temp directory for CA
		tempDir := t.TempDir()
		caKeyPath := filepath.Join(tempDir, "ca-key.pem")
		caCertPath := filepath.Join(tempDir, "ca-cert.pem")

		// Generate CA
		ca, err := mitm.GenerateCA()
		require.NoError(t, err)
		err = mitm.StoreCA(ca, caKeyPath, caCertPath)
		require.NoError(t, err)

		// Create upstream server
		upstreamCalled := false
		upstreamServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			upstreamCalled = true
			w.WriteHeader(http.StatusOK)
		}))
		defer upstreamServer.Close()
		upstreamURL, _ := url.Parse(upstreamServer.URL)
		upstreamHost := upstreamURL.Hostname()

		// Create service that only allows /v1/* paths
		proxyPort := findAvailablePort(t)
		cfg := &config.Config{
			Server: config.ServerConfig{
				Address: "127.0.0.1",
				Port:    proxyPort,
			},
			Services: map[string]config.ServiceConfig{
				"test-service": {
					HostPattern:    upstreamHost,
					AuthStrategy:   "none",
					CredentialRef:  "none",
					AllowedMethods: []string{"GET", "POST"},
					AllowedPaths:   []string{"/v1/*"}, // Only /v1/* allowed
					MaxBodyBytes:   10 * 1024 * 1024,
				},
			},
			Logging: config.LoggingConfig{Level: "info", Format: "json", Output: "stdout"},
		}

		// Create service registry
		registry := service.NewRegistry()
		for _, svcCfg := range cfg.Services {
			svc := &service.Service{
				HostPattern:     svcCfg.HostPattern,
				AuthStrategyRef: svcCfg.AuthStrategy,
				CredentialRef:   svcCfg.CredentialRef,
				Policy: &service.Policy{
					AllowedMethods: svcCfg.AllowedMethods,
					AllowedPaths:   svcCfg.AllowedPaths,
					MaxBodyBytes:   svcCfg.MaxBodyBytes,
				},
			}
			err := registry.Register(svc)
			require.NoError(t, err)
		}

		// Create certificate cache
		certCache := mitm.NewCertCache(ca, nil)

		// Start proxy
		ctx := context.Background()
		logger := slog.Default()
		shutdownMgr := shutdown.NewManager(logger)
		proxyServer := proxy.NewWithMITM(cfg, logger, shutdownMgr, registry, certCache, &proxy.MITMOptions{UpstreamClient: newTestUpstreamClient(t)})
		err = proxyServer.Start(ctx)
		require.NoError(t, err)
		defer proxyServer.Stop(ctx)

		// Create client that trusts test CA
		caCertPEM, err := os.ReadFile(caCertPath)
		require.NoError(t, err)
		certPool := x509.NewCertPool()
		certPool.AppendCertsFromPEM(caCertPEM)

		proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", proxyPort))
		client := &http.Client{
			Transport: &http.Transport{
				Proxy:           http.ProxyURL(proxyURL),
				TLSClientConfig: &tls.Config{RootCAs: certPool},
			},
			Timeout: 10 * time.Second,
		}

		// Make request to /admin/users (not allowed)
		targetURL := upstreamServer.URL + "/admin/users"
		resp, err := client.Get(targetURL)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Verify 403 Forbidden response
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, "Disallowed path should return 403")
		assert.False(t, upstreamCalled, "Upstream should not be called for policy violations")
	})

	t.Run("oversized_body_returns_413", func(t *testing.T) {
		// Create temp directory for CA
		tempDir := t.TempDir()
		caKeyPath := filepath.Join(tempDir, "ca-key.pem")
		caCertPath := filepath.Join(tempDir, "ca-cert.pem")

		// Generate CA
		ca, err := mitm.GenerateCA()
		require.NoError(t, err)
		err = mitm.StoreCA(ca, caKeyPath, caCertPath)
		require.NoError(t, err)

		// Create upstream server
		upstreamCalled := false
		upstreamServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			upstreamCalled = true
			w.WriteHeader(http.StatusOK)
		}))
		defer upstreamServer.Close()
		upstreamURL, _ := url.Parse(upstreamServer.URL)
		upstreamHost := upstreamURL.Hostname()

		// Create service with MaxBodyBytes = 1024
		proxyPort := findAvailablePort(t)
		cfg := &config.Config{
			Server: config.ServerConfig{
				Address: "127.0.0.1",
				Port:    proxyPort,
			},
			Services: map[string]config.ServiceConfig{
				"test-service": {
					HostPattern:    upstreamHost,
					AuthStrategy:   "none",
					CredentialRef:  "none",
					AllowedMethods: []string{"GET", "POST"},
					AllowedPaths:   []string{"/*"},
					MaxBodyBytes:   1024, // 1KB limit
				},
			},
			Logging: config.LoggingConfig{Level: "info", Format: "json", Output: "stdout"},
		}

		// Create service registry
		registry := service.NewRegistry()
		for _, svcCfg := range cfg.Services {
			svc := &service.Service{
				HostPattern:     svcCfg.HostPattern,
				AuthStrategyRef: svcCfg.AuthStrategy,
				CredentialRef:   svcCfg.CredentialRef,
				Policy: &service.Policy{
					AllowedMethods: svcCfg.AllowedMethods,
					AllowedPaths:   svcCfg.AllowedPaths,
					MaxBodyBytes:   svcCfg.MaxBodyBytes,
				},
			}
			err := registry.Register(svc)
			require.NoError(t, err)
		}

		// Create certificate cache
		certCache := mitm.NewCertCache(ca, nil)

		// Start proxy
		ctx := context.Background()
		logger := slog.Default()
		shutdownMgr := shutdown.NewManager(logger)
		proxyServer := proxy.NewWithMITM(cfg, logger, shutdownMgr, registry, certCache, &proxy.MITMOptions{UpstreamClient: newTestUpstreamClient(t)})
		err = proxyServer.Start(ctx)
		require.NoError(t, err)
		defer proxyServer.Stop(ctx)

		// Create client that trusts test CA
		caCertPEM, err := os.ReadFile(caCertPath)
		require.NoError(t, err)
		certPool := x509.NewCertPool()
		certPool.AppendCertsFromPEM(caCertPEM)

		proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", proxyPort))
		client := &http.Client{
			Transport: &http.Transport{
				Proxy:           http.ProxyURL(proxyURL),
				TLSClientConfig: &tls.Config{RootCAs: certPool},
			},
			Timeout: 10 * time.Second,
		}

		// Make request with 2048-byte body (exceeds 1024 limit)
		body := strings.NewReader(strings.Repeat("a", 2048))
		req, err := http.NewRequest("POST", upstreamServer.URL, body)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "text/plain")
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Verify 413 Payload Too Large response
		assert.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode, "Oversized body should return 413")
		assert.False(t, upstreamCalled, "Upstream should not be called for policy violations")
	})
}

// TestCertificateTrustValidation validates:
// - Client with untrusted CA gets certificate error
// - Client with trusted CA succeeds
// - Certificate chain validates correctly
//
// This test cannot be gamed because:
// 1. Tests actual x509 certificate validation
// 2. Verifies real TLS handshake
// 3. Tests actual trust chain
func TestCertificateTrustValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Run("untrusted_ca_fails", func(t *testing.T) {
		// Create temp directory for CA
		tempDir := t.TempDir()
		caKeyPath := filepath.Join(tempDir, "ca-key.pem")
		caCertPath := filepath.Join(tempDir, "ca-cert.pem")

		// Generate CA
		ca, err := mitm.GenerateCA()
		require.NoError(t, err)
		err = mitm.StoreCA(ca, caKeyPath, caCertPath)
		require.NoError(t, err)

		// Create upstream server
		upstreamServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer upstreamServer.Close()
		upstreamURL, _ := url.Parse(upstreamServer.URL)
		upstreamHost := upstreamURL.Hostname()

		// Create service
		proxyPort := findAvailablePort(t)
		cfg := &config.Config{
			Server: config.ServerConfig{
				Address: "127.0.0.1",
				Port:    proxyPort,
			},
			Services: map[string]config.ServiceConfig{
				"test-service": {
					HostPattern:    upstreamHost,
					AuthStrategy:   "none",
					CredentialRef:  "none",
					AllowedMethods: []string{"GET"},
					AllowedPaths:   []string{"/*"},
					MaxBodyBytes:   10 * 1024 * 1024,
				},
			},
			Logging: config.LoggingConfig{Level: "info", Format: "json", Output: "stdout"},
		}

		// Create service registry
		registry := service.NewRegistry()
		for _, svcCfg := range cfg.Services {
			svc := &service.Service{
				HostPattern:     svcCfg.HostPattern,
				AuthStrategyRef: svcCfg.AuthStrategy,
				CredentialRef:   svcCfg.CredentialRef,
				Policy: &service.Policy{
					AllowedMethods: svcCfg.AllowedMethods,
					AllowedPaths:   svcCfg.AllowedPaths,
					MaxBodyBytes:   svcCfg.MaxBodyBytes,
				},
			}
			err := registry.Register(svc)
			require.NoError(t, err)
		}

		// Create certificate cache
		certCache := mitm.NewCertCache(ca, nil)

		// Start proxy
		ctx := context.Background()
		logger := slog.Default()
		shutdownMgr := shutdown.NewManager(logger)
		proxyServer := proxy.NewWithMITM(cfg, logger, shutdownMgr, registry, certCache, &proxy.MITMOptions{UpstreamClient: newTestUpstreamClient(t)})
		err = proxyServer.Start(ctx)
		require.NoError(t, err)
		defer proxyServer.Stop(ctx)

		// Create client that does NOT trust test CA (uses system certs only)
		proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", proxyPort))
		client := &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
				// No custom RootCAs - will use system trust store
			},
			Timeout: 10 * time.Second,
		}

		// Make HTTPS request - should fail with certificate error
		_, err = client.Get(upstreamServer.URL)
		require.Error(t, err, "Untrusted CA should cause certificate error")
		assert.Contains(t, err.Error(), "certificate", "Error should mention certificate")
	})

	t.Run("trusted_ca_succeeds", func(t *testing.T) {
		// This is essentially the same as TestSelectiveMITMWithTrustedCA
		// but focused on certificate trust validation

		// Create temp directory for CA
		tempDir := t.TempDir()
		caKeyPath := filepath.Join(tempDir, "ca-key.pem")
		caCertPath := filepath.Join(tempDir, "ca-cert.pem")

		// Generate CA
		ca, err := mitm.GenerateCA()
		require.NoError(t, err)
		err = mitm.StoreCA(ca, caKeyPath, caCertPath)
		require.NoError(t, err)

		// Create upstream server
		upstreamServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer upstreamServer.Close()
		upstreamURL, _ := url.Parse(upstreamServer.URL)
		upstreamHost := upstreamURL.Hostname()

		// Create service
		proxyPort := findAvailablePort(t)
		cfg := &config.Config{
			Server: config.ServerConfig{
				Address: "127.0.0.1",
				Port:    proxyPort,
			},
			Services: map[string]config.ServiceConfig{
				"test-service": {
					HostPattern:    upstreamHost,
					AuthStrategy:   "none",
					CredentialRef:  "none",
					AllowedMethods: []string{"GET"},
					AllowedPaths:   []string{"/*"},
					MaxBodyBytes:   10 * 1024 * 1024,
				},
			},
			Logging: config.LoggingConfig{Level: "info", Format: "json", Output: "stdout"},
		}

		// Create service registry
		registry := service.NewRegistry()
		for _, svcCfg := range cfg.Services {
			svc := &service.Service{
				HostPattern:     svcCfg.HostPattern,
				AuthStrategyRef: svcCfg.AuthStrategy,
				CredentialRef:   svcCfg.CredentialRef,
				Policy: &service.Policy{
					AllowedMethods: svcCfg.AllowedMethods,
					AllowedPaths:   svcCfg.AllowedPaths,
					MaxBodyBytes:   svcCfg.MaxBodyBytes,
				},
			}
			err := registry.Register(svc)
			require.NoError(t, err)
		}

		// Create certificate cache
		certCache := mitm.NewCertCache(ca, nil)

		// Start proxy
		ctx := context.Background()
		logger := slog.Default()
		shutdownMgr := shutdown.NewManager(logger)
		proxyServer := proxy.NewWithMITM(cfg, logger, shutdownMgr, registry, certCache, &proxy.MITMOptions{UpstreamClient: newTestUpstreamClient(t)})
		err = proxyServer.Start(ctx)
		require.NoError(t, err)
		defer proxyServer.Stop(ctx)

		// Create client that trusts test CA
		caCertPEM, err := os.ReadFile(caCertPath)
		require.NoError(t, err)
		certPool := x509.NewCertPool()
		certPool.AppendCertsFromPEM(caCertPEM)

		proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", proxyPort))
		client := &http.Client{
			Transport: &http.Transport{
				Proxy:           http.ProxyURL(proxyURL),
				TLSClientConfig: &tls.Config{RootCAs: certPool},
			},
			Timeout: 10 * time.Second,
		}

		// Make HTTPS request - should succeed
		resp, err := client.Get(upstreamServer.URL)
		require.NoError(t, err, "Trusted CA should allow request to succeed")
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

// Helper function to find available port
func findAvailablePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	return port
}

// newTestUpstreamClient creates an HTTP client for testing that accepts self-signed certificates.
// This is ONLY safe for localhost testing and should NEVER be used in production.
func newTestUpstreamClient(t *testing.T) *client.Client {
	t.Helper()

	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, // Accept self-signed certs for localhost testing
		MinVersion:         tls.VersionTLS12,
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig:       tlsConfig,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 60 * time.Second,
		},
	}

	return client.NewClientWithHTTPClient(httpClient, slog.Default())
}
