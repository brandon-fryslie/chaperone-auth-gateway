// Package audit provides request logging and audit trail functionality.
package audit

import (
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"
)

// Event types
const (
	//nolint:gosec // G101: audit event-name constant, not a credential
	EventCredentialInjected  = "credential_injected"
	EventAuthFailure         = "auth_failure"
	EventPolicyDenied        = "policy_denied"
	EventRequestDropped      = "request_dropped"
	EventAuthHeaderStripped  = "auth_header_stripped"
	EventPlaceholderMismatch = "placeholder_mismatch"
)

// AuditLogger is the interface for audit logging.
// Implementations must be safe for concurrent use.
type AuditLogger interface {
	// Log writes an audit entry. Returns nil for disabled loggers.
	Log(entry Entry) error
	// Close releases any resources held by the logger.
	Close() error
}

// Ensure Logger implements AuditLogger
var _ AuditLogger = (*Logger)(nil)

// noopLogger is a no-op implementation of AuditLogger.
type noopLogger struct{}

func (n *noopLogger) Log(Entry) error { return nil }
func (n *noopLogger) Close() error    { return nil }

// Noop returns a no-op audit logger that discards all entries.
// Use this instead of nil to avoid nil checks in handler code.
func Noop() AuditLogger { return &noopLogger{} }

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
