package secrets

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/bmf/chaperone/internal/filetrust"
)

// secretFileTrust is the trust bar a secret file must meet before its
// contents are used. Unlike a config file, a secret file IS the credential
// value, so any group/world access — read or write — is a refusal.
var secretFileTrust = filetrust.File{
	Desc:   "secret file",
	Stakes: "any other local user could read the credential value or replace it",
	Bar:    filetrust.OwnerOnly,
}

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
// The file must be a regular file owned by the running user with no
// group/world permission bits; a symlinked path is judged by the target
// actually read. An untrusted file is a loud error, never a silent read.
//
// Returns:
//   - ErrSecretNotFound if the file doesn't exist
//   - Error if path is empty
//   - Error if the file is untrusted or cannot be read
//
// Returns the file contents verbatim; Registry.Fetch owns whitespace
// normalization and empty-value rejection for every provider.
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

	content, err := secretFileTrust.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", ErrSecretNotFound
		}
		return "", err
	}

	return string(content), nil
}
