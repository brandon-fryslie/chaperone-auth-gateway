// Package auth provides authentication strategies for API requests.
package auth

import (
	"context"
	"net/http"
)

// AuthStrategy applies authentication to outbound requests.
// Implementations must be safe for concurrent use.
type AuthStrategy interface {
	// Apply modifies the request to include authentication.
	// The secret parameter contains the credential to use.
	Apply(ctx context.Context, req *http.Request, secret string) error
}
