package secrets

import (
	"context"
	"fmt"
	"os"
)

// FileProvider fetches secrets from files.
// It is safe for concurrent use.
type FileProvider struct{}

// NewFileProvider creates a new file-based secret provider.
func NewFileProvider() *FileProvider {
	return &FileProvider{}
}

// Fetch retrieves a secret from a file.
//
// The path parameter is the file path:
//   - "/path/to/secret.txt" → reads file contents
//   - "./config/api-key" → reads file contents
//
// Returns:
//   - ErrSecretNotFound if the file doesn't exist
//   - Error if path is empty
//   - Error if file cannot be read
//
// Trims whitespace from the file contents.
// Empty files are treated as not found.
//
// Respects context cancellation.
func (p *FileProvider) Fetch(ctx context.Context, path string) (string, error) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	if path == "" {
		return "", fmt.Errorf("empty file path")
	}

	// Read file
	// Note: path is from user configuration, not direct user input
	content, err := os.ReadFile(path) //nolint:gosec // path from config, not direct user input
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrSecretNotFound
		}
		return "", fmt.Errorf("failed to read secret file: %w", err)
	}

	// Trim whitespace and check if empty
	secret := string(content)
	if len(secret) == 0 {
		return "", ErrSecretNotFound
	}

	return secret, nil
}
