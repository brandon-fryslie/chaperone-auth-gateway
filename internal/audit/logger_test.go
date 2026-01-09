package audit

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestNewLoggerDisabled verifies that NewLogger with enabled=false returns a disabled logger.
//
// A disabled logger should:
// - Return without error
// - Have enabled=false
// - Log() calls should be no-ops (not write anything)
func TestNewLoggerDisabled(t *testing.T) {
	cfg := Config{
		Enabled: false,
		Path:    "", // Shouldn't matter
	}

	logger, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("FAIL: NewLogger with enabled=false returned error: %v", err)
	}

	if logger == nil {
		t.Fatal("FAIL: NewLogger returned nil logger")
	}

	if logger.enabled {
		t.Fatal("FAIL: Disabled logger has enabled=true")
	}

	// Verify Log() is a no-op
	entry := Entry{
		Event:        EventCredentialInjected,
		Service:      "test",
		Host:         "example.com",
		Path:         "/api",
		Method:       "GET",
		AuthStrategy: "bearer",
		RequestID:    "req-123",
	}

	err = logger.Log(entry)
	if err != nil {
		t.Fatalf("FAIL: Disabled logger.Log() returned error: %v", err)
	}

	t.Log("PASS: Disabled logger is a no-op")
}

// TestNewLoggerStdout verifies NewLogger with path="stdout" uses os.Stdout.
func TestNewLoggerStdout(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"empty_path", ""},
		{"stdout_explicit", "stdout"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				Enabled: true,
				Path:    tt.path,
			}

			logger, err := NewLogger(cfg)
			if err != nil {
				t.Fatalf("FAIL: NewLogger returned error: %v", err)
			}

			if logger == nil {
				t.Fatal("FAIL: NewLogger returned nil logger")
			}

			if !logger.enabled {
				t.Fatal("FAIL: Logger should be enabled")
			}

			if logger.writer != os.Stdout {
				t.Fatal("FAIL: Logger writer should be os.Stdout")
			}

			t.Logf("PASS: NewLogger(%q) uses os.Stdout", tt.path)
		})
	}
}

// TestNewLoggerFile verifies NewLogger creates file with correct permissions.
//
// File-based audit logger should:
// - Create file if it doesn't exist
// - Set permissions to 0600 (read/write owner only)
// - Append to existing file
func TestNewLoggerFile(t *testing.T) {
	// Create temp directory for test
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	cfg := Config{
		Enabled: true,
		Path:    logPath,
	}

	logger, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("FAIL: NewLogger returned error: %v", err)
	}
	defer logger.Close()

	if logger == nil {
		t.Fatal("FAIL: NewLogger returned nil logger")
	}

	if !logger.enabled {
		t.Fatal("FAIL: Logger should be enabled")
	}

	// Verify file was created
	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("FAIL: Log file not created: %v", err)
	}

	// Verify file permissions are 0600
	mode := info.Mode().Perm()
	if mode != 0600 {
		t.Fatalf("FAIL: Log file permissions = %o, want 0600", mode)
	}

	t.Logf("PASS: NewLogger created file with 0600 permissions")
}

// TestLogWritesValidJSON verifies Log() writes valid JSON entries with all fields.
//
// Each log entry must:
// - Be valid JSON
// - Contain all fields from Entry struct
// - Have timestamp automatically set
// - Be newline-delimited
func TestLogWritesValidJSON(t *testing.T) {
	// Use in-memory buffer to capture output
	var buf bytes.Buffer

	logger := &Logger{
		writer:  &buf,
		encoder: json.NewEncoder(&buf),
		enabled: true,
	}

	entry := Entry{
		Event:        EventCredentialInjected,
		Service:      "openai",
		Host:         "api.openai.com",
		Path:         "/v1/chat/completions",
		Method:       "POST",
		AuthStrategy: "bearer",
		RequestID:    "req-abc-123",
	}

	// Record time before logging
	beforeLog := time.Now().UTC()

	err := logger.Log(entry)
	if err != nil {
		t.Fatalf("FAIL: Log() returned error: %v", err)
	}

	afterLog := time.Now().UTC()

	// Parse the JSON output
	var logged Entry
	err = json.Unmarshal(buf.Bytes(), &logged)
	if err != nil {
		t.Fatalf("FAIL: Log output is not valid JSON: %v\nOutput: %s", err, buf.String())
	}

	// Verify all fields were written
	if logged.Event != EventCredentialInjected {
		t.Errorf("FAIL: Event field = %q, want %q", logged.Event, EventCredentialInjected)
	}
	if logged.Service != "openai" {
		t.Errorf("FAIL: Service field = %q, want %q", logged.Service, "openai")
	}
	if logged.Host != "api.openai.com" {
		t.Errorf("FAIL: Host field = %q, want %q", logged.Host, "api.openai.com")
	}
	if logged.Path != "/v1/chat/completions" {
		t.Errorf("FAIL: Path field = %q, want %q", logged.Path, "/v1/chat/completions")
	}
	if logged.Method != "POST" {
		t.Errorf("FAIL: Method field = %q, want %q", logged.Method, "POST")
	}
	if logged.AuthStrategy != "bearer" {
		t.Errorf("FAIL: AuthStrategy field = %q, want %q", logged.AuthStrategy, "bearer")
	}
	if logged.RequestID != "req-abc-123" {
		t.Errorf("FAIL: RequestID field = %q, want %q", logged.RequestID, "req-abc-123")
	}

	// Verify timestamp was set and is reasonable
	if logged.Timestamp.IsZero() {
		t.Fatal("FAIL: Timestamp field is zero - Log() should set timestamp")
	}
	if logged.Timestamp.Before(beforeLog) || logged.Timestamp.After(afterLog) {
		t.Errorf("FAIL: Timestamp %v is outside expected range [%v, %v]", logged.Timestamp, beforeLog, afterLog)
	}

	t.Logf("PASS: Log() writes valid JSON with all fields")
	t.Logf("JSON output: %s", buf.String())
}

