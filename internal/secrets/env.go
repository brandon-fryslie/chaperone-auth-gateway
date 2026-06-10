package secrets

import (
	"context"
	"fmt"
	"os"
)

// EnvProvider fetches secrets from environment variables.
// It is safe for concurrent use (os.Getenv is concurrent-safe).
type EnvProvider struct{}

// NewEnvProvider creates a new environment variable provider.
func NewEnvProvider() *EnvProvider {
	return &EnvProvider{}
}

// Fetch retrieves a secret from an environment variable.
//
// The path parameter is the name of the environment variable:
//   - "MY_API_KEY" → reads the MY_API_KEY environment variable
//   - "SECRET_TOKEN" → reads the SECRET_TOKEN environment variable
//
// Returns:
//   - ErrSecretNotFound if the environment variable is not set
//   - Error if path is empty
//
// Returns the variable's value verbatim; Registry.Fetch owns whitespace
// normalization and empty-value rejection for every provider.
//
// Respects context cancellation.
func (p *EnvProvider) Fetch(ctx context.Context, path string) (string, error) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	if path == "" {
		return "", fmt.Errorf("empty environment variable name")
	}

	value, ok := os.LookupEnv(path)
	if !ok {
		return "", ErrSecretNotFound
	}

	return value, nil
}
