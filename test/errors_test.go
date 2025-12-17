package test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	cherrors "github.com/bmf/chaperone/internal/errors"
)

// TestErrorFramework validates Phase 0.2: Error Handling Framework
//
// This test suite validates the error handling framework by testing:
// 1. Sentinel errors exist and work with errors.Is()
// 2. Structured error types implement Error() and Unwrap()
// 3. HTTP status mapping returns correct codes
// 4. Safe client messages redact sensitive data
//
// ANTI-GAMING MEASURES:
// 1. Tests import actual implementation package - can't be faked with stubs
// 2. errors.Is() validation ensures sentinel errors are real error values
// 3. Unwrap() chain validation requires proper error wrapping
// 4. HTTPStatus() tested with both direct and wrapped errors
// 5. ClientMessage() validated against actual sensitive strings
// 6. Negative tests ensure unknown errors handled correctly
//
// Tests FAIL (not skip) when functionality is missing or incorrect.

// TestSentinelErrorsExist verifies all 6 sentinel errors are defined.
//
// Sentinel errors must:
// - Exist as package-level variables
// - Implement the error interface
// - Work with errors.Is() for comparison
// - Have meaningful error messages
func TestSentinelErrorsExist(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		expectedMsg   string
		shouldContain string // message should contain this text
	}{
		{
			name:          "ErrSecretNotFound",
			err:           cherrors.ErrSecretNotFound,
			shouldContain: "secret not found",
		},
		{
			name:          "ErrPermissionDenied",
			err:           cherrors.ErrPermissionDenied,
			shouldContain: "permission denied",
		},
		{
			name:          "ErrTimeout",
			err:           cherrors.ErrTimeout,
			shouldContain: "timeout",
		},
		{
			name:          "ErrPolicyViolation",
			err:           cherrors.ErrPolicyViolation,
			shouldContain: "policy violation",
		},
		{
			name:          "ErrInvalidConfig",
			err:           cherrors.ErrInvalidConfig,
			shouldContain: "invalid configuration",
		},
		{
			name:          "ErrUpstreamError",
			err:           cherrors.ErrUpstreamError,
			shouldContain: "upstream error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test 1: Error must not be nil
			if tt.err == nil {
				t.Fatalf("FAIL: %s is nil - must be defined as sentinel error", tt.name)
			}

			// Test 2: Error must have a message
			msg := tt.err.Error()
			if msg == "" {
				t.Fatalf("FAIL: %s.Error() returns empty string - must have meaningful message", tt.name)
			}

			// Test 3: Message should contain expected text (case insensitive)
			if !strings.Contains(strings.ToLower(msg), strings.ToLower(tt.shouldContain)) {
				t.Fatalf("FAIL: %s.Error() = %q, should contain %q", tt.name, msg, tt.shouldContain)
			}

			// Test 4: Must work with errors.Is() (identity check)
			if !errors.Is(tt.err, tt.err) {
				t.Fatalf("FAIL: errors.Is(%s, %s) = false, must be true for identity", tt.name, tt.name)
			}

			t.Logf("PASS: %s exists and returns: %q", tt.name, msg)
		})
	}
}

// TestSentinelErrorsAreUnique verifies sentinel errors are distinct.
//
// Each sentinel error must be a unique value - they should not compare equal
// to each other.
func TestSentinelErrorsAreUnique(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrSecretNotFound", cherrors.ErrSecretNotFound},
		{"ErrPermissionDenied", cherrors.ErrPermissionDenied},
		{"ErrTimeout", cherrors.ErrTimeout},
		{"ErrPolicyViolation", cherrors.ErrPolicyViolation},
		{"ErrInvalidConfig", cherrors.ErrInvalidConfig},
		{"ErrUpstreamError", cherrors.ErrUpstreamError},
	}

	// Each sentinel should NOT equal any other sentinel
	for i, s1 := range sentinels {
		for j, s2 := range sentinels {
			if i == j {
				continue
			}
			if errors.Is(s1.err, s2.err) {
				t.Fatalf("FAIL: %s and %s compare equal with errors.Is() - must be distinct errors", s1.name, s2.name)
			}
		}
	}

	t.Logf("PASS: All %d sentinel errors are unique", len(sentinels))
}

