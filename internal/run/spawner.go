package run

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

// ProcessManager manages the lifecycle of a child process.
type ProcessManager struct {
	cmd      *exec.Cmd
	fdConfig *FDConfig
	ctx      context.Context
	cancel   context.CancelFunc
	exitCode int
	waitDone chan struct{} // closed when process exits
}

// NewProcessManager creates a new process manager.
// The process is not started until Start() is called.
func NewProcessManager(ctx context.Context, command string, args []string, env []string, fdConfig *FDConfig) (*ProcessManager, error) {
	if command == "" {
		return nil, fmt.Errorf("command cannot be empty")
	}

	// Create cancellable context for timeout handling
	pmCtx, cancel := context.WithCancel(ctx)

	// Create command with context
	cmd := exec.CommandContext(pmCtx, command, args...)
	cmd.Env = env

	// Set up file descriptors
	cmd.Stdin = fdConfig.Stdin
	cmd.Stdout = fdConfig.Stdout
	cmd.Stderr = fdConfig.Stderr

	// Set process group for signal isolation
	// This allows us to send signals to just the child process
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	return &ProcessManager{
		cmd:      cmd,
		fdConfig: fdConfig,
		ctx:      pmCtx,
		cancel:   cancel,
		waitDone: make(chan struct{}),
	}, nil
}

// Start starts the child process.
func (pm *ProcessManager) Start() error {
	if err := pm.cmd.Start(); err != nil {
		pm.cancel()
		return fmt.Errorf("failed to start process: %w", err)
	}

	// Start goroutine to wait for process exit
	go pm.waitForExit()

	return nil
}

// waitForExit waits for the process to exit and captures the exit code.
func (pm *ProcessManager) waitForExit() {
	defer close(pm.waitDone)

	err := pm.cmd.Wait()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			pm.exitCode = exitErr.ExitCode()
		} else {
			// Process was killed or never started properly
			pm.exitCode = -1
		}
	} else {
		pm.exitCode = 0
	}
}

// Wait blocks until the process exits and returns the exit code.
func (pm *ProcessManager) Wait() int {
	<-pm.waitDone
	return pm.exitCode
}

// IsRunning returns true if the process is still running.
// Uses non-blocking channel check - idiomatic Go pattern.
func (pm *ProcessManager) IsRunning() bool {
	select {
	case <-pm.waitDone:
		return false
	default:
		return true
	}
}

// Signal sends a signal to the child process.
func (pm *ProcessManager) Signal(sig syscall.Signal) error {
	if pm.cmd.Process == nil {
		return fmt.Errorf("process not started")
	}

	return pm.cmd.Process.Signal(sig)
}

// GracefulStop attempts to gracefully stop the child process.
// Sends SIGTERM, waits up to timeout, then sends SIGKILL if needed.
func (pm *ProcessManager) GracefulStop(timeout time.Duration) error {
	// Check if already exited using non-blocking channel check
	select {
	case <-pm.waitDone:
		return nil // Already exited
	default:
	}

	// Send SIGTERM
	if err := pm.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send SIGTERM: %w", err)
	}

	// Wait for exit with timeout
	select {
	case <-pm.waitDone:
		// Process exited gracefully
		return nil
	case <-time.After(timeout):
		// Timeout expired, force kill
		if err := pm.Signal(syscall.SIGKILL); err != nil {
			return fmt.Errorf("failed to send SIGKILL: %w", err)
		}
		// Wait for kill to complete
		<-pm.waitDone
		return fmt.Errorf("process did not exit gracefully within %v, forced kill", timeout)
	}
}

// Cleanup closes file descriptors and cancels context.
// Should be called after Wait() returns.
func (pm *ProcessManager) Cleanup() error {
	pm.cancel()
	return pm.fdConfig.Close()
}
