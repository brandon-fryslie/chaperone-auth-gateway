package control_test

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"log/slog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bmf/chaperone/internal/audit"
	"github.com/bmf/chaperone/internal/config"
	"github.com/bmf/chaperone/internal/control"
	"github.com/bmf/chaperone/internal/grant"
	"github.com/bmf/chaperone/internal/service"
)

// captureLogger records audit entries so a test can assert the grant lifecycle is
// audited without parsing log output.
type captureLogger struct {
	mu      sync.Mutex
	entries []audit.Entry
}

func (c *captureLogger) Log(e audit.Entry) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, e)
	return nil
}
func (c *captureLogger) Close() error { return nil }

func (c *captureLogger) entry(event string) (audit.Entry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.entries {
		if e.Event == event {
			return e, true
		}
	}
	return audit.Entry{}, false
}

// requireAttributed asserts the audit entry for event exists and is attributed
// to THIS process by kernel-attested uid/pid — values that can only be present
// if the real peer-credential syscall ran on the real socket connection.
func requireAttributed(t *testing.T, log *captureLogger, event string) {
	t.Helper()
	e, ok := log.entry(event)
	require.True(t, ok, "event %s must be audited", event)
	require.NotNil(t, e.Caller, "event %s must carry the attested caller", event)
	assert.Equal(t, os.Geteuid(), e.Caller.UID, "caller uid must be the connecting process's euid")
	assert.Equal(t, os.Getpid(), e.Caller.PID, "caller pid must be the connecting process's pid")
}

// secretValue is what the credential_ref WOULD resolve to. It must never appear
// anywhere on the control plane (the boundary moves references, not secrets).
const secretValue = "sk-super-secret-never-on-the-wire"

// testUniverse builds a human-approved grantable universe: one bearer pairing for
// api.openai.com scoped to GET/POST under /v1/*.
func testUniverse(t *testing.T) (*grant.Enforcer, service.ServiceRegistry, *captureLogger) {
	t.Helper()
	grantable := []config.GrantableConfig{
		{
			CredentialRef:  "env:OPENAI_API_KEY",
			HostPattern:    "api.openai.com",
			AuthStrategy:   "bearer",
			AllowedMethods: []string{"GET", "POST"},
			AllowedPaths:   []string{"/v1/*"},
			MaxBodyBytes:   1 << 20,
		},
	}
	enf, err := grant.NewEnforcer(grantable)
	require.NoError(t, err)
	return enf, service.NewRegistry(), &captureLogger{}
}

// startControl wires the API + server on a temp socket and returns a typed client.
func startControl(t *testing.T, enf *grant.Enforcer, reg service.ServiceRegistry, log *captureLogger) (*control.Client, string) {
	t.Helper()
	api, err := control.NewAPI(enf, reg, log, slog.Default())
	require.NoError(t, err)

	socket := shortSocketPath(t)
	srv, err := control.NewServer(api, socket, slog.Default())
	require.NoError(t, err)
	require.NoError(t, srv.Start())
	t.Cleanup(func() { _ = srv.Stop(context.Background()) })

	return control.NewClient(socket), socket
}

// shortSocketPath returns a unix socket path short enough for the platform's
// sun_path limit (104 bytes on macOS) — t.TempDir() paths are too long.
func shortSocketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "cc")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "c.sock")
}

// validGrant is a fully-scoped request that narrows within the test universe's
// bound (methods ⊆ {GET,POST}, paths ⊆ /v1/*, body ≤ 1MB). Tests tweak one field
// to drive a specific accept/reject case.
func validGrant() control.GrantRequest {
	return control.GrantRequest{
		HostPattern:    "api.openai.com",
		CredentialRef:  "env:OPENAI_API_KEY",
		AuthStrategy:   "bearer",
		AllowedMethods: []string{"GET", "POST"},
		AllowedPaths:   []string{"/v1/*"},
		MaxBodyBytes:   1 << 20,
	}
}