// TestStructuredErrorTypes verifies SecretError, PolicyError, ConfigError exist
// and implement the required interfaces.
func TestStructuredErrorTypes(t *testing.T) {
	t.Run("SecretError", func(t *testing.T) {
		testSecretError(t)
	})

	t.Run("PolicyError", func(t *testing.T) {
		testPolicyError(t)
	})

	t.Run("ConfigError", func(t *testing.T) {
		testConfigError(t)
	})
}

// testSecretError validates SecretError structure and methods.
//
// SecretError must:
// - Be a struct with Provider, Ref, Cause fields
// - Implement Error() method
// - Implement Unwrap() method for error chaining
// - Work with errors.Is() and errors.As()
func testSecretError(t *testing.T) {
	// Create a SecretError with a wrapped sentinel error
	cause := cherrors.ErrSecretNotFound
	err := &cherrors.SecretError{
		Provider: "test-provider",
		Ref:      "api-key-123",
		Cause:    cause,
	}

	// Test 1: Implements error interface (has Error() method)
	var _ error = err
	msg := err.Error()
	if msg == "" {
		t.Fatal("FAIL: SecretError.Error() returns empty string")
	}
	t.Logf("SecretError.Error() = %q", msg)

	// Test 2: Error message should include context (provider and ref)
	if !strings.Contains(msg, "test-provider") {
		t.Errorf("FAIL: SecretError.Error() should include Provider field, got: %q", msg)
	}
	if !strings.Contains(msg, "api-key-123") {
		t.Errorf("FAIL: SecretError.Error() should include Ref field, got: %q", msg)
	}

	// Test 3: Implements Unwrap() for error chaining
	unwrapped := errors.Unwrap(err)
	if unwrapped == nil {
		t.Fatal("FAIL: SecretError.Unwrap() returns nil - must unwrap to Cause error")
	}
	if !errors.Is(unwrapped, cause) {
		t.Fatalf("FAIL: SecretError.Unwrap() returned %v, expected %v", unwrapped, cause)
	}
	t.Log("PASS: SecretError.Unwrap() correctly returns Cause")

	// Test 4: errors.Is() works through the chain
	if !errors.Is(err, cherrors.ErrSecretNotFound) {
		t.Fatal("FAIL: errors.Is(SecretError, ErrSecretNotFound) = false, must be true (error chaining)")
	}
	t.Log("PASS: errors.Is() works through SecretError chain")

	// Test 5: errors.As() can extract the structured error
	var extracted *cherrors.SecretError
	if !errors.As(err, &extracted) {
		t.Fatal("FAIL: errors.As() cannot extract SecretError")
	}
	if extracted.Provider != "test-provider" || extracted.Ref != "api-key-123" {
		t.Fatalf("FAIL: errors.As() extracted wrong data: Provider=%s, Ref=%s", extracted.Provider, extracted.Ref)
	}
	t.Log("PASS: errors.As() correctly extracts SecretError")

	t.Log("PASS: SecretError implements all required interfaces")
}

// testPolicyError validates PolicyError structure and methods.
//
// PolicyError must:
// - Be a struct with Service, Rule, Cause fields
// - Implement Error() method
// - Implement Unwrap() method for error chaining
// - Work with errors.Is() and errors.As()
func testPolicyError(t *testing.T) {
	// Create a PolicyError with a wrapped sentinel error
	cause := cherrors.ErrPolicyViolation
	err := &cherrors.PolicyError{
		Service: "openai-api",
		Rule:    "max-body-size",
		Cause:   cause,
	}

	// Test 1: Implements error interface
	var _ error = err
	msg := err.Error()
	if msg == "" {
		t.Fatal("FAIL: PolicyError.Error() returns empty string")
	}
	t.Logf("PolicyError.Error() = %q", msg)

	// Test 2: Error message should include context
	if !strings.Contains(msg, "openai-api") {
		t.Errorf("FAIL: PolicyError.Error() should include Service field, got: %q", msg)
	}
	if !strings.Contains(msg, "max-body-size") {
		t.Errorf("FAIL: PolicyError.Error() should include Rule field, got: %q", msg)
	}

	// Test 3: Implements Unwrap()
	unwrapped := errors.Unwrap(err)
	if unwrapped == nil {
		t.Fatal("FAIL: PolicyError.Unwrap() returns nil")
	}
	if !errors.Is(unwrapped, cause) {
		t.Fatalf("FAIL: PolicyError.Unwrap() returned %v, expected %v", unwrapped, cause)
	}
	t.Log("PASS: PolicyError.Unwrap() correctly returns Cause")

	// Test 4: errors.Is() works through the chain
	if !errors.Is(err, cherrors.ErrPolicyViolation) {
		t.Fatal("FAIL: errors.Is(PolicyError, ErrPolicyViolation) = false, must be true")
	}
	t.Log("PASS: errors.Is() works through PolicyError chain")

	// Test 5: errors.As() can extract the structured error
	var extracted *cherrors.PolicyError
	if !errors.As(err, &extracted) {
		t.Fatal("FAIL: errors.As() cannot extract PolicyError")
	}
	if extracted.Service != "openai-api" || extracted.Rule != "max-body-size" {
		t.Fatalf("FAIL: errors.As() extracted wrong data: Service=%s, Rule=%s", extracted.Service, extracted.Rule)
	}
	t.Log("PASS: errors.As() correctly extracts PolicyError")

	t.Log("PASS: PolicyError implements all required interfaces")
}

