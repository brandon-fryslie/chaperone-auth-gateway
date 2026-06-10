package proxy

import (
	"crypto/tls"
	"crypto/x509"
	"testing"

	"github.com/elazarl/goproxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfigureTransportOwnsOutboundTrust proves the transport seam replaces
// goproxy's insecure default (InsecureSkipVerify=true) with the owned policy:
// verification on, TLS 1.2 floor, trust anchored to system roots or the
// caller's pinned pool. The wire-level proof that forged upstream certs are
// refused lives in test/integration/upstream_tls_verify_integration_test.go.
func TestConfigureTransportOwnsOutboundTrust(t *testing.T) {
	t.Run("default trust is system roots with verification on", func(t *testing.T) {
		p := goproxy.NewProxyHttpServer()
		configureTransport(p, nil)

		cfg := p.Tr.TLSClientConfig
		require.NotNil(t, cfg, "outbound transport must carry an owned TLS config")
		assert.False(t, cfg.InsecureSkipVerify, "upstream certificate verification must be on")
		assert.Nil(t, cfg.RootCAs, "nil RootCAs means the system root store")
		assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion, "outbound TLS floor must be 1.2")
		assert.False(t, p.AllowHTTP2, "goproxy's h2 outbound leg bypasses the trust policy and must stay off")
	})

	t.Run("pinned pool replaces system roots, verification stays on", func(t *testing.T) {
		pool := x509.NewCertPool()
		p := goproxy.NewProxyHttpServer()
		configureTransport(p, pool)

		cfg := p.Tr.TLSClientConfig
		require.NotNil(t, cfg)
		assert.False(t, cfg.InsecureSkipVerify, "pinning must not disable verification")
		assert.Same(t, pool, cfg.RootCAs, "pinned pool must be the trust anchor set")
	})
}
