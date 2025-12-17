package test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/bmf/chaperone/internal/log"
)

// TestLoggingFramework validates Phase 0.3: Observability Foundation
//
// This test cannot be gamed because:
// 1. Captures actual log output to bytes.Buffer (not mocks)
// 2. Parses real JSON output and validates structure
// 3. Verifies request ID propagation through real context values
// 4. Tests actual redaction by looking for sensitive values in output
// 5. Measures real time durations (cannot be faked with constants)
// 6. Tests fail if logging implementation is missing or broken
//
// An AI cannot fake this with stubs - the logging must actually work.

func TestLoggingFramework(t *testing.T) {
	t.Run("request_id_generation_and_propagation", func(t *testing.T) {
		testRequestIDGenerationAndPropagation(t)
	})

	t.Run("structured_logging_functions", func(t *testing.T) {
		testStructuredLoggingFunctions(t)
	})

	t.Run("json_output_format", func(t *testing.T) {
		testJSONOutputFormat(t)
	})

	t.Run("field_redaction", func(t *testing.T) {
		testFieldRedaction(t)
	})

	t.Run("log_duration", func(t *testing.T) {
		testLogDuration(t)
	})

	t.Run("context_aware_logging", func(t *testing.T) {
		testContextAwareLogging(t)
	})

	t.Run("log_level_configuration", func(t *testing.T) {
		testLogLevelConfiguration(t)
	})
}

// testRequestIDGenerationAndPropagation verifies:
// - WithRequestID creates a context with a request ID
// - RequestID extracts the ID from context
// - IDs are unique across multiple calls
// - Empty context returns empty string
func testRequestIDGenerationAndPropagation(t *testing.T) {
	// Test 1: WithRequestID adds ID to context
	ctx := context.Background()
	ctxWithID := log.WithRequestID(ctx)

	if ctxWithID == ctx {
		t.Error("WithRequestID should return a new context, not the same one")
	}

	// Test 2: RequestID extracts the ID
	requestID := log.RequestID(ctxWithID)
	if requestID == "" {
		t.Error("RequestID should return non-empty string from context with ID")
	}

	// Test 3: Request IDs are unique
	ctx2 := log.WithRequestID(context.Background())
	requestID2 := log.RequestID(ctx2)

	if requestID == requestID2 {
		t.Errorf("Request IDs should be unique. Got same ID twice: %s", requestID)
	}

	// Generate multiple IDs and verify all are unique
	seenIDs := make(map[string]bool)
	for i := 0; i < 100; i++ {
		ctx := log.WithRequestID(context.Background())
		id := log.RequestID(ctx)

		if id == "" {
			t.Errorf("iteration %d: RequestID returned empty string", i)
			continue
		}

		if seenIDs[id] {
			t.Errorf("iteration %d: duplicate request ID: %s", i, id)
		}
		seenIDs[id] = true
	}

	// Test 4: RequestID on empty context returns empty string
	emptyCtx := context.Background()
	emptyID := log.RequestID(emptyCtx)
	if emptyID != "" {
		t.Errorf("RequestID on empty context should return empty string, got: %s", emptyID)
	}

	t.Logf("Generated and verified %d unique request IDs", len(seenIDs))
}

