package init

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestToEnvVarName(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		want        string
	}{
		{
			name:        "simple lowercase",
			serviceName: "openai",
			want:        "OPENAI_API_KEY",
		},
		{
			name:        "with hyphens",
			serviceName: "my-service",
			want:        "MY_SERVICE_API_KEY",
		},
		{
			name:        "with underscores",
			serviceName: "foo_bar",
			want:        "FOO_BAR_API_KEY",
		},
		{
			name:        "mixed case",
			serviceName: "OpenAI",
			want:        "OPENAI_API_KEY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toEnvVarName(tt.serviceName)
			if got != tt.want {
				t.Errorf("toEnvVarName(%q) = %q, want %q", tt.serviceName, got, tt.want)
			}
		})
	}
}

func TestWriteToFile(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "chaperone-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Override home directory for test
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	serviceName := "testservice"
	credentialValue := "test-secret-123"

	credentialRef, err := writeToFile(serviceName, credentialValue)
	if err != nil {
		t.Fatalf("writeToFile() error = %v", err)
	}

	// Verify credential ref format
	expectedPath := filepath.Join(tmpDir, ".config", "chaperone", "secrets", serviceName)
	expectedRef := "file:" + expectedPath
	if credentialRef != expectedRef {
		t.Errorf("credentialRef = %q, want %q", credentialRef, expectedRef)
	}

	// Verify file exists and has correct content
	content, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("Failed to read credential file: %v", err)
	}

	if string(content) != credentialValue {
		t.Errorf("File content = %q, want %q", string(content), credentialValue)
	}

	// Verify file permissions (should be 0600)
	stat, err := os.Stat(expectedPath)
	if err != nil {
		t.Fatalf("Failed to stat credential file: %v", err)
	}

	mode := stat.Mode()
	// On Unix, check exact permissions
	if runtime.GOOS != "windows" {
		if mode.Perm() != 0600 {
			t.Errorf("File permissions = %o, want 0600", mode.Perm())
		}
	}
}

func TestWriteToEnvFile(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "chaperone-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Change to temp directory
	originalWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(originalWd)

	serviceName := "testservice"
	credentialValue := "test-secret-456"

	credentialRef, err := writeToEnvFile(serviceName, credentialValue)
	if err != nil {
		t.Fatalf("writeToEnvFile() error = %v", err)
	}

	// Verify credential ref format
	expectedRef := "env:TESTSERVICE_API_KEY"
	if credentialRef != expectedRef {
		t.Errorf("credentialRef = %q, want %q", credentialRef, expectedRef)
	}

	// Verify .env file exists and has correct content
	envPath := filepath.Join(tmpDir, ".env")
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("Failed to read .env file: %v", err)
	}

	contentStr := string(content)

	// Verify comment and entry
	if !strings.Contains(contentStr, "# testservice API key") {
		t.Error(".env missing comment")
	}
	if !strings.Contains(contentStr, "TESTSERVICE_API_KEY=test-secret-456") {
		t.Error(".env missing key=value entry")
	}

	// Test appending to existing .env
	serviceName2 := "another-service"
	credentialValue2 := "another-secret-789"

	credentialRef2, err := writeToEnvFile(serviceName2, credentialValue2)
	if err != nil {
		t.Fatalf("writeToEnvFile() (second) error = %v", err)
	}

	expectedRef2 := "env:ANOTHER_SERVICE_API_KEY"
	if credentialRef2 != expectedRef2 {
		t.Errorf("credentialRef2 = %q, want %q", credentialRef2, expectedRef2)
	}

	// Verify both entries exist
	content, err = os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("Failed to read .env file (second): %v", err)
	}

	contentStr = string(content)

	if !strings.Contains(contentStr, "TESTSERVICE_API_KEY=test-secret-456") {
		t.Error(".env missing first entry after append")
	}
	if !strings.Contains(contentStr, "ANOTHER_SERVICE_API_KEY=another-secret-789") {
		t.Error(".env missing second entry after append")
	}
}

// TestWriteToKeychain is skipped on non-macOS platforms
func TestWriteToKeychain(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Keychain tests only run on macOS")
	}

	// Note: This test will actually write to the user's keychain
	// We use a test-specific service name to avoid conflicts
	serviceName := "chaperone-test-temp"
	credentialValue := "test-credential-xyz"

	credentialRef, err := writeToKeychain(serviceName, credentialValue)
	if err != nil {
		t.Fatalf("writeToKeychain() error = %v", err)
	}

	// Verify credential ref format
	expectedRef := "keychain:chaperone/" + serviceName
	if credentialRef != expectedRef {
		t.Errorf("credentialRef = %q, want %q", credentialRef, expectedRef)
	}

	// Clean up: delete the test keychain entry
	// (This is best effort - if it fails, the test keychain entry remains)
	t.Cleanup(func() {
		deleteCmd := exec.Command("security", "delete-generic-password", "-s", "chaperone", "-a", serviceName)
		_ = deleteCmd.Run()
	})
}
