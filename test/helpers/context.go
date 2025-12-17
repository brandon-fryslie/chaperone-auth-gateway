package helpers

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// TestContext creates a context with a unique request ID for testing.
// The request_id is stored with a string key for test interoperability.
func TestContext() context.Context {
	// Generate unique request ID
	requestID := uuid.New().String()

	// Create context with request_id value
	// Note: Using string key for test interoperability, despite staticcheck SA1029
	ctx := context.WithValue(context.Background(), "request_id", requestID) //nolint:staticcheck // String key used for test interoperability
	return ctx
}

// TestContextWithTimeout creates a context with timeout and request ID for testing.
func TestContextWithTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	// Create base context with request ID
	ctx := TestContext()

	// Add timeout
	return context.WithTimeout(ctx, d)
}
