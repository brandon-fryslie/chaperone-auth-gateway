package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bmf/chaperone/internal/config"
	"github.com/spf13/cobra"
)

// TestRunInit validates the init command's runInit function
//
// This test suite validates:
// 1. Valid service (openai/anthropic) creates correct config file
// 2. Invalid service returns appropriate error
// 3. Overwrite existing file works with warning
func TestRunInit(t *testing.T) {
	t.Run("valid_service_openai", func(t *testing.T) {
		testRunInitValidService(t, "openai", "api.openai.com")
	})

	t.Run("valid_service_anthropic", func(t *testing.T) {
		testRunInitValidService(t, "anthropic", "api.anthropic.com")
	})

	t.Run("invalid_service_returns_error", func(t *testing.T) {
		testRunInitInvalidService(t)
	})

	t.Run("overwrite_existing_file", func(t *testing.T) {
		testRunInitOverwriteFile(t)
	})

	t.Run("file_permissions", func(t *testing.T) {
		testRunInitFilePermissions(t)
	})

	t.Run("file_content_is_valid_toml", func(t *testing.T) {
		testRunInitValidTOML(t)
	})
}

// testRunInitValidService verifies that init creates correct config file for valid service
func testRunInitValidService(t *testing.T, service, expectedHost string) {
	t.Helper()

	// Create temp directory for test
	tmpDir := t.TempDir()

	// Change to temp directory for test
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Logf("warning: failed to restore directory: %v", err)
		}
	}()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}

	// Create a test command with args
	testCmd := &cobra.Command{}
	args := []string{service}

	// Execute runInit
	err = runInit(testCmd, args)
	if err != nil {
		t.Fatalf("runInit(%s) failed: %v", service, err)
	}

	// Verify file was created
	configPath := filepath.Join(tmpDir, "chaperone.toml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatalf("config file not created at %s", configPath)
	}

	// Read and verify file content
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	contentStr := string(content)

	// Verify content contains expected service configuration
	if !strings.Contains(contentStr, expectedHost) {
		t.Errorf("config file should contain host %s, got:\n%s", expectedHost, contentStr)
	}

	// Verify content contains TOML structure
	if !strings.Contains(contentStr, "[server]") {
		t.Errorf("config file should contain [server] section")
	}

	if !strings.Contains(contentStr, "[logging]") {
		t.Errorf("config file should contain [logging] section")
	}

	if !strings.Contains(contentStr, "[services.") {
		t.Errorf("config file should contain [services.*] section")
	}

	t.Logf("Created valid config for %s with %d bytes", service, len(content))
}

// testRunInitInvalidService verifies that init returns error for invalid service
func testRunInitInvalidService(t *testing.T) {
	t.Helper()

	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Logf("warning: failed to restore directory: %v", err)
		}
	}()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}

	testCmd := &cobra.Command{}
	args := []string{"invalidservice"}

	// Execute runInit - should fail
	err = runInit(testCmd, args)
	if err == nil {
		t.Fatal("runInit with invalid service should return error")
	}

	// Verify error message mentions unsupported service
	errMsg := err.Error()
	if !strings.Contains(errMsg, "unsupported service") {
		t.Errorf("error message should mention 'unsupported service', got: %s", errMsg)
	}

	// Verify error message lists supported services
	if !strings.Contains(errMsg, "openai") || !strings.Contains(errMsg, "anthropic") {
		t.Errorf("error message should list supported services, got: %s", errMsg)
	}

	// Verify no config file was created
	configPath := filepath.Join(tmpDir, "chaperone.toml")
	if _, err := os.Stat(configPath); err == nil {
		t.Error("config file should not be created for invalid service")
	}
}

