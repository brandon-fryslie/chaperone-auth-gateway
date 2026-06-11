//go:build darwin

package control

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// peerCred reads the connecting process's identity from the kernel via
// LOCAL_PEERCRED (uid) and LOCAL_PEERPID (pid). These are attested at
// connect time and cannot be forged by the peer.
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
		cred, err := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		if err != nil {
			credErr = fmt.Errorf("control: LOCAL_PEERCRED: %w", err)
			return
		}
		pid, err := unix.GetsockoptInt(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERPID)
		if err != nil {
			credErr = fmt.Errorf("control: LOCAL_PEERPID: %w", err)
			return
		}
		peer = Peer{UID: int(cred.Uid), PID: pid}
	}); err != nil {
		return Peer{}, fmt.Errorf("control: read peer credentials: %w", err)
	}
	if credErr != nil {
		return Peer{}, credErr
	}
	return peer, nil
}
