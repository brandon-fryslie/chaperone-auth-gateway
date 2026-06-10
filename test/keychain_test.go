package test

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"testing"

	"github.com/bmf/chaperone/internal/errors"
	"github.com/bmf/chaperone/internal/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPhase7_KeychainProvider is the comprehensive test suite for Phase 7 Keychain Provider.
//
// This test suite validates:
// - CHAP-xxx: macOS Keychain secret provider implementation
//
// Gaming Resistance: These tests cannot be satisfied by stubs because:
// 1. Platform detection tests use REAL runtime.GOOS
// 2. Path parsing tests use actual string parsing logic
// 3. Keychain tests use REAL `security` command (on macOS)
// 4. Tests verify ACTUAL error messages and types
// 5. Context cancellation uses real context handling
//
// An AI cannot fake these tests with mocks - the real keychain provider must work.
func TestPhase7_KeychainProvider(t *testing.T) {
	t.Run("platform_check", testKeychainPlatformCheck)
	t.Run("path_parsing", testKeychainPathParsing)
	t.Run("context_cancellation", testKeychainContextCancellation)

	// Only run macOS-specific tests on macOS
	if runtime.GOOS == "darwin" {
		t.Run("keychain_integration", testKeychainIntegration)
	}
}

// ============================================================================
// Platform Check Tests
// ============================================================================

// testKeychainPlatformCheck validates that keychain provider only works on macOS.
//
// Gaming Resistance:
// - Uses REAL runtime.GOOS check
// - Verifies actual platform detection
// - Cannot be satisfied by skipping platform check
func testKeychainPlatformCheck(t *testing.T) {
	t.Run("non_macos_platforms", testKeychainNonMacOSPlatforms)
}

func testKeychainNonMacOSPlatforms(t *testing.T) {
	// This test validates that non-macOS platforms return an error.
	// Gaming Resistance:
	// - Tests actual platform check using runtime.GOOS
	// - Verifies error message contains platform information

	if runtime.GOOS == "darwin" {
		t.Skip("Skipping non-macOS test on macOS")
	}

	provider := secrets.NewKeychainProvider()
	_, err := provider.Fetch(context.Background(), "chaperone/openai")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "keychain provider only works on macOS")
	assert.Contains(t, err.Error(), runtime.GOOS)
}

// ============================================================================
// Path Parsing Tests
// ============================================================================

// testKeychainPathParsing validates the service/account path format parsing.
//
// Gaming Resistance:
// - Tests use actual string parsing, not mocks
// - Multiple edge cases tested to ensure robust implementation
// - Cannot be satisfied by hardcoded responses
func testKeychainPathParsing(t *testing.T) {
	t.Run("valid_path", testKeychainValidPath)
	t.Run("empty_path", testKeychainEmptyPath)
	t.Run("missing_slash", testKeychainMissingSlash)
	t.Run("empty_service", testKeychainEmptyService)
	t.Run("empty_account", testKeychainEmptyAccount)
	t.Run("multiple_slashes", testKeychainMultipleSlashes)
}

func testKeychainValidPath(t *testing.T) {
	// This test validates parsing of valid service/account paths.
	// Gaming Resistance:
	// - Tests actual parsing logic
	// - Verifies service and account are extracted correctly

	if runtime.GOOS != "darwin" {
		t.Skip("Keychain tests only run on macOS")
	}

	provider := secrets.NewKeychainProvider()

	// Test with non-existent keychain item (will fail at fetch, not parse)
	_, err := provider.Fetch(context.Background(), "test-service/test-account")
	require.Error(t, err)
	// Should fail with "not found", not parsing error
	assert.ErrorIs(t, err, errors.ErrSecretNotFound)
}

