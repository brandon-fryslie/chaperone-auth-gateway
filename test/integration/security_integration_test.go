package integration

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bmf/chaperone/internal/audit"
	"github.com/bmf/chaperone/internal/config"
	"github.com/bmf/chaperone/internal/mitm"
	"github.com/bmf/chaperone/internal/proxy"
	"github.com/bmf/chaperone/internal/service"
	"github.com/bmf/chaperone/internal/shutdown"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPlaceholderEnforcement validates:
// - Valid placeholder triggers credential injection
// - Missing placeholder skips injection (passthrough)
// - Wrong placeholder skips injection (passthrough)
//
// UNGAMEABLE: Real proxy, real TLS, real placeholder matching logic
func TestPlaceholderEnforcement(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Test 1: Valid placeholder triggers injection
	t.Run("valid placeholder triggers injection", func(t *testing.T) {
		// Setup: Create upstream server that verifies bearer token
		var upstreamReceivedAuth string
		upstreamHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			upstreamReceivedAuth = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		})
		upstreamServer := httptest.NewTLSServer(upstreamHandler)
		defer upstreamServer.Close()
		upstreamURL, _ := url.Parse(upstreamServer.URL)
		upstreamHost := upstreamURL.Hostname()

		// Setup: Environment variable for secret
		testSecretEnvVar := "TEST_PLACEHOLDER_SECRET"
		expectedSecret := "real-secret-token-12345"
		os.Setenv(testSecretEnvVar, expectedSecret)
		defer os.Unsetenv(testSecretEnvVar)

		// Setup: Generate CA
		tempDir := t.TempDir()
		caKeyPath := filepath.Join(tempDir, "ca-key.pem")
		caCertPath := filepath.Join(tempDir, "ca-cert.pem")

		ca, err := mitm.GenerateCA()
		require.NoError(t, err)
		err = mitm.StoreCA(ca, caKeyPath, caCertPath)
		require.NoError(t, err)

		// Setup: Configure service with placeholder
		placeholder := "chap_test_placeholder_12345"
		proxyPort := findAvailablePort(t)
		cfg := &config.Config{
			Server: config.ServerConfig{
				Address: "127.0.0.1",
				Port:    proxyPort,
			},
			Services: map[string]config.ServiceConfig{
				"test-api": {
					HostPattern:    upstreamHost,
					AuthStrategy:   "bearer",
					CredentialRef:  fmt.Sprintf("env:%s", testSecretEnvVar),
					Placeholder:    placeholder,
					AllowedMethods: []string{"GET", "POST"},
					AllowedPaths:   []string{"/*"},
					MaxBodyBytes:   10 * 1024 * 1024,
				},
			},
			Logging: config.LoggingConfig{
				Level:  "debug",
				Format: "json",
				Output: "stdout",
			},
		}

		// Setup: Create service registry
		registry := service.NewRegistry()
		for _, svcCfg := range cfg.Services {
			svc := &service.Service{
				Name:            "test-api",
				HostPattern:     svcCfg.HostPattern,
				AuthStrategyRef: svcCfg.AuthStrategy,
				CredentialRef:   svcCfg.CredentialRef,
				Placeholder:     svcCfg.Placeholder,
				Policy: &service.Policy{
					AllowedMethods: svcCfg.AllowedMethods,
					AllowedPaths:   svcCfg.AllowedPaths,
					MaxBodyBytes:   svcCfg.MaxBodyBytes,
				},
			}
			err := registry.Register(svc)
			require.NoError(t, err)
		}

		// Setup: Start proxy server
		certCache := mitm.NewCertCache(ca, nil)
		ctx := context.Background()
		logger := slog.Default()
		shutdownMgr := shutdown.NewManager(logger)

		secretRegistry, authRegistry := setupAuthRegistries()

		proxyServer := proxy.NewWithMITM(cfg, logger, shutdownMgr, registry, certCache, &proxy.MITMOptions{
			SecretRegistry: secretRegistry,
			AuthRegistry:   authRegistry,
		})
		err = proxyServer.Start()
		require.NoError(t, err)
		defer proxyServer.Stop(ctx)

		// Setup: Create client
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

		// EXECUTE: Make request with VALID placeholder
		req, err := http.NewRequest("GET", upstreamServer.URL+"/api/test", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+placeholder) // Client sends placeholder

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// VERIFY: Injection occurred
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "Bearer "+expectedSecret, upstreamReceivedAuth,
			"Upstream MUST receive real secret when client sends valid placeholder")

		t.Log("PASS: Valid placeholder triggered credential injection")
	})

	// Test 2: No placeholder skips injection
	t.Run("missing placeholder skips injection", func(t *testing.T) {
		// Setup: Create upstream server that verifies NO auth header
		var upstreamReceivedAuth string
		upstreamHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			upstreamReceivedAuth = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		})
		upstreamServer := httptest.NewTLSServer(upstreamHandler)
		defer upstreamServer.Close()
		upstreamURL, _ := url.Parse(upstreamServer.URL)
		upstreamHost := upstreamURL.Hostname()

		// Setup: Environment variable for secret
		testSecretEnvVar := "TEST_PLACEHOLDER_SECRET_2"
		os.Setenv(testSecretEnvVar, "should-not-be-injected")
		defer os.Unsetenv(testSecretEnvVar)

		// Setup: Generate CA
		tempDir := t.TempDir()
		caKeyPath := filepath.Join(tempDir, "ca-key.pem")
		caCertPath := filepath.Join(tempDir, "ca-cert.pem")

		ca, err := mitm.GenerateCA()
		require.NoError(t, err)
		err = mitm.StoreCA(ca, caKeyPath, caCertPath)
		require.NoError(t, err)

		// Setup: Configure service with placeholder
		placeholder := "chap_test_placeholder_12345"
		proxyPort := findAvailablePort(t)
		cfg := &config.Config{
			Server: config.ServerConfig{
				Address: "127.0.0.1",
				Port:    proxyPort,
			},
			Services: map[string]config.ServiceConfig{
				"test-api": {
					HostPattern:    upstreamHost,
					AuthStrategy:   "bearer",
					CredentialRef:  fmt.Sprintf("env:%s", testSecretEnvVar),
					Placeholder:    placeholder,
					AllowedMethods: []string{"GET", "POST"},
					AllowedPaths:   []string{"/*"},
					MaxBodyBytes:   10 * 1024 * 1024,
				},
			},
			Logging: config.LoggingConfig{
				Level:  "debug",
				Format: "json",
				Output: "stdout",
			},
		}

		// Setup: Create service registry
		registry := service.NewRegistry()
		for _, svcCfg := range cfg.Services {
			svc := &service.Service{
				Name:            "test-api",
				HostPattern:     svcCfg.HostPattern,
				AuthStrategyRef: svcCfg.AuthStrategy,
				CredentialRef:   svcCfg.CredentialRef,
				Placeholder:     svcCfg.Placeholder,
				Policy: &service.Policy{
					AllowedMethods: svcCfg.AllowedMethods,
					AllowedPaths:   svcCfg.AllowedPaths,
					MaxBodyBytes:   svcCfg.MaxBodyBytes,
				},
			}
			err := registry.Register(svc)
			require.NoError(t, err)
		}

		// Setup: Start proxy server
		certCache := mitm.NewCertCache(ca, nil)
		ctx := context.Background()
		logger := slog.Default()
		shutdownMgr := shutdown.NewManager(logger)

		secretRegistry, authRegistry := setupAuthRegistries()

		proxyServer := proxy.NewWithMITM(cfg, logger, shutdownMgr, registry, certCache, &proxy.MITMOptions{
			SecretRegistry: secretRegistry,
			AuthRegistry:   authRegistry,
		})
		err = proxyServer.Start()
		require.NoError(t, err)
		defer proxyServer.Stop(ctx)

		// Setup: Create client
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

		// EXECUTE: Make request WITHOUT Authorization header
		req, err := http.NewRequest("GET", upstreamServer.URL+"/api/test", nil)
		require.NoError(t, err)
		// NO Authorization header set

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// VERIFY: No injection occurred (passthrough)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Empty(t, upstreamReceivedAuth,
			"Upstream MUST NOT receive any auth header when client sends no placeholder")

		t.Log("PASS: Missing placeholder skipped injection")
	})

	// Test 3: Wrong placeholder skips injection
	t.Run("wrong placeholder skips injection", func(t *testing.T) {
		// Setup: Create upstream server that verifies passthrough
		var upstreamReceivedAuth string
		upstreamHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			upstreamReceivedAuth = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		})
		upstreamServer := httptest.NewTLSServer(upstreamHandler)
		defer upstreamServer.Close()
		upstreamURL, _ := url.Parse(upstreamServer.URL)
		upstreamHost := upstreamURL.Hostname()

		// Setup: Environment variable for secret
		testSecretEnvVar := "TEST_PLACEHOLDER_SECRET_3"
		os.Setenv(testSecretEnvVar, "should-not-be-injected")
		defer os.Unsetenv(testSecretEnvVar)

		// Setup: Generate CA
		tempDir := t.TempDir()
		caKeyPath := filepath.Join(tempDir, "ca-key.pem")
		caCertPath := filepath.Join(tempDir, "ca-cert.pem")

		ca, err := mitm.GenerateCA()
		require.NoError(t, err)
		err = mitm.StoreCA(ca, caKeyPath, caCertPath)
		require.NoError(t, err)

		// Setup: Configure service with placeholder
		placeholder := "chap_test_placeholder_12345"
		wrongPlaceholder := "wrong_token_value"
		proxyPort := findAvailablePort(t)
		cfg := &config.Config{
			Server: config.ServerConfig{
				Address: "127.0.0.1",
				Port:    proxyPort,
			},
			Services: map[string]config.ServiceConfig{
				"test-api": {
					HostPattern:    upstreamHost,
					AuthStrategy:   "bearer",
					CredentialRef:  fmt.Sprintf("env:%s", testSecretEnvVar),
					Placeholder:    placeholder,
					AllowedMethods: []string{"GET", "POST"},
					AllowedPaths:   []string{"/*"},
					MaxBodyBytes:   10 * 1024 * 1024,
				},
			},
			Logging: config.LoggingConfig{
				Level:  "debug",
				Format: "json",
				Output: "stdout",
			},
		}

		// Setup: Create service registry
		registry := service.NewRegistry()
		for _, svcCfg := range cfg.Services {
			svc := &service.Service{
				Name:            "test-api",
				HostPattern:     svcCfg.HostPattern,
				AuthStrategyRef: svcCfg.AuthStrategy,
				CredentialRef:   svcCfg.CredentialRef,
				Placeholder:     svcCfg.Placeholder,
				Policy: &service.Policy{
					AllowedMethods: svcCfg.AllowedMethods,
					AllowedPaths:   svcCfg.AllowedPaths,
					MaxBodyBytes:   svcCfg.MaxBodyBytes,
				},
			}
			err := registry.Register(svc)
			require.NoError(t, err)
		}

		// Setup: Start proxy server
		certCache := mitm.NewCertCache(ca, nil)
		ctx := context.Background()
		logger := slog.Default()
		shutdownMgr := shutdown.NewManager(logger)

		secretRegistry, authRegistry := setupAuthRegistries()

		proxyServer := proxy.NewWithMITM(cfg, logger, shutdownMgr, registry, certCache, &proxy.MITMOptions{
			SecretRegistry: secretRegistry,
			AuthRegistry:   authRegistry,
		})
		err = proxyServer.Start()
		require.NoError(t, err)
		defer proxyServer.Stop(ctx)

		// Setup: Create client
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

		// EXECUTE: Make request with WRONG placeholder
		req, err := http.NewRequest("GET", upstreamServer.URL+"/api/test", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+wrongPlaceholder) // Wrong token

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// VERIFY: Request passed through unchanged
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Empty(t, upstreamReceivedAuth,
			"Upstream MUST NOT receive wrong token (stripped for security) when placeholder doesn't match")

		t.Log("PASS: Wrong placeholder was stripped for security")
	})
}

