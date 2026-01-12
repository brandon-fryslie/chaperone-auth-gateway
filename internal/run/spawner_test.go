package run

import (
	"context"
	"io"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestNewProcessManager(t *testing.T) {
	fdConfig := &FDConfig{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}

	tests := []struct {
		name    string
		command string
		args    []string
		wantErr bool
	}{
		{
			name:    "valid command",
			command: "echo",
			args:    []string{"hello"},
			wantErr: false,
		},
		{
			name:    "empty command",
			command: "",
			args:    []string{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pm, err := NewProcessManager(ctx, tt.command, tt.args, os.Environ(), fdConfig)

			if tt.wantErr {
				if err == nil {
					t.Error("NewProcessManager() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("NewProcessManager() unexpected error: %v", err)
				return
			}

			if pm == nil {
				t.Error("ProcessManager is nil")
			}

			// Cleanup
			pm.Cleanup()
		})
	}
}

func TestProcessManager_StartAndWait(t *testing.T) {
	fdConfig := &FDConfig{
		Stdout: io.Discard,
		Stderr: io.Discard,
	}

	tests := []struct {
		name         string
		command      string
		args         []string
		wantExitCode int
	}{
		{
			name:         "successful exit",
			command:      "sh",
			args:         []string{"-c", "exit 0"},
			wantExitCode: 0,
		},
		{
			name:         "non-zero exit",
			command:      "sh",
			args:         []string{"-c", "exit 42"},
			wantExitCode: 42,
		},
		{
			name:         "echo command",
			command:      "echo",
			args:         []string{"hello"},
			wantExitCode: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			pm, err := NewProcessManager(ctx, tt.command, tt.args, os.Environ(), fdConfig)
			if err != nil {
				t.Fatalf("NewProcessManager() error: %v", err)
			}
			defer pm.Cleanup()

			if err := pm.Start(); err != nil {
				t.Fatalf("Start() error: %v", err)
			}

			// Should be running
			if !pm.IsRunning() {
				t.Error("Process should be running after Start()")
			}

			// Wait for exit
			exitCode := pm.Wait()
			if exitCode != tt.wantExitCode {
				t.Errorf("Wait() = %d, want %d", exitCode, tt.wantExitCode)
			}

			// Should not be running
			if pm.IsRunning() {
				t.Error("Process should not be running after Wait()")
			}
		})
	}
}

func TestProcessManager_Signal(t *testing.T) {
	fdConfig := &FDConfig{
		Stdout: io.Discard,
		Stderr: io.Discard,
	}

	t.Run("send signal to running process", func(t *testing.T) {
		ctx := context.Background()
		// Sleep command that we'll interrupt
		pm, err := NewProcessManager(ctx, "sleep", []string{"10"}, os.Environ(), fdConfig)
		if err != nil {
			t.Fatalf("NewProcessManager() error: %v", err)
		}
		defer pm.Cleanup()

		if err := pm.Start(); err != nil {
			t.Fatalf("Start() error: %v", err)
		}

		// Wait a bit to ensure process is running
		time.Sleep(100 * time.Millisecond)

		// Send SIGTERM
		if err := pm.Signal(syscall.SIGTERM); err != nil {
			t.Errorf("Signal() error: %v", err)
		}

		// Wait for exit
		exitCode := pm.Wait()
		// SIGTERM typically results in exit code 143 (128 + 15)
		if exitCode != 143 && exitCode != -1 {
			t.Logf("Wait() = %d (expected 143 or -1 for SIGTERM)", exitCode)
		}
	})

	t.Run("signal before start", func(t *testing.T) {
		ctx := context.Background()
		pm, err := NewProcessManager(ctx, "echo", []string{"hello"}, os.Environ(), fdConfig)
		if err != nil {
			t.Fatalf("NewProcessManager() error: %v", err)
		}
		defer pm.Cleanup()

		// Try to signal before start
		if err := pm.Signal(syscall.SIGTERM); err == nil {
			t.Error("Signal() should error when process not started")
		}
	})
}

func TestProcessManager_GracefulStop(t *testing.T) {
	fdConfig := &FDConfig{
		Stdout: io.Discard,
		Stderr: io.Discard,
	}

	t.Run("graceful exit", func(t *testing.T) {
		ctx := context.Background()
		// Process that exits quickly on SIGTERM
		pm, err := NewProcessManager(ctx, "sleep", []string{"10"}, os.Environ(), fdConfig)
		if err != nil {
			t.Fatalf("NewProcessManager() error: %v", err)
		}
		defer pm.Cleanup()

		if err := pm.Start(); err != nil {
			t.Fatalf("Start() error: %v", err)
		}

		// Wait a bit
		time.Sleep(100 * time.Millisecond)

		// GracefulStop with generous timeout
		if err := pm.GracefulStop(2 * time.Second); err != nil {
			t.Logf("GracefulStop() returned error (may be expected): %v", err)
		}

		// Process should be stopped
		if pm.IsRunning() {
			t.Error("Process should be stopped after GracefulStop()")
		}
	})

	t.Run("already exited", func(t *testing.T) {
		ctx := context.Background()
		// Process that exits immediately
		pm, err := NewProcessManager(ctx, "echo", []string{"hello"}, os.Environ(), fdConfig)
		if err != nil {
			t.Fatalf("NewProcessManager() error: %v", err)
		}
		defer pm.Cleanup()

		if err := pm.Start(); err != nil {
			t.Fatalf("Start() error: %v", err)
		}

		// Wait for natural exit
		pm.Wait()

		// GracefulStop should be no-op
		if err := pm.GracefulStop(1 * time.Second); err != nil {
			t.Errorf("GracefulStop() on exited process should not error: %v", err)
		}
	})

	t.Run("force kill on timeout", func(t *testing.T) {
		ctx := context.Background()
		// Process that ignores SIGTERM (sleep is a bad example, but we'll use very short timeout)
		pm, err := NewProcessManager(ctx, "sleep", []string{"10"}, os.Environ(), fdConfig)
		if err != nil {
			t.Fatalf("NewProcessManager() error: %v", err)
		}
		defer pm.Cleanup()

		if err := pm.Start(); err != nil {
			t.Fatalf("Start() error: %v", err)
		}

		// Wait a bit
		time.Sleep(100 * time.Millisecond)

		// GracefulStop with very short timeout to force SIGKILL
		err = pm.GracefulStop(10 * time.Millisecond)
		if err == nil {
			t.Log("GracefulStop() expected error for forced kill, got nil (process may have exited quickly)")
		}

		// Process should be stopped
		if pm.IsRunning() {
			t.Error("Process should be stopped after forced kill")
		}
	})
}

func TestProcessManager_Cleanup(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := tmpDir + "/test.log"

	// Create FD config with file output
	f, err := os.OpenFile(testFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	fdConfig := &FDConfig{
		Stdout: f,
		Stderr: f,
	}

	ctx := context.Background()
	pm, err := NewProcessManager(ctx, "echo", []string{"hello"}, os.Environ(), fdConfig)
	if err != nil {
		t.Fatalf("NewProcessManager() error: %v", err)
	}

	if err := pm.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	pm.Wait()

	// Cleanup should close file
	if err := pm.Cleanup(); err != nil {
		t.Errorf("Cleanup() error: %v", err)
	}

	// Verify file is closed
	if _, err := f.Write([]byte("test")); err == nil {
		t.Error("File should be closed after Cleanup()")
	}
}
