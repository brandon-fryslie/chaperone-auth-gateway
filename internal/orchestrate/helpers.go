package orchestrate

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/bmf/chaperone/internal/config"
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
// Returns a MITM-enabled proxy if services are configured, otherwise a transparent proxy.
func CreateProxy(ctx context.Context, cfg *config.Config, logger *slog.Logger, shutdownMgr *shutdown.Manager, result *SetupResult) *proxy.Server {
	var proxyServer *proxy.Server
	if len(result.ServiceRegistry.ListAll()) > 0 {
		// Use MITM-enabled proxy if services are configured
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