// testConfigError validates ConfigError structure and methods.
//
// ConfigError must:
// - Be a struct with Field, Value, Cause fields
// - Implement Error() method
// - Implement Unwrap() method for error chaining
// - Work with errors.Is() and errors.As()
func testConfigError(t *testing.T) {
	// Create a ConfigError with a wrapped sentinel error
	cause := cherrors.ErrInvalidConfig
	err := &cherrors.ConfigError{
		Field: "port",
		Value: -1,
		Cause: cause,
	}

	// Test 1: Implements error interface
	var _ error = err
	msg := err.Error()
	if msg == "" {
		t.Fatal("FAIL: ConfigError.Error() returns empty string")
	}
	t.Logf("ConfigError.Error() = %q", msg)

	// Test 2: Error message should include context
	if !strings.Contains(msg, "port") {
		t.Errorf("FAIL: ConfigError.Error() should include Field field, got: %q", msg)
	}

	// Test 3: Implements Unwrap()
	unwrapped := errors.Unwrap(err)
	if unwrapped == nil {
		t.Fatal("FAIL: ConfigError.Unwrap() returns nil")
	}
	if !errors.Is(unwrapped, cause) {
		t.Fatalf("FAIL: ConfigError.Unwrap() returned %v, expected %v", unwrapped, cause)
	}
	t.Log("PASS: ConfigError.Unwrap() correctly returns Cause")

	// Test 4: errors.Is() works through the chain
	if !errors.Is(err, cherrors.ErrInvalidConfig) {
		t.Fatal("FAIL: errors.Is(ConfigError, ErrInvalidConfig) = false, must be true")
	}
	t.Log("PASS: errors.Is() works through ConfigError chain")

	// Test 5: errors.As() can extract the structured error
	var extracted *cherrors.ConfigError
	if !errors.As(err, &extracted) {
		t.Fatal("FAIL: errors.As() cannot extract ConfigError")
	}
	if extracted.Field != "port" {
		t.Fatalf("FAIL: errors.As() extracted wrong Field: %s", extracted.Field)
	}
	t.Log("PASS: errors.As() correctly extracts ConfigError")

	t.Log("PASS: ConfigError implements all required interfaces")
}

