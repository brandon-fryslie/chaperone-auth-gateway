package control

import (
	"context"
	"fmt"
	"net"

	"log/slog"

	"github.com/bmf/chaperone/internal/audit"
)

// Peer is the kernel-attested identity of the process on the other end of a
// control-socket connection. It is read from the socket itself (SO_PEERCRED on
// Linux, LOCAL_PEERCRED on macOS), never from request content, so a client
// cannot claim an identity it does not hold.
type Peer struct {
	UID int
	PID int
}

// auditCaller is the audit-record view of the peer. [LAW:one-way-deps]: audit
// cannot import control, so the record type mirrors this one at the boundary.
func (p Peer) auditCaller() *audit.Caller { return &audit.Caller{UID: p.UID, PID: p.PID} }

// peerGate authenticates every connection at the accept seam, before HTTP sees
// a byte: the kernel-reported peer UID must equal the daemon's own euid, or the
// connection is closed. Socket file permissions are the first layer; this gate
// is the authenticating one, and it is the ONLY place the access decision is
// made ([LAW:single-enforcer], [LAW:effects-at-boundaries]).
type peerGate struct {
	net.Listener
	ownUID int
	// creds extracts the attested identity; a seam so the rejection path is
	// testable without a second uid. Production wiring is always peerCred.
	creds  func(*net.UnixConn) (Peer, error)
	logger *slog.Logger
}

// peerConn carries the verified Peer with its connection so attribution flows
// to the handlers as data ([LAW:dataflow-not-control-flow]).
type peerConn struct {
	net.Conn
	peer Peer
}

// Accept returns the next connection whose peer passed authentication.
// Connections that cannot be attested, or whose uid differs from the daemon's,
// are closed and surfaced loudly — never admitted unattributed
// ([LAW:no-silent-failure]).
func (g *peerGate) Accept() (net.Conn, error) {
	for {
		conn, err := g.Listener.Accept()
		if err != nil {
			return nil, err
		}
		uc, ok := conn.(*net.UnixConn)
		if !ok {
			g.logger.Warn("control: rejected connection without peer credentials",
				"conn_type", fmt.Sprintf("%T", conn))
			_ = conn.Close()
			continue
		}
		peer, err := g.creds(uc)
		if err != nil {
			g.logger.Warn("control: rejected connection with unreadable peer credentials", "error", err)
			_ = conn.Close()
			continue
		}
		if peer.UID != g.ownUID {
			g.logger.Warn("control: rejected cross-user connection",
				"peer_uid", peer.UID, "peer_pid", peer.PID, "own_uid", g.ownUID)
			_ = conn.Close()
			continue
		}
		return &peerConn{Conn: conn, peer: peer}, nil
	}
}

type peerCtxKey struct{}

// peerContext lifts the connection's verified Peer into the request context;
// installed as the http.Server's ConnContext so every handler sees the identity
// the gate attested for its connection.
func peerContext(ctx context.Context, c net.Conn) context.Context {
	if pc, ok := c.(*peerConn); ok {
		return context.WithValue(ctx, peerCtxKey{}, pc.peer)
	}
	return ctx
}

// peerFrom recovers the verified Peer. Absence means the request arrived on a
// connection that never passed the gate — unreachable through this server's
// listener, so callers must treat it as an internal fault, never as an
// anonymous success.
func peerFrom(ctx context.Context) (Peer, bool) {
	p, ok := ctx.Value(peerCtxKey{}).(Peer)
	return p, ok
}
