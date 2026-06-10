package orchestrate

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/bmf/chaperone/internal/audit"
	"github.com/bmf/chaperone/internal/config"
	"github.com/bmf/chaperone/internal/control"
	"github.com/bmf/chaperone/internal/log"
	"github.com/bmf/chaperone/internal/mitm"
	"github.com/bmf/chaperone/internal/proxy"
	"github.com/bmf/chaperone/internal/shutdown"
)

// InitializeCA loads or generates a persistent CA certificate for inject mode.
// Returns the CA and a boolean indicating if it was newly created.
func InitializeCA(ctx context.Context, caKeyPath, caCertPath string) (*mitm.CA, bool, error) {
	// Ensure CA directory exists
	caDir := filepath.Dir(caCertPath)
	if err := os.MkdirAll(caDir, 0700); err != nil {
		return nil, false, fmt.Errorf("failed to create CA directory: %w", err)
	}

	// Check if CA files already exist
	_, keyErr := os.Stat(caKeyPath)
	_, certErr := os.Stat(caCertPath)
	isNewCA := keyErr != nil || certErr != nil

	ca, err := mitm.LoadOrGenerateCA(caKeyPath, caCertPath)
	if err != nil {
		return nil, false, fmt.Errorf("failed to initialize CA: %w", err)
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

	return ca, isNewCA, nil
}

// InitializeEphemeralCA creates a temporary CA certificate for run mode.
// Registers cleanup callbacks with the shutdown manager.
func InitializeEphemeralCA(ctx context.Context, pid int, shutdownMgr *shutdown.Manager) (*mitm.CA, string, string, error) {
	// Create ephemeral CA directory
	ephemeralCADir := fmt.Sprintf("/tmp/chaperone-ca-%d", pid)
	if err := os.MkdirAll(ephemeralCADir, 0700); err != nil {
		return nil, "", "", fmt.Errorf("failed to create ephemeral CA directory: %w", err)
	}

	// Register cleanup callback to delete CA directory on exit
	shutdownMgr.Register(func(ctx context.Context) error {
		log.Info(ctx, "cleaning up ephemeral CA directory", "path", ephemeralCADir)
		return os.RemoveAll(ephemeralCADir)
	})

	caCertPath := filepath.Join(ephemeralCADir, "ca-cert.pem")
	caKeyPath := filepath.Join(ephemeralCADir, "ca-key.pem")

	log.Info(ctx, "generating ephemeral CA certificate",
		"ca_dir", ephemeralCADir,
		"ca_cert", caCertPath,
	)

	// Generate fresh CA (LoadOrGenerateCA will generate if files don't exist)
	ca, err := mitm.LoadOrGenerateCA(caKeyPath, caCertPath)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to generate ephemeral CA: %w", err)
	}

	return ca, caKeyPath, caCertPath, nil
}

// CreateProxy creates a proxy server based on the setup result.
// It installs the MITM pipeline whenever the daemon could ever need to inject —
// either static services exist now OR the grantable universe is non-empty (a
// runtime grant can add an injection-eligible host without a restart). Only a
// daemon that can never MITM anything falls back to a transparent proxy.
// If proxySecret is provided, proxy requires authentication.
// If auditLogger is non-nil, the proxy writes to it (so the daemon shares one
// audit trail with the control plane); if nil, the proxy owns its own.
func CreateProxy(ctx context.Context, cfg *config.Config, logger *slog.Logger, shutdownMgr *shutdown.Manager, result *SetupResult, proxySecret string, auditLogger audit.AuditLogger) *proxy.Server {
	grantsPossible := result.GrantEnforcer != nil && len(result.GrantEnforcer.ListPairings()) > 0

	var proxyServer *proxy.Server
	if len(result.ServiceRegistry.ListAll()) > 0 || grantsPossible {
		// Use MITM-enabled proxy if services are configured or grants are possible
		// Pass registries via options to enable authentication
		proxyServer = proxy.NewWithMITM(
			cfg,
			logger,
			shutdownMgr,
			result.ServiceRegistry,
			result.CertCache,
			&proxy.MITMOptions{
				SecretRegistry: result.SecretRegistry,
				AuthRegistry:   result.AuthRegistry,
				ProxySecret:    proxySecret,
				AuditLogger:    auditLogger,
				UpstreamCAs:    result.UpstreamCAs,
			},
		)
		log.Info(ctx, "proxy server created with MITM support and authentication")
	} else {
		// Use transparent proxy if no services configured
		proxyServer = proxy.New(cfg, logger, shutdownMgr)
		log.Info(ctx, "proxy server created in transparent mode (no services configured)")
	}
	return proxyServer
}

// CreateAuditLogger creates the daemon's single audit sink from config and
// registers its Close on shutdown. Audit is a security control, so a creation
// failure is loud (returned), never silently disabled.
func CreateAuditLogger(cfg *config.Config, shutdownMgr *shutdown.Manager) (audit.AuditLogger, error) {
	logger, err := audit.NewLogger(audit.Config{
		Enabled: cfg.Audit.Enabled,
		Path:    cfg.Audit.Path,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create audit logger: %w", err)
	}
	if shutdownMgr != nil {
		shutdownMgr.Register(func(context.Context) error { return logger.Close() })
	}
	return logger, nil
}

// StartControlPlane brings up the localhost-only control socket on the running
// daemon and registers its shutdown. The control API defers all grant decisions
// to the enforcer and writes grant/revoke events to the shared audit sink.
func StartControlPlane(ctx context.Context, result *SetupResult, auditLogger audit.AuditLogger, socketPath string, shutdownMgr *shutdown.Manager, logger *slog.Logger) error {
	api, err := control.NewAPI(result.GrantEnforcer, result.ServiceRegistry, auditLogger, logger)
	if err != nil {
		return fmt.Errorf("failed to build control API: %w", err)
	}

	server, err := control.NewServer(api, socketPath, logger)
	if err != nil {
		return fmt.Errorf("failed to build control server: %w", err)
	}

	if err := server.Start(); err != nil {
		return fmt.Errorf("failed to start control plane: %w", err)
	}

	shutdownMgr.Register(server.Stop)
	log.Info(ctx, "control plane started", "socket", socketPath)
	return nil
}

// LogStartup logs the startup configuration.
func LogStartup(ctx context.Context, cfg *config.Config, version, configPath string, serviceFilter []string) {
	log.Info(ctx, "chaperone starting",
		"version", version,
		"config", configPath,
		"address", cfg.Server.Address,
		"port", cfg.Server.Port,
	)

	if len(serviceFilter) > 0 {
		log.Info(ctx, "service filter enabled", "service", serviceFilter[0])
	}
}
