package cmd

import (
	"github.com/bmf/chaperone/internal/control"
	"github.com/bmf/chaperone/internal/mcpgrants"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

// mcpCmd runs the stdio MCP server Claude Code spawns as a subprocess. It is a
// thin client of the running daemon's control plane: each MCP tool call becomes a
// control-API call over the well-known unix socket. It resolves no secrets and
// enforces no policy ([LAW:single-enforcer]).
var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run the stdio MCP server for dynamic credential grants",
	Long: `Run a Model Context Protocol server over stdio that exposes the running
chaperone daemon's grant control plane as agent tools.

Claude Code (or any MCP client) spawns this as a subprocess and talks JSON-RPC
over stdin/stdout. Each tool call is forwarded to the daemon's control socket at
~/.config/chaperone/control.sock:

  chaperone_list_grantable  Discover the approved universe of grantable pairings.
  chaperone_grant           Activate a pairing (scope narrowed within its bound).
  chaperone_revoke          Tear down an active grant.
  chaperone_list            List currently active grants.

The server holds no secrets: credential_ref values are pointers (env:/file:/
keychain:), and the daemon resolves and injects credentials itself. If no daemon
is running, tool calls fail loudly — start one with 'chaperone inject'.

Register with Claude Code (.mcp.json):

  {
    "mcpServers": {
      "chaperone": { "command": "chaperone", "args": ["mcp"] }
    }
  }`,
	Args: cobra.NoArgs,
	RunE: runMCPServer,
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

func runMCPServer(cmd *cobra.Command, _ []string) error {
	socketPath, err := getControlSocketPath()
	if err != nil {
		return err
	}

	server := mcpgrants.NewServer(control.NewClient(socketPath))

	// Run blocks until the client closes the connection or the context is
	// canceled (cobra cancels it on SIGINT/SIGTERM).
	return server.Run(cmd.Context(), &mcp.StdioTransport{})
}
