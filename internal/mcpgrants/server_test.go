package mcpgrants

import (
	"context"
	"errors"
	"testing"

	"github.com/bmf/chaperone/internal/control"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeControl is a scripted ControlClient: each method returns the canned result
// and error it was given, and records the request it was called with so a test
// can assert the MCP argument was translated 1:1 onto the control type.
type fakeControl struct {
	grantReq  control.GrantRequest
	grantRes  control.GrantResult
	grantErr  error
	revokeReq control.RevokeRequest
	revokeRes control.RevokeResult
	listRes   control.ListResult
	grantRes2 control.ListGrantableResult
}

func (f *fakeControl) Grant(_ context.Context, req control.GrantRequest) (control.GrantResult, error) {
	f.grantReq = req
	return f.grantRes, f.grantErr
}

func (f *fakeControl) Revoke(_ context.Context, req control.RevokeRequest) (control.RevokeResult, error) {
	f.revokeReq = req
	return f.revokeRes, nil
}

func (f *fakeControl) List(_ context.Context) (control.ListResult, error) {
	return f.listRes, nil
}

func (f *fakeControl) ListGrantable(_ context.Context) (control.ListGrantableResult, error) {
	return f.grantRes2, nil
}

// connect wires the MCP server (backed by fake) to a client over an in-memory
// transport and returns an initialized session plus a cleanup func.
func connect(t *testing.T, fake ControlClient) (*mcp.ClientSession, context.Context) {
	t.Helper()
	ctx := context.Background()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	server := NewServer(fake)

	serverSession, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = clientSession.Close() })

	return clientSession, ctx
}

func TestToolsAdvertised(t *testing.T) {
	session, ctx := connect(t, &fakeControl{})

	res, err := session.ListTools(ctx, nil)
	require.NoError(t, err)

	got := make(map[string]bool)
	for _, tool := range res.Tools {
		got[tool.Name] = true
	}
	for _, want := range []string{"chaperone_list_grantable", "chaperone_grant", "chaperone_revoke", "chaperone_list"} {
		assert.True(t, got[want], "tool %q should be advertised", want)
	}
}

func TestGrantTranslatesArgsAndReturnsResult(t *testing.T) {
	fake := &fakeControl{
		grantRes: control.GrantResult{Service: control.ServiceView{
			Name: "openai", HostPattern: "api.openai.com", CredentialRef: "env:OPENAI_API_KEY", AuthStrategy: "bearer",
		}},
	}
	session, ctx := connect(t, fake)

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "chaperone_grant",
		Arguments: map[string]any{
			"host_pattern":   "api.openai.com",
			"credential_ref": "env:OPENAI_API_KEY",
			"auth_strategy":  "bearer",
			"allowed_paths":  []string{"/v1/chat/completions"},
		},
	})
	require.NoError(t, err)
	assert.False(t, res.IsError, "a successful grant is not an error result")

	// Args reached the control client 1:1, including the narrowed scope.
	assert.Equal(t, "api.openai.com", fake.grantReq.HostPattern)
	assert.Equal(t, "env:OPENAI_API_KEY", fake.grantReq.CredentialRef)
	assert.Equal(t, "bearer", fake.grantReq.AuthStrategy)
	assert.Equal(t, []string{"/v1/chat/completions"}, fake.grantReq.AllowedPaths)
}

func TestEnforcerRejectionSurfacesVerbatimAsToolError(t *testing.T) {
	const msg = "grant rejected: no approved pairing for env:OPENAI_API_KEY -> evil.example.com"
	fake := &fakeControl{grantErr: errors.New(msg)}
	session, ctx := connect(t, fake)

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "chaperone_grant",
		Arguments: map[string]any{
			"host_pattern":   "evil.example.com",
			"credential_ref": "env:OPENAI_API_KEY",
			"auth_strategy":  "bearer",
		},
	})
	// A tool error is NOT a protocol error: the call succeeds, IsError is set, and
	// the enforcer's message is carried verbatim so the agent can correct itself.
	require.NoError(t, err)
	assert.True(t, res.IsError, "enforcer rejection must be an IsError result")
	assert.Equal(t, msg, textOf(t, res))
}

func TestNoDaemonErrorSurfacesVerbatim(t *testing.T) {
	// The real client returns a "not reachable" error when no daemon is up; any
	// error from the control client must reach the agent, not be swallowed.
	fake := &fakeControl{grantErr: errors.New("control plane not reachable at /tmp/x.sock (is the chaperone daemon running?)")}
	session, ctx := connect(t, fake)

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "chaperone_grant",
		Arguments: map[string]any{
			"host_pattern": "api.openai.com", "credential_ref": "env:K", "auth_strategy": "bearer",
		},
	})
	require.NoError(t, err)
	assert.True(t, res.IsError)
	assert.Contains(t, textOf(t, res), "not reachable")
}

func TestRevokeTranslatesHost(t *testing.T) {
	fake := &fakeControl{revokeRes: control.RevokeResult{HostPattern: "api.openai.com", Revoked: true}}
	session, ctx := connect(t, fake)

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "chaperone_revoke",
		Arguments: map[string]any{"host_pattern": "api.openai.com"},
	})
	require.NoError(t, err)
	assert.False(t, res.IsError)
	assert.Equal(t, "api.openai.com", fake.revokeReq.HostPattern)
}

// textOf returns the concatenated text content of a tool result.
func textOf(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	out := ""
	for _, c := range res.Content {
		tc, ok := c.(*mcp.TextContent)
		require.True(t, ok, "expected text content, got %T", c)
		out += tc.Text
	}
	return out
}
