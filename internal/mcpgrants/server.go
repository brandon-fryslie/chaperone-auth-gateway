// Package mcpgrants is the agent-facing surface of the dynamic-grant flow: a
// stdio MCP server Claude Code spawns as a subprocess. Every tool call becomes a
// call to the running daemon's control API.
//
// This layer adds VOCABULARY and DELIVERY only. It resolves no secrets, holds no
// registry, and enforces no policy ([LAW:single-enforcer], [LAW:one-source-of-truth]):
// the daemon's proxy pipeline (match host → fetch secret → enforce policy → inject
// → audit) remains the one authority. The tool input types ARE the control wire
// types (control.GrantRequest / control.RevokeRequest), so the MCP schema and the
// control contract cannot drift, and the operator-only policy fields the wire type
// omits stay unrepresentable at the agent boundary too ([LAW:types-are-the-program]).
package mcpgrants

import (
	"context"

	"github.com/bmf/chaperone/internal/control"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ControlClient is the slice of the daemon's control API this server delivers.
// The consumer declares exactly what it needs ([LAW:one-way-deps]); *control.Client
// satisfies it for production, a fake satisfies it for tests. When no daemon is
// listening the underlying client returns a LOUD "not reachable" error — this
// server propagates it verbatim, never fabricating a fallback ([LAW:no-silent-failure]).
type ControlClient interface {
	Grant(ctx context.Context, req control.GrantRequest) (control.GrantResult, error)
	Revoke(ctx context.Context, req control.RevokeRequest) (control.RevokeResult, error)
	List(ctx context.Context) (control.ListResult, error)
	ListGrantable(ctx context.Context) (control.ListGrantableResult, error)
}

// Server metadata advertised to the MCP client during initialize.
const (
	serverName    = "chaperone-grants"
	serverVersion = "0.1.0"
)

// NewServer builds the MCP server, registering the four grant tools against the
// given control client. Pure construction: no IO, no stdio, no socket. The caller
// runs it over a transport (StdioTransport in production), keeping the effect at
// the edge ([LAW:effects-at-boundaries]).
func NewServer(client ControlClient) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: serverVersion}, nil)

	// Each handler is a pure relay: call the one control method, return its typed
	// result and error. A non-nil error is packed by the SDK into the tool result
	// with IsError set (a tool error, not a protocol error), so the enforcer's
	// verbatim rejection reaches the agent for it to narrow and retry.
	mcp.AddTool(s, listGrantableTool, func(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, control.ListGrantableResult, error) {
		res, err := client.ListGrantable(ctx)
		return nil, res, err
	})
	mcp.AddTool(s, grantTool, func(ctx context.Context, _ *mcp.CallToolRequest, in control.GrantRequest) (*mcp.CallToolResult, control.GrantResult, error) {
		res, err := client.Grant(ctx, in)
		return nil, res, err
	})
	mcp.AddTool(s, revokeTool, func(ctx context.Context, _ *mcp.CallToolRequest, in control.RevokeRequest) (*mcp.CallToolResult, control.RevokeResult, error) {
		res, err := client.Revoke(ctx, in)
		return nil, res, err
	})
	mcp.AddTool(s, listTool, func(ctx context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, control.ListResult, error) {
		res, err := client.List(ctx)
		return nil, res, err
	})

	return s
}

// noArgs is the input type for the discovery/listing tools, which take no
// parameters. AddTool requires a struct so the inferred input schema is an object.
type noArgs struct{}

var listGrantableTool = &mcp.Tool{
	Name: "chaperone_list_grantable",
	Description: "List the approved universe of grantable credential→host pairings, with each " +
		"pairing's max_bound (the widest scope you may request). Call this FIRST to discover what " +
		"you may grant and how wide a scope is allowed before calling chaperone_grant. " +
		"References only — credential_ref is a pointer (env:/file:/keychain:), never a secret value.",
}

var grantTool = &mcp.Tool{
	Name: "chaperone_grant",
	Description: "Activate a credential→host pairing so the proxy injects the credential for matching " +
		"requests. credential_ref is a POINTER to a credential (env:NAME, file:/path, or keychain:service/account) " +
		"— NEVER pass a secret value; there is no field for one and the daemon resolves secrets itself. " +
		"You may only grant a pairing present in chaperone_list_grantable, and may narrow allowed_methods/" +
		"allowed_paths/max_body_bytes within that pairing's max_bound. If the daemon rejects the grant " +
		"(off-universe pairing, or scope wider than the bound), its message is returned verbatim — narrow " +
		"your request and retry.",
}

var revokeTool = &mcp.Tool{
	Name: "chaperone_revoke",
	Description: "Tear down the active grant for a host_pattern so the proxy stops injecting for it. " +
		"Idempotent: revoking a host with no active grant succeeds with revoked=false.",
}

var listTool = &mcp.Tool{
	Name: "chaperone_list",
	Description: "List the currently active grants (everything the proxy will inject for right now). " +
		"References only — no secret values are ever returned.",
}
