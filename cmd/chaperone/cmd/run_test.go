package cmd

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/bmf/chaperone/internal/config"
	"github.com/bmf/chaperone/internal/log"
)

// TestSetupLogging validates the setupLogging helper function
//
// This test suite validates:
// 1. setupLogging() configures logger correctly for each level
// 2. Invalid log level defaults to info
func TestSetupLogging(t *testing.T) {
	t.Run("debug_level", func(t *testing.T) {
		testSetupLoggingLevel(t, "debug", slog.LevelDebug)
	})

	t.Run("info_level", func(t *testing.T) {
		testSetupLoggingLevel(t, "info", slog.LevelInfo)
	})

	t.Run("warn_level", func(t *testing.T) {
		testSetupLoggingLevel(t, "warn", slog.LevelWarn)
	})

	t.Run("error_level", func(t *testing.T) {
		testSetupLoggingLevel(t, "error", slog.LevelError)
	})

	t.Run("invalid_level_defaults_to_info", func(t *testing.T) {
		testSetupLoggingLevel(t, "invalid", slog.LevelInfo)
	})

	t.Run("empty_level_defaults_to_info", func(t *testing.T) {
		testSetupLoggingLevel(t, "", slog.LevelInfo)
	})

	t.Run("uppercase_level_defaults_to_info", func(t *testing.T) {
		// Levels should be lowercase; uppercase should default to info
		testSetupLoggingLevel(t, "DEBUG", slog.LevelInfo)
	})
}

// testSetupLoggingLevel verifies setupLogging configures correct log level
func testSetupLoggingLevel(t *testing.T, configLevel string, expectedLevel slog.Level) {
	t.Helper()

	// Create test config
	cfg := &config.Config{
		Logging: config.LoggingConfig{
			Level: configLevel,
		},
	}

	// Call setupLogging
	setupLogging(cfg)

	// Verify the log level was set correctly
	// We can check this by getting the current log level from our log package
	currentLevel := log.GetLevel()

	if currentLevel != expectedLevel {
		t.Errorf("expected log level %v, got %v", expectedLevel, currentLevel)
	}

	t.Logf("setupLogging(%q) correctly set level to %v", configLevel, currentLevel)
}

// TestGetCAPath validates the getCAPath helper function
//
// This test suite validates:
// 1. getCAPath() returns valid paths
// 2. Paths are absolute
// 3. Paths follow expected structure
func TestGetCAPath(t *testing.T) {
	t.Run("returns_valid_paths", func(t *testing.T) {
		testGetCAPathValid(t)
	})

	t.Run("paths_are_absolute", func(t *testing.T) {
		testGetCAPathAbsolute(t)
	})

	t.Run("paths_follow_structure", func(t *testing.T) {
		testGetCAPathStructure(t)
	})

	t.Run("paths_are_in_user_config", func(t *testing.T) {
		testGetCAPathInUserConfig(t)
	})
}

// testGetCAPathValid verifies getCAPath returns valid paths without error
func testGetCAPathValid(t *testing.T) {
	t.Helper()

	dir, keyPath, certPath, err := getCAPath()
	if err != nil {
		t.Fatalf("getCAPath should not return error: %v", err)
	}

	if dir == "" {
		t.Error("CA directory path should not be empty")
	}

	if keyPath == "" {
		t.Error("CA key path should not be empty")
	}

	if certPath == "" {
		t.Error("CA cert path should not be empty")
	}

	t.Logf("getCAPath returned valid paths:")
	t.Logf("  dir:  %s", dir)
	t.Logf("  key:  %s", keyPath)
	t.Logf("  cert: %s", certPath)
}

// testGetCAPathAbsolute verifies returned paths are absolute
func testGetCAPathAbsolute(t *testing.T) {
	t.Helper()

	dir, keyPath, certPath, err := getCAPath()
	if err != nil {
		t.Fatalf("getCAPath failed: %v", err)
	}

	if !filepath.IsAbs(dir) {
		t.Errorf("CA directory path should be absolute, got: %s", dir)
	}

	if !filepath.IsAbs(keyPath) {
		t.Errorf("CA key path should be absolute, got: %s", keyPath)
	}

	if !filepath.IsAbs(certPath) {
		t.Errorf("CA cert path should be absolute, got: %s", certPath)
	}
}

