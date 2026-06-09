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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bmf/chaperone/internal/audit"
	"github.com/bmf/chaperone/internal/config"
	"github.com/bmf/chaperone/internal/control"
	"github.com/bmf/chaperone/internal/mcpgrants"
	"github.com/bmf/chaperone/internal/mitm"
	"github.com/bmf/chaperone/internal/orchestrate"
	"github.com/bmf/chaperone/internal/proxy"
	"github.com/bmf/chaperone/internal/shutdown"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file is the completion gate for the dynamic-grant epic (vf4.x): it proves
// the WHOLE pipeline on the wire — an agent grants a pre-approved pairing through
// the MCP tool surface, and the running daemon injects (or refuses to inject) the
// credential on a real HTTPS request through the MITM proxy.
//
// What is real here: the proxy + control plane are assembled through the SAME
// orchestrate path `chaperone inject` uses; they share one service registry, grant
// enforcer, and audit sink. Grants travel agent → MCP tool → control.Client → unix
// socket → control API → enforcer → registry, then a real http.Client makes a real
// TLS request through the proxy to a real upstream. The only seams not exercised are
// the stdio framing and the os/exec subprocess spawn — pure delivery, proven in
// internal/mcpgrants unit tests. ([LAW:behavior-not-structure]: every assertion
// below is on observable wire behavior — header present/absent, request allowed/
// rejected — never on internal registry state.)
//
// The secret value used throughout; deliberately distinctive so the
// "no secret ever crosses the boundary" assertions are meaningful greps.
const wireSecret = "sk-vf45-on-the-wire-secret-DO-NOT-LEAK"

const grantEnvVar = "TEST_VF45_TOKEN"

// upstreamObservation records exactly what an upstream request carried, so a test
// can assert on the wire what the proxy did (or did not) inject.
type upstreamObservation struct {
	mu     sync.Mutex
	called bool
	auth   string
	apiKey string
	path   string
	method string
}

func (o *upstreamObservation) record(r *http.Request) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.called = true
	o.auth = r.Header.Get("Authorization")
	o.apiKey = r.Header.Get("X-API-Key")
	o.path = r.URL.Path
	o.method = r.Method
}

func (o *upstreamObservation) snapshot() upstreamObservation {
	o.mu.Lock()
	defer o.mu.Unlock()
	return upstreamObservation{called: o.called, auth: o.auth, apiKey: o.apiKey, path: o.path, method: o.method}
}

// reset clears the recorded data without touching the mutex (overwriting the whole
// struct would copy a zero mutex over a held lock).
func (o *upstreamObservation) reset() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.called, o.auth, o.apiKey, o.path, o.method = false, "", "", "", ""
}

// grantDaemon is an in-process chaperone daemon plus an agent-facing MCP session and
// a trusting HTTP client — everything a grant E2E needs, assembled once.
type grantDaemon struct {
	t           *testing.T
	ctx         context.Context
	mcp         *mcp.ClientSession
	client      *http.Client
	upstreamURL string
	auditPath   string
	obs         *upstreamObservation
}

