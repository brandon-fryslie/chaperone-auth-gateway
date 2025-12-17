package log

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"strings"
	"time"
)

// Context key types for type safety
type contextKey string

const requestIDKey contextKey = "request_id"

var (
	defaultLogger *slog.Logger = slog.Default()
	// Default to Debug level - let the handler do the filtering unless explicitly set
	currentLevel slog.Level = slog.LevelDebug
)

// SetLogger configures the global logger used by all logging functions.
// Resets the current level to Debug to allow the handler to do the filtering.
func SetLogger(logger *slog.Logger) {
	if logger != nil {
		defaultLogger = logger
		// Reset level to Debug when setting a new logger
		// This allows the handler to control filtering unless SetLevel is explicitly called
		currentLevel = slog.LevelDebug
	}
}

// SetLevel sets the minimum log level.
func SetLevel(level slog.Level) {
	currentLevel = level
}

// GetLevel returns the current log level.
func GetLevel() slog.Level {
	return currentLevel
}

// WithRequestID generates a unique request ID and adds it to the context.
func WithRequestID(ctx context.Context) context.Context {
	id := generateRequestID()
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestID extracts the request ID from the context.
// Returns empty string if no request ID is set.
func RequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// FromContext returns a logger with context fields attached.
func FromContext(ctx context.Context) *slog.Logger {
	logger := defaultLogger
	if id := RequestID(ctx); id != "" {
		logger = logger.With("request_id", id)
	}
	return logger
}

// Info logs an info-level message with context fields.
func Info(ctx context.Context, msg string, args ...any) {
	if currentLevel > slog.LevelInfo {
		return
	}
	logger := FromContext(ctx)
	redactedArgs := redactSensitiveArgs(args)
	logger.Info(msg, redactedArgs...)
}

// Error logs an error-level message with the error included.
func Error(ctx context.Context, msg string, err error, args ...any) {
	logger := FromContext(ctx)
	allArgs := append([]any{"error", err.Error()}, args...)
	redactedArgs := redactSensitiveArgs(allArgs)
	logger.Error(msg, redactedArgs...)
}

// Debug logs a debug-level message with context fields.
func Debug(ctx context.Context, msg string, args ...any) {
	if currentLevel > slog.LevelDebug {
		return
	}
	logger := FromContext(ctx)
	redactedArgs := redactSensitiveArgs(args)
	logger.Debug(msg, redactedArgs...)
}

// LogDuration logs the duration of an operation.
func LogDuration(ctx context.Context, operation string, start time.Time) {
	duration := time.Since(start)
	Info(ctx, "operation completed",
		"operation", operation,
		"duration_ms", duration.Milliseconds())
}

// generateRequestID creates a unique request ID using crypto/rand.
func generateRequestID() string {
	// Generate 16 random bytes
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based ID if crypto/rand fails
		return "req-" + hex.EncodeToString([]byte(time.Now().String()))
	}
	return "req-" + hex.EncodeToString(bytes)
}

// RedactSensitive returns a redacted version of sensitive values.
// Use this before logging headers or secrets.
func RedactSensitive(value string) string {
	if value == "" {
		return ""
	}
	return "[REDACTED]"
}

// redactSensitiveArgs scans through variadic args and redacts sensitive fields.
// It looks for key-value pairs where the key indicates sensitive data.
func redactSensitiveArgs(args []any) []any {
	if len(args) == 0 {
		return args
	}

	// Process pairs of arguments (key, value, key, value, ...)
	result := make([]any, len(args))
	copy(result, args)

	for i := 0; i < len(result)-1; i += 2 {
		key, ok := result[i].(string)
		if !ok {
			continue
		}

		// Check if this key indicates sensitive data
		if isSensitiveKey(key) {
			// Redact the value (which is at i+1)
			if _, ok := result[i+1].(string); ok {
				result[i+1] = "[REDACTED]"
			}
		}
	}

	return result
}

// isSensitiveKey checks if a key name indicates sensitive data.
func isSensitiveKey(key string) bool {
	keyLower := strings.ToLower(key)
	sensitiveKeys := []string{
		"authorization",
		"secret",
		"password",
		"token",
		"api_key",
		"apikey",
		"credential",
	}

	for _, sensitiveKey := range sensitiveKeys {
		if strings.Contains(keyLower, sensitiveKey) {
			return true
		}
	}

	return false
}
