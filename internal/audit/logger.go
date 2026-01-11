// Package audit provides request logging and audit trail functionality.
package audit

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"
)

// Event types
const (
	EventCredentialInjected  = "credential_injected"
	EventAuthFailure         = "auth_failure"
	EventPolicyDenied        = "policy_denied"
	EventRequestDropped      = "request_dropped"
	EventAuthHeaderStripped  = "auth_header_stripped"
	EventPlaceholderMismatch = "placeholder_mismatch"
)

// Entry represents a single audit log entry for security-relevant events.
// Fields align with FedRAMP AU-3 requirements: who, what, when, where, outcome.
type Entry struct {
	Timestamp    time.Time `json:"timestamp"`
	Event        string    `json:"event"`
	Service      string    `json:"service"`
	Host         string    `json:"host"`
	Path         string    `json:"path"`
	Method       string    `json:"method"`
	AuthStrategy string    `json:"auth_strategy"`
	RequestID    string    `json:"request_id"`

	// AU-3 compliance fields
	ClientIP     string `json:"client_ip"`             // WHO: source of request
	Outcome      string `json:"outcome"`               // OUTCOME: success|failure|blocked|pass_through
	StatusCode   int    `json:"status_code,omitempty"` // HTTP status when applicable
	ErrorMessage string `json:"error,omitempty"`       // Error details on failure
	Detail       string `json:"detail,omitempty"`      // Event-specific context
}

// Logger writes audit events to a configurable output.
type Logger struct {
	mu      sync.Mutex
	writer  io.Writer
	encoder *json.Encoder
	enabled bool
}

// Config for audit logging.
type Config struct {
	Enabled bool
	Path    string // File path or "stdout"
}

// NewLogger creates an audit logger.
func NewLogger(cfg Config) (*Logger, error) {
	if !cfg.Enabled {
		return &Logger{enabled: false}, nil
	}

	var writer io.Writer
	if cfg.Path == "" || cfg.Path == "stdout" {
		writer = os.Stdout
	} else {
		f, err := os.OpenFile(cfg.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return nil, err
		}
		writer = f
	}

	return &Logger{
		writer:  writer,
		encoder: json.NewEncoder(writer),
		enabled: true,
	}, nil
}

// Log writes an audit entry.
func (l *Logger) Log(entry Entry) error {
	if !l.enabled {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	entry.Timestamp = time.Now().UTC()
	return l.encoder.Encode(entry)
}

// Close closes the underlying writer if it's a file.
func (l *Logger) Close() error {
	if closer, ok := l.writer.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// Legacy types for compatibility (future use)

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