// newGrantDaemon stands up the real daemon (proxy + control plane) over the given
// grantable universe, with grantEnvVar set to wireSecret for the run of the test.
// The HTTP client trusts BOTH the MITM CA (so injected requests verify) and the
// upstream's own cert (so a request to an UNGRANTED host — a transparent tunnel —
// still completes and can be inspected for the absence of injection).
func newGrantDaemon(t *testing.T, grantable []config.GrantableConfig) *grantDaemon {
	t.Helper()
	t.Setenv(grantEnvVar, wireSecret)
	ctx := context.Background()

	obs := &upstreamObservation{}
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		obs.record(r)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(upstream.Close)
	upstreamURL, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	upstreamHost := upstreamURL.Hostname()

	// Stamp the upstream host into every grantable pairing so the universe matches
	// the live test server (the caller declares scope/strategy, not the ephemeral host).
	for i := range grantable {
		grantable[i].HostPattern = upstreamHost
	}

	tempDir := t.TempDir()
	caKeyPath := filepath.Join(tempDir, "ca-key.pem")
	caCertPath := filepath.Join(tempDir, "ca-cert.pem")
	ca, err := mitm.GenerateCA()
	require.NoError(t, err)
	require.NoError(t, mitm.StoreCA(ca, caKeyPath, caCertPath))

	auditPath := filepath.Join(tempDir, "audit.log")
	cfg := &config.Config{
		Server:    config.ServerConfig{Address: "127.0.0.1", Port: findAvailablePort(t)},
		Logging:   config.LoggingConfig{Level: "info", Format: "json", Output: "stdout"},
		Grantable: grantable,
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	shutdownMgr := shutdown.NewManager(logger)

	result, err := orchestrate.Setup(ctx, orchestrate.SetupConfig{
		Config:     cfg,
		CAKeyPath:  caKeyPath,
		CACertPath: caCertPath,
	}, ca, logger)
	require.NoError(t, err)

	auditLogger, err := audit.NewLogger(audit.Config{Enabled: true, Path: auditPath})
	require.NoError(t, err)

	proxyServer := orchestrate.CreateProxy(ctx, cfg, logger, shutdownMgr, result, "", auditLogger)
	require.NoError(t, proxyServer.Start())

	// Unix socket paths are capped (104 chars on macOS); t.TempDir() is too deep, so
	// keep the socket short and shallow. The port is unique while the proxy holds it.
	socketPath := filepath.Join("/tmp", fmt.Sprintf("ch-vf45-%d.sock", cfg.Server.Port))
	t.Cleanup(func() { _ = os.Remove(socketPath) })
	require.NoError(t, orchestrate.StartControlPlane(ctx, result, auditLogger, socketPath, shutdownMgr, logger))

	// Proxy and control server both self-register their Stop with the manager;
	// one Shutdown tears the whole daemon down (and closes the audit file).
	t.Cleanup(func() { _ = shutdownMgr.Shutdown(5 * time.Second) })

	// Client trusts the MITM CA AND the upstream's own cert (dual trust — see above).
	caCertPEM, err := os.ReadFile(caCertPath)
	require.NoError(t, err)
	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM(caCertPEM))
	pool.AddCert(upstream.Certificate())

	proxyURL, dialer := proxy.GetProxyURL(cfg)
	client := &http.Client{
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(proxyURL),
			DialContext:     dialer,
			TLSClientConfig: &tls.Config{RootCAs: pool},
		},
		Timeout: 10 * time.Second,
	}

	// Agent-facing MCP session backed by the REAL control client over the socket.
	mcpServer := mcpgrants.NewServer(control.NewClient(socketPath))
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := mcpServer.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Close() })
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "vf45-test", Version: "0"}, nil)
	clientSession, err := mcpClient.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = clientSession.Close() })

	return &grantDaemon{
		t:           t,
		ctx:         ctx,
		mcp:         clientSession,
		client:      client,
		upstreamURL: upstream.URL,
		auditPath:   auditPath,
		obs:         obs,
	}
}

// grant issues a chaperone_grant tool call. The host_pattern is filled from the
// live upstream so callers only declare credential/strategy/scope.
func (d *grantDaemon) grant(args map[string]any) *mcp.CallToolResult {
	d.t.Helper()
	if _, ok := args["host_pattern"]; !ok {
		host, _ := url.Parse(d.upstreamURL)
		args["host_pattern"] = host.Hostname()
	}
	res, err := d.mcp.CallTool(d.ctx, &mcp.CallToolParams{Name: "chaperone_grant", Arguments: args})
	require.NoError(d.t, err, "tool call transport must succeed; rejection is carried in the result, not as a protocol error")
	return res
}

// revoke issues a chaperone_revoke for the live upstream host.
func (d *grantDaemon) revoke() *mcp.CallToolResult {
	d.t.Helper()
	host, _ := url.Parse(d.upstreamURL)
	res, err := d.mcp.CallTool(d.ctx, &mcp.CallToolParams{
		Name:      "chaperone_revoke",
		Arguments: map[string]any{"host_pattern": host.Hostname()},
	})
	require.NoError(d.t, err)
	return res
}

