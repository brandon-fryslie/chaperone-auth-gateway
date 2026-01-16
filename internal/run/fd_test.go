package run

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewFDConfig(t *testing.T) {
	tests := []struct {
		name       string
		stdin      string
		stdout     string
		stderr     string
		wantErr    bool
		checkFiles bool // Whether to check that files were created
	}{
		{
			name:    "inherit all",
			stdin:   "inherit",
			stdout:  "inherit",
			stderr:  "inherit",
			wantErr: false,
		},
		{
			name:    "empty defaults to inherit",
			stdin:   "",
			stdout:  "",
			stderr:  "",
			wantErr: false,
		},
		{
			name:    "discard all",
			stdin:   "discard",
			stdout:  "discard",
			stderr:  "discard",
			wantErr: false,
		},
		{
			name:       "file output",
			stdin:      "inherit",
			stdout:     "file:/tmp/test_stdout.log",
			stderr:     "file:/tmp/test_stderr.log",
			wantErr:    false,
			checkFiles: true,
		},
		{
			name:    "mixed modes",
			stdin:   "inherit",
			stdout:  "inherit",
			stderr:  "discard",
			wantErr: false,
		},
		{
			name:    "invalid stdin mode",
			stdin:   "invalid",
			stdout:  "inherit",
			stderr:  "inherit",
			wantErr: true,
		},
		{
			name:    "invalid stdout mode",
			stdin:   "inherit",
			stdout:  "invalid",
			stderr:  "inherit",
			wantErr: true,
		},
		{
			name:    "invalid stderr mode",
			stdin:   "inherit",
			stdout:  "inherit",
			stderr:  "invalid",
			wantErr: true,
		},
		{
			name:    "empty file path",
			stdin:   "inherit",
			stdout:  "file:",
			stderr:  "inherit",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up test files if needed
			if tt.checkFiles {
				defer os.Remove("/tmp/test_stdout.log")
				defer os.Remove("/tmp/test_stderr.log")
			}

			config, err := NewFDConfig(tt.stdin, tt.stdout, tt.stderr)
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewFDConfig() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("NewFDConfig() unexpected error: %v", err)
				return
			}

			if config.Stdin == nil {
				t.Error("Stdin is nil")
			}
			if config.Stdout == nil {
				t.Error("Stdout is nil")
			}
			if config.Stderr == nil {
				t.Error("Stderr is nil")
			}

			// Clean up
			if err := config.Close(); err != nil {
				t.Errorf("Close() error: %v", err)
			}

			// Check that files were created
			if tt.checkFiles {
				if _, err := os.Stat("/tmp/test_stdout.log"); os.IsNotExist(err) {
					t.Error("stdout file was not created")
				}
				if _, err := os.Stat("/tmp/test_stderr.log"); os.IsNotExist(err) {
					t.Error("stderr file was not created")
				}
			}
		})
	}
}

func TestParseFDMode(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		wantErr bool
		wantNil bool
	}{
		{
			name:    "inherit mode",
			mode:    "inherit",
			wantErr: false,
		},
		{
			name:    "empty mode defaults to inherit",
			mode:    "",
			wantErr: false,
		},
		{
			name:    "discard mode",
			mode:    "discard",
			wantErr: false,
		},
		{
			name:    "file mode with path",
			mode:    "file:/tmp/test.log",
			wantErr: false,
		},
		{
			name:    "invalid mode",
			mode:    "invalid",
			wantErr: true,
		},
		{
			name:    "file mode without path",
			mode:    "file:",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up test file if needed
			if tt.mode == "file:/tmp/test.log" {
				defer os.Remove("/tmp/test.log")
			}

			writer, err := parseFDMode(tt.mode, os.Stdout)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseFDMode() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("parseFDMode() unexpected error: %v", err)
				return
			}

			if writer == nil {
				t.Error("writer is nil")
			}

			// Close if it's a file
			if closer, ok := writer.(io.Closer); ok && writer != os.Stdout {
				closer.Close()
			}
		})
	}
}

func TestFDConfig_Close(t *testing.T) {
	tmpDir := t.TempDir()
	testFile1 := filepath.Join(tmpDir, "out1.log")
	testFile2 := filepath.Join(tmpDir, "out2.log")

	// Create files
	f1, err := os.OpenFile(testFile1, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	f2, err := os.OpenFile(testFile2, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	config := &FDConfig{
		Stdin:  os.Stdin,
		Stdout: f1,
		Stderr: f2,
	}

	// Close should work
	if err := config.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}

	// Verify files are closed by trying to write (should fail)
	if _, err := f1.Write([]byte("test")); err == nil {
		t.Error("File should be closed but write succeeded")
	}

	// Double close should be safe
	if err := config.Close(); err != nil {
		t.Errorf("Second Close() should not error: %v", err)
	}
}

func TestFDConfig_CloseInheritedFDs(t *testing.T) {
	// Config with inherited FDs (os.Stdin/Stdout/Stderr)
	config := &FDConfig{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}

	// Close should not close os.Stdin/Stdout/Stderr
	if err := config.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}

	// Verify os.Stdin still works
	if _, err := os.Stdin.Read(make([]byte, 0)); err != nil {
		t.Error("os.Stdin should still be open")
	}
}

func TestFDConfig_CloseDiscard(t *testing.T) {
	// Config with discard (io.Discard doesn't need closing)
	config := &FDConfig{
		Stdin:  io.NopCloser(strings.NewReader("")),
		Stdout: io.Discard,
		Stderr: io.Discard,
	}

	// Close should not error
	if err := config.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}
}
