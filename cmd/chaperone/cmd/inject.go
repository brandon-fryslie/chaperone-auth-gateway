package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/bmf/chaperone/internal/config"
	"github.com/bmf/chaperone/internal/log"
	"github.com/bmf/chaperone/internal/orchestrate"
	"github.com/bmf/chaperone/internal/proxy"
	"github.com/bmf/chaperone/internal/shutdown"
	"github.com/spf13/cobra"
)

var (
	version = "0.1.0" // Set during build via ldflags
)

// injectCmd represents the inject command
var injectCmd = &cobra.Command{
	Use:   "inject [service]",
	Short: "Start proxy with credential injection",
	Long: `Start the chaperone proxy server with credential injection.

When called without arguments, serves all services from config.
When called with a service name, serves only that specific service.

Always listens on 127.0.0.1 with OS-allocated port for security.

Examples:
  chaperone inject                    # Serve all services
  chaperone inject openai             # Serve only the openai service`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInject,
}

func init() {
	rootCmd.AddCommand(injectCmd)
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

	// Create the daemon's single audit sink, shared by the proxy and the control
	// plane so grant events and injection events land in one trail.
	auditLogger, err := orchestrate.CreateAuditLogger(cfg, shutdownMgr)
	if err != nil {
		return err
	}

	// Generate the per-run proxy access secret. The injecting proxy refuses to
	// start without one — anything that can reach the listener would otherwise
	// get real credentials attached for free.
	proxySecret, err := proxy.GenerateProxySecret()
	if err != nil {
		return fmt.Errorf("failed to generate proxy secret: %w", err)
	}

	// Create proxy server with MITM support or transparent mode
	proxyServer, err := orchestrate.CreateProxy(ctx, cfg, slog.Default(), shutdownMgr, result, proxySecret, auditLogger)
	if err != nil {
		return err
	}

	// Start proxy server
	if err := proxyServer.Start(); err != nil {
		return fmt.Errorf("failed to start proxy server: %w", err)
	}

	log.Info(ctx, "proxy server started successfully")

	// Surface the credentialed proxy URL — it is the only way through the gate,
	// and it changes every run.
	proxyURL := proxyServer.ProxyURL()
	fmt.Printf("\nProxy listening on: %s\n", proxyServer.Addr())
	fmt.Printf("Access requires this per-run proxy credential (regenerated on every start):\n")
	fmt.Printf("  export HTTP_PROXY=%s\n", proxyURL)
	fmt.Printf("  export HTTPS_PROXY=%s\n\n", proxyURL)

	// Bring up the localhost-only control plane so grants can be applied/revoked
	// at runtime without a restart.
	socketPath, err := getControlSocketPath()
	if err != nil {
		return err
	}
	if err := orchestrate.StartControlPlane(ctx, result, auditLogger, socketPath, shutdownMgr, slog.Default()); err != nil {
		return err
	}

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