// testGetCAPathStructure verifies paths follow expected structure
func testGetCAPathStructure(t *testing.T) {
	t.Helper()

	dir, keyPath, certPath, err := getCAPath()
	if err != nil {
		t.Fatalf("getCAPath failed: %v", err)
	}

	// Verify key and cert paths are under the CA directory
	if filepath.Dir(keyPath) != dir {
		t.Errorf("key path should be in CA directory, got key=%s dir=%s", keyPath, dir)
	}

	if filepath.Dir(certPath) != dir {
		t.Errorf("cert path should be in CA directory, got cert=%s dir=%s", certPath, dir)
	}

	// Verify expected filenames
	if filepath.Base(keyPath) != "ca-key.pem" {
		t.Errorf("expected key filename 'ca-key.pem', got '%s'", filepath.Base(keyPath))
	}

	if filepath.Base(certPath) != "ca-cert.pem" {
		t.Errorf("expected cert filename 'ca-cert.pem', got '%s'", filepath.Base(certPath))
	}

	// Verify directory name
	if filepath.Base(dir) != "chaperone" {
		t.Errorf("expected directory name 'chaperone', got '%s'", filepath.Base(dir))
	}
}

// testGetCAPathInUserConfig verifies paths are in user config directory
func testGetCAPathInUserConfig(t *testing.T) {
	t.Helper()

	dir, _, _, err := getCAPath()
	if err != nil {
		t.Fatalf("getCAPath failed: %v", err)
	}

	// Implementation uses XDG convention: ~/.config/chaperone
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home directory: %v", err)
	}

	expectedDir := filepath.Join(homeDir, ".config", "chaperone")

	if dir != expectedDir {
		t.Errorf("expected CA directory to be %s, got %s", expectedDir, dir)
	}
}

// TestGetCAPathConsistency verifies getCAPath returns consistent results
func TestGetCAPathConsistency(t *testing.T) {
	// Call getCAPath multiple times
	dir1, key1, cert1, err1 := getCAPath()
	if err1 != nil {
		t.Fatalf("first getCAPath call failed: %v", err1)
	}

	dir2, key2, cert2, err2 := getCAPath()
	if err2 != nil {
		t.Fatalf("second getCAPath call failed: %v", err2)
	}

	// Verify results are identical
	if dir1 != dir2 {
		t.Errorf("getCAPath should return consistent directory, got %s and %s", dir1, dir2)
	}

	if key1 != key2 {
		t.Errorf("getCAPath should return consistent key path, got %s and %s", key1, key2)
	}

	if cert1 != cert2 {
		t.Errorf("getCAPath should return consistent cert path, got %s and %s", cert1, cert2)
	}

	t.Log("getCAPath returns consistent results across multiple calls")
}

// TestRunCommandFlags verifies the run command has required flags
func TestRunCommandFlags(t *testing.T) {
	t.Run("config_flag_inherited", func(t *testing.T) {
		// The --config flag is a persistent flag on rootCmd, inherited by runCmd
		// Check via InheritedFlags which includes parent persistent flags
		flag := rootCmd.PersistentFlags().Lookup("config")
		if flag == nil {
			t.Fatal("root command should have --config persistent flag")
		}

		if flag.Shorthand != "c" {
			t.Errorf("config flag should have shorthand 'c', got '%s'", flag.Shorthand)
		}

		t.Logf("--config flag (inherited): %s (shorthand: -%s)", flag.Usage, flag.Shorthand)
	})

	t.Run("socket_flag_exists", func(t *testing.T) {
		// The run command has its own --socket flag
		flag := runCmd.Flags().Lookup("socket")
		if flag == nil {
			t.Fatal("run command should have --socket flag")
		}

		t.Logf("--socket flag: %s", flag.Usage)
	})
}

// TestRunCommandProperties verifies run command configuration
func TestRunCommandProperties(t *testing.T) {
	t.Run("use_field", func(t *testing.T) {
		if runCmd.Use != "run" {
			t.Errorf("expected Use='run', got '%s'", runCmd.Use)
		}
	})

	t.Run("short_description", func(t *testing.T) {
		if runCmd.Short == "" {
			t.Error("Short description should not be empty")
		}
	})

	t.Run("long_description", func(t *testing.T) {
		if runCmd.Long == "" {
			t.Error("Long description should not be empty")
		}
	})

	t.Run("has_run_function", func(t *testing.T) {
		if runCmd.RunE == nil && runCmd.Run == nil {
			t.Error("run command should have a Run or RunE function")
		}
	})
}
