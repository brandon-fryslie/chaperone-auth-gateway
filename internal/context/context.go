package context

import (
	"context"
)

// Define type-safe context keys
type contextKey string

const (
	requestIDKey contextKey = "request_id"
	serviceKey   contextKey = "service"
	hostnameKey  contextKey = "hostname"
	clientIDKey  contextKey = "client_id"
)

// NewRequestContext creates a new cancellable context for an incoming request.
// Use this at the start of each request handler.
func NewRequestContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

// WithRequestID attaches a request ID to the context.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestID extracts the request ID from the context.
// Returns empty string if not set.
func RequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// WithService attaches the service name to the context.
func WithService(ctx context.Context, service string) context.Context {
	return context.WithValue(ctx, serviceKey, service)
}

// Service extracts the service name from the context.
// Returns empty string if not set.
func Service(ctx context.Context) string {
	if svc, ok := ctx.Value(serviceKey).(string); ok {
		return svc
	}
	return ""
}

// WithHostname attaches the target hostname to the context.
func WithHostname(ctx context.Context, hostname string) context.Context {
	return context.WithValue(ctx, hostnameKey, hostname)
}

// Hostname extracts the target hostname from the context.
// Returns empty string if not set.
func Hostname(ctx context.Context) string {
	if host, ok := ctx.Value(hostnameKey).(string); ok {
		return host
	}
	return ""
}

// WithClientID attaches the client identifier to the context.
func WithClientID(ctx context.Context, clientID string) context.Context {
	return context.WithValue(ctx, clientIDKey, clientID)
}

// ClientID extracts the client identifier from the context.
// Returns empty string if not set.
func ClientID(ctx context.Context) string {
	if cid, ok := ctx.Value(clientIDKey).(string); ok {
		return cid
	}
	return ""
}
