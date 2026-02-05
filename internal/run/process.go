package run

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/bmf/chaperone/internal/log"
)

// ProcessConfig contains all configuration needed to spawn a child process.
type ProcessConfig struct {
	Command     []string          // Command and arguments to execute
	ProxyURL    string            // Proxy URL with embedded credentials
	CACertPath  string            // Path to CA certificate file
	CAEnvVars   []string          // List of CA env vars to set (uses defaults if nil)
	EnvFile     string            // Optional: path to env file to load
	UserEnvVars map[string]string // Additional env vars from CLI (-e flags)
}

// ProcessResult contains handles to the running process.
type ProcessResult struct {
	Cmd         *exec.Cmd
	exitChannel chan int
}

// SpawnChild creates and starts a child process with configured environment.
// Returns immediately after starting the process.
// The child process:
// - Inherits stdin/stdout/stderr
// - Stays in same process group (no Setpgid) for proper signal handling
// - Has proxy and CA environment variables configured
func SpawnChild(ctx context.Context, cfg ProcessConfig) (*ProcessResult, error) {
	if len(cfg.Command) == 0 {
		return nil, fmt.Errorf("command is required")
	}

	// Build environment for child process
	childEnv, err := BuildProcessEnvironment(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to build environment: %w", err)
	}

	// Create command
	cmd := exec.Command(cfg.Command[0], cfg.Command[1:]...)
	cmd.Env = childEnv
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// No Setpgid - child stays in same process group for proper signal handling

	log.Info(ctx, "starting child process",
		"command", cfg.Command[0],
		"args", cfg.Command[1:],
	)

	// Start child process
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start child process: %w", err)
	}

	log.Info(ctx, "child process started successfully", "pid", cmd.Process.Pid)

	// Create result with exit channel
	result := &ProcessResult{
		Cmd:         cmd,
		exitChannel: make(chan int, 1),
	}

	// Start goroutine to wait for exit and send to channel
	go func() {
		err := cmd.Wait()
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = 1
			}
		}
		result.exitChannel <- exitCode
	}()

	return result, nil
}

// Wait blocks until the child process exits.
// Returns the exit code.
func (pr *ProcessResult) Wait(ctx context.Context) int {
	exitCode := <-pr.exitChannel
	log.Info(ctx, "child process exited", "exit_code", exitCode)
	return exitCode
}

// WaitWithSentinel waits for either the child process to exit or the sentinel channel to signal.
// If sentinel is triggered:
// - Sends SIGINT to child process
// - Waits up to 10 seconds for graceful shutdown
// - Kills process if still running
// - Returns exit code 0 (sentinel-triggered exit is considered success)
// If child exits naturally:
// - Returns the child's actual exit code
func (pr *ProcessResult) WaitWithSentinel(ctx context.Context, sentinelChan chan struct{}) int {
	select {
	case <-sentinelChan:
		// Sentinel found - terminate child gracefully
		log.Info(ctx, "sentinel value detected, terminating child process")
		if err := pr.Cmd.Process.Signal(os.Interrupt); err != nil {
			_ = pr.Cmd.Process.Kill()
		}
		// Wait for graceful shutdown or kill after timeout
		// The exitChannel will receive the exit code once process terminates
		<-pr.exitChannel
		log.Info(ctx, "child process terminated after sentinel detection")
		return 0 // Sentinel-triggered exit is success

	case exitCode := <-pr.exitChannel:
		// Child exited naturally
		log.Info(ctx, "child process exited", "exit_code", exitCode)
		return exitCode
	}
}

// BuildProcessEnvironment creates the environment for the child process.
// Sets proxy vars, loads env file, sets CA environment variables, and user vars.
func BuildProcessEnvironment(ctx context.Context, cfg ProcessConfig) ([]string, error) {
	envBuilder := NewEnvBuilder()
	envBuilder.InheritParent()

	// Load env_file if specified
	if cfg.EnvFile != "" {
		log.Info(ctx, "loading env file", "path", cfg.EnvFile)
		if err := envBuilder.LoadEnvFile(cfg.EnvFile); err != nil {
			return nil, fmt.Errorf("failed to load env file: %w", err)
		}
	}

	// Set proxy environment variables
	envBuilder.SetProxyVars(cfg.ProxyURL)

	// Set CA environment variables (use defaults if not specified)
	caEnvVars := cfg.CAEnvVars
	if caEnvVars == nil {
		// Use comprehensive defaults (SSL_CERT_FILE, REQUESTS_CA_BUNDLE, NODE_EXTRA_CA_CERTS, etc.)
		caEnvVars = []string{
			"SSL_CERT_FILE",
			"REQUESTS_CA_BUNDLE",
			"NODE_EXTRA_CA_CERTS",
			"HTTPX_CA_BUNDLE",
			"PERL_LWP_SSL_CA_FILE",
			"HTTPS_CA_FILE",
			"AWS_CA_BUNDLE",
			"HOMEBREW_CERTIFICATE_AUTHORITY",
			"GIT_SSL_CAINFO",
			"CURL_CA_BUNDLE",
		}
	}
	log.Info(ctx, "setting CA environment variables",
		"ca_cert", cfg.CACertPath,
		"ca_env_vars", caEnvVars,
	)
	envBuilder.SetCAEnvVars(cfg.CACertPath, caEnvVars)

	// Set user-provided environment variables
	for k, v := range cfg.UserEnvVars {
		envBuilder.Set(k, v)
	}

	return envBuilder.Build(), nil
}