// request resets the upstream observation, makes a real proxied request, and
// returns the response status plus what the upstream actually received.
func (d *grantDaemon) request(method, path string) (int, upstreamObservation) {
	d.t.Helper()
	d.obs.reset()

	req, err := http.NewRequest(method, d.upstreamURL+path, nil)
	require.NoError(d.t, err)
	resp, err := d.client.Do(req)
	require.NoError(d.t, err, "proxied request should complete (client trusts both MITM and upstream certs)")
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, d.obs.snapshot()
}

func (d *grantDaemon) readAudit() string {
	d.t.Helper()
	data, err := os.ReadFile(d.auditPath)
	require.NoError(d.t, err)
	return string(data)
}

// bearerPairing is the standard grantable universe used by most tests: one bearer
// pairing whose widest scope is GET/POST under /v1/*.
func bearerPairing() []config.GrantableConfig {
	return []config.GrantableConfig{{
		CredentialRef:  "env:" + grantEnvVar,
		AuthStrategy:   "bearer",
		AllowedMethods: []string{"GET", "POST"},
		AllowedPaths:   []string{"/v1/*"},
	}}
}

func textOfResult(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// TestGrantedHostWithinScopeIsInjected: a granted pairing causes the proxy to
// inject the credential on a real HTTPS request within scope.
func TestGrantedHostWithinScopeIsInjected(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	d := newGrantDaemon(t, bearerPairing())

	res := d.grant(map[string]any{
		"credential_ref":  "env:" + grantEnvVar,
		"auth_strategy":   "bearer",
		"allowed_paths":   []string{"/v1/*"},
		"allowed_methods": []string{"GET", "POST"},
	})
	require.False(t, res.IsError, "grant within the approved universe must succeed: %s", textOfResult(t, res))

	status, obs := d.request("GET", "/v1/chat/completions")

	assert.Equal(t, http.StatusOK, status)
	assert.True(t, obs.called, "upstream must be reached for an in-scope request")
	assert.Equal(t, "Bearer "+wireSecret, obs.auth,
		"upstream must receive the injected Bearer credential on the wire")
}

// TestGrantedHostOutsideScopeIsRejectedBeforeInjection: with a grant active, a
// request outside the granted paths/methods is rejected by policy (fail-fast) and
// the upstream is never reached — so the credential is never injected.
func TestGrantedHostOutsideScopeIsRejectedBeforeInjection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	d := newGrantDaemon(t, bearerPairing())

	// Narrow the grant to GET under /v1/* — strictly within the pairing's bound.
	res := d.grant(map[string]any{
		"credential_ref":  "env:" + grantEnvVar,
		"auth_strategy":   "bearer",
		"allowed_methods": []string{"GET"},
		"allowed_paths":   []string{"/v1/*"},
	})
	require.False(t, res.IsError, "narrowed grant must succeed: %s", textOfResult(t, res))

	t.Run("path outside scope", func(t *testing.T) {
		status, obs := d.request("GET", "/admin/secrets")
		assert.Equal(t, http.StatusForbidden, status, "out-of-path request must be denied by policy")
		assert.False(t, obs.called, "upstream must NOT be reached when policy denies (fail-fast before injection)")
	})

	t.Run("method outside scope", func(t *testing.T) {
		status, obs := d.request("POST", "/v1/chat/completions")
		assert.Equal(t, http.StatusForbidden, status, "out-of-method request must be denied by policy")
		assert.False(t, obs.called, "upstream must NOT be reached when policy denies (fail-fast before injection)")
	})
}