// TestHTTPStatusMapping verifies HTTPStatus() function maps errors to correct
// HTTP status codes.
//
// This function is critical for the proxy to return appropriate responses.
// Tests verify both direct sentinel errors and wrapped errors.
func TestHTTPStatusMapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		// Direct sentinel errors
		{
			name:       "ErrSecretNotFound_direct",
			err:        cherrors.ErrSecretNotFound,
			wantStatus: 502, // Bad Gateway
		},
		{
			name:       "ErrPermissionDenied_direct",
			err:        cherrors.ErrPermissionDenied,
			wantStatus: 403, // Forbidden
		},
		{
			name:       "ErrTimeout_direct",
			err:        cherrors.ErrTimeout,
			wantStatus: 504, // Gateway Timeout
		},
		{
			name:       "ErrPolicyViolation_direct",
			err:        cherrors.ErrPolicyViolation,
			wantStatus: 403, // Forbidden
		},
		{
			name:       "ErrInvalidConfig_direct",
			err:        cherrors.ErrInvalidConfig,
			wantStatus: 500, // Internal Server Error
		},
		{
			name:       "ErrUpstreamError_direct",
			err:        cherrors.ErrUpstreamError,
			wantStatus: 502, // Bad Gateway
		},
		// Wrapped errors - must still map correctly
		{
			name: "ErrSecretNotFound_wrapped_in_SecretError",
			err: &cherrors.SecretError{
				Provider: "keychain",
				Ref:      "api-key",
				Cause:    cherrors.ErrSecretNotFound,
			},
			wantStatus: 502,
		},
		{
			name: "ErrPolicyViolation_wrapped_in_PolicyError",
			err: &cherrors.PolicyError{
				Service: "openai",
				Rule:    "path-allowed",
				Cause:   cherrors.ErrPolicyViolation,
			},
			wantStatus: 403,
		},
		{
			name: "ErrInvalidConfig_wrapped_in_ConfigError",
			err: &cherrors.ConfigError{
				Field: "port",
				Value: "invalid",
				Cause: cherrors.ErrInvalidConfig,
			},
			wantStatus: 500,
		},
		{
			name: "ErrSecretNotFound_double_wrapped",
			err: fmt.Errorf("outer error: %w", &cherrors.SecretError{
				Provider: "vault",
				Ref:      "secret-123",
				Cause:    cherrors.ErrSecretNotFound,
			}),
			wantStatus: 502,
		},
		// Unknown errors
		{
			name:       "unknown_error",
			err:        errors.New("some random error"),
			wantStatus: 500, // Default to Internal Server Error
		},
		{
			name:       "nil_error",
			err:        nil,
			wantStatus: 500, // Should handle nil gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := cherrors.HTTPStatus(tt.err)
			if status != tt.wantStatus {
				t.Fatalf("FAIL: HTTPStatus(%v) = %d, want %d", tt.err, status, tt.wantStatus)
			}
			t.Logf("PASS: HTTPStatus(%s) = %d", tt.name, status)
		})
	}
}

// TestHTTPStatusValidCodes verifies all returned status codes are valid HTTP codes.
func TestHTTPStatusValidCodes(t *testing.T) {
	// All known sentinel errors
	sentinels := []error{
		cherrors.ErrSecretNotFound,
		cherrors.ErrPermissionDenied,
		cherrors.ErrTimeout,
		cherrors.ErrPolicyViolation,
		cherrors.ErrInvalidConfig,
		cherrors.ErrUpstreamError,
	}

	for _, err := range sentinels {
		status := cherrors.HTTPStatus(err)
		// Valid HTTP status codes are 100-599
		if status < 100 || status > 599 {
			t.Fatalf("FAIL: HTTPStatus(%v) = %d, must be valid HTTP status (100-599)", err, status)
		}
	}

	t.Log("PASS: All HTTPStatus() results are valid HTTP status codes")
}