// testRunInitOverwriteFile verifies that init can overwrite existing file
func testRunInitOverwriteFile(t *testing.T) {
	t.Helper()

	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Logf("warning: failed to restore directory: %v", err)
		}
	}()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}

	// Create existing config file
	configPath := filepath.Join(tmpDir, "chaperone.toml")
	existingContent := "# existing config\n[server]\nport = 9999\n"
	if err := os.WriteFile(configPath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("failed to create existing config: %v", err)
	}

	testCmd := &cobra.Command{}
	args := []string{"openai"}

	// Execute runInit
	err = runInit(testCmd, args)
	if err != nil {
		t.Fatalf("runInit should succeed even with existing file: %v", err)
	}

	// Verify file was overwritten
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	contentStr := string(content)

	// Should contain new content, not old content
	if strings.Contains(contentStr, "port = 9999") {
		t.Error("file should be overwritten with new content")
	}

	if !strings.Contains(contentStr, "api.openai.com") {
		t.Error("overwritten file should contain openai config")
	}

	// File should be valid config
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Errorf("overwritten config should be valid TOML: %v", err)
	}
	if cfg != nil && len(cfg.Services) == 0 {
		t.Error("overwritten config should have services")
	}
}

// testRunInitFilePermissions verifies created file has correct permissions
func testRunInitFilePermissions(t *testing.T) {
	t.Helper()

	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Logf("warning: failed to restore directory: %v", err)
		}
	}()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}

	testCmd := &cobra.Command{}
	args := []string{"openai"}

	err = runInit(testCmd, args)
	if err != nil {
		t.Fatalf("runInit failed: %v", err)
	}

	// Check file permissions
	configPath := filepath.Join(tmpDir, "chaperone.toml")
	fileInfo, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("failed to stat config file: %v", err)
	}

	// File should be readable and writable by owner
	mode := fileInfo.Mode()

	// Verify it's readable by owner (at minimum)
	if mode&0400 == 0 {
		t.Error("config file should be readable by owner")
	}

	// Verify it's writable by owner
	if mode&0200 == 0 {
		t.Error("config file should be writable by owner")
	}

	t.Logf("Config file has permissions %v", mode)
}

// testRunInitValidTOML verifies the generated file is valid TOML
func testRunInitValidTOML(t *testing.T) {
	t.Helper()

	// Test both templates
	services := []string{"openai", "anthropic"}

	for _, service := range services {
		t.Run(service, func(t *testing.T) {
			tmpDir := t.TempDir()
			originalDir, err := os.Getwd()
			if err != nil {
				t.Fatalf("failed to get current directory: %v", err)
			}
			defer func() {
				if err := os.Chdir(originalDir); err != nil {
					t.Logf("warning: failed to restore directory: %v", err)
				}
			}()

			if err := os.Chdir(tmpDir); err != nil {
				t.Fatalf("failed to change to temp directory: %v", err)
			}

			testCmd := &cobra.Command{}
			args := []string{service}

			err = runInit(testCmd, args)
			if err != nil {
				t.Fatalf("runInit(%s) failed: %v", service, err)
			}

			configPath := filepath.Join(tmpDir, "chaperone.toml")

			// Try to load the config - this validates it's valid TOML
			cfg, err := config.Load(configPath)
			if err != nil {
				t.Fatalf("generated config is not valid TOML: %v", err)
			}

			// Verify config has expected structure
			if cfg.Server.Address == "" {
				t.Error("config should have server address")
			}

			if cfg.Server.Port == 0 {
				t.Error("config should have server port")
			}

			if cfg.Logging.Level == "" {
				t.Error("config should have logging level")
			}

			if len(cfg.Services) == 0 {
				t.Error("config should have at least one service")
			}

			t.Logf("Generated valid TOML for %s with %d services", service, len(cfg.Services))
		})
	}
}

// TestSupportedServices verifies the supported services map
func TestSupportedServices(t *testing.T) {
	// Verify openai template exists
	if template, ok := supportedServices["openai"]; !ok {
		t.Error("openai should be in supported services")
	} else if template == "" {
		t.Error("openai template should not be empty")
	}

	// Verify anthropic template exists
	if template, ok := supportedServices["anthropic"]; !ok {
		t.Error("anthropic should be in supported services")
	} else if template == "" {
		t.Error("anthropic template should not be empty")
	}

	// Verify templates are different
	if supportedServices["openai"] == supportedServices["anthropic"] {
		t.Error("openai and anthropic templates should be different")
	}

	// Verify templates contain expected content
	if !strings.Contains(supportedServices["openai"], "openai") {
		t.Error("openai template should mention openai")
	}

	if !strings.Contains(supportedServices["anthropic"], "anthropic") {
		t.Error("anthropic template should mention anthropic")
	}
}
