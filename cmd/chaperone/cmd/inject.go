package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/bmf/chaperone/internal/config"
	"github.com/bmf/chaperone/internal/log"
	"github.com/bmf/chaperone/internal/orchestrate"
	"github.com/bmf/chaperone/internal/shutdown"
	"github.com/spf13/cobra"
)

var (
	version    = "0.1.0" // Set during build via ldflags
	socketPath string    // CLI flag for Unix socket path
	httpMode   bool      // CLI flag to force HTTP/TCP mode
	httpPort   int       // CLI flag for HTTP port (implies HTTP mode)
	httpAddr   string    // CLI flag for HTTP address (implies HTTP mode)
)

// injectCmd represents the inject command
var injectCmd = &cobra.Command{
	Use:   "inject [service]",
	Short: "Start proxy with credential injection",
	Long: `Start the chaperone proxy server with credential injection.

When called without arguments, serves all services from config.
When called with a service name, serves only that specific service.

By default, Chaperone listens on a Unix socket (auto-generated path) for better security.
Use --http flag to enable TCP mode instead.

Examples:
  chaperone inject                    # Serve all services (Unix socket mode)
  chaperone inject openai             # Serve only the openai service
  chaperone inject --socket /run/chaperone/proxy.sock  # Custom socket path
  chaperone inject --http             # Use TCP mode (127.0.0.1:4010)
  chaperone inject --http --port 8080 # Use TCP mode on custom port`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInject,
}

func init() {
	rootCmd.AddCommand(injectCmd)
	injectCmd.Flags().StringVar(&socketPath, "socket", "", "Unix socket path (default: auto-generated)")
	injectCmd.Flags().BoolVar(&httpMode, "http", false, "Use HTTP/TCP mode instead of Unix socket")
	injectCmd.Flags().IntVar(&httpPort, "port", 0, "Port to listen on (implies --http, default 4010)")
	injectCmd.Flags().StringVar(&httpAddr, "addr", "", "Address to listen on (implies --http, default 127.0.0.1)")
}

func runInject(cmd *cobra.Command, args []string) error {
	// Create context for the application
	ctx := context.Background()

	// Determine service filter (if provided)
	var serviceFilter []string
	if len(args) > 0 {
		serviceFilter = []string{args[0]}
	}

	// Load configuration using shared getConfigPath()
	configPath, err := getConfigPath()
	if err != nil {
		return fmt.Errorf("failed to resolve config path: %w", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Apply CLI flags for transport mode
	orchestrate.ApplyTransportFlags(cfg, orchestrate.TransportFlags{
		SocketPath: socketPath,
		HTTPMode:   httpMode,
		HTTPPort:   httpPort,
		HTTPAddr:   httpAddr,
	})

	// Apply defaults and validate
	cfg.SetDefaults()
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// Set up logging based on config
	setupLogging(cfg)

	// Log startup with appropriate mode
	orchestrate.LogStartup(ctx, cfg, version, configPath, serviceFilter)

	// Create shutdown manager
	shutdownMgr := shutdown.NewManager(slog.Default())

	// Initialize CA for MITM
	_, caKeyPath, caCertPath, err := getCAPath()
	if err != nil {
		return fmt.Errorf("failed to get CA path: %w", err)
	}

	// Load or generate CA certificate with appropriate logging
	ca, _, err := orchestrate.InitializeCA(ctx, caKeyPath, caCertPath)
	if err != nil {
		return err
	}

	// Use orchestrate.Setup() for shared initialization logic
	setupCfg := orchestrate.SetupConfig{
		Config:       cfg,
		ServiceNames: serviceFilter,
		CAKeyPath:    caKeyPath,
		CACertPath:   caCertPath,
	}

	result, err := orchestrate.Setup(ctx, setupCfg, ca, slog.Default())
	if err != nil {
		return err
	}

	// Create proxy server with MITM support or transparent mode
	proxyServer := orchestrate.CreateProxy(ctx, cfg, slog.Default(), shutdownMgr, result)

	// Start proxy server
	if err := proxyServer.Start(); err != nil {
		return fmt.Errorf("failed to start proxy server: %w", err)
	}

	log.Info(ctx, "proxy server started successfully")

	log.Info(ctx, "waiting for shutdown signal (SIGTERM/SIGINT)")

	// Wait for shutdown signal
	if err := shutdownMgr.WaitForShutdown(); err != nil {
		log.Error(ctx, "error waiting for shutdown", err)
		return err
	}

	log.Info(ctx, "shutdown signal received, shutting down")

	// Execute shutdown with timeout
	if err := shutdownMgr.Shutdown(30 * time.Second); err != nil {
		log.Error(ctx, "error during shutdown", err)
		return err
	}

	log.Info(ctx, "chaperone stopped")
	return nil
}

// setupLogging configures the global logger based on configuration and format flag
func setupLogging(cfg *config.Config) {
	log.Setup(log.Config{
		Level:  cfg.Logging.Level,
		Format: log.Format(logFormat),
	})
}