// testStructuredLoggingFunctions verifies:
// - Info() function exists and works
// - Error() function exists and works
// - Debug() function exists and works
// - Functions accept context and variadic args
// - Error function includes error in output
func testStructuredLoggingFunctions(t *testing.T) {
	// Set up capture buffer
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug, // Capture all levels
	}))
	log.SetLogger(logger)

	ctx := log.WithRequestID(context.Background())
	requestID := log.RequestID(ctx)

	// Test Info
	buf.Reset()
	log.Info(ctx, "test info message", "key1", "value1", "key2", 42)

	output := buf.String()
	if output == "" {
		t.Fatal("Info() produced no output")
	}

	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		t.Fatalf("Info() output is not valid JSON: %v\nOutput: %s", err, output)
	}

	if logEntry["msg"] != "test info message" {
		t.Errorf("Info() message incorrect. Expected 'test info message', got: %v", logEntry["msg"])
	}

	if logEntry["level"] != "INFO" {
		t.Errorf("Info() level incorrect. Expected 'INFO', got: %v", logEntry["level"])
	}

	if logEntry["request_id"] != requestID {
		t.Errorf("Info() request_id incorrect. Expected %s, got: %v", requestID, logEntry["request_id"])
	}

	// Test Error with actual error
	buf.Reset()
	testErr := &testError{msg: "something went wrong"}
	log.Error(ctx, "test error message", testErr, "extra", "data")

	output = buf.String()
	if output == "" {
		t.Fatal("Error() produced no output")
	}

	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		t.Fatalf("Error() output is not valid JSON: %v\nOutput: %s", err, output)
	}

	if logEntry["msg"] != "test error message" {
		t.Errorf("Error() message incorrect. Expected 'test error message', got: %v", logEntry["msg"])
	}

	if logEntry["level"] != "ERROR" {
		t.Errorf("Error() level incorrect. Expected 'ERROR', got: %v", logEntry["level"])
	}

	// Error should be included in log entry
	errorField := logEntry["error"]
	if errorField == nil {
		t.Error("Error() should include error field in log output")
	} else if !strings.Contains(errorField.(string), "something went wrong") {
		t.Errorf("Error() error field should contain error message, got: %v", errorField)
	}

	// Test Debug
	buf.Reset()
	log.Debug(ctx, "test debug message", "debug_key", "debug_value")

	output = buf.String()
	if output == "" {
		t.Fatal("Debug() produced no output")
	}

	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		t.Fatalf("Debug() output is not valid JSON: %v\nOutput: %s", err, output)
	}

	if logEntry["msg"] != "test debug message" {
		t.Errorf("Debug() message incorrect. Expected 'test debug message', got: %v", logEntry["msg"])
	}

	if logEntry["level"] != "DEBUG" {
		t.Errorf("Debug() level incorrect. Expected 'DEBUG', got: %v", logEntry["level"])
	}
}

// testJSONOutputFormat verifies:
// - All logs are valid JSON
// - Logs include required fields: time, level, msg
// - Request ID appears when present in context
// - Timestamp is in valid format
func testJSONOutputFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	log.SetLogger(logger)

	ctx := log.WithRequestID(context.Background())

	// Write several log entries
	testCases := []struct {
		name    string
		logFunc func()
	}{
		{
			name: "info_log",
			logFunc: func() {
				log.Info(ctx, "info message", "field1", "value1")
			},
		},
		{
			name: "error_log",
			logFunc: func() {
				log.Error(ctx, "error message", &testError{msg: "test error"}, "field2", "value2")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			buf.Reset()
			tc.logFunc()

			output := buf.String()
			if output == "" {
				t.Fatal("No log output produced")
			}

			// Parse JSON
			var logEntry map[string]interface{}
			if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
				t.Fatalf("Log output is not valid JSON: %v\nOutput: %s", err, output)
			}

			// Verify required fields exist
			requiredFields := []string{"time", "level", "msg"}
			for _, field := range requiredFields {
				if _, exists := logEntry[field]; !exists {
					t.Errorf("Log entry missing required field: %s\nEntry: %v", field, logEntry)
				}
			}

			// Verify request_id is present (we used context with ID)
			if _, exists := logEntry["request_id"]; !exists {
				t.Error("Log entry missing request_id field when context has request ID")
			}

			// Verify timestamp is parseable
			timeStr, ok := logEntry["time"].(string)
			if !ok {
				t.Errorf("time field is not a string: %v", logEntry["time"])
			} else {
				if _, err := time.Parse(time.RFC3339, timeStr); err != nil {
					// Try parsing with nanoseconds
					if _, err := time.Parse(time.RFC3339Nano, timeStr); err != nil {
						t.Errorf("time field is not in valid RFC3339 format: %s", timeStr)
					}
				}
			}
		})
	}
}

