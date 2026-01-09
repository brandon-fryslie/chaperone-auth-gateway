package integration

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
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

// TestDropPatternBlocksRequests validates that the drop feature blocks matching requests
func TestDropPatternBlocksRequests(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tests := []struct {
		name            string
		dropPatternFunc func(string) string // Function that takes upstream host and returns pattern
		requestPath     string
		shouldBeBlocked bool
	}{
		{
			name:            "drop all paths",
			dropPatternFunc: func(host string) string { return host },
			requestPath:     "/api/users",
			shouldBeBlocked: true,
		},
		{
			name:            "drop specific path",
			dropPatternFunc: func(host string) string { return host + "/blocked" },
			requestPath:     "/blocked",
			shouldBeBlocked: true,
		},
		{
			name:            "drop with wildcard",
			dropPatternFunc: func(host string) string { return host + "/**/sensitive" },
			requestPath:     "/api/v1/sensitive",
			shouldBeBlocked: true,
		},
		{
			name:            "allow non-matching path",
			dropPatternFunc: func(host string) string { return host + "/blocked" },
			requestPath:     "/allowed",
			shouldBeBlocked: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup: Create upstream server
			upstreamCalled := false
			upstreamHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upstreamCalled = true
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("upstream response"))
			})
			upstreamServer := httptest.NewTLSServer(upstreamHandler)
			defer upstreamServer.Close()
			upstreamURL, _ := url.Parse(upstreamServer.URL)
			upstreamHost := upstreamURL.Hostname()

			// Setup: Generate CA and create proxy
			tempDir := t.TempDir()
			ca, err := mitm.GenerateCA()
			require.NoError(t, err)
			caKeyPath := filepath.Join(tempDir, "ca-key.pem")
			caCertPath := filepath.Join(tempDir, "ca-cert.pem")
			err = mitm.StoreCA(ca, caKeyPath, caCertPath)
			require.NoError(t, err)

			certCache := mitm.NewCertCache(ca, slog.Default())
			serviceRegistry := service.NewRegistry()

			// Register service with drop pattern
			dropPattern := tt.dropPatternFunc(upstreamHost)
			svc := &service.Service{
				HostPattern:     upstreamHost,
				AuthStrategyRef: "bearer",
				CredentialRef:   "env:TEST_SECRET",
				Policy: &service.Policy{
					Drop: []string{dropPattern},
				},
			}
			err = serviceRegistry.Register(svc)
			require.NoError(t, err)

			// Create secret and auth registries
			secretRegistry := secrets.NewRegistry()
			secretRegistry.Register("env", secrets.NewEnvProvider())
			os.Setenv("TEST_SECRET", "test-token")
			defer os.Unsetenv("TEST_SECRET")

			authRegistry := auth.NewRegistry()
			authRegistry.Register("bearer", &auth.BearerStrategy{})

			// Create proxy server
			cfg := &config.Config{
				Server: config.ServerConfig{
					Address: "127.0.0.1",
					Port:    findAvailablePort(t), // Use random port
				},
				Logging: config.LoggingConfig{
					Level: "error",
				},
			}
			shutdownMgr := shutdown.NewManager(slog.Default())
			proxyServer := proxy.NewWithMITM(
				cfg,
				slog.Default(),
				shutdownMgr,
				serviceRegistry,
				certCache,
				&proxy.MITMOptions{
					SecretRegistry: secretRegistry,
					AuthRegistry:   authRegistry,
				},
			)

			// Start proxy
			ctx := context.Background()
			err = proxyServer.Start()
			require.NoError(t, err)
			defer proxyServer.Stop(ctx)

			// Get proxy address
			proxyAddr := fmt.Sprintf("http://127.0.0.1:%d", cfg.Server.Port)
			proxyURL, err := url.Parse(proxyAddr)
			require.NoError(t, err)

			// Create client that trusts our CA
			caCertPool := x509.NewCertPool()
			caCert, err := os.ReadFile(caCertPath)
			require.NoError(t, err)
			caCertPool.AppendCertsFromPEM(caCert)

			client := &http.Client{
				Transport: &http.Transport{
					Proxy: http.ProxyURL(proxyURL),
					TLSClientConfig: &tls.Config{
						RootCAs: caCertPool,
					},
				},
			}

			// Make request through proxy (use full upstream URL with port)
			requestURL := upstreamServer.URL + tt.requestPath
			resp, err := client.Get(requestURL)

			if tt.shouldBeBlocked {
				// Request should be blocked by proxy
				require.NoError(t, err)
				assert.Equal(t, http.StatusForbidden, resp.StatusCode)
				assert.False(t, upstreamCalled, "Upstream should not be called when request is dropped")

				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				assert.Contains(t, string(body), "Request blocked by drop policy")
			} else {
				// Request should reach upstream
				require.NoError(t, err)
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				assert.True(t, upstreamCalled, "Upstream should be called when request is allowed")

				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				assert.Contains(t, string(body), "upstream response")
			}
		})
	}
}

