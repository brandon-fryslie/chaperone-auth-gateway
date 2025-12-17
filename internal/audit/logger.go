// Package audit provides request logging and audit trail functionality.
package audit

import (
	"context"
	"time"
)

// RequestLog captures details about a proxied request for auditing.
type RequestLog struct {
	Timestamp     time.Time
	RequestID     string
	ClientID      string
	Service       string
	Host          string
	Method        string
	Path          string
	Status        int
	RequestBytes  int64
	ResponseBytes int64
	DurationMs    int64
	PolicyResult  PolicyResult
}

// PolicyResult captures policy enforcement outcomes.
type PolicyResult struct {
	RateLimited bool
	Denied      bool
}

// AuditLogger records request audit entries.
// Implementations must be safe for concurrent use.
type AuditLogger interface {
	// LogRequest records a completed request for auditing.
	LogRequest(ctx context.Context, entry *RequestLog) error
}
