package control

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"log/slog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// shortSock returns a socket path under the platform sun_path limit (104 bytes
// on macOS) — t.TempDir() paths are too long.
func shortSock(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "cp")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "p.sock")
}

// TestListenUnixOwnerOnlyAtBind forces the most permissive umask the daemon
// could inherit and proves the socket file is 0600 the moment the bind creates
// it. This stats BEFORE any later narrowing could run, so a bind-then-chmod
// implementation fails deterministically — there is no race to win.
func TestListenUnixOwnerOnlyAtBind(t *testing.T) {
	oldMask := syscall.Umask(0)
	defer syscall.Umask(oldMask)

	path := shortSock(t)
	ln, err := listenUnix(path)
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()

	fi, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm(),
		"socket must be owner-only from its first observable instant")

	// The bracket must restore the umask it found, not leak the tightened one.
	assert.Equal(t, 0, syscall.Umask(0), "listenUnix must restore the process umask")
}

// TestListenUnixReclaimsStaleSocketOwnerOnly covers the second bind inside
// listenUnix (the stale-residue retry): the reclaimed socket must be owner-only
// at creation exactly like the first-attempt path.
func TestListenUnixReclaimsStaleSocketOwnerOnly(t *testing.T) {
	oldMask := syscall.Umask(0)
	defer syscall.Umask(oldMask)

	path := shortSock(t)
	stale, err := net.Listen("unix", path)
	require.NoError(t, err)
	ul, ok := stale.(*net.UnixListener)
	require.True(t, ok)
	ul.SetUnlinkOnClose(false)
	require.NoError(t, ul.Close())
	_, err = os.Stat(path)
	require.NoError(t, err, "precondition: a dead socket file is left behind")

	ln, err := listenUnix(path)
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()

	fi, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm())
}

// gateOver builds a peerGate on a fresh unix listener with an injected
// credential reader — the only seam that lets a test present a foreign uid
// without a second user account. Everything else (sockets, dials, closes) is real.
func gateOver(t *testing.T, creds func(*net.UnixConn) (Peer, error)) (*peerGate, string) {
	t.Helper()
	path := shortSock(t)
	ln, err := net.Listen("unix", path)
	require.NoError(t, err)
	gate := &peerGate{Listener: ln, ownUID: os.Geteuid(), creds: creds, logger: slog.Default()}
	t.Cleanup(func() { _ = gate.Close() })
	return gate, path
}

// expectRejected dials the gate and proves the connection is closed without
// ever being admitted: the client's first read sees EOF, and Accept returns
// only when the listener itself closes.
func expectRejected(t *testing.T, gate *peerGate, path string) {
	t.Helper()

	type acceptOutcome struct {
		conn net.Conn
		err  error
	}
	accepted := make(chan acceptOutcome, 1)
	go func() {
		c, err := gate.Accept()
		accepted <- acceptOutcome{conn: c, err: err}
	}()

	client, err := net.Dial("unix", path)
	require.NoError(t, err, "dial lands in the listen backlog regardless of the gate")
	defer func() { _ = client.Close() }()

	require.NoError(t, client.SetReadDeadline(time.Now().Add(3*time.Second)))
	buf := make([]byte, 1)
	_, readErr := client.Read(buf)
	assert.ErrorIs(t, readErr, io.EOF, "the gate must close a rejected connection")

	require.NoError(t, gate.Close())
	out := <-accepted
	assert.Error(t, out.err, "Accept must never admit the rejected connection")
	assert.Nil(t, out.conn)
}

func TestPeerGateRejectsCrossUserConnection(t *testing.T) {
	gate, path := gateOver(t, func(*net.UnixConn) (Peer, error) {
		return Peer{UID: os.Geteuid() + 1, PID: 4242}, nil
	})
	expectRejected(t, gate, path)
}

func TestPeerGateRejectsUnattestableConnection(t *testing.T) {
	gate, path := gateOver(t, func(*net.UnixConn) (Peer, error) {
		return Peer{}, fmt.Errorf("credentials unavailable")
	})
	expectRejected(t, gate, path)
}

// TestPeerGateAdmitsSameUserAndCarriesIdentity proves the admit path end to
// end: a same-uid peer is accepted, and the identity the gate attested is the
// one peerContext/peerFrom deliver to a handler.
func TestPeerGateAdmitsSameUserAndCarriesIdentity(t *testing.T) {
	want := Peer{UID: os.Geteuid(), PID: 777}
	gate, path := gateOver(t, func(*net.UnixConn) (Peer, error) { return want, nil })

	client, err := net.Dial("unix", path)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	conn, err := gate.Accept()
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	got, ok := peerFrom(peerContext(context.Background(), conn))
	require.True(t, ok, "an admitted connection must carry its attested identity")
	assert.Equal(t, want, got)
}