func TestGrantWithinUniverseBecomesInjectionEligible(t *testing.T) {
	enf, reg, log := testUniverse(t)
	client, _ := startControl(t, enf, reg, log)
	ctx := context.Background()

	req := validGrant()
	req.AllowedMethods = []string{"GET"}                // narrows within {GET,POST}
	req.AllowedPaths = []string{"/v1/chat/completions"} // narrows within /v1/*
	req.MaxBodyBytes = 1 << 19                          // narrows within 1MB
	res, err := client.Grant(ctx, req)
	require.NoError(t, err)

	// The view carries the pointer, never a resolved secret.
	assert.Equal(t, "env:OPENAI_API_KEY", res.Service.CredentialRef)
	assert.NotContains(t, res.Service.CredentialRef, secretValue)

	// The host is now injection-eligible against the LIVE registry.
	assert.True(t, service.ShouldMITM(reg, "api.openai.com"))
	svc, err := reg.Lookup("api.openai.com")
	require.NoError(t, err)
	assert.Equal(t, []string{"GET"}, svc.Policy.AllowedMethods) // narrowed scope stored
	assert.Equal(t, []string{"/v1/chat/completions"}, svc.Policy.AllowedPaths)

	requireAttributed(t, log, audit.EventGrantApplied)
}

func TestGrantOutsideUniverseRefused(t *testing.T) {
	enf, reg, log := testUniverse(t)
	client, _ := startControl(t, enf, reg, log)

	req := validGrant()
	req.HostPattern = "api.evil.com"
	_, err := client.Grant(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no approved pairing")

	_, lookupErr := reg.Lookup("api.evil.com")
	assert.Error(t, lookupErr, "refused grant must not become eligible")
	requireAttributed(t, log, audit.EventGrantRejected)
}

func TestGrantWideningScopeRefused(t *testing.T) {
	enf, reg, log := testUniverse(t)
	client, _ := startControl(t, enf, reg, log)

	req := validGrant()
	req.AllowedPaths = []string{"/admin/*"} // outside the /v1/* bound
	_, err := client.Grant(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scope")

	_, lookupErr := reg.Lookup("api.openai.com")
	assert.Error(t, lookupErr)
}

func TestRevokeRemovesEligibilityAndIsIdempotent(t *testing.T) {
	enf, reg, log := testUniverse(t)
	client, _ := startControl(t, enf, reg, log)
	ctx := context.Background()

	_, err := client.Grant(ctx, validGrant())
	require.NoError(t, err)

	rev, err := client.Revoke(ctx, control.RevokeRequest{HostPattern: "api.openai.com"})
	require.NoError(t, err)
	assert.True(t, rev.Revoked, "a present grant reports revoked=true")
	_, lookupErr := reg.Lookup("api.openai.com")
	assert.Error(t, lookupErr, "revoke must remove eligibility")
	requireAttributed(t, log, audit.EventGrantRevoked)

	// Revoking again is a soft success (idempotent DELETE-style), not an error.
	rev2, err := client.Revoke(ctx, control.RevokeRequest{HostPattern: "api.openai.com"})
	require.NoError(t, err)
	assert.False(t, rev2.Revoked, "absent grant reports revoked=false")
}

func TestListReturnsReferencesOnly(t *testing.T) {
	enf, reg, log := testUniverse(t)
	client, _ := startControl(t, enf, reg, log)
	ctx := context.Background()

	_, err := client.Grant(ctx, validGrant())
	require.NoError(t, err)

	list, err := client.List(ctx)
	require.NoError(t, err)
	require.Len(t, list.Services, 1)
	assert.Equal(t, "env:OPENAI_API_KEY", list.Services[0].CredentialRef)
	assert.NotContains(t, list.Services[0].CredentialRef, secretValue)

	grantable, err := client.ListGrantable(ctx)
	require.NoError(t, err)
	require.Len(t, grantable.Pairings, 1)
	p := grantable.Pairings[0]
	assert.Equal(t, "env:OPENAI_API_KEY", p.CredentialRef)
	assert.Equal(t, "api.openai.com", p.HostPattern)
	assert.Equal(t, "bearer", p.AuthStrategy)
	assert.Equal(t, []string{"/v1/*"}, p.MaxBound.AllowedPaths)
}

func TestNoDaemonFailsLoudly(t *testing.T) {
	client := control.NewClient(filepath.Join(t.TempDir(), "absent.sock"))
	_, err := client.List(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not reachable")
}

// TestOperatorOnlyFieldRejectedAtWire proves the wire boundary refuses a smuggled
// operator-only field even from a raw client that bypasses the typed GrantRequest.
func TestOperatorOnlyFieldRejectedAtWire(t *testing.T) {
	enf, reg, log := testUniverse(t)
	_, socket := startControl(t, enf, reg, log)

	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", socket)
			},
		},
		Timeout: 5 * time.Second,
	}
	body := `{"host_pattern":"api.openai.com","credential_ref":"env:OPENAI_API_KEY","auth_strategy":"bearer","client_groups":["admins"]}`
	resp, err := httpClient.Post("http://chaperone"+control.PathGrant, "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"operator-only field must be rejected at decode, before the enforcer")
}