// TestOffUniversePairingIsRefused: a grant for a pairing the human never approved
// is refused by the enforcer — the refusal reaches the agent verbatim as a tool
// error, and no injection-eligible state is created (an ungranted host is never
// injected on the wire).
func TestOffUniversePairingIsRefused(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	d := newGrantDaemon(t, bearerPairing())

	// Same approved credential, but a host that is NOT in the universe.
	res := d.grant(map[string]any{
		"host_pattern":   "evil.example.com",
		"credential_ref": "env:" + grantEnvVar,
		"auth_strategy":  "bearer",
	})
	require.True(t, res.IsError, "an off-universe grant MUST be refused")
	assert.Contains(t, textOfResult(t, res), "no approved pairing",
		"the enforcer's refusal must reach the agent verbatim")

	// No grant was applied for the upstream host, so the proxy never injects:
	// the request is a transparent tunnel and the upstream sees no credential.
	status, obs := d.request("GET", "/v1/chat/completions")
	assert.Equal(t, http.StatusOK, status)
	assert.True(t, obs.called)
	assert.Empty(t, obs.auth, "an ungranted host must receive NO injected credential on the wire")
}

// TestRevokeStopsInjection: after a grant injects, revoking it makes the proxy stop
// injecting — the same request that carried the credential now carries none.
func TestRevokeStopsInjection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	d := newGrantDaemon(t, bearerPairing())

	res := d.grant(map[string]any{
		"credential_ref":  "env:" + grantEnvVar,
		"auth_strategy":   "bearer",
		"allowed_methods": []string{"GET", "POST"},
		"allowed_paths":   []string{"/v1/*"},
	})
	require.False(t, res.IsError, "grant must succeed: %s", textOfResult(t, res))

	// Injection is live.
	_, obs := d.request("GET", "/v1/chat/completions")
	require.Equal(t, "Bearer "+wireSecret, obs.auth, "precondition: grant injects before revoke")

	revRes := d.revoke()
	require.False(t, revRes.IsError, "revoke must succeed: %s", textOfResult(t, revRes))

	// Same request, post-revoke: no MITM, no injection. The upstream is reached as a
	// transparent tunnel and sees no credential.
	status, after := d.request("GET", "/v1/chat/completions")
	assert.Equal(t, http.StatusOK, status)
	assert.True(t, after.called)
	assert.Empty(t, after.auth, "after revoke the upstream must receive NO injected credential")
}

// TestNoSecretCrossesTheGrantBoundary: across a full grant+inject cycle, the secret
// VALUE appears nowhere the agent or operator can read it — not in the MCP traffic
// (which carries only the credential_ref pointer) and not in the audit trail.
func TestNoSecretCrossesTheGrantBoundary(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	d := newGrantDaemon(t, bearerPairing())

	grantRes := d.grant(map[string]any{
		"credential_ref":  "env:" + grantEnvVar,
		"auth_strategy":   "bearer",
		"allowed_methods": []string{"GET", "POST"},
		"allowed_paths":   []string{"/v1/*"},
	})
	require.False(t, grantRes.IsError)

	listRes, err := d.mcp.CallTool(d.ctx, &mcp.CallToolParams{Name: "chaperone_list"})
	require.NoError(t, err)
	grantableRes, err := d.mcp.CallTool(d.ctx, &mcp.CallToolParams{Name: "chaperone_list_grantable"})
	require.NoError(t, err)

	// Drive a real injection so the credential is actually resolved and used.
	_, obs := d.request("GET", "/v1/chat/completions")
	require.Equal(t, "Bearer "+wireSecret, obs.auth)

	// MCP traffic carries the pointer, never the value.
	for name, text := range map[string]string{
		"grant":          textOfResult(t, grantRes),
		"list":           textOfResult(t, listRes),
		"list_grantable": textOfResult(t, grantableRes),
	} {
		assert.NotContains(t, text, wireSecret, "%s tool result must not contain the secret value", name)
		assert.Contains(t, text, "env:"+grantEnvVar, "%s tool result should carry the credential_ref pointer", name)
	}

	// The audit trail records the grant lifecycle by reference, never the value.
	auditText := d.readAudit()
	assert.Contains(t, auditText, audit.EventGrantApplied, "the grant must be audited")
	assert.NotContains(t, auditText, wireSecret, "the audit trail must never contain the secret value")
}