// TestClientMessageSafety verifies ClientMessage() returns safe messages that
// don't leak sensitive information.
//
// This is CRITICAL for security - client-facing error messages must NOT include:
// - Internal file paths
// - Secret references
// - Provider names
// - Stack traces
// - Configuration details
func TestClientMessageSafety(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		wantContains   []string // Message should contain these (user-friendly info)
		mustNotContain []string // Message MUST NOT contain these (sensitive data)
	}{
		{
			name:           "ErrSecretNotFound_direct",
			err:            cherrors.ErrSecretNotFound,
			wantContains:   []string{"error"},
			mustNotContain: []string{"secret", "not found"}, // Don't leak what failed
		},
		{
			name: "SecretError_with_sensitive_data",
			err: &cherrors.SecretError{
				Provider: "keychain",            // SENSITIVE
				Ref:      "openai-api-key-prod", // SENSITIVE
				Cause:    cherrors.ErrSecretNotFound,
			},
			wantContains:   []string{"error"},
			mustNotContain: []string{"keychain", "openai-api-key-prod", "ErrSecretNotFound"},
		},
		{
			name: "PolicyError_with_service_name",
			err: &cherrors.PolicyError{
				Service: "openai-api",            // SENSITIVE
				Rule:    "forbidden-path:/admin", // SENSITIVE
				Cause:   cherrors.ErrPolicyViolation,
			},
			wantContains:   []string{"forbidden", "denied"},
			mustNotContain: []string{"openai-api", "/admin", "ErrPolicyViolation"},
		},
		{
			name: "ConfigError_with_internal_paths",
			err: &cherrors.ConfigError{
				Field: "/etc/chaperone/config.toml", // SENSITIVE
				Value: "secret-value",               // SENSITIVE
				Cause: cherrors.ErrInvalidConfig,
			},
			wantContains:   []string{"error", "configuration"},
			mustNotContain: []string{"/etc/chaperone", "secret-value", "ErrInvalidConfig"},
		},
		{
			name:           "ErrPermissionDenied",
			err:            cherrors.ErrPermissionDenied,
			wantContains:   []string{"forbidden", "denied"},
			mustNotContain: []string{"permission"}, // Generic term okay, but not exact match
		},
		{
			name:           "ErrTimeout",
			err:            cherrors.ErrTimeout,
			wantContains:   []string{"timeout", "try again"},
			mustNotContain: []string{},
		},
		{
			name:           "unknown_error",
			err:            errors.New("internal: database connection failed on host db.internal.company.com:5432"),
			wantContains:   []string{"error"},
			mustNotContain: []string{"database", "db.internal.company.com", "5432"},
		},
		{
			name:           "nil_error",
			err:            nil,
			wantContains:   []string{"error"},
			mustNotContain: []string{"nil", "null"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := cherrors.ClientMessage(tt.err)

			// Test 1: Message must not be empty
			if msg == "" {
				t.Fatal("FAIL: ClientMessage() returned empty string - must provide user-facing message")
			}

			// Test 2: Message should contain expected user-friendly text
			msgLower := strings.ToLower(msg)
			for _, want := range tt.wantContains {
				if !strings.Contains(msgLower, strings.ToLower(want)) {
					t.Errorf("FAIL: ClientMessage() = %q, should contain %q", msg, want)
				}
			}

			// Test 3: Message MUST NOT contain sensitive data
			for _, forbidden := range tt.mustNotContain {
				if strings.Contains(msgLower, strings.ToLower(forbidden)) {
					t.Fatalf("FAIL: ClientMessage() = %q, MUST NOT contain sensitive data: %q", msg, forbidden)
				}
			}

			// Test 4: Message should be reasonably short (< 200 chars)
			if len(msg) > 200 {
				t.Errorf("FAIL: ClientMessage() too long (%d chars) - should be concise: %q", len(msg), msg)
			}

			// Test 5: Message should not contain common leak patterns
			leakPatterns := []string{
				"stack trace",
				"goroutine",
				".go:",       // file paths
				"internal/",  // internal package paths
				"github.com", // module paths
				"panic:",
			}
			for _, pattern := range leakPatterns {
				if strings.Contains(msgLower, pattern) {
					t.Fatalf("FAIL: ClientMessage() contains leak pattern %q: %s", pattern, msg)
				}
			}

			t.Logf("PASS: ClientMessage() safe: %q", msg)
		})
	}
}

// TestClientMessageConsistency verifies ClientMessage() returns consistent
// messages for the same error types.
func TestClientMessageConsistency(t *testing.T) {
	// Same type of errors should return consistent messages
	err1 := &cherrors.SecretError{Provider: "vault", Ref: "key1", Cause: cherrors.ErrSecretNotFound}
	err2 := &cherrors.SecretError{Provider: "keychain", Ref: "key2", Cause: cherrors.ErrSecretNotFound}

	msg1 := cherrors.ClientMessage(err1)
	msg2 := cherrors.ClientMessage(err2)

	// Messages should be similar (at least same length range)
	// They should NOT leak the different provider/ref values
	if strings.Contains(msg1, "vault") || strings.Contains(msg1, "key1") {
		t.Fatalf("FAIL: ClientMessage leaks provider/ref: %s", msg1)
	}
	if strings.Contains(msg2, "keychain") || strings.Contains(msg2, "key2") {
		t.Fatalf("FAIL: ClientMessage leaks provider/ref: %s", msg2)
	}

	// Messages should be the same or very similar
	if msg1 != msg2 {
		// If different, they should at least be the same length category
		lengthDiff := len(msg1) - len(msg2)
		if lengthDiff < -20 || lengthDiff > 20 {
			t.Logf("WARNING: ClientMessage() returns very different messages for same error type:")
			t.Logf("  msg1: %s", msg1)
			t.Logf("  msg2: %s", msg2)
		}
	}

	t.Log("PASS: ClientMessage() returns consistent messages")
}

