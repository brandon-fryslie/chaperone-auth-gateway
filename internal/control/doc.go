// Package control is the daemon's localhost-only control plane: the surface that
// lets a separately-spawned client (the chaperone-mcp server) apply and revoke
// credential grants on the RUNNING proxy without a restart.
//
// # Parts and seams
//
// The package is cut into three parts at their real joints:
//
//   - protocol.go — the wire types shared by both ends (one source of truth for
//     the format). GrantRequest carries only references and the requested scope;
//     it cannot carry a secret value or an operator-only policy field, so those
//     illegal asks are unrepresentable rather than accepted-then-rejected.
//   - api.go — the four operations over live state (grant / revoke / list /
//     list-grantable). Pure logic plus exactly two effects at this boundary:
//     registry mutation and an audit write. It resolves no secrets.
//   - server.go / client.go — the world boundary: HTTP over a unix domain socket.
//     Server owns the listener and socket lifecycle; Client is the loud-on-no-
//     daemon counterpart that vf4.4's MCP server is a thin wrapper around.
//
// # Single enforcer
//
// The control plane re-decides nothing about what is grantable. A grant is built
// into a *service.Service and handed to grant.Enforcer.Authorize; its verdict is
// final and its rejection message is surfaced to the client verbatim. Secret
// resolution and per-request policy enforcement remain in the proxy's existing
// pipeline — this package only moves references and scopes.
//
// # Reachability
//
// The socket is owner-only (0600) and bound to a unix path, so it is unreachable
// off-box. If no daemon is listening, a client call fails loudly; the control
// plane never silently degrades.
package control