// TestStripHeadersRemovesHeaders validates that the strip feature removes specified headers
func TestStripHeadersRemovesHeaders(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tests := []struct {
		name           string
		stripHeaders   []string
		clientHeaders  map[string]string
		expectedAbsent []string
		expectedPresent []string
	}{
		{
			name:         "strip single header",
			stripHeaders: []string{"X-Sensitive-Token"},
			clientHeaders: map[string]string{
				"X-Sensitive-Token": "should-be-removed",
				"X-Keep-This":       "should-remain",
			},
			expectedAbsent:  []string{"X-Sensitive-Token"},
			expectedPresent: []string{"X-Keep-This"},
		},
		{
			name:         "strip multiple headers",
			stripHeaders: []string{"X-Token-A", "X-Token-B"},
			clientHeaders: map[string]string{
				"X-Token-A":   "remove-me",
				"X-Token-B":   "remove-me-too",
				"X-Keep-This": "keep-me",
			},
			expectedAbsent:  []string{"X-Token-A", "X-Token-B"},
			expectedPresent: []string{"X-Keep-This"},
		},
		{
			name:         "strip authorization header (prevents wrong creds)",
			stripHeaders: []string{"Authorization"},
			clientHeaders: map[string]string{
				"Authorization": "Bearer wrong-token",
				"X-Custom":      "keep-me",
			},
			expectedAbsent:  []string{}, // Authorization will be re-injected by proxy with correct token
			expectedPresent: []string{"X-Custom"},
		},
		{
			name:         "case insensitive stripping",
			stripHeaders: []string{"authorization"},
			clientHeaders: map[string]string{
				"Authorization": "Bearer wrong-token",
			},
			expectedAbsent: []string{}, // Authorization will be re-injected by proxy with correct token
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup: Create upstream server that captures headers
			var receivedHeaders http.Header
			upstreamHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedHeaders = r.Header.Clone()
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("ok"))
			})
			upstreamServer := httptest.NewTLSServer(upstreamHandler)
			defer upstreamServer.Close()
			upstreamURL, _ := url.Parse(upstreamServer.URL)
			upstreamHost := upstreamURL.Hostname()

			// Setup: Generate CA and create proxy
			tempDir := t.TempDir()
			ca, err := mitm.GenerateCA()
			require.NoError(t, err)
			caKeyPath := filepath.Join(tempDir, "ca-key.pem")
			caCertPath := filepath.Join(tempDir, "ca-cert.pem")
			err = mitm.StoreCA(ca, caKeyPath, caCertPath)
			require.NoError(t, err)

			certCache := mitm.NewCertCache(ca, slog.Default())
			serviceRegistry := service.NewRegistry()

			// Register service with strip configuration
			svc := &service.Service{
				HostPattern:     upstreamHost,
				AuthStrategyRef: "bearer",
				CredentialRef:   "env:TEST_SECRET",
				Policy: &service.Policy{
					Strip: tt.stripHeaders,
				},
			}
			err = serviceRegistry.Register(svc)
			require.NoError(t, err)

			// Create secret and auth registries
			secretRegistry := secrets.NewRegistry()
			secretRegistry.Register("env", secrets.NewEnvProvider())
			os.Setenv("TEST_SECRET", "correct-token")
			defer os.Unsetenv("TEST_SECRET")

			authRegistry := auth.NewRegistry()
			authRegistry.Register("bearer", &auth.BearerStrategy{})

			// Create proxy server
			cfg := &config.Config{
				Server: config.ServerConfig{
					Address: "127.0.0.1",
					Port:    findAvailablePort(t),
				},
				Logging: config.LoggingConfig{
					Level: "error",
				},
			}
			shutdownMgr := shutdown.NewManager(slog.Default())
			proxyServer := proxy.NewWithMITM(
				cfg,
				slog.Default(),
				shutdownMgr,
				serviceRegistry,
				certCache,
				&proxy.MITMOptions{
					SecretRegistry: secretRegistry,
					AuthRegistry:   authRegistry,
				},
			)

			// Start proxy
			ctx := context.Background()
			err = proxyServer.Start()
			require.NoError(t, err)
			defer proxyServer.Stop(ctx)

			// Get proxy address
			proxyAddr := fmt.Sprintf("http://127.0.0.1:%d", cfg.Server.Port)
			proxyURL, err := url.Parse(proxyAddr)
			require.NoError(t, err)

			// Create client that trusts our CA
			caCertPool := x509.NewCertPool()
			caCert, err := os.ReadFile(caCertPath)
			require.NoError(t, err)
			caCertPool.AppendCertsFromPEM(caCert)

			client := &http.Client{
				Transport: &http.Transport{
					Proxy: http.ProxyURL(proxyURL),
					TLSClientConfig: &tls.Config{
						RootCAs: caCertPool,
					},
				},
			}

			// Create request with client headers
			requestURL := upstreamServer.URL + "/test"
			req, err := http.NewRequest("GET", requestURL, nil)
			require.NoError(t, err)

			for key, value := range tt.clientHeaders {
				req.Header.Set(key, value)
			}

			// Make request through proxy
			resp, err := client.Do(req)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			resp.Body.Close()

			// Verify headers were stripped
			for _, header := range tt.expectedAbsent {
				assert.Empty(t, receivedHeaders.Get(header),
					"Header %s should have been stripped but was present: %s",
					header, receivedHeaders.Get(header))
			}

			// Verify headers were kept
			for _, header := range tt.expectedPresent {
				assert.NotEmpty(t, receivedHeaders.Get(header),
					"Header %s should have been kept but was absent", header)
			}

			// Verify correct auth was injected
			// Even if Authorization was stripped, the auth handler re-injects the CORRECT token
			assert.Equal(t, "Bearer correct-token", receivedHeaders.Get("Authorization"),
				"Correct authentication should be injected by proxy (even if client sent wrong token)")

			// If Authorization was in the strip list, verify the client's wrong token was NOT sent
			if contains(tt.stripHeaders, "Authorization") || contains(tt.stripHeaders, "authorization") {
				// The presence of "Bearer correct-token" proves the client's wrong token was stripped
				// and replaced with the correct one
				assert.NotContains(t, receivedHeaders.Get("Authorization"), "wrong-token",
					"Client's wrong token should have been stripped and replaced")
			}
		})
	}
}