// TestLogMultipleEntries verifies multiple Log() calls produce multiple JSON lines.
func TestLogMultipleEntries(t *testing.T) {
	var buf bytes.Buffer

	logger := &Logger{
		writer:  &buf,
		encoder: json.NewEncoder(&buf),
		enabled: true,
	}

	// Log 3 entries
	entries := []Entry{
		{
			Event:        EventCredentialInjected,
			Service:      "openai",
			Host:         "api.openai.com",
			Path:         "/v1/models",
			Method:       "GET",
			AuthStrategy: "bearer",
			RequestID:    "req-1",
		},
		{
			Event:        EventCredentialInjected,
			Service:      "anthropic",
			Host:         "api.anthropic.com",
			Path:         "/v1/messages",
			Method:       "POST",
			AuthStrategy: "header:x-api-key",
			RequestID:    "req-2",
		},
		{
			Event:        EventCredentialInjected,
			Service:      "openai",
			Host:         "api.openai.com",
			Path:         "/v1/completions",
			Method:       "POST",
			AuthStrategy: "bearer",
			RequestID:    "req-3",
		},
	}

	for _, e := range entries {
		err := logger.Log(e)
		if err != nil {
			t.Fatalf("FAIL: Log() returned error: %v", err)
		}
	}

	// Split output by newlines
	output := buf.String()
	lines := bytes.Split([]byte(output), []byte("\n"))

	// Should have 3 JSON lines + 1 empty (trailing newline)
	if len(lines) != 4 {
		t.Fatalf("FAIL: Expected 4 lines (3 entries + trailing newline), got %d\nOutput: %s", len(lines), output)
	}

	// Parse each line and verify
	for i := 0; i < 3; i++ {
		var logged Entry
		err := json.Unmarshal(lines[i], &logged)
		if err != nil {
			t.Fatalf("FAIL: Line %d is not valid JSON: %v\nLine: %s", i+1, err, lines[i])
		}

		if logged.Service != entries[i].Service {
			t.Errorf("FAIL: Entry %d Service = %q, want %q", i+1, logged.Service, entries[i].Service)
		}
		if logged.RequestID != entries[i].RequestID {
			t.Errorf("FAIL: Entry %d RequestID = %q, want %q", i+1, logged.RequestID, entries[i].RequestID)
		}
	}

	t.Logf("PASS: Multiple Log() calls produce multiple JSON lines")
}

// TestLogConcurrentSafety verifies concurrent Log() calls don't corrupt output.
//
// The logger uses a mutex to protect writes. This test verifies:
// - Concurrent calls don't panic
// - All entries are written
// - Each entry is valid JSON
func TestLogConcurrentSafety(t *testing.T) {
	var buf bytes.Buffer

	logger := &Logger{
		writer:  &buf,
		encoder: json.NewEncoder(&buf),
		enabled: true,
	}

	// Launch 10 goroutines, each writing 10 entries
	const goroutines = 10
	const entriesPerGoroutine = 10
	expectedTotal := goroutines * entriesPerGoroutine

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < entriesPerGoroutine; i++ {
				entry := Entry{
					Event:        EventCredentialInjected,
					Service:      "test",
					Host:         "example.com",
					Path:         "/api",
					Method:       "GET",
					AuthStrategy: "bearer",
					RequestID:    string(rune(id*1000 + i)), // Unique per entry
				}
				err := logger.Log(entry)
				if err != nil {
					t.Errorf("FAIL: Log() returned error in goroutine %d: %v", id, err)
				}
			}
		}(g)
	}

	wg.Wait()

	// Verify all entries were written
	output := buf.String()
	lines := bytes.Split([]byte(output), []byte("\n"))

	// Should have expectedTotal + 1 empty line
	if len(lines) != expectedTotal+1 {
		t.Fatalf("FAIL: Expected %d lines, got %d", expectedTotal+1, len(lines))
	}

	// Verify each line is valid JSON
	validCount := 0
	for i := 0; i < expectedTotal; i++ {
		var logged Entry
		err := json.Unmarshal(lines[i], &logged)
		if err != nil {
			t.Errorf("FAIL: Line %d is not valid JSON: %v\nLine: %s", i+1, err, lines[i])
			continue
		}
		validCount++
	}

	if validCount != expectedTotal {
		t.Fatalf("FAIL: Only %d/%d entries are valid JSON", validCount, expectedTotal)
	}

	t.Logf("PASS: %d concurrent Log() calls produced %d valid JSON entries", goroutines, expectedTotal)
}

