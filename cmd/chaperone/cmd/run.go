package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/bmf/chaperone/internal/auth"
	"github.com/bmf/chaperone/internal/config"
	"github.com/bmf/chaperone/internal/log"
	"github.com/bmf/chaperone/internal/mitm"
	"github.com/bmf/chaperone/internal/proxy"
	"github.com/bmf/chaperone/internal/secrets"
	"github.com/bmf/chaperone/internal/service"
	"github.com/bmf/chaperone/internal/shutdown"
	"github.com/spf13/cobra"
)

var (
	configPath string
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Start the chaperone proxy server",
	Long: `Start the chaperone proxy server with the specified configuration.

The proxy will listen on the configured address and port, and forward requests
to upstream services with injected authentication credentials.

Example:
  chaperone run --config chaperone.toml`,
	RunE: runServer,
}

func init() {
	rootCmd.AddCommand(runCmd)

	// Add config flag
	runCmd.Flags().StringVarP(&configPath, "config", "c", "", "path to configuration file (required)")
	_ = runCmd.MarkFlagRequired("config") //nolint:errcheck // Cobra handles missing required flag at runtime
}

func runServer(cmd *cobra.Command, args []string) error {
	// Create context for the application
	ctx := context.Background()

	// Load configuration
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

	// Log startup
	log.Info(ctx, "chaperone starting",
		"version", version,
		"config", configPath,
		"address", cfg.Server.Address,
		"port", cfg.Server.Port,
	)

	// Create shutdown manager
	shutdownMgr := shutdown.NewManager(slog.Default())

	// Initialize CA for MITM
	caDir, caKeyPath, caCertPath, err := getCAPath()
	if err != nil {
		return fmt.Errorf("failed to get CA path: %w", err)
	}

	// Ensure CA directory exists
	if err := os.MkdirAll(caDir, 0700); err != nil {
		return fmt.Errorf("failed to create CA directory: %w", err)
	}

	// Load or generate CA certificate
	ca, err := mitm.LoadOrGenerateCA(caKeyPath, caCertPath)
	if err != nil {
		return fmt.Errorf("failed to initialize CA: %w", err)
	}

	log.Info(ctx, "CA certificate initialized",
		"cert_path", caCertPath,
		"key_path", caKeyPath,
	)
	log.Info(ctx, "Trust this CA in your browser/system to avoid certificate warnings",
		"cert_path", caCertPath,
	)

	// Create certificate cache
	certCache := mitm.NewCertCache(ca)

	// Create service registry
	registry := service.NewRegistry()

	// Load services from config
	serviceCount := 0
	headerStrategies := make(map[string]bool) // Track header strategies to register
	for name, svcCfg := range cfg.Services {
		// Determine auth strategy reference
		// For "header" strategy, use "header:<header_name>" format
		authStrategyRef := svcCfg.AuthStrategy
		if svcCfg.AuthStrategy == "header" && svcCfg.HeaderName != "" {
			authStrategyRef = "header:" + svcCfg.HeaderName
			headerStrategies[svcCfg.HeaderName] = true
		}

		// Convert config.ServiceConfig to service.Service
		svc := &service.Service{
			HostPattern:     svcCfg.HostPattern,
			AuthStrategyRef: authStrategyRef,
			HeaderName:      svcCfg.HeaderName,
			CredentialRef:   svcCfg.CredentialRef,
			Policy: &service.Policy{
				AllowedMethods: svcCfg.AllowedMethods,
				AllowedPaths:   svcCfg.AllowedPaths,
				MaxBodyBytes:   svcCfg.MaxBodyBytes,
				ClientGroups:   svcCfg.ClientGroups,
			},
		}

		// Apply policy defaults
		svc.Policy.ApplyDefaults()

		// Register service
		if err := registry.Register(svc); err != nil {
			log.Error(ctx, "failed to register service", err,
				"service", name,
				"host_pattern", svcCfg.HostPattern,
			)
			return fmt.Errorf("failed to register service %s: %w", name, err)
		}

		serviceCount++
		log.Info(ctx, "registered service",
			"name", name,
			"host_pattern", svcCfg.HostPattern,
			"auth_strategy", authStrategyRef,
		)
	}

	log.Info(ctx, "MITM enabled for configured services",
		"service_count", serviceCount,
	)

	// Initialize secret and auth registries
	secretRegistry := secrets.NewRegistry()
	authRegistry := auth.NewRegistry()

	// Register built-in secret providers
	secretRegistry.Register("env", secrets.NewEnvProvider())
	secretRegistry.Register("file", secrets.NewFileProvider())
	secretRegistry.Register("keychain", secrets.NewKeychainProvider())

	// Register built-in auth strategies
	authRegistry.Register("bearer", &auth.BearerStrategy{})

	// Register header strategies for each unique header name used in config
	for headerName := range headerStrategies {
		strategyKey := "header:" + headerName
		authRegistry.Register(strategyKey, &auth.HeaderStrategy{HeaderName: headerName})
		log.Debug(ctx, "registered header auth strategy",
			"strategy", strategyKey,
			"header_name", headerName,
		)
	}

	// Create proxy server with MITM support
	var proxyServer *proxy.Server
	if serviceCount > 0 {
		// Use MITM-enabled proxy if services are configured
		// Pass registries via options to enable authentication
		proxyServer = proxy.NewWithMITM(
			cfg,
			slog.Default(),
			shutdownMgr,
			registry,
			certCache,
			&proxy.MITMOptions{
				SecretRegistry: secretRegistry,
				AuthRegistry:   authRegistry,
			},
		)
		log.Info(ctx, "proxy server created with MITM support and authentication")
	} else {
		// Use transparent proxy if no services configured
		proxyServer = proxy.New(cfg, slog.Default(), shutdownMgr)
		log.Info(ctx, "proxy server created in transparent mode (no services configured)")
	}

	// Start proxy server
	if err := proxyServer.Start(ctx); err != nil {
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

// setupLogging configures the global logger based on configuration
func setupLogging(cfg *config.Config) {
	var level slog.Level
	switch cfg.Logging.Level {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	// Create handler with configured level
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})

	// Create and set logger
	logger := slog.New(handler)
	log.SetLogger(logger)
	log.SetLevel(level)
}

// getCAPath returns the CA directory and file paths.
// Uses ~/.config/chaperone/ as the default location.
func getCAPath() (dir, keyPath, certPath string, err error) {
	// Get user config directory
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", "", "", fmt.Errorf("failed to get user config directory: %w", err)
	}

	// Build CA paths
	dir = filepath.Join(configDir, "chaperone")
	keyPath = filepath.Join(dir, "ca-key.pem")
	certPath = filepath.Join(dir, "ca-cert.pem")

	return dir, keyPath, certPath, nil
}
