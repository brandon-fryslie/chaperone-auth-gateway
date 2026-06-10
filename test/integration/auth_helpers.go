package integration

import (
	"crypto/x509"
	"net/http/httptest"

	"github.com/bmf/chaperone/internal/auth"
	"github.com/bmf/chaperone/internal/secrets"
)

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
