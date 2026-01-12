package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bmf/chaperone/internal/auth"
	"github.com/bmf/chaperone/internal/config"
	"github.com/bmf/chaperone/internal/log"
	"github.com/bmf/chaperone/internal/mitm"
	"github.com/bmf/chaperone/internal/proxy"
	"github.com/bmf/chaperone/internal/run"
	"github.com/bmf/chaperone/internal/secrets"
	"github.com/bmf/chaperone/internal/service"
	"github.com/bmf/chaperone/internal/shutdown"
	"github.com/spf13/cobra"
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run <service-name>",
	Short: "Spawn application with proxy environment",
	Long: `Spawn and manage an application process with automatic proxy configuration.

This command:
1. Starts a proxy server on a Unix socket
2. Spawns the configured application with proxy environment variables
3. Manages the lifecycle of both proxy and application
4. Forwards signals (SIGTERM) to the application
5. Exits with the application's exit code

The application is configured in [services.<name>.run] section of the config.

Examples:
  chaperone run openai           # Run the openai service
  chaperone run -c custom.toml myservice`,
	Args: cobra.ExactArgs(1),
	RunE: runWithProxy,
}

func init() {
	rootCmd.AddCommand(runCmd)
}

func runWithProxy(cmd *cobra.Command, args []string) error {
	serviceName := args[0]
	ctx := context.Background()

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

	// Validate service has RunConfig
	if svc.Run == nil {
		return fmt.Errorf("service %q does not have a [services.%s.run] section", serviceName, serviceName)
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

	// Set up logging
	setupLogging(cfg)

	log.Info(ctx, "chaperone run mode starting",
		"version", version,
		"service", serviceName,
		"socket", socketPath,
		"command", svc.Run.Command,
	)

	// Create shutdown manager
	shutdownMgr := shutdown.NewManager(slog.Default())

	// Initialize CA for MITM
	caDir, caKeyPath, caCertPath, err := getCAPath()
	if err != nil {
		return fmt.Errorf("failed to get CA path: %w", err)
	}

	if err := os.MkdirAll(caDir, 0700); err != nil {
		return fmt.Errorf("failed to create CA directory: %w", err)
	}

	ca, err := mitm.LoadOrGenerateCA(caKeyPath, caCertPath)
	if err != nil {
		return fmt.Errorf("failed to initialize CA: %w", err)
	}

	certCache := mitm.NewCertCache(ca, slog.Default())

	// Create service registry (single service)
	registry := service.NewRegistry()

	// Register the service
	headerStrategies := make(map[string]bool)
	authStrategyRef := svc.AuthStrategy
	headerName := svc.HeaderName

	if strings.HasPrefix(svc.AuthStrategy, "header:") {
		headerName = svc.AuthStrategy[7:]
		authStrategyRef = svc.AuthStrategy
		headerStrategies[headerName] = true
	} else if svc.AuthStrategy == "header" && svc.HeaderName != "" {
		authStrategyRef = "header:" + svc.HeaderName
		headerStrategies[svc.HeaderName] = true
	}

	serviceDef := &service.Service{
		Name:            serviceName,
		HostPattern:     svc.HostPattern,
		AuthStrategyRef: authStrategyRef,
		HeaderName:      svc.HeaderName,
		CredentialRef:   svc.CredentialRef,
		Placeholder:     svc.Placeholder,
		Policy: &service.Policy{
			AllowedMethods: svc.AllowedMethods,
			AllowedPaths:   svc.AllowedPaths,
			MaxBodyBytes:   svc.MaxBodyBytes,
			ClientGroups:   svc.ClientGroups,
			Drop:           svc.Drop,
			Strip:          svc.Strip,
		},
	}

	serviceDef.Policy.ApplyDefaults()

	if err := registry.Register(serviceDef); err != nil {
		return fmt.Errorf("failed to register service: %w", err)
	}

	log.Info(ctx, "registered service",
		"name", serviceName,
		"host_pattern", svc.HostPattern,
		"auth_strategy", authStrategyRef,
	)

	// Initialize secret and auth registries
	secretRegistry := secrets.NewRegistry()
	authRegistry := auth.NewRegistry()

	secretRegistry.Register("env", secrets.NewEnvProvider())
	secretRegistry.Register("file", secrets.NewFileProvider())
	secretRegistry.Register("keychain", secrets.NewKeychainProvider())

	// Preload secrets
	if svc.CredentialRef != "" {
		if err := secretRegistry.PreloadSecrets(ctx, svc.CredentialRef); err != nil {
			return fmt.Errorf("failed to preload secrets: %w", err)
		}
	}

	// Register auth strategies
	authRegistry.Register("bearer", &auth.BearerStrategy{})
	for headerName := range headerStrategies {
		strategyKey := "header:" + headerName
		authRegistry.Register(strategyKey, &auth.HeaderStrategy{HeaderName: headerName})
	}

	// Validate configuration
	if err := validateConfiguration(ctx, registry, authRegistry, secretRegistry); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	// Create and start proxy server
	proxyServer := proxy.NewWithMITM(
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

	// Create FD config
	fdConfig, err := run.NewFDConfig(svc.Run.Stdout, svc.Run.Stderr)
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
