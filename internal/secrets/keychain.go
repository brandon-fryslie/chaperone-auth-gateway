package secrets

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/bmf/chaperone/internal/errors"
)

// KeychainProvider fetches secrets from macOS Keychain.
// It is safe for concurrent use (exec.Command is concurrent-safe).
//
// Only works on macOS. Returns an error on other platforms.
type KeychainProvider struct{}

// NewKeychainProvider creates a new macOS Keychain provider.
func NewKeychainProvider() *KeychainProvider {
	return &KeychainProvider{}
}

// Fetch retrieves a secret from macOS Keychain.
//
// The path parameter should be in the format "service/account":
//   - "chaperone/openai" → service="chaperone", account="openai"
//   - "myapp/api-key" → service="myapp", account="api-key"
//
// Platform Requirements:
//   - Only works on macOS (darwin)
//   - Returns error on Linux, Windows, etc.
//
// Keychain Access:
//   - Uses `security find-generic-password` command
//   - Respects keychain access controls
//   - May prompt user for keychain access if not previously authorized
//
// Returns:
//   - ErrSecretNotFound if the keychain item doesn't exist
//   - Error if not on macOS
//   - Error if path format is invalid (must be "service/account")
//   - Error if keychain access is denied
//
// Respects context cancellation by checking ctx.Done() before retrieval.
func (p *KeychainProvider) Fetch(ctx context.Context, path string) (string, error) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	// Platform check - only works on macOS
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("keychain provider only works on macOS (current platform: %s)", runtime.GOOS)
	}

	// Parse service/account from path
	service, account, err := parseKeychainPath(path)
	if err != nil {
		return "", err
	}

	// Execute security command to fetch password
	// security find-generic-password -s <service> -a <account> -w
	// -s: service name
	// -a: account name
	// -w: output password only (no other metadata)
	//
	// Note: service and account are from user configuration (not user input)
	// and are validated by parseKeychainPath above
	cmd := exec.CommandContext(ctx, "security", "find-generic-password", "-s", service, "-a", account, "-w") //nolint:gosec // service/account from config, validated above

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if item not found
		outputStr := string(output)
		if strings.Contains(outputStr, "could not be found") {
			return "", errors.ErrSecretNotFound
		}

		// Other errors (access denied, etc.)
		return "", fmt.Errorf("failed to fetch from keychain: %w (output: %s)", err, strings.TrimSpace(outputStr))
	}

	// Trim whitespace from password
	password := strings.TrimSpace(string(output))

	// Empty passwords are treated as not found
	if password == "" {
		return "", errors.ErrSecretNotFound
	}

	return password, nil
}

// parseKeychainPath parses "service/account" format into service and account components.
//
// Examples:
//   - "chaperone/openai" → service="chaperone", account="openai"
//   - "myapp/api-key" → service="myapp", account="api-key"
//
// Returns error if:
//   - path is empty
//   - path contains no slash
//   - service is empty (e.g., "/account")
//   - account is empty (e.g., "service/")
func parseKeychainPath(path string) (service, account string, err error) {
	if path == "" {
		return "", "", fmt.Errorf("empty keychain path")
	}

	// Split on first slash only
	idx := strings.Index(path, "/")
	if idx == -1 {
		return "", "", fmt.Errorf("invalid keychain path format: missing slash separator (expected 'service/account')")
	}

	service = path[:idx]
	account = path[idx+1:]

	if service == "" {
		return "", "", fmt.Errorf("invalid keychain path format: empty service (expected 'service/account')")
	}

	if account == "" {
		return "", "", fmt.Errorf("invalid keychain path format: empty account (expected 'service/account')")
	}

	return service, account, nil
}
