package log

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"
)

// Context key types for type safety
type contextKey string

const (
	requestIDKey contextKey = "request_id"
	colorRKey    contextKey = "color_r"
	colorGKey    contextKey = "color_g"
	colorBKey    contextKey = "color_b"
)

var (
	defaultLogger *slog.Logger = slog.Default()
	// Default to Debug level - let the handler do the filtering unless explicitly set
	currentLevel slog.Level = slog.LevelDebug
)

// colorPalette holds 20 visually distinct colors for request/response correlation.
// Colors are ordered to maximize perceptual difference between adjacent indices.
var colorPalette = [][3]int{
	{255, 85, 85},   // 0: bright red
	{85, 255, 255},  // 1: cyan (opposite)
	{255, 200, 85},  // 2: gold
	{85, 85, 255},   // 3: blue (opposite)
	{85, 255, 85},   // 4: green
	{255, 85, 255},  // 5: magenta (opposite)
	{255, 150, 85},  // 6: orange
	{85, 200, 255},  // 7: sky (opposite)
	{200, 255, 85},  // 8: lime
	{200, 85, 255},  // 9: purple (opposite)
	{85, 255, 150},  // 10: mint
	{255, 85, 150},  // 11: pink (opposite)
	{255, 255, 85},  // 12: yellow
	{85, 150, 255},  // 13: cornflower (opposite)
	{150, 255, 85},  // 14: chartreuse
	{255, 85, 200},  // 15: hot pink (opposite)
	{85, 255, 200},  // 16: aqua
	{255, 150, 150}, // 17: salmon (opposite)
	{150, 200, 255}, // 18: light blue
	{255, 200, 150}, // 19: peach (opposite)
}

var colorIndex atomic.Uint64

// NextCorrelationColor returns the next color from the palette for request/response correlation.
// Rotates through the palette to ensure adjacent connections have different colors.
func NextCorrelationColor() (r, g, b int) {
	// Reduce modulo the palette length in uint64 space and index directly —
	// a uint64 is a valid slice index, so no narrowing conversion is needed.
	idx := colorIndex.Add(1) % uint64(len(colorPalette))
	c := colorPalette[idx]
	return c[0], c[1], c[2]
}

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

// WithRequestIDValue sets a specific request ID in the context.
// Use this when you need to propagate an existing request ID.
func WithRequestIDValue(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// WithCorrelationColor sets the correlation color in the context.
func WithCorrelationColor(ctx context.Context, r, g, b int) context.Context {
	ctx = context.WithValue(ctx, colorRKey, r)
	ctx = context.WithValue(ctx, colorGKey, g)
	ctx = context.WithValue(ctx, colorBKey, b)
	return ctx
}

// CorrelationColor retrieves the correlation color from the context.
// Returns (0, 0, 0) if not set.
func CorrelationColor(ctx context.Context) (r, g, b int) {
	if rv, ok := ctx.Value(colorRKey).(int); ok {
		r = rv
	}
	if gv, ok := ctx.Value(colorGKey).(int); ok {
		g = gv
	}
	if bv, ok := ctx.Value(colorBKey).(int); ok {
		b = bv
	}
	return r, g, b
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
	// Add correlation color if set
	if r, g, b := CorrelationColor(ctx); r != 0 || g != 0 || b != 0 {
		logger = logger.With("color_r", r, "color_g", g, "color_b", b)
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

// Warn logs a warn-level message with context fields.
func Warn(ctx context.Context, msg string, args ...any) {
	if currentLevel > slog.LevelWarn {
		return
	}
	logger := FromContext(ctx)
	redactedArgs := redactSensitiveArgs(args)
	logger.Warn(msg, redactedArgs...)
}

// Detail logs a formatted detail line with request correlation tag.
// Useful for logging auxiliary information that should appear on its own line
// with the same request tag as the originating request.
// Args should include "detail_text" (required) and optionally "detail_color" (green/yellow/blue).
// Example: log.Detail(ctx, "detail", "detail_text", "Found standard auth header: Authorization=Bearer s...IwAA", "detail_color", "green")
func Detail(ctx context.Context, args ...any) {
	if currentLevel > slog.LevelInfo {
		return
	}
	logger := FromContext(ctx)
	redactedArgs := redactSensitiveArgs(args)
	logger.Info("detail", redactedArgs...)
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
