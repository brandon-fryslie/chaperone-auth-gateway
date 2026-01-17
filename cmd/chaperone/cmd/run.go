package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bmf/chaperone/internal/config"
	"github.com/bmf/chaperone/internal/log"
	"github.com/bmf/chaperone/internal/orchestrate"
	"github.com/bmf/chaperone/internal/run"
	"github.com/bmf/chaperone/internal/shutdown"
	"github.com/spf13/cobra"
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run <service-name> [-- <command> <arg1> <arg2> ...]",
	Short: "Spawn application with proxy environment",
	Long: `Spawn and manage an application process with automatic proxy configuration.

This command:
1. Starts a proxy server on a Unix socket
2. Spawns the configured application with proxy environment variables
3. Manages the lifecycle of both proxy and application
4. Forwards signals (SIGTERM) to the application
5. Exits with the application's exit code

The application can be configured in [services.<name>.run] section of the config,
or specified on the command line after the '--' separator.

Security:
  The proxy generates a fresh CA certificate for each invocation,
  stored in /tmp/chaperone-ca-<pid>/. This CA is only trusted by
  the spawned application (via environment variables) and is
  automatically deleted when the proxy exits.

  No system-wide CA trust is required.

Examples:
  chaperone run openai                          # Run the openai service (uses config)
  chaperone run -c custom.toml myservice        # Run with custom config
  chaperone run openai -- python script.py      # Override config with CLI command
  chaperone run zai -- claude --dangerously-skip-permissions`,
	Args: cobra.MinimumNArgs(1),
	RunE: runWithProxy,
}

func init() {
	rootCmd.AddCommand(runCmd)
}

