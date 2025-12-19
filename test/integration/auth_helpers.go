package integration

import (
	"github.com/bmf/chaperone/internal/auth"
	"github.com/bmf/chaperone/internal/secrets"
)

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
