package run

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bmf/chaperone/internal/config"
	"github.com/bmf/chaperone/internal/log"
)

// PrepareRunConfig validates and prepares the RunConfig for execution.
// Handles CLI command override and variable expansion.
func PrepareRunConfig(cfg *config.Config, serviceName string, cliCommand []string) (*config.ServiceConfig, error) {
	svc, ok := cfg.Services[serviceName]
	if !ok {
		return nil, fmt.Errorf("service %q not found in config", serviceName)
	}

	// Handle CLI command override or require config RunConfig
	if len(cliCommand) > 0 {
		// CLI command provided: create RunConfig if needed
		if svc.Run == nil {
			svc.Run = &config.RunConfig{}
		}
		// Override with CLI command
		svc.Run.Command = cliCommand[0]
		svc.Run.Args = cliCommand[1:]
	} else {
		// No CLI command: require RunConfig from config file
		if svc.Run == nil {
			return nil, fmt.Errorf("service %q does not have a [services.%s.run] section and no command provided via CLI", serviceName, serviceName)
		}
	}

	// Expand variables in RunConfig
	if err := config.ExpandRunConfig(svc.Run); err != nil {
		return nil, fmt.Errorf("failed to expand variables in run config: %w", err)
	}

	return &svc, nil
}

// BuildChildEnvironment creates the environment for the child process.
// Sets proxy vars, loads env file, and sets CA environment variables.
func BuildChildEnvironment(ctx context.Context, svc *config.ServiceConfig, serviceName, proxyAddress, caCertPath string) ([]string, error) {
	envBuilder := NewEnvBuilder()
	envBuilder.InheritParent()

	// Load env_file if specified
	if svc.Run.EnvFile != "" {
		log.Info(ctx, "loading env file", "path", svc.Run.EnvFile)
		if err := envBuilder.LoadEnvFile(svc.Run.EnvFile); err != nil {
			return nil, fmt.Errorf("failed to load env file: %w", err)
		}
	}

	// Set proxy environment variables
	envBuilder.SetProxyVars(proxyAddress, serviceName)

	// Set CA environment variables
	log.Info(ctx, "setting CA environment variables",
		"ca_cert", caCertPath,
		"ca_env_vars", svc.Run.CAEnvVars,
	)
	envBuilder.SetCAEnvVars(caCertPath, svc.Run.CAEnvVars)

	return envBuilder.Build(), nil
}

// RunWithSignals runs a process with signal forwarding and returns its exit code.
// Handles SIGTERM/SIGINT by forwarding to child and waiting for graceful exit.
func RunWithSignals(ctx context.Context, pm *ProcessManager) int {
	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	// Wait for either child exit or signal
	exitChan := make(chan int, 1)
	go func() {
		exitCode := pm.Wait()
		exitChan <- exitCode
	}()

	var childExitCode int
	select {
	case sig := <-sigChan:
		log.Info(ctx, "received signal, forwarding to child", "signal", sig)
		// Forward signal to child
		if err := pm.Signal(sig.(syscall.Signal)); err != nil {
			log.Error(ctx, "failed to forward signal to child", err)
		}
		// Wait for child to exit (with timeout)
		childExitCode = pm.Wait()
		log.Info(ctx, "child process exited", "exit_code", childExitCode)

	case exitCode := <-exitChan:
		log.Info(ctx, "child process exited naturally", "exit_code", exitCode)
		childExitCode = exitCode
	}

	return childExitCode
}

// CreateTempLogFile creates a temporary log file for run mode logging.
// Returns the open file and its path. The caller is responsible for closing the file.
func CreateTempLogFile() (*os.File, string, error) {
	// Create temp file in system temp directory
	f, err := os.CreateTemp("", "chaperone-run-*.log")
	if err != nil {
		return nil, "", fmt.Errorf("failed to create temporary log file: %w", err)
	}
	return f, f.Name(), nil
}

// SetupLoggingToFile sets up logging to a specific file.
func SetupLoggingToFile(cfg *config.Config, logFormat string, writer *os.File) {
	log.Setup(log.Config{
		Level:  cfg.Logging.Level,
		Format: log.Format(logFormat),
		Output: writer,
	})
}

// CleanupProcess performs cleanup and shutdown for run mode.
// Returns the child exit code to be used for os.Exit().
func CleanupProcess(ctx context.Context, pm *ProcessManager, shutdownMgr interface {
	Shutdown(timeout time.Duration) error
}, childExitCode int) int {
	// Clean up child process resources
	if err := pm.Cleanup(); err != nil {
		log.Error(ctx, "error cleaning up process manager", err)
	}

	// Stop proxy server
	log.Info(ctx, "stopping proxy server")
	if err := shutdownMgr.Shutdown(10 * time.Second); err != nil {
		log.Error(ctx, "error shutting down proxy", err)
	}

	log.Info(ctx, "chaperone run mode complete", "exit_code", childExitCode)
	return childExitCode
}
