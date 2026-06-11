//go:build linux

package control

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// peerCred reads the connecting process's identity from the kernel via
// SO_PEERCRED. It is attested at connect time and cannot be forged by the peer.
func peerCred(conn *net.UnixConn) (Peer, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return Peer{}, fmt.Errorf("control: raw conn: %w", err)
	}
	var (
		peer    Peer
		credErr error
	)
	if err := raw.Control(func(fd uintptr) {
		cred, err := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		if err != nil {
			credErr = fmt.Errorf("control: SO_PEERCRED: %w", err)
			return
		}
		peer = Peer{UID: int(cred.Uid), PID: int(cred.Pid)}
	}); err != nil {
		return Peer{}, fmt.Errorf("control: read peer credentials: %w", err)
	}
	if credErr != nil {
		return Peer{}, credErr
	}
	return peer, nil
}