// TestLogToFile verifies end-to-end file logging.
func TestLogToFile(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	cfg := Config{
		Enabled: true,
		Path:    logPath,
	}

	logger, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("FAIL: NewLogger returned error: %v", err)
	}

	// Log some entries
	entries := []Entry{
		{
			Event:        EventCredentialInjected,
			Service:      "openai",
			Host:         "api.openai.com",
			Path:         "/v1/models",
			Method:       "GET",
			AuthStrategy: "bearer",
			RequestID:    "req-1",
		},
		{
			Event:        EventCredentialInjected,
			Service:      "anthropic",
			Host:         "api.anthropic.com",
			Path:         "/v1/messages",
			Method:       "POST",
			AuthStrategy: "header:x-api-key",
			RequestID:    "req-2",
		},
	}

	for _, e := range entries {
		err := logger.Log(e)
		if err != nil {
			t.Fatalf("FAIL: Log() returned error: %v", err)
		}
	}

	// Close logger to flush
	err = logger.Close()
	if err != nil {
		t.Fatalf("FAIL: Close() returned error: %v", err)
	}

	// Read file and verify
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("FAIL: Cannot read log file: %v", err)
	}

	lines := bytes.Split(data, []byte("\n"))
	if len(lines) != 3 { // 2 entries + trailing newline
		t.Fatalf("FAIL: Expected 3 lines, got %d", len(lines))
	}

	// Verify first entry
	var logged Entry
	err = json.Unmarshal(lines[0], &logged)
	if err != nil {
		t.Fatalf("FAIL: First line is not valid JSON: %v", err)
	}

	if logged.Service != "openai" || logged.RequestID != "req-1" {
		t.Fatalf("FAIL: First entry has wrong data: Service=%s, RequestID=%s", logged.Service, logged.RequestID)
	}

	t.Logf("PASS: End-to-end file logging works")
}

// TestLoggerClose verifies Close() closes file properly.
func TestLoggerClose(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	cfg := Config{
		Enabled: true,
		Path:    logPath,
	}

	logger, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("FAIL: NewLogger returned error: %v", err)
	}

	// Close should not error
	err = logger.Close()
	if err != nil {
		t.Fatalf("FAIL: Close() returned error: %v", err)
	}

	// Verify file exists and is readable
	_, err = os.Stat(logPath)
	if err != nil {
		t.Fatalf("FAIL: Log file doesn't exist after Close(): %v", err)
	}

	t.Log("PASS: Close() closes file properly")
}

// TestLoggerCloseStdout verifies Close() on stdout logger doesn't error.
func TestLoggerCloseStdout(t *testing.T) {
	cfg := Config{
		Enabled: true,
		Path:    "stdout",
	}

	logger, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("FAIL: NewLogger returned error: %v", err)
	}

	// Close should not error (stdout shouldn't be closed)
	err = logger.Close()
	if err != nil {
		t.Fatalf("FAIL: Close() on stdout logger returned error: %v", err)
	}

	t.Log("PASS: Close() on stdout logger is safe")
}

// TestNewLoggerInvalidPath verifies NewLogger handles invalid paths.
func TestNewLoggerInvalidPath(t *testing.T) {
	cfg := Config{
		Enabled: true,
		Path:    "/nonexistent/directory/that/does/not/exist/audit.log",
	}

	logger, err := NewLogger(cfg)
	if err == nil {
		if logger != nil {
			logger.Close()
		}
		t.Fatal("FAIL: NewLogger should return error for invalid path")
	}

	t.Logf("PASS: NewLogger returns error for invalid path: %v", err)
}

// TestDisabledLoggerClose verifies Close() on disabled logger is safe.
func TestDisabledLoggerClose(t *testing.T) {
	cfg := Config{
		Enabled: false,
	}

	logger, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("FAIL: NewLogger returned error: %v", err)
	}

	err = logger.Close()
	if err != nil {
		t.Fatalf("FAIL: Close() on disabled logger returned error: %v", err)
	}

	t.Log("PASS: Close() on disabled logger is safe")
}