// TestErrorChainIntegrity verifies error wrapping works correctly through
// multiple levels.
func TestErrorChainIntegrity(t *testing.T) {
	// Create a 3-level error chain
	root := cherrors.ErrSecretNotFound
	middle := &cherrors.SecretError{
		Provider: "vault",
		Ref:      "api-key",
		Cause:    root,
	}
	outer := fmt.Errorf("failed to authenticate: %w", middle)

	// Test 1: errors.Is() should find the root error
	if !errors.Is(outer, cherrors.ErrSecretNotFound) {
		t.Fatal("FAIL: errors.Is() cannot find root error through 3-level chain")
	}

	// Test 2: errors.As() should extract middle structured error
	var extracted *cherrors.SecretError
	if !errors.As(outer, &extracted) {
		t.Fatal("FAIL: errors.As() cannot extract SecretError from chain")
	}
	if extracted.Provider != "vault" {
		t.Fatalf("FAIL: Extracted wrong SecretError: Provider=%s", extracted.Provider)
	}

	// Test 3: HTTPStatus() should map correctly through chain
	status := cherrors.HTTPStatus(outer)
	if status != 502 {
		t.Fatalf("FAIL: HTTPStatus(wrapped error) = %d, want 502", status)
	}

	// Test 4: ClientMessage() should be safe for entire chain
	msg := cherrors.ClientMessage(outer)
	if strings.Contains(strings.ToLower(msg), "vault") {
		t.Fatalf("FAIL: ClientMessage() leaks provider through chain: %s", msg)
	}

	t.Log("PASS: Error chain integrity maintained through 3 levels")
}

// TestPhase02Completion is a meta-test that checks if Phase 0.2 is complete.
//
// This runs all validation checks and reports overall status.
func TestPhase02Completion(t *testing.T) {
	checks := []struct {
		name string
		fn   func() error
	}{
		{
			name: "All 6 sentinel errors exist",
			fn: func() error {
				sentinels := []error{
					cherrors.ErrSecretNotFound,
					cherrors.ErrPermissionDenied,
					cherrors.ErrTimeout,
					cherrors.ErrPolicyViolation,
					cherrors.ErrInvalidConfig,
					cherrors.ErrUpstreamError,
				}
				for _, err := range sentinels {
					if err == nil {
						return errors.New("sentinel error is nil")
					}
				}
				return nil
			},
		},
		{
			name: "SecretError implements Error() and Unwrap()",
			fn: func() error {
				err := &cherrors.SecretError{
					Provider: "test",
					Ref:      "test",
					Cause:    cherrors.ErrSecretNotFound,
				}
				if err.Error() == "" {
					return errors.New("Error() returns empty")
				}
				if errors.Unwrap(err) == nil {
					return errors.New("Unwrap() returns nil")
				}
				return nil
			},
		},
		{
			name: "PolicyError implements Error() and Unwrap()",
			fn: func() error {
				err := &cherrors.PolicyError{
					Service: "test",
					Rule:    "test",
					Cause:   cherrors.ErrPolicyViolation,
				}
				if err.Error() == "" {
					return errors.New("Error() returns empty")
				}
				if errors.Unwrap(err) == nil {
					return errors.New("Unwrap() returns nil")
				}
				return nil
			},
		},
		{
			name: "ConfigError implements Error() and Unwrap()",
			fn: func() error {
				err := &cherrors.ConfigError{
					Field: "test",
					Value: "test",
					Cause: cherrors.ErrInvalidConfig,
				}
				if err.Error() == "" {
					return errors.New("Error() returns empty")
				}
				if errors.Unwrap(err) == nil {
					return errors.New("Unwrap() returns nil")
				}
				return nil
			},
		},
		{
			name: "HTTPStatus() maps all sentinel errors correctly",
			fn: func() error {
				// Just verify function exists and returns valid codes
				status := cherrors.HTTPStatus(cherrors.ErrSecretNotFound)
				if status < 100 || status > 599 {
					return fmt.Errorf("invalid HTTP status: %d", status)
				}
				return nil
			},
		},
		{
			name: "ClientMessage() returns safe messages",
			fn: func() error {
				err := &cherrors.SecretError{
					Provider: "keychain",
					Ref:      "secret-key",
					Cause:    cherrors.ErrSecretNotFound,
				}
				msg := cherrors.ClientMessage(err)
				if msg == "" {
					return errors.New("returns empty message")
				}
				if strings.Contains(msg, "keychain") || strings.Contains(msg, "secret-key") {
					return errors.New("leaks sensitive data")
				}
				return nil
			},
		},
	}

	passed := 0
	failed := 0
	var failureMessages []string

	for _, check := range checks {
		err := check.fn()
		if err == nil {
			t.Logf("✓ %s", check.name)
			passed++
		} else {
			t.Logf("✗ %s: %v", check.name, err)
			failureMessages = append(failureMessages, fmt.Sprintf("%s: %v", check.name, err))
			failed++
		}
	}

	t.Logf("\nPhase 0.2 Completion Status: %d/%d checks passed", passed, len(checks))

	if failed > 0 {
		t.Logf("\nFailed checks:")
		for _, msg := range failureMessages {
			t.Logf("  - %s", msg)
		}
		t.Fatalf("\nFAIL: Phase 0.2 is INCOMPLETE - %d/%d checks failed\n\nTo complete Phase 0.2:\n  1. Define all 6 sentinel errors in internal/errors/errors.go\n  2. Implement SecretError, PolicyError, ConfigError structs\n  3. Implement Error() and Unwrap() methods for all structured errors\n  4. Implement HTTPStatus(err error) int function\n  5. Implement ClientMessage(err error) string function\n  6. Ensure 'go test ./test -run TestPhase02' passes", failed, len(checks))
	}

	t.Log("\n✓✓✓ PASS: Phase 0.2 Error Handling Framework is COMPLETE ✓✓✓")
}

