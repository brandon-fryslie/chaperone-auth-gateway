//go:build !darwin && !linux

package control

import (
	"fmt"
	"net"
	"runtime"
)

// peerCred has no kernel attestation on this platform, so the gate fails
// closed: every connection is rejected rather than admitted unattributed
// ([LAW:no-silent-failure]).
func peerCred(*net.UnixConn) (Peer, error) {
	return Peer{}, fmt.Errorf("control: peer credentials unsupported on %s", runtime.GOOS)
}
