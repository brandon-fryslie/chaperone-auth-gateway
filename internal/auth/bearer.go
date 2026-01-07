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
// If an Authorization header already exists with different capitalization,
// it will be replaced using the existing capitalization and a warning will be logged.
func (s *BearerStrategy) Apply(ctx context.Context, req *http.Request, secret string) error {
	if secret == "" {
		return errors.ErrSecretNotFound
	}

	// Set Authorization header with Bearer token
	// This preserves existing capitalization if present (e.g., "authorization", "AUTHORIZATION")
	replaced := setHeaderPreservingCapitalization(ctx, req, "Authorization", "Bearer "+secret)

	// Log injection without revealing the secret
	if replaced {
		slog.DebugContext(ctx, "injected bearer token authentication (replaced existing header)")
	} else {
		slog.DebugContext(ctx, "injected bearer token authentication")
	}

	return nil
}