// TestStripPreventsWrongCredentialLeakage validates the real-world scenario:
// Client has wrong credentials, proxy strips them and injects correct ones
func TestStripPreventsWrongCredentialLeakage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup: Create upstream that validates it receives ONLY correct credentials
	upstreamHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")

		// Verify we got the CORRECT token (injected by proxy)
		if auth != "Bearer correct-proxy-token" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(fmt.Sprintf("Wrong token received: %s", auth)))
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("authenticated with correct token"))
	})
	upstreamServer := httptest.NewTLSServer(upstreamHandler)
	defer upstreamServer.Close()
	upstreamURL, _ := url.Parse(upstreamServer.URL)
	upstreamHost := upstreamURL.Hostname()

	// Setup: Generate CA and create proxy
	tempDir := t.TempDir()
	ca, err := mitm.GenerateCA()
	require.NoError(t, err)
	caKeyPath := filepath.Join(tempDir, "ca-key.pem")
	caCertPath := filepath.Join(tempDir, "ca-cert.pem")
	err = mitm.StoreCA(ca, caKeyPath, caCertPath)
	require.NoError(t, err)

	certCache := mitm.NewCertCache(ca, slog.Default())
	serviceRegistry := service.NewRegistry()

	// Register service that strips Authorization and injects correct one
	svc := &service.Service{
		HostPattern:     upstreamHost,
		AuthStrategyRef: "bearer",
		CredentialRef:   "env:CORRECT_TOKEN",
		Policy: &service.Policy{
			Strip: []string{"Authorization"}, // Strip wrong credentials
		},
	}
	err = serviceRegistry.Register(svc)
	require.NoError(t, err)

	// Setup secrets
	secretRegistry := secrets.NewRegistry()
	secretRegistry.Register("env", secrets.NewEnvProvider())
	os.Setenv("CORRECT_TOKEN", "correct-proxy-token")
	defer os.Unsetenv("CORRECT_TOKEN")

	authRegistry := auth.NewRegistry()
	authRegistry.Register("bearer", &auth.BearerStrategy{})

	// Create proxy server
	cfg := &config.Config{
		Server: config.ServerConfig{
			Address: "127.0.0.1",
			Port:    findAvailablePort(t),
		},
		Logging: config.LoggingConfig{
			Level: "error",
		},
	}
	shutdownMgr := shutdown.NewManager(slog.Default())
	proxyServer := proxy.NewWithMITM(
		cfg,
		slog.Default(),
		shutdownMgr,
		serviceRegistry,
		certCache,
		&proxy.MITMOptions{
			SecretRegistry: secretRegistry,
			AuthRegistry:   authRegistry,
		},
	)

	// Start proxy
	ctx := context.Background()
	err = proxyServer.Start()
	require.NoError(t, err)
	defer proxyServer.Stop(ctx)

	// Get proxy address
	proxyAddr := fmt.Sprintf("http://127.0.0.1:%d", cfg.Server.Port)
	proxyURL, err := url.Parse(proxyAddr)
	require.NoError(t, err)

	// Create client that trusts our CA
	caCertPool := x509.NewCertPool()
	caCert, err := os.ReadFile(caCertPath)
	require.NoError(t, err)
	caCertPool.AppendCertsFromPEM(caCert)

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{
				RootCAs: caCertPool,
			},
		},
	}

	// Create request with WRONG credentials (simulating Claude Code's subscription token)
	requestURL := upstreamServer.URL + "/test"
	req, err := http.NewRequest("GET", requestURL, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer wrong-user-subscription-token")

	// Make request - proxy should strip wrong token and inject correct one
	resp, err := client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Request should succeed with correct token")

	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Contains(t, string(body), "authenticated with correct token")
}

// Helper function
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
