package auth

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/bmf/chaperone/internal/errors"
)

// BearerStrategy implements bearer token authentication.
// It adds an "Authorization: Bearer <secret>" header to the request.
type BearerStrategy struct{}

// Apply injects bearer token authentication into the request.
// The request is modified in-place to include the Authorization header.
func (s *BearerStrategy) Apply(ctx context.Context, req *http.Request, secret string) error {
	if secret == "" {
		return errors.ErrSecretNotFound
	}

	// Set Authorization header with Bearer token
	// Use Set() to replace any existing Authorization header
	req.Header.Set("Authorization", "Bearer "+secret)

	// Log injection without revealing the secret
	slog.DebugContext(ctx, "injected bearer token authentication")

	return nil
}