func runWithProxy(cmd *cobra.Command, args []string) error {
	serviceName := args[0]
	ctx := context.Background()

	// Check if CLI command is provided after '--'
	var cliCommand []string
	if len(args) > 1 && args[1] == "--" {
		// CLI command provided: "run service -- command arg1 arg2"
		cliCommand = args[2:]
	} else if len(args) > 1 {
		// Invalid syntax: arguments without '--' separator
		return fmt.Errorf("invalid syntax: extra arguments must be preceded by '--' separator\n" +
			"Usage: chaperone run <service> -- <command> <arg1> ...",
		)
	}

	// Load and merge configs (user + project)
	cfg, err := config.LoadWithMerge()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Look up service
	svc, ok := cfg.Services[serviceName]
	if !ok {
		return fmt.Errorf("service %q not found in config", serviceName)
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
		// Update service in map
		cfg.Services[serviceName] = svc
	} else {
		// No CLI command: require RunConfig from config file
		if svc.Run == nil {
			return fmt.Errorf("service %q does not have a [services.%s.run] section and no command provided via CLI", serviceName, serviceName)
		}
	}

	// Expand variables in RunConfig
	if err := config.ExpandRunConfig(svc.Run); err != nil {
		return fmt.Errorf("failed to expand variables in run config: %w", err)
	}

	// Update service in map (necessary because Services is a map of values)
	cfg.Services[serviceName] = svc

	// Generate socket path (use proxy PID, so we use our own PID)
	socketPath := svc.Run.SocketPath
	if socketPath == "" {
		socketPath = run.GenerateSocketPath(serviceName, os.Getpid())
	}

	// Override server socket configuration
	cfg.Server.Socket = socketPath
	cfg.Server.Port = 0 // Disable TCP mode

	// Apply defaults and validate
	cfg.SetDefaults()
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// Create temporary log file for run mode
	logFile, logPath, err := createTempLogFile()
	if err != nil {
		return fmt.Errorf("failed to create temporary log file: %w", err)
	}

	// Set up logging to the temporary file
	setupLoggingToWriter(cfg, logFile)

	log.Info(ctx, "chaperone run mode starting",
		"version", version,
		"service", serviceName,
		"socket", socketPath,
		"command", svc.Run.Command,
	)

	// Create shutdown manager
	shutdownMgr := shutdown.NewManager(slog.Default())

	// Register cleanup callback to close log file on exit
	shutdownMgr.Register(func(ctx context.Context) error {
		log.Info(ctx, "closing temporary log file", "path", logPath)
		return logFile.Close()
	})

	// Initialize ephemeral CA with cleanup
	ca, caKeyPath, caCertPath, err := orchestrate.InitializeEphemeralCA(ctx, os.Getpid(), shutdownMgr)
	if err != nil {
		return err
	}

	// Use orchestrate.Setup() for shared initialization logic
	setupCfg := orchestrate.SetupConfig{
		Config:       cfg,
		ServiceNames: []string{serviceName},
		CAKeyPath:    caKeyPath,
		CACertPath:   caCertPath,
	}

	result, err := orchestrate.Setup(ctx, setupCfg, ca, slog.Default())
	if err != nil {
		return err
	}

	// Create and start proxy server
	proxyServer := orchestrate.CreateProxy(ctx, cfg, slog.Default(), shutdownMgr, result)

	log.Info(ctx, "starting proxy server", "socket", socketPath)
	if err := proxyServer.Start(); err != nil {
		return fmt.Errorf("failed to start proxy server: %w", err)
	}

	log.Info(ctx, "proxy server started successfully")

	// Build environment for child process
	envBuilder := run.NewEnvBuilder()
	envBuilder.InheritParent()

	// Load env_file if specified
	if svc.Run.EnvFile != "" {
		log.Info(ctx, "loading env file", "path", svc.Run.EnvFile)
		if err := envBuilder.LoadEnvFile(svc.Run.EnvFile); err != nil {
			shutdownMgr.Shutdown(5 * time.Second)
			return fmt.Errorf("failed to load env file: %w", err)
		}
	}

	// Set proxy environment variables
	envBuilder.SetProxyVars(socketPath, serviceName)

	// Set CA environment variables
	log.Info(ctx, "setting CA environment variables",
		"ca_cert", caCertPath,
		"ca_env_vars", svc.Run.CAEnvVars,
	)
	envBuilder.SetCAEnvVars(caCertPath, svc.Run.CAEnvVars)

	// Create FD config with stdin, stdout, stderr
	// Default stdin to "inherit" for interactive applications
	stdinMode := "inherit"
	if svc.Run.Stdin != "" {
		stdinMode = svc.Run.Stdin
	}
	fdConfig, err := run.NewFDConfig(stdinMode, svc.Run.Stdout, svc.Run.Stderr)
	if err != nil {
		shutdownMgr.Shutdown(5 * time.Second)
		return fmt.Errorf("failed to create FD config: %w", err)
	}

	// Create process manager
	pm, err := run.NewProcessManager(ctx, svc.Run.Command, svc.Run.Args, envBuilder.Build(), fdConfig)
	if err != nil {
		shutdownMgr.Shutdown(5 * time.Second)
		return fmt.Errorf("failed to create process manager: %w", err)
	}

	// Start child process
	log.Info(ctx, "starting child process",
		"command", svc.Run.Command,
		"args", svc.Run.Args,
	)
	if err := pm.Start(); err != nil {
		shutdownMgr.Shutdown(5 * time.Second)
		return fmt.Errorf("failed to start child process: %w", err)
	}

	log.Info(ctx, "child process started successfully")

	// Print log file path to stderr before passing control to child
	// This informs the user where to find logs without polluting stdout
	fmt.Fprintf(os.Stderr, "Log file: %s\n", logPath)

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

	// Exit with child's exit code
	os.Exit(childExitCode)
	return nil // Never reached
}

// createTempLogFile creates a temporary log file for run mode logging.
// Returns the open file and its path. The caller is responsible for closing the file.
func createTempLogFile() (*os.File, string, error) {
	// Create temp file in system temp directory
	f, err := os.CreateTemp("", "chaperone-run-*.log")
	if err != nil {
		return nil, "", fmt.Errorf("failed to create temporary log file: %w", err)
	}
	return f, f.Name(), nil
}

// setupLoggingToWriter sets up logging to a specific writer.
// Similar to setupLogging but allows specifying the output destination.
func setupLoggingToWriter(cfg *config.Config, writer *os.File) {
	log.Setup(log.Config{
		Level:  cfg.Logging.Level,
		Format: log.Format(logFormat),
		Output: writer,
	})
}
