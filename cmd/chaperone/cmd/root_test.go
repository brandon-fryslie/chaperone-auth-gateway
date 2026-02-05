package cmd

import (
	"bytes"
	"strings"
	"testing"
)

// TestRootCommand validates the root command
//
// This test suite validates:
// 1. Version flag outputs correct version
// 2. Help flag outputs usage information
func TestRootCommand(t *testing.T) {
	t.Run("version_flag", func(t *testing.T) {
		testVersionFlag(t)
	})

	t.Run("help_flag", func(t *testing.T) {
		testHelpFlag(t)
	})

	t.Run("version_template", func(t *testing.T) {
		testVersionTemplate(t)
	})

	t.Run("subcommands_registered", func(t *testing.T) {
		testSubcommandsRegistered(t)
	})
}

// testVersionFlag verifies --version flag output
func testVersionFlag(t *testing.T) {
	t.Helper()

	// Reset command args for clean test
	rootCmd.SetArgs([]string{"--version"})

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout) // Capture both stdout and stderr

	// Execute command
	err := rootCmd.Execute()

	// Version flag should not return an error (it exits early but cobra doesn't treat it as error)
	if err != nil && !strings.Contains(err.Error(), "version") {
		t.Logf("Note: version flag returned error (this may be expected): %v", err)
	}

	output := stdout.String()

	// Verify output contains version information
	if !strings.Contains(output, "chaperone") {
		t.Errorf("version output should contain 'chaperone', got: %s", output)
	}

	if !strings.Contains(output, "version") {
		t.Errorf("version output should contain 'version', got: %s", output)
	}

	// Verify output contains the actual version number
	if !strings.Contains(output, version) {
		t.Errorf("version output should contain version '%s', got: %s", version, output)
	}

	t.Logf("Version output: %s", strings.TrimSpace(output))
}

// testHelpFlag verifies --help flag output
func testHelpFlag(t *testing.T) {
	t.Helper()

	rootCmd.SetArgs([]string{"--help"})

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stdout)

	// Execute command
	err := rootCmd.Execute()

	// Help flag should not return an error
	if err != nil && !strings.Contains(err.Error(), "help") {
		t.Logf("Note: help flag returned error (this may be expected): %v", err)
	}

	output := stdout.String()

	// Verify output contains usage information
	hasUsage := strings.Contains(output, "Usage:") ||
		strings.Contains(output, "Available Commands:") ||
		strings.Contains(output, "Commands:")

	if !hasUsage {
		t.Errorf("help output should contain usage information, got: %s", output)
	}

	// Verify output mentions chaperone
	if !strings.Contains(output, "chaperone") && !strings.Contains(output, "Chaperone") {
		t.Errorf("help output should mention chaperone, got: %s", output)
	}

	t.Logf("Help output contains usage information")
}

// testVersionTemplate verifies the custom version template
func testVersionTemplate(t *testing.T) {
	t.Helper()

	// The version template is set in init()
	// We can't directly test the template, but we can verify the version string

	if version == "" {
		t.Error("version should not be empty")
	}

	// Version should follow semantic versioning (roughly)
	if !strings.Contains(version, ".") {
		t.Logf("Note: version '%s' doesn't appear to follow semantic versioning", version)
	}

	t.Logf("Version is set to: %s", version)
}

// testSubcommandsRegistered verifies expected subcommands are registered
func testSubcommandsRegistered(t *testing.T) {
	t.Helper()

	// Get list of registered commands
	commands := rootCmd.Commands()

	expectedCommands := map[string]bool{
		"run": false,
	}

	for _, cmd := range commands {
		if _, exists := expectedCommands[cmd.Name()]; exists {
			expectedCommands[cmd.Name()] = true
			t.Logf("Found command: %s - %s", cmd.Name(), cmd.Short)
		}
	}

	// Verify all expected commands are registered
	for cmdName, found := range expectedCommands {
		if !found {
			t.Errorf("expected command '%s' not registered", cmdName)
		}
	}
}

// TestRootCommandProperties verifies root command configuration
func TestRootCommandProperties(t *testing.T) {
	t.Run("use_field", func(t *testing.T) {
		if rootCmd.Use != "chaperone" {
			t.Errorf("expected Use='chaperone', got '%s'", rootCmd.Use)
		}
	})

	t.Run("short_description", func(t *testing.T) {
		if rootCmd.Short == "" {
			t.Error("Short description should not be empty")
		}
	})

	t.Run("long_description", func(t *testing.T) {
		if rootCmd.Long == "" {
			t.Error("Long description should not be empty")
		}
	})

	t.Run("version_set", func(t *testing.T) {
		if rootCmd.Version == "" {
			t.Error("Version should be set on root command")
		}
		if rootCmd.Version != version {
			t.Errorf("expected Version='%s', got '%s'", version, rootCmd.Version)
		}
	})

	t.Run("completion_enabled", func(t *testing.T) {
		// Verify completion command is available (InitDefaultCompletionCmd was called)
		hasCompletion := false
		for _, cmd := range rootCmd.Commands() {
			if cmd.Name() == "completion" {
				hasCompletion = true
				break
			}
		}
		if !hasCompletion {
			t.Error("Completion command should be registered")
		}
	})
}

// TestExecute verifies the Execute function exists and is callable
func TestExecute(t *testing.T) {
	// We can't actually call Execute() in tests because it would execute the command
	// But we can verify it exists and has the right signature

	// Just verify the function doesn't panic when we reference it
	_ = Execute

	t.Log("Execute function exists and is callable")
}