func testKeychainEmptyPath(t *testing.T) {
	// This test validates that empty paths return an error.
	// Gaming Resistance:
	// - Tests actual error handling for empty input

	if runtime.GOOS != "darwin" {
		t.Skip("Keychain tests only run on macOS")
	}

	provider := secrets.NewKeychainProvider()
	_, err := provider.Fetch(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty keychain path")
}

func testKeychainMissingSlash(t *testing.T) {
	// This test validates that paths without slash separator return an error.
	// Gaming Resistance:
	// - Tests actual validation logic

	if runtime.GOOS != "darwin" {
		t.Skip("Keychain tests only run on macOS")
	}

	provider := secrets.NewKeychainProvider()
	_, err := provider.Fetch(context.Background(), "chaperone-openai")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing slash separator")
}

func testKeychainEmptyService(t *testing.T) {
	// This test validates that paths with empty service return an error.
	// Gaming Resistance:
	// - Tests actual validation logic

	if runtime.GOOS != "darwin" {
		t.Skip("Keychain tests only run on macOS")
	}

	provider := secrets.NewKeychainProvider()
	_, err := provider.Fetch(context.Background(), "/account")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty service")
}

func testKeychainEmptyAccount(t *testing.T) {
	// This test validates that paths with empty account return an error.
	// Gaming Resistance:
	// - Tests actual validation logic

	if runtime.GOOS != "darwin" {
		t.Skip("Keychain tests only run on macOS")
	}

	provider := secrets.NewKeychainProvider()
	_, err := provider.Fetch(context.Background(), "service/")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty account")
}

func testKeychainMultipleSlashes(t *testing.T) {
	// This test validates handling of paths with multiple slashes.
	// Gaming Resistance:
	// - Tests actual parsing logic for edge cases

	if runtime.GOOS != "darwin" {
		t.Skip("Keychain tests only run on macOS")
	}

	provider := secrets.NewKeychainProvider()

	// Path with multiple slashes should split on first slash only
	// "service/account/extra" → service="service", account="account/extra"
	_, err := provider.Fetch(context.Background(), "test/account/extra")
	require.Error(t, err)
	// Should fail with "not found" (valid path format, item doesn't exist)
	assert.ErrorIs(t, err, errors.ErrSecretNotFound)
}

// ============================================================================
// Context Cancellation Tests
// ============================================================================

// testKeychainContextCancellation validates context cancellation is respected.
//
// Gaming Resistance:
// - Uses real context cancellation
// - Verifies actual context.Done() handling
func testKeychainContextCancellation(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Keychain tests only run on macOS")
	}

	provider := secrets.NewKeychainProvider()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := provider.Fetch(ctx, "chaperone/openai")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// ============================================================================
// Keychain Integration Tests (macOS only)
// ============================================================================

// testKeychainIntegration validates actual keychain access on macOS.
//
// Gaming Resistance:
// - Uses REAL macOS `security` command
// - Creates and reads REAL keychain items
// - Verifies actual keychain integration
// - Cannot be satisfied by mocks
func testKeychainIntegration(t *testing.T) {
	t.Run("fetch_existing_item", testKeychainFetchExisting)
	t.Run("fetch_missing_item", testKeychainFetchMissing)
	t.Run("whitespace_trimmed", testKeychainWhitespaceTrimmed)
}

func testKeychainFetchExisting(t *testing.T) {
	// This test validates fetching an existing keychain item.
	// Gaming Resistance:
	// - Creates REAL keychain item using `security` command
	// - Fetches REAL password from keychain
	// - Cleans up keychain item after test

	// Create a test keychain item
	service := "chaperone-test"
	account := "test-account"
	password := "sk-test-key-12345"

	// Add to keychain
	cleanup := addKeychainItem(t, service, account, password)
	defer cleanup()

	// Fetch through the registry — the production path — since the provider
	// returns the security tool's output verbatim and Registry.Fetch owns
	// normalization.
	registry := secrets.NewRegistry()
	registry.Register("keychain", secrets.NewKeychainProvider())
	secret, err := registry.Fetch(context.Background(), "keychain:"+service+"/"+account)
	require.NoError(t, err)
	assert.Equal(t, password, secret)
}

func testKeychainFetchMissing(t *testing.T) {
	// This test validates fetching a missing keychain item.
	// Gaming Resistance:
	// - Attempts to fetch non-existent item
	// - Verifies ErrSecretNotFound is returned

	provider := secrets.NewKeychainProvider()
	_, err := provider.Fetch(context.Background(), "nonexistent-service/nonexistent-account")
	require.Error(t, err)
	assert.ErrorIs(t, err, errors.ErrSecretNotFound)
}

func testKeychainWhitespaceTrimmed(t *testing.T) {
	// This test validates whitespace trimming from keychain passwords.
	// Gaming Resistance:
	// - Creates REAL keychain item with trailing newline (security command adds it)
	// - Verifies actual whitespace trimming
	//
	// Trimming is the registry's job (the provider returns the security
	// tool's output verbatim), so the fetch goes through Registry.Fetch —
	// the same path the proxy uses.

	// Create a test keychain item
	service := "chaperone-test-whitespace"
	account := "test-account"
	password := "sk-test-key-with-spaces"

	// Add to keychain
	cleanup := addKeychainItem(t, service, account, password)
	defer cleanup()

	registry := secrets.NewRegistry()
	registry.Register("keychain", secrets.NewKeychainProvider())
	secret, err := registry.Fetch(context.Background(), "keychain:"+service+"/"+account)
	require.NoError(t, err)
	assert.Equal(t, password, secret)
	assert.NotContains(t, secret, "\n")
	assert.NotContains(t, secret, " ")
}

// ============================================================================
// Test Helpers
// ============================================================================

// addKeychainItem adds a generic password to the macOS keychain and returns a cleanup function.
// This helper is used to create test keychain items for integration tests.
func addKeychainItem(t *testing.T, service, account, password string) func() {
	t.Helper()

	// Skip if not on macOS
	if runtime.GOOS != "darwin" {
		t.Skip("Keychain operations only supported on macOS")
	}

	// Skip if running in CI environment (may not have keychain access)
	if os.Getenv("CI") != "" {
		t.Skip("Skipping keychain test in CI environment")
	}

	// Add item to keychain
	// security add-generic-password -s <service> -a <account> -w <password> -U
	// -U: update if exists
	cmd := exec.Command("security", "add-generic-password", "-s", service, "-a", account, "-w", password, "-U")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to add keychain item: %v", err)
	}

	// Return cleanup function
	return func() {
		// Delete item from keychain
		// security delete-generic-password -s <service> -a <account>
		cmd := exec.Command("security", "delete-generic-password", "-s", service, "-a", account)
		_ = cmd.Run() // Ignore errors - item may not exist
	}
}