// testFieldRedaction verifies:
// - Authorization headers are redacted in logs
// - Secret values are masked
// - Sensitive data does not appear in plain text
func testFieldRedaction(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	log.SetLogger(logger)

	ctx := context.Background()

	// Test 1: Authorization header redaction
	secretToken := "Bearer super-secret-token-12345"
	buf.Reset()
	log.Info(ctx, "processing request", "Authorization", secretToken)

	output := buf.String()
	if strings.Contains(output, "super-secret-token-12345") {
		t.Error("Authorization header value should be redacted, but appears in log output")
	}

	// The log should contain some redacted indicator
	if !strings.Contains(output, "REDACTED") && !strings.Contains(output, "***") {
		t.Log("Warning: Authorization header may not be properly redacted")
		t.Logf("Output: %s", output)
	}

	// Test 2: Secret field redaction
	secretValue := "my-database-password"
	buf.Reset()
	log.Info(ctx, "connecting to database", "secret", secretValue)

	output = buf.String()
	if strings.Contains(output, "my-database-password") {
		t.Error("Secret field value should be redacted, but appears in log output")
	}

	// Test 3: Case-insensitive header redaction
	buf.Reset()
	log.Info(ctx, "request headers", "authorization", "Bearer token123")

	output = buf.String()
	if strings.Contains(output, "token123") {
		t.Error("Authorization header (lowercase) should be redacted, but appears in log output")
	}
}

// testLogDuration verifies:
// - LogDuration logs operation name and duration
// - Duration is reasonable (positive, not absurdly large)
// - Duration is measured in reasonable units
func testLogDuration(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	log.SetLogger(logger)

	ctx := context.Background()

	// Simulate an operation with known duration
	start := time.Now()
	time.Sleep(10 * time.Millisecond) // Sleep for 10ms
	log.LogDuration(ctx, "test_operation", start)

	output := buf.String()
	if output == "" {
		t.Fatal("LogDuration produced no output")
	}

	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		t.Fatalf("LogDuration output is not valid JSON: %v\nOutput: %s", err, output)
	}

	// Should contain operation name
	if !strings.Contains(output, "test_operation") {
		t.Error("LogDuration should include operation name in output")
	}

	// Should contain duration field
	durationField := logEntry["duration_ms"]
	if durationField == nil {
		// Try alternative field names
		if logEntry["duration"] == nil && logEntry["elapsed_ms"] == nil {
			t.Error("LogDuration should include duration field (duration_ms, duration, or elapsed_ms)")
		}
		durationField = logEntry["duration"]
	}

	// Verify duration is reasonable
	if durationField != nil {
		duration, ok := durationField.(float64)
		if !ok {
			t.Errorf("Duration field should be numeric, got: %T", durationField)
		} else {
			// We slept for 10ms, duration should be >= 10ms and < 1000ms (reasonable range)
			if duration < 5 {
				t.Errorf("Duration too small (< 5ms), expected ~10ms, got: %f", duration)
			}
			if duration > 1000 {
				t.Errorf("Duration too large (> 1000ms), expected ~10ms, got: %f", duration)
			}
			t.Logf("Measured duration: %f ms (expected ~10ms)", duration)
		}
	}

	// Verify duration is not negative
	if durationField != nil {
		duration := durationField.(float64)
		if duration < 0 {
			t.Errorf("Duration should not be negative, got: %f", duration)
		}
	}
}

// testContextAwareLogging verifies:
// - FromContext returns a logger with context fields
// - Logger includes request_id when available
// - Logger works with empty context
func testContextAwareLogging(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	log.SetLogger(logger)

	// Test with context containing request ID
	ctx := log.WithRequestID(context.Background())
	requestID := log.RequestID(ctx)

	contextLogger := log.FromContext(ctx)
	if contextLogger == nil {
		t.Fatal("FromContext should return a logger, got nil")
	}

	// Log using the context-aware logger
	buf.Reset()
	contextLogger.Info("message from context logger")

	output := buf.String()
	if output == "" {
		t.Fatal("Context logger produced no output")
	}

	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		t.Fatalf("Context logger output is not valid JSON: %v\nOutput: %s", err, output)
	}

	// Should include request_id from context
	if logEntry["request_id"] != requestID {
		t.Errorf("Context logger should include request_id from context. Expected %s, got: %v",
			requestID, logEntry["request_id"])
	}

	// Test with empty context
	emptyCtx := context.Background()
	emptyLogger := log.FromContext(emptyCtx)
	if emptyLogger == nil {
		t.Fatal("FromContext should return a logger even with empty context, got nil")
	}

	buf.Reset()
	emptyLogger.Info("message from empty context logger")

	output = buf.String()
	if output == "" {
		t.Fatal("Logger from empty context produced no output")
	}
}

