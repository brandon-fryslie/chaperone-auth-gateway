package integration

import (
	"context"
	"crypto/x509"
	"log/slog"
	"net"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/bmf/chaperone/internal/auth"
	"github.com/bmf/chaperone/internal/config"
	"github.com/bmf/chaperone/internal/mitm"
	"github.com/bmf/chaperone/internal/proxy"
	"github.com/bmf/chaperone/internal/secrets"
	"github.com/bmf/chaperone/internal/service"
	"github.com/bmf/chaperone/internal/shutdown"
	"github.com/stretchr/testify/require"
)

// testProxySecret is the per-suite proxy access secret. The injecting proxy
// cannot be constructed without one, so every integration test authenticates
// at the proxy boundary exactly as a real client must.
const testProxySecret = "integration-test-proxy-secret"

// newGatedMITMProxy constructs the injecting proxy with the suite's proxy
// access secret. Construction failure is fatal — there is no ungated variant
// to fall back to.
func newGatedMITMProxy(t *testing.T, cfg *config.Config, logger *slog.Logger, shutdownMgr *shutdown.Manager, registry service.ServiceRegistry, certCache *mitm.CertCache, opts *proxy.MITMOptions) *proxy.Server {
	t.Helper()
	s, err := proxy.NewWithMITM(cfg, logger, shutdownMgr, registry, certCache, testProxySecret, opts)
	require.NoError(t, err)
	return s
}

// gatedProxyURL returns the started server's credentialed proxy URL (the
// secret rides in the userinfo, which Go's transport turns into
// Proxy-Authorization) plus a nil dialer, mirroring proxy.GetProxyURL's shape.
func gatedProxyURL(t *testing.T, s *proxy.Server) (*url.URL, func(ctx context.Context, network, addr string) (net.Conn, error)) {
	t.Helper()
	u, err := url.Parse(s.ProxyURL())
	require.NoError(t, err)
	return u, nil
}

// trustUpstreams builds the proxy's outbound trust pool (MITMOptions.UpstreamCAs)
// pinned to the given test upstreams' self-signed certificates. The proxy
// verifies upstream certs on MITM'd connections, so every test upstream must be
// explicitly anchored — exactly as a production deployment trusts real CAs.
func trustUpstreams(servers ...*httptest.Server) *x509.CertPool {
	pool := x509.NewCertPool()
	for _, s := range servers {
		pool.AddCert(s.Certificate())
	}
	return pool
}

// setupAuthRegistries creates and configures secret and auth strategy registries
// for integration testing. This sets up the standard providers (env, file, keychain) and
// standard strategies (bearer, header).
func setupAuthRegistries() (*secrets.Registry, *auth.Registry) {
	// Create secret registry and register providers
	secretRegistry := secrets.NewRegistry()
	secretRegistry.Register("env", secrets.NewEnvProvider())
	secretRegistry.Register("file", secrets.NewFileProvider())
	secretRegistry.Register("keychain", secrets.NewKeychainProvider())

	// Create auth registry and register strategies
	authRegistry := auth.NewRegistry()
	authRegistry.Register("bearer", &auth.BearerStrategy{})
	authRegistry.Register("header", &auth.HeaderStrategy{HeaderName: "X-API-Key"})

	return secretRegistry, authRegistry
}
