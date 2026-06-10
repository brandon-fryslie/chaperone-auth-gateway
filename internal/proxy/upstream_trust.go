// Package proxy provides HTTP/HTTPS proxy handlers for MITM and credential injection.
// This file owns the outbound-trust policy: how the proxy decides whether the
// upstream server it dials is the server it claims to be.
package proxy

import (
	"crypto/tls"
	"crypto/x509"
)

// upstreamTLSConfig is the single outbound-trust policy for every MITM'd
// upstream connection. [LAW:single-enforcer]
//
// Why this exists: goproxy's default transport ships with
// InsecureSkipVerify=true, so without this seam the proxy would complete a
// handshake with ANY server presenting ANY certificate — and then inject the
// real credential into it. An on-path attacker between the proxy and the
// internet could therefore harvest credentials while the client sees nothing
// wrong (the proxy still presents a cert the client trusts).
//
// rootCAs == nil means the system root store; non-nil means ONLY those roots
// are trusted (pinning). There is deliberately no way to express "do not
// verify" — that state is unrepresentable. [LAW:types-are-the-program]
func upstreamTLSConfig(rootCAs *x509.CertPool) *tls.Config {
	return &tls.Config{
		RootCAs:    rootCAs,
		MinVersion: tls.VersionTLS12,
	}
}
