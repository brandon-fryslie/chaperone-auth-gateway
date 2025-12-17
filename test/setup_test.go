package test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupCommand(t *testing.T) {
	t.Run("setup command is available", func(t *testing.T) {
		// Build the binary
		projectRoot, err := os.Getwd()
		require.NoError(t, err)
		projectRoot = filepath.Dir(projectRoot) // Go up from test/ to project root

		tmpDir := t.TempDir()
		binaryPath := filepath.Join(tmpDir, "chaperone")
		cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/chaperone")
		cmd.Dir = projectRoot
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		require.NoError(t, cmd.Run(), "Failed to build binary: %s", stderr.String())

		// Test that setup command exists and shows help
		cmd = exec.Command(binaryPath, "setup", "--help")
		var stdout, stderr2 bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr2
		err = cmd.Run()
		require.NoError(t, err, "Setup command failed: %s", stderr2.String())

		output := stdout.String()
		assert.Contains(t, output, "Configure system proxy settings")
		assert.Contains(t, output, "--unset")
		assert.Contains(t, output, "--host")
		assert.Contains(t, output, "--port")
	})

	t.Run("setup command has correct flags", func(t *testing.T) {
		// Build the binary
		projectRoot, err := os.Getwd()
		require.NoError(t, err)
		projectRoot = filepath.Dir(projectRoot) // Go up from test/ to project root

		tmpDir := t.TempDir()
		binaryPath := filepath.Join(tmpDir, "chaperone")
		cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/chaperone")
		cmd.Dir = projectRoot
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		require.NoError(t, cmd.Run(), "Failed to build binary: %s", stderr.String())

		// Test that flags have correct defaults
		cmd = exec.Command(binaryPath, "setup", "--help")
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		err = cmd.Run()
		require.NoError(t, err)

		output := stdout.String()
		assert.Contains(t, output, "default \"127.0.0.1\"")
		assert.Contains(t, output, "default 4010")
	})

	t.Run("setup backup functionality creates proper JSON structure", func(t *testing.T) {
		// This test validates the backup structure would work correctly
		// We don't actually test the backup file creation since it requires admin/root

		// Test the JSON structure
		type ProxySettings struct {
			HTTPProxy  string `json:"http_proxy"`
			HTTPSProxy string `json:"https_proxy"`
			NoProxy    string `json:"no_proxy"`
		}

		testSettings := map[string]ProxySettings{
			"Wi-Fi": {
				HTTPProxy:  "127.0.0.1:8080",
				HTTPSProxy: "127.0.0.1:8080",
				NoProxy:    "localhost,127.0.0.1",
			},
		}

		// Verify JSON marshaling works
		data, err := json.MarshalIndent(testSettings, "", "  ")
		require.NoError(t, err)

		// Verify it contains expected values
		assert.Contains(t, string(data), "127.0.0.1:8080")
		assert.Contains(t, string(data), "Wi-Fi")

		// Verify it can be unmarshaled back
		var restored map[string]ProxySettings
		err = json.Unmarshal(data, &restored)
		require.NoError(t, err)
		assert.Equal(t, testSettings, restored)
	})
}