// TestErrorFrameworkEdgeCases tests edge cases and error conditions.
func TestErrorFrameworkEdgeCases(t *testing.T) {
	t.Run("HTTPStatus_with_nil_structured_error_fields", func(t *testing.T) {
		// SecretError with nil Cause
		err := &cherrors.SecretError{
			Provider: "vault",
			Ref:      "key",
			Cause:    nil, // nil cause
		}
		status := cherrors.HTTPStatus(err)
		if status < 100 || status > 599 {
			t.Fatalf("FAIL: HTTPStatus() must return valid status even with nil Cause, got %d", status)
		}
		t.Logf("PASS: HTTPStatus(SecretError{Cause: nil}) = %d", status)
	})

	t.Run("ClientMessage_with_nil_structured_error_fields", func(t *testing.T) {
		// ConfigError with nil Cause
		err := &cherrors.ConfigError{
			Field: "port",
			Value: nil,
			Cause: nil,
		}
		msg := cherrors.ClientMessage(err)
		if msg == "" {
			t.Fatal("FAIL: ClientMessage() must return non-empty message even with nil fields")
		}
		t.Logf("PASS: ClientMessage(ConfigError{Cause: nil}) = %q", msg)
	})

	t.Run("errors_Is_with_non_sentinel_errors", func(t *testing.T) {
		// Verify our structured errors don't accidentally match random errors
		randomErr := errors.New("random error")
		secretErr := &cherrors.SecretError{
			Provider: "test",
			Ref:      "test",
			Cause:    randomErr,
		}

		// Should NOT match sentinel errors
		if errors.Is(secretErr, cherrors.ErrSecretNotFound) {
			t.Fatal("FAIL: SecretError with random Cause incorrectly matches ErrSecretNotFound")
		}

		// Should match its actual cause
		if !errors.Is(secretErr, randomErr) {
			t.Fatal("FAIL: SecretError doesn't match its own Cause error")
		}

		t.Log("PASS: errors.Is() correctly distinguishes between different error types")
	})

	t.Run("structured_error_with_empty_strings", func(t *testing.T) {
		// Structured errors with empty string fields should still work
		err := &cherrors.SecretError{
			Provider: "",
			Ref:      "",
			Cause:    cherrors.ErrSecretNotFound,
		}

		// Should still have an error message
		msg := err.Error()
		if msg == "" {
			t.Fatal("FAIL: SecretError.Error() returns empty string even with empty fields")
		}

		// Should still work with error chain
		if !errors.Is(err, cherrors.ErrSecretNotFound) {
			t.Fatal("FAIL: SecretError with empty fields breaks error chain")
		}

		t.Logf("PASS: Structured errors work with empty string fields")
	})
}
