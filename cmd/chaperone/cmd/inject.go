package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
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

By default, Chaperone listens on a Unix socket (/tmp/chaperone.sock) for better security.
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
	injectCmd.Flags().StringVar(&socketPath, "socket", "", "Unix socket path to listen on (overrides default /tmp/chaperone.sock)")
	injectCmd.Flags().BoolVar(&httpMode, "http", false, "Use HTTP/TCP mode instead of Unix socket")
	injectCmd.Flags().IntVar(&httpPort, "port", 0, "Port to listen on (implies --http, default 4010)")
	injectCmd.Flags().StringVar(&httpAddr, "addr", "", "Address to listen on (implies --http, default 127.0.0.1)")
}

func runInject(cmd *cobra.Command, args []string) error {
	// Create context for the application
	ctx := context.Background()

	// Determine service filter (if provided)
	var serviceFilter string
	if len(args) > 0 {
		serviceFilter = args[0]
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
	// Priority: --socket > --http/--port/--addr > config file > default (Unix socket)

	if socketPath != "" {
		// Explicit socket path provided
		cfg.Server.Socket = socketPath
		cfg.Server.Port = 0 // Clear port to avoid validation warning
	} else if httpMode || httpPort != 0 || httpAddr != "" {
		// HTTP mode requested via flags
		cfg.Server.Socket = "" // Disable socket mode

		if httpPort != 0 {
			cfg.Server.Port = httpPort
		} else if cfg.Server.Port == 0 {
			cfg.Server.Port = 4010 // Default HTTP port
		}

		if httpAddr != "" {
			cfg.Server.Address = httpAddr
		} else if cfg.Server.Address == "" {
			cfg.Server.Address = "127.0.0.1" // Default HTTP address
		}
	}
	// else: use config file settings, or SetDefaults will apply Unix socket mode

	// Apply defaults and validate
	cfg.SetDefaults()
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// Set up logging based on config
	setupLogging(cfg)

	// Log startup with appropriate mode
	if cfg.Server.Socket != "" {
		log.Info(ctx, "chaperone starting",
			"version", version,
			"config", configPath,
			"socket", cfg.Server.Socket,
		)
	} else {
		log.Info(ctx, "chaperone starting",
			"version", version,
			"config", configPath,
			"address", cfg.Server.Address,
			"port", cfg.Server.Port,
		)
	}

	if serviceFilter != "" {
		log.Info(ctx, "service filter enabled", "service", serviceFilter)
	}

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
	// Check if CA files already exist
	_, keyErr := os.Stat(caKeyPath)
	_, certErr := os.Stat(caCertPath)
	isNewCA := keyErr != nil || certErr != nil

	ca, err := mitm.LoadOrGenerateCA(caKeyPath, caCertPath)
	if err != nil {
		return fmt.Errorf("failed to initialize CA: %w", err)
	}

	if isNewCA {
		log.Info(ctx, "generated new CA certificate",
			"cert_path", caCertPath,
			"key_path", caKeyPath,
		)
		log.Info(ctx, "Trust this CA in your browser/system to avoid certificate warnings",
			"cert_path", caCertPath,
		)
	} else {
		log.Info(ctx, "loaded existing CA certificate",
			"cert_path", caCertPath,
		)
	}

	// Create certificate cache
	certCache := mitm.NewCertCache(ca, slog.Default())

	// Create service registry
	registry := service.NewRegistry()

	// Load services from config
	serviceCount := 0
	headerStrategies := make(map[string]bool) // Track header strategies to register
	credentialRefs := make([]string, 0)       // Collect credential refs for preloading
	for name, svcCfg := range cfg.Services {
		// Apply service filter if specified
		if serviceFilter != "" && name != serviceFilter {
			continue
		}

		// Determine auth strategy reference
		// Support both documented formats:
		// 1. Combined format (recommended): auth_strategy = "header:x-api-key"
		// 2. Separate fields format: auth_strategy = "header", header_name = "x-api-key"
		authStrategyRef := svcCfg.AuthStrategy
		headerName := svcCfg.HeaderName

		if strings.HasPrefix(svcCfg.AuthStrategy, "header:") {
			// Combined format: auth_strategy = "header:x-api-key"
			headerName = svcCfg.AuthStrategy[7:] // Extract "x-api-key" from "header:x-api-key"
			authStrategyRef = svcCfg.AuthStrategy
			headerStrategies[headerName] = true
		} else if svcCfg.AuthStrategy == "header" && svcCfg.HeaderName != "" {
			// Separate fields format: auth_strategy = "header", header_name = "x-api-key"
			authStrategyRef = "header:" + svcCfg.HeaderName
			headerStrategies[svcCfg.HeaderName] = true
		}

		// Collect credential refs for preloading
		if svcCfg.CredentialRef != "" {
			credentialRefs = append(credentialRefs, svcCfg.CredentialRef)
		}

		// Convert config.ServiceConfig to service.Service
		svc := &service.Service{
			Name:            name,
			HostPattern:     svcCfg.HostPattern,
			AuthStrategyRef: authStrategyRef,
			HeaderName:      svcCfg.HeaderName,
			CredentialRef:   svcCfg.CredentialRef,
			Placeholder:     svcCfg.Placeholder,
			Policy: &service.Policy{
				AllowedMethods: svcCfg.AllowedMethods,
				AllowedPaths:   svcCfg.AllowedPaths,
				MaxBodyBytes:   svcCfg.MaxBodyBytes,
				ClientGroups:   svcCfg.ClientGroups,
				Drop:           svcCfg.Drop,
				Strip:          svcCfg.Strip,
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

	if serviceFilter != "" && serviceCount == 0 {
		return fmt.Errorf("service %q not found in config", serviceFilter)
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

	// Preload secrets at startup to avoid expensive lookups during request handling
	if len(credentialRefs) > 0 {
		if err := secretRegistry.PreloadSecrets(ctx, credentialRefs...); err != nil {
			return fmt.Errorf("failed to preload secrets at startup: %w", err)
		}
		log.Info(ctx, "preloaded secrets at startup",
			"secret_count", len(credentialRefs),
		)
	}

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

	// Validate all services have registered auth strategies and credentials
	if serviceCount > 0 {
		if err := validateConfiguration(ctx, registry, authRegistry, secretRegistry); err != nil {
			return fmt.Errorf("configuration validation failed: %w", err)
		}
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

// validateConfiguration checks that all configured services have valid configuration:
// - auth strategies exist
// - secret providers exist (for credential references)
// - host patterns are valid
// - policy configuration is valid
// This is called at startup to catch configuration errors early.
func validateConfiguration(ctx context.Context, registry service.ServiceRegistry, authRegistry *auth.Registry, secretRegistry *secrets.Registry) error {
	var validationErrors []string

	for _, svc := range registry.ListAll() {
		// Validate auth strategy
		strategyRef := svc.AuthStrategyRef
		if strategyRef == "" {
			strategyRef = "bearer" // Default strategy
		}

		if !authRegistry.Has(strategyRef) {
			errMsg := fmt.Sprintf("auth strategy %q not registered (host pattern: %s)", strategyRef, svc.HostPattern)
			validationErrors = append(validationErrors, errMsg)
			log.Error(ctx, "auth strategy not found",
				fmt.Errorf("%s", errMsg),
				"host_pattern", svc.HostPattern,
			)
		}

		// Validate secret provider (if credential reference exists)
		if svc.CredentialRef != "" {
			provider := parseSecretProvider(svc.CredentialRef)
			if provider == "" {
				errMsg := fmt.Sprintf("invalid credential_ref format %q (host pattern: %s)", svc.CredentialRef, svc.HostPattern)
				validationErrors = append(validationErrors, errMsg)
				log.Error(ctx, "invalid credential reference format",
					fmt.Errorf("%s", errMsg),
					"credential_ref", svc.CredentialRef,
					"host_pattern", svc.HostPattern,
				)
			} else if !secretRegistry.HasProvider(provider) {
				errMsg := fmt.Sprintf("secret provider %q not found for credential_ref %q (host pattern: %s)", provider, svc.CredentialRef, svc.HostPattern)
				validationErrors = append(validationErrors, errMsg)
				log.Error(ctx, "secret provider not found",
					fmt.Errorf("%s", errMsg),
					"provider", provider,
					"credential_ref", svc.CredentialRef,
					"host_pattern", svc.HostPattern,
				)
			}
		}

		// Validate host pattern (basic check - should be non-empty)
		if svc.HostPattern == "" {
			errMsg := "host pattern is empty"
			validationErrors = append(validationErrors, errMsg)
			log.Error(ctx, "invalid service configuration",
				fmt.Errorf("%s", errMsg),
				"service", svc,
			)
		}

		// Validate policy configuration
		if svc.Policy != nil {
			if svc.Policy.MaxBodyBytes < 0 {
				errMsg := fmt.Sprintf("max_body_bytes is negative (host pattern: %s)", svc.HostPattern)
				validationErrors = append(validationErrors, errMsg)
				log.Error(ctx, "invalid policy configuration",
					fmt.Errorf("%s", errMsg),
					"host_pattern", svc.HostPattern,
				)
			}
		}
	}

	if len(validationErrors) > 0 {
		return fmt.Errorf("configuration validation failed: %d error(s): %v", len(validationErrors), validationErrors)
	}

	log.Info(ctx, "configuration validation passed")
	return nil
}

// parseSecretProvider extracts the provider name from a secret reference.
// Returns empty string if format is invalid.
// Format: "provider:path"
func parseSecretProvider(ref string) string {
	idx := -1
	for i, r := range ref {
		if r == ':' {
			idx = i
			break
		}
	}
	if idx == -1 {
		return "" // Invalid format
	}
	return ref[:idx]
}
