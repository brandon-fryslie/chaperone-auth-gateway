package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/bmf/chaperone/internal/auth"
	"github.com/bmf/chaperone/internal/client"
	"github.com/bmf/chaperone/internal/config"
	"github.com/bmf/chaperone/internal/log"
	"github.com/bmf/chaperone/internal/mitm"
	"github.com/bmf/chaperone/internal/secrets"
	"github.com/bmf/chaperone/internal/service"
	"github.com/bmf/chaperone/internal/shutdown"
)

// Server is an HTTP proxy server that handles CONNECT tunnels.
type Server struct {
	config      *config.Config
	logger      *slog.Logger
	shutdownMgr *shutdown.Manager
	httpServer  *http.Server
	listener    net.Listener
	mu          sync.Mutex
	started     bool
}

// New creates a new proxy server with the given configuration.
// This creates a transparent proxy without MITM capabilities.
func New(cfg *config.Config, logger *slog.Logger, shutdownMgr *shutdown.Manager) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	s := &Server{
		config:      cfg,
		logger:      logger,
		shutdownMgr: shutdownMgr,
	}

	// Create the tunnel handler (transparent mode only)
	tunnelHandler := &TunnelHandler{
		logger: logger,
	}

	// Create HTTP server with the tunnel handler
	s.httpServer = &http.Server{
		Handler:           tunnelHandler,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       1 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Register shutdown function
	if shutdownMgr != nil {
		shutdownMgr.Register(s.Stop)
	}

	return s
}

// MITMOptions allows configuring optional parameters for MITM mode.
type MITMOptions struct {
	// UpstreamClient optionally overrides the default upstream HTTP client.
	// If nil, a default client with system trust store is used.
	// This is primarily useful for testing.
	UpstreamClient *client.Client

	// SecretRegistry provides secret fetching capabilities.
	// If nil, authentication will be skipped (backward compatibility).
	SecretRegistry *secrets.Registry

	// AuthRegistry provides authentication strategy implementations.
	// If nil, authentication will be skipped (backward compatibility).
	AuthRegistry *auth.Registry
}

// NewWithMITM creates a new proxy server with MITM capabilities.
// It accepts a service registry for domain matching and a certificate cache
// for dynamic certificate generation.
// Optional MITMOptions can be passed to customize behavior (mainly for testing).
func NewWithMITM(cfg *config.Config, logger *slog.Logger, shutdownMgr *shutdown.Manager, registry service.ServiceRegistry, certCache *mitm.CertCache, opts ...*MITMOptions) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	s := &Server{
		config:      cfg,
		logger:      logger,
		shutdownMgr: shutdownMgr,
	}

	// Get options (if provided)
	var options *MITMOptions
	if len(opts) > 0 && opts[0] != nil {
		options = opts[0]
	}

	// Create upstream client (use provided or create default)
	var upstreamClient *client.Client
	if options != nil && options.UpstreamClient != nil {
		upstreamClient = options.UpstreamClient
	} else {
		upstreamClient = client.NewClient(logger)
	}

	// Get secret registry (if provided)
	var secretRegistry *secrets.Registry
	if options != nil && options.SecretRegistry != nil {
		secretRegistry = options.SecretRegistry
	}

	// Get auth registry (if provided)
	var authRegistry *auth.Registry
	if options != nil && options.AuthRegistry != nil {
		authRegistry = options.AuthRegistry
	}

	// Create policy enforcer
	enforcer := service.NewPolicyEnforcer(logger)

	// Create MITM handler with registries
	mitmHandler := NewMITMHandler(upstreamClient, logger, registry, enforcer, secretRegistry, authRegistry)

	// Create the tunnel handler with MITM support
	tunnelHandler := &TunnelHandler{
		logger:      logger,
		registry:    registry,
		certCache:   certCache,
		mitmHandler: mitmHandler,
	}

	// Create HTTP server with the tunnel handler
	s.httpServer = &http.Server{
		Handler:           tunnelHandler,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       1 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Register shutdown function
	if shutdownMgr != nil {
		shutdownMgr.Register(s.Stop)
	}

	return s
}

// Start starts the proxy server on the configured address and port.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("server already started")
	}

	// Build listen address
	addr := fmt.Sprintf("%s:%d", s.config.Server.Address, s.config.Server.Port)

	// Create listener
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	s.listener = listener
	s.started = true

	log.Info(ctx, "proxy server listening",
		"address", s.config.Server.Address,
		"port", s.config.Server.Port,
	)

	// Start serving in a goroutine
	go func() {
		if err := s.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Error(ctx, "proxy server error", err)
		}
	}()

	return nil
}

// Stop gracefully stops the proxy server.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return nil
	}

	log.Info(ctx, "stopping proxy server")

	// Shutdown the HTTP server with the provided context
	if err := s.httpServer.Shutdown(ctx); err != nil {
		// If shutdown times out due to context deadline, force close
		// This is expected behavior when there are lingering connections
		if err == context.DeadlineExceeded || err == context.Canceled {
			log.Info(ctx, "shutdown timeout reached, forcing close")
			if closeErr := s.httpServer.Close(); closeErr != nil {
				log.Error(ctx, "error force closing proxy server", closeErr)
			}
			// Don't return the timeout error - shutdown was successful
		} else {
			log.Error(ctx, "error shutting down proxy server", err)
			// Force close on other errors
			if closeErr := s.httpServer.Close(); closeErr != nil {
				log.Error(ctx, "error force closing proxy server", closeErr)
			}
			return err
		}
	}

	s.started = false
	log.Info(ctx, "proxy server stopped")

	return nil
}
