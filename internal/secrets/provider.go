package secrets

import "context"

// SecretProvider fetches secrets from a source.
// Implementations include EnvProvider, FileProvider, KeychainProvider.
type SecretProvider interface {
	// Fetch retrieves a secret value for the given path.
	// The path format is provider-specific.
	Fetch(ctx context.Context, path string) (string, error)
}
