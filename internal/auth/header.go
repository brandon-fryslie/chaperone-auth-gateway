package auth

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/bmf/chaperone/internal/errors"
)

// HeaderStrategy implements custom header authentication.
// It adds a custom header with the secret as the value.
type HeaderStrategy struct {
	// HeaderName is the name of the header to set (e.g., "X-API-Key")
	HeaderName string
}

// Apply injects custom header authentication into the request.
// The request is modified in-place to include the configured header.
func (s *HeaderStrategy) Apply(ctx context.Context, req *http.Request, secret string) error {
	if secret == "" {
		return errors.ErrSecretNotFound
	}

	if s.HeaderName == "" {
		return fmt.Errorf("header name not configured")
	}

	// Set custom header with secret value
	// Use Set() to replace any existing header
	req.Header.Set(s.HeaderName, secret)

	// Log injection without revealing the secret
	slog.DebugContext(ctx, "injected custom header authentication", "header", s.HeaderName)

	return nil
}
