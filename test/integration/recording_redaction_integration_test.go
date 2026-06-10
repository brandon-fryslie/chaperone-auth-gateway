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

	"github.com/bmf/chaperone/internal/config"
	"github.com/bmf/chaperone/internal/examine"
	"github.com/bmf/chaperone/internal/mitm"
	"github.com/bmf/chaperone/internal/proxy"
	"github.com/bmf/chaperone/internal/recorder"
	"github.com/bmf/chaperone/internal/redact"
	"github.com/bmf/chaperone/internal/service"
	"github.com/bmf/chaperone/internal/shutdown"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// harArchive mirrors the HAR JSON shape closely enough to inspect recorded
// request headers structurally, not just by substring.
type harArchive struct {
	Entries []struct {
		Request struct {
			URL     string `json:"url"`
			Headers []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"headers"`
		} `json:"request"`
	} `json:"entries"`
}

// TestRecordingContainsNoInjectedCredential is the acceptance gate for
// chaperone-security-3at.3: a transaction whose headers, cookies, and body
// all carry known secret values is recorded, the real credential is proven
// to have reached the upstream on the wire, and the secret string appears
// nowhere in the recorder output — neither the in-memory JSON nor the
// written file bytes. The recorded request must also lack the injected
// credential entirely (it is captured pre-injection).
//
// UNGAMEABLE: real proxy, real TLS MITM, real secret provider, and the
// upstream asserts it received the REAL credential — so the recording being
// clean cannot be explained by injection not having happened.
func TestRecordingContainsNoInjectedCredential(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	const (
		injectedSecret = "sk-canary-injected-secret-77af31"
		clientCookie   = "session=client-cookie-canary-4202"
		clientAPIKey   = "client-key-canary-1d9e"
		bodyWithSecret = `{"prompt":"my key is sk-canary-injected-secret-77af31"}`
		secretEnvVar   = "TEST_RECORDING_REDACTION_SECRET"
	)

	// Upstream verifies the REAL credential arrived on the wire.
	var upstreamReceivedAuth, upstreamReceivedCookie string
	upstreamServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamReceivedAuth = r.Header.Get("Authorization")
		upstreamReceivedCookie = r.Header.Get("Cookie")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer upstreamServer.Close()
	upstreamURL, _ := url.Parse(upstreamServer.URL)
	upstreamHost := upstreamURL.Hostname()

	os.Setenv(secretEnvVar, injectedSecret)
	defer os.Unsetenv(secretEnvVar)

	tempDir := t.TempDir()
	ca, err := mitm.GenerateCA()
	require.NoError(t, err)
	caKeyPath := filepath.Join(tempDir, "ca-key.pem")
	caCertPath := filepath.Join(tempDir, "ca-cert.pem")
	require.NoError(t, mitm.StoreCA(ca, caKeyPath, caCertPath))

	cfg := &config.Config{
		Server: config.ServerConfig{Address: "127.0.0.1", Port: findAvailablePort(t)},
		Services: map[string]config.ServiceConfig{
			"test-api": {
				HostPattern:    upstreamHost,
				AuthStrategy:   "bearer",
				CredentialRef:  fmt.Sprintf("env:%s", secretEnvVar),
				AllowedMethods: []string{"GET", "POST"},
				AllowedPaths:   []string{"/*"},
				MaxBodyBytes:   10 * 1024 * 1024,
			},
		},
		Logging: config.LoggingConfig{Level: "info", Format: "json", Output: "stdout"},
	}

	registry := service.NewRegistry()
	for _, svcCfg := range cfg.Services {
		require.NoError(t, registry.Register(&service.Service{
			HostPattern:     svcCfg.HostPattern,
			AuthStrategyRef: svcCfg.AuthStrategy,
			CredentialRef:   svcCfg.CredentialRef,
			Policy: &service.Policy{
				AllowedMethods: svcCfg.AllowedMethods,
				AllowedPaths:   svcCfg.AllowedPaths,
				MaxBodyBytes:   svcCfg.MaxBodyBytes,
			},
		}))
	}

	certCache := mitm.NewCertCache(ca, nil)
	logger := slog.Default()
	shutdownMgr := shutdown.NewManager(logger)
	secretRegistry, authRegistry := setupAuthRegistries()

	proxyServer := newGatedMITMProxy(t, cfg, logger, shutdownMgr, registry, certCache, &proxy.MITMOptions{
		UpstreamCAs:    trustUpstreams(upstreamServer),
		SecretRegistry: secretRegistry,
		AuthRegistry:   authRegistry,
	})
	require.NoError(t, proxyServer.Start())
	defer proxyServer.Stop(context.Background())

	caCertPEM, err := os.ReadFile(caCertPath)
	require.NoError(t, err)
	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(caCertPEM)

	proxyURL, dialer := gatedProxyURL(t, proxyServer)
	client := &http.Client{
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(proxyURL),
			DialContext:     dialer,
			TLSClientConfig: &tls.Config{RootCAs: certPool},
		},
		Timeout: 10 * time.Second,
	}

	// EXECUTE: a transaction carrying the secret in the body, a live session
	// cookie, and a client-supplied API key header.
	req, err := http.NewRequest("POST", upstreamServer.URL+"/v1/complete", strings.NewReader(bodyWithSecret))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", clientCookie)
	req.Header.Set("X-API-Key", clientAPIKey)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// VERIFY: the real credential and cookie were on the wire — a clean
	// recording must not be explained by injection or forwarding failing.
	require.Equal(t, "Bearer "+injectedSecret, upstreamReceivedAuth,
		"upstream must receive the real injected credential")
	require.Equal(t, clientCookie, upstreamReceivedCookie,
		"upstream must receive the live cookie — redaction is recording-only")

	// VERIFY: in-memory recorder JSON carries none of the secret material.
	harJSON, err := proxyServer.GetRecorder().ToJSON()
	require.NoError(t, err)
	harText := string(harJSON)

	assert.NotContains(t, harText, injectedSecret, "injected secret must not appear in recording")
	assert.NotContains(t, harText, "client-cookie-canary-4202", "cookie value must not appear in recording")
	assert.NotContains(t, harText, clientAPIKey, "client-supplied API key must not appear in recording")
	assert.NotContains(t, harText, testProxySecret, "proxy access secret must not appear in recording")

	// VERIFY structurally: the recorded request was captured pre-injection,
	// so it has no Authorization header at all.
	var har harArchive
	require.NoError(t, json.Unmarshal(harJSON, &har))
	require.NotEmpty(t, har.Entries, "transaction must have been recorded")
	for _, entry := range har.Entries {
		for _, h := range entry.Request.Headers {
			assert.NotEqual(t, "authorization", strings.ToLower(h.Name),
				"recorded request must be the pre-injection state with no Authorization header")
			if strings.ToLower(h.Name) == "cookie" {
				assert.Equal(t, redact.Placeholder, h.Value, "cookie position must be redacted")
			}
		}
	}

	// VERIFY: the written file bytes are equally clean.
	harPath := filepath.Join(tempDir, "recording.har")
	require.NoError(t, proxyServer.GetRecorder().WriteToFile(harPath))
	fileBytes, err := os.ReadFile(harPath)
	require.NoError(t, err)
	assert.NotContains(t, string(fileBytes), injectedSecret, "secret must not reach disk")
	assert.NotContains(t, string(fileBytes), "client-cookie-canary-4202")

	t.Log("PASS: recording contains no injected credential, cookie, or proxy secret")
}

// TestExamineRecordingRedactsClientCredentials covers the examine-mode leak:
// examine forwards the client's LIVE credentials untouched (it must — it is
// a discovery tool), but its recording on disk must not retain their values.
// Header names stay visible so auth discovery still works.
//
// Uses absolute-form HTTP through the examine proxy: the recording pipeline
// is identical to the MITM'd path (the MITM leg itself is exercised by
// TestRecordingContainsNoInjectedCredential).
func TestExamineRecordingRedactsClientCredentials(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	const (
		liveToken  = "live-client-token-canary-83c0"
		liveCookie = "session=examine-cookie-canary-5511"
	)

	var upstreamReceivedAuth string
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamReceivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstreamServer.Close()

	ca, err := mitm.GenerateCA()
	require.NoError(t, err)
	certCache := mitm.NewCertCache(ca, nil)

	cfg := &config.Config{
		Server:  config.ServerConfig{Address: "127.0.0.1", Port: findAvailablePort(t)},
		Logging: config.LoggingConfig{Level: "info", Format: "json", Output: "stdout"},
	}

	// Built exactly as cmd/examine.go builds it: the redactor knows the
	// per-run proxy secret; client credentials are covered positionally.
	rec := recorder.NewRecorder(redact.NewRedactor(redact.Static(testProxySecret)))
	examineLogger := examine.NewLogger(examine.Config{})

	server, err := proxy.NewExamineProxy(cfg, slog.Default(), shutdown.NewManager(slog.Default()), certCache, examineLogger, rec, nil, testProxySecret)
	require.NoError(t, err)
	require.NoError(t, server.Start())
	defer server.Stop(context.Background())

	proxyURL, _ := gatedProxyURL(t, server)
	client := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		Timeout:   10 * time.Second,
	}

	req, err := http.NewRequest("GET", upstreamServer.URL+"/login", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+liveToken)
	req.Header.Set("Cookie", liveCookie)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// VERIFY: examine passed the live credential through untouched.
	require.Equal(t, "Bearer "+liveToken, upstreamReceivedAuth,
		"examine must forward client credentials unmodified")

	// VERIFY: the recording retains the header NAME (discovery) but not the value.
	harJSON, err := rec.ToJSON()
	require.NoError(t, err)
	harText := string(harJSON)

	assert.NotContains(t, harText, liveToken, "client token must not appear in examine recording")
	assert.NotContains(t, harText, "examine-cookie-canary-5511", "cookie value must not appear in examine recording")

	var har harArchive
	require.NoError(t, json.Unmarshal(harJSON, &har))
	require.NotEmpty(t, har.Entries)

	foundAuthHeader := false
	for _, h := range har.Entries[0].Request.Headers {
		if strings.ToLower(h.Name) == "authorization" {
			foundAuthHeader = true
			assert.Equal(t, redact.Placeholder, h.Value)
		}
	}
	assert.True(t, foundAuthHeader,
		"the Authorization header name must remain visible for auth discovery")

	t.Log("PASS: examine recording redacts live client credentials, keeps header names")
}
