package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
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
1. Starts a proxy server on 127.0.0.1 with OS-allocated port
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

	// Prepare run config with CLI override and variable expansion
	svc, err := run.PrepareRunConfig(cfg, serviceName, cliCommand)
	if err != nil {
		return err
	}

	// Apply defaults and validate
	cfg.SetDefaults()
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// Create temporary log file for run mode
	logFile, logPath, err := run.CreateTempLogFile()
	if err != nil {
		return fmt.Errorf("failed to create temporary log file: %w", err)
	}

	// Set up logging to the temporary file
	run.SetupLoggingToFile(cfg, logFormat, logFile)

	log.Info(ctx, "chaperone run mode starting",
		"version", version,
		"service", serviceName,
		"address", cfg.Server.Address,
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

	log.Info(ctx, "starting proxy server", "address", cfg.Server.Address)
	if err := proxyServer.Start(); err != nil {
		return fmt.Errorf("failed to start proxy server: %w", err)
	}

	log.Info(ctx, "proxy server started successfully")

	// Get the actual port assigned by the OS
	actualAddr := proxyServer.Addr()
	proxyAddress := fmt.Sprintf("http://%s", actualAddr)

	// Build environment for child process
	childEnv, err := run.BuildChildEnvironment(ctx, svc, serviceName, proxyAddress, caCertPath)
	if err != nil {
		shutdownMgr.Shutdown(5 * time.Second)
		return err
	}

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
	pm, err := run.NewProcessManager(ctx, svc.Run.Command, svc.Run.Args, childEnv, fdConfig)
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

	// Run process with signal forwarding
	childExitCode := run.RunWithSignals(ctx, pm)

	// Cleanup and exit
	exitCode := run.CleanupProcess(ctx, pm, shutdownMgr, childExitCode)
	os.Exit(exitCode)
	return nil // Never reached
}