// TestAuditLogging validates:
// - Audit entry written after successful injection
// - No audit entry when injection doesn't occur
//
// UNGAMEABLE: Real audit file I/O, real JSON parsing, real proxy behavior
func TestAuditLogging(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Test 1: Audit entry written on injection
	t.Run("audit entry written on injection", func(t *testing.T) {
		// Setup: Create upstream server
		upstreamHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		})
		upstreamServer := httptest.NewTLSServer(upstreamHandler)
		defer upstreamServer.Close()
		upstreamURL, _ := url.Parse(upstreamServer.URL)
		upstreamHost := upstreamURL.Hostname()

		// Setup: Environment variable for secret
		testSecretEnvVar := "TEST_AUDIT_SECRET"
		os.Setenv(testSecretEnvVar, "audit-test-secret")
		defer os.Unsetenv(testSecretEnvVar)

		// Setup: Generate CA
		tempDir := t.TempDir()
		caKeyPath := filepath.Join(tempDir, "ca-key.pem")
		caCertPath := filepath.Join(tempDir, "ca-cert.pem")

		ca, err := mitm.GenerateCA()
		require.NoError(t, err)
		err = mitm.StoreCA(ca, caKeyPath, caCertPath)
		require.NoError(t, err)

		// Setup: Configure audit logging to temp file
		auditPath := filepath.Join(tempDir, "audit.log")
		placeholder := "chap_audit_test_12345"
		proxyPort := findAvailablePort(t)
		cfg := &config.Config{
			Server: config.ServerConfig{
				Address: "127.0.0.1",
				Port:    proxyPort,
			},
			Services: map[string]config.ServiceConfig{
				"test-api": {
					HostPattern:    upstreamHost,
					AuthStrategy:   "bearer",
					CredentialRef:  fmt.Sprintf("env:%s", testSecretEnvVar),
					Placeholder:    placeholder,
					AllowedMethods: []string{"GET", "POST"},
					AllowedPaths:   []string{"/*"},
					MaxBodyBytes:   10 * 1024 * 1024,
				},
			},
			Logging: config.LoggingConfig{
				Level:  "debug",
				Format: "json",
				Output: "stdout",
			},
			Audit: config.AuditConfig{
				Enabled: true,
				Path:    auditPath,
			},
		}

		// Setup: Create service registry
		registry := service.NewRegistry()
		for _, svcCfg := range cfg.Services {
			svc := &service.Service{
				Name:            "test-api",
				HostPattern:     svcCfg.HostPattern,
				AuthStrategyRef: svcCfg.AuthStrategy,
				CredentialRef:   svcCfg.CredentialRef,
				Placeholder:     svcCfg.Placeholder,
				Policy: &service.Policy{
					AllowedMethods: svcCfg.AllowedMethods,
					AllowedPaths:   svcCfg.AllowedPaths,
					MaxBodyBytes:   svcCfg.MaxBodyBytes,
				},
			}
			err := registry.Register(svc)
			require.NoError(t, err)
		}

		// Setup: Start proxy server
		certCache := mitm.NewCertCache(ca, nil)
		ctx := context.Background()
		logger := slog.Default()
		shutdownMgr := shutdown.NewManager(logger)

		secretRegistry, authRegistry := setupAuthRegistries()

		proxyServer := proxy.NewWithMITM(cfg, logger, shutdownMgr, registry, certCache, &proxy.MITMOptions{
			SecretRegistry: secretRegistry,
			AuthRegistry:   authRegistry,
		})
		err = proxyServer.Start()
		require.NoError(t, err)
		defer proxyServer.Stop(ctx)

		// Setup: Create client
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

		// EXECUTE: Make request that triggers injection
		req, err := http.NewRequest("GET", upstreamServer.URL+"/api/test", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+placeholder)

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// VERIFY: Audit file exists and contains entry
		auditData, err := os.ReadFile(auditPath)
		require.NoError(t, err, "Audit file should exist")

		// Parse audit entries
		entries := strings.Split(strings.TrimSpace(string(auditData)), "\n")
		require.Greater(t, len(entries), 0, "Audit file should have at least one entry")

		// Parse the last entry
		var entry audit.Entry
		err = json.Unmarshal([]byte(entries[len(entries)-1]), &entry)
		require.NoError(t, err, "Audit entry should be valid JSON")

		// Verify entry fields
		assert.Equal(t, audit.EventCredentialInjected, entry.Event)
		assert.Equal(t, "test-api", entry.Service)
		assert.Contains(t, entry.Host, upstreamHost)
		assert.Equal(t, "/api/test", entry.Path)
		assert.Equal(t, "GET", entry.Method)
		assert.Equal(t, "bearer", entry.AuthStrategy)
		assert.NotEmpty(t, entry.RequestID)

		t.Log("PASS: Audit entry written on successful injection")
	})

	// Test 2: No audit entry when injection skipped
	t.Run("no audit entry when injection skipped", func(t *testing.T) {
		// Setup: Create upstream server
		upstreamHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		})
		upstreamServer := httptest.NewTLSServer(upstreamHandler)
		defer upstreamServer.Close()
		upstreamURL, _ := url.Parse(upstreamServer.URL)
		upstreamHost := upstreamURL.Hostname()

		// Setup: Environment variable for secret
		testSecretEnvVar := "TEST_NO_AUDIT_SECRET"
		os.Setenv(testSecretEnvVar, "no-audit-secret")
		defer os.Unsetenv(testSecretEnvVar)

		// Setup: Generate CA
		tempDir := t.TempDir()
		caKeyPath := filepath.Join(tempDir, "ca-key.pem")
		caCertPath := filepath.Join(tempDir, "ca-cert.pem")

		ca, err := mitm.GenerateCA()
		require.NoError(t, err)
		err = mitm.StoreCA(ca, caKeyPath, caCertPath)
		require.NoError(t, err)

		// Setup: Configure audit logging to temp file
		auditPath := filepath.Join(tempDir, "audit.log")
		placeholder := "chap_no_audit_12345"
		proxyPort := findAvailablePort(t)
		cfg := &config.Config{
			Server: config.ServerConfig{
				Address: "127.0.0.1",
				Port:    proxyPort,
			},
			Services: map[string]config.ServiceConfig{
				"test-api": {
					HostPattern:    upstreamHost,
					AuthStrategy:   "bearer",
					CredentialRef:  fmt.Sprintf("env:%s", testSecretEnvVar),
					Placeholder:    placeholder,
					AllowedMethods: []string{"GET", "POST"},
					AllowedPaths:   []string{"/*"},
					MaxBodyBytes:   10 * 1024 * 1024,
				},
			},
			Logging: config.LoggingConfig{
				Level:  "debug",
				Format: "json",
				Output: "stdout",
			},
			Audit: config.AuditConfig{
				Enabled: true,
				Path:    auditPath,
			},
		}

		// Setup: Create service registry
		registry := service.NewRegistry()
		for _, svcCfg := range cfg.Services {
			svc := &service.Service{
				Name:            "test-api",
				HostPattern:     svcCfg.HostPattern,
				AuthStrategyRef: svcCfg.AuthStrategy,
				CredentialRef:   svcCfg.CredentialRef,
				Placeholder:     svcCfg.Placeholder,
				Policy: &service.Policy{
					AllowedMethods: svcCfg.AllowedMethods,
					AllowedPaths:   svcCfg.AllowedPaths,
					MaxBodyBytes:   svcCfg.MaxBodyBytes,
				},
			}
			err := registry.Register(svc)
			require.NoError(t, err)
		}

		// Setup: Start proxy server
		certCache := mitm.NewCertCache(ca, nil)
		ctx := context.Background()
		logger := slog.Default()
		shutdownMgr := shutdown.NewManager(logger)

		secretRegistry, authRegistry := setupAuthRegistries()

		proxyServer := proxy.NewWithMITM(cfg, logger, shutdownMgr, registry, certCache, &proxy.MITMOptions{
			SecretRegistry: secretRegistry,
			AuthRegistry:   authRegistry,
		})
		err = proxyServer.Start()
		require.NoError(t, err)
		defer proxyServer.Stop(ctx)

		// Setup: Create client
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

		// EXECUTE: Make request WITHOUT placeholder (should skip injection)
		req, err := http.NewRequest("GET", upstreamServer.URL+"/api/test", nil)
		require.NoError(t, err)
		// NO Authorization header

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// VERIFY: Audit file either doesn't exist or is empty
		auditData, err := os.ReadFile(auditPath)
		if err == nil {
			// File exists - check it's empty
			trimmed := strings.TrimSpace(string(auditData))
			assert.Empty(t, trimmed, "Audit file should be empty when no injection occurs")
		} else {
			// File doesn't exist - that's also valid
			assert.True(t, os.IsNotExist(err), "Audit file should not exist when no injection occurs")
		}

		t.Log("PASS: No audit entry when injection skipped")
	})
}