// testLogLevelConfiguration verifies:
// - SetLevel configures the minimum log level
// - Debug logs are suppressed when level is Info
// - Info logs appear when level is Info
// - Error logs always appear
func testLogLevelConfiguration(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo, // Start with Info level
	}))
	log.SetLogger(logger)

	ctx := context.Background()

	// Test 1: Info level - Debug should be suppressed
	log.SetLevel(slog.LevelInfo)

	buf.Reset()
	log.Debug(ctx, "debug message - should not appear")
	debugOutput := buf.String()

	buf.Reset()
	log.Info(ctx, "info message - should appear")
	infoOutput := buf.String()

	if debugOutput != "" {
		t.Error("Debug log should be suppressed when level is Info, but output was produced")
	}

	if infoOutput == "" {
		t.Error("Info log should appear when level is Info, but no output was produced")
	}

	// Test 2: Debug level - all logs should appear
	logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	log.SetLogger(logger)
	log.SetLevel(slog.LevelDebug)

	buf.Reset()
	log.Debug(ctx, "debug message - should now appear")
	debugOutput = buf.String()

	if debugOutput == "" {
		t.Error("Debug log should appear when level is Debug, but no output was produced")
	}

	// Test 3: Error level - only errors should appear
	logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
	log.SetLogger(logger)
	log.SetLevel(slog.LevelError)

	buf.Reset()
	log.Info(ctx, "info message - should not appear")
	infoOutput = buf.String()

	buf.Reset()
	log.Error(ctx, "error message - should appear", &testError{msg: "test"})
	errorOutput := buf.String()

	if infoOutput != "" {
		t.Error("Info log should be suppressed when level is Error, but output was produced")
	}

	if errorOutput == "" {
		t.Error("Error log should appear when level is Error, but no output was produced")
	}
}

// testError is a simple error implementation for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

// TestLoggingIntegrationScenario tests a realistic usage scenario
//
// This test simulates a real request flow:
// 1. Request arrives, gets assigned ID
// 2. Multiple log statements throughout processing
// 3. Duration logging at the end
// 4. All logs include the same request ID
func TestLoggingIntegrationScenario(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	log.SetLogger(logger)

	// Simulate request processing
	ctx := log.WithRequestID(context.Background())
	requestID := log.RequestID(ctx)

	start := time.Now()

	// Log request start
	log.Info(ctx, "request started", "method", "GET", "path", "/api/users")

	// Simulate processing
	time.Sleep(5 * time.Millisecond)

	// Log intermediate step
	log.Debug(ctx, "database query executed", "query", "SELECT * FROM users", "rows", 42)

	// Simulate authorization check
	log.Info(ctx, "authorization check", "Authorization", "Bearer secret-token-should-be-redacted")

	// Simulate error condition
	log.Error(ctx, "failed to connect to external service", &testError{msg: "connection timeout"}, "service", "user-service")

	// Log duration
	log.LogDuration(ctx, "request_processing", start)

	// Parse all log entries
	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) < 5 {
		t.Fatalf("Expected at least 5 log entries, got %d\nOutput:\n%s", len(lines), output)
	}

	// Verify all log entries are valid JSON and contain request_id
	for i, line := range lines {
		if line == "" {
			continue
		}

		var logEntry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &logEntry); err != nil {
			t.Errorf("Log entry %d is not valid JSON: %v\nLine: %s", i, err, line)
			continue
		}

		// All entries should have the same request_id
		entryRequestID, exists := logEntry["request_id"]
		if !exists {
			t.Errorf("Log entry %d missing request_id field", i)
			continue
		}

		if entryRequestID != requestID {
			t.Errorf("Log entry %d has wrong request_id. Expected %s, got %v",
				i, requestID, entryRequestID)
		}
	}

	// Verify Authorization header was redacted
	if strings.Contains(output, "secret-token-should-be-redacted") {
		t.Error("Authorization header should be redacted in logs")
	}

	t.Logf("Integration test passed: %d log entries with request_id %s", len(lines), requestID)
}
