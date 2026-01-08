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
	"github.com/bmf/chaperone/internal/config"
	"github.com/bmf/chaperone/internal/examine"
	chaperoneInit "github.com/bmf/chaperone/internal/init"
	"github.com/bmf/chaperone/internal/log"
	"github.com/bmf/chaperone/internal/mitm"
	"github.com/bmf/chaperone/internal/recorder"
	"github.com/bmf/chaperone/internal/secrets"
	"github.com/bmf/chaperone/internal/service"
	"github.com/bmf/chaperone/internal/shutdown"
	"github.com/elazarl/goproxy"
)

// Server is an HTTP proxy server that handles CONNECT tunnels.
type Server struct {
	config      *config.Config
	logger      *slog.Logger
	shutdownMgr *shutdown.Manager
	proxy       *goproxy.ProxyHttpServer
	httpServer  *http.Server
	listener    net.Listener
	recorder    *recorder.Recorder
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

	// Create goproxy server
	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = (cfg.Logging.Level == "debug")

	// Disable proxy for upstream connections to avoid proxy loops
	proxy.Tr.Proxy = nil

	s.proxy = proxy

	// Create HTTP server with the proxy
	s.httpServer = &http.Server{
		Handler:           proxy,
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
		recorder:    recorder.NewRecorder(),
	}

	// Get options (if provided)
	var options *MITMOptions
	if len(opts) > 0 && opts[0] != nil {
		options = opts[0]
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

	// Create goproxy server
	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = (cfg.Logging.Level == "debug")

	// Disable proxy for upstream connections to avoid proxy loops
	proxy.Tr.Proxy = nil

	// Create certificate store adapter
	certStore := NewGoproxyCertStore(certCache)

	// Configure CONNECT handler (MITM vs transparent tunnel decision)
	proxy.OnRequest().HandleConnectFunc(connectHandler(registry, certStore, logger))

	// Configure request handlers in order:
	// 0. Request ID setup (MUST be first so all handlers have request ID for logging)
	proxy.OnRequest(ChaperoneCondition(registry)).DoFunc(requestIDHandler())

	// 1. Drop handler (block requests matching drop patterns)
	proxy.OnRequest(ChaperoneCondition(registry)).DoFunc(dropHandler(registry, logger))

	// 2. SECURITY: Auto-strip ALL known auth headers (prevents credential leakage)
	// This is NOT configurable - it's a security measure that always runs
	proxy.OnRequest(ChaperoneCondition(registry)).DoFunc(securityStripAuthHandler(registry, logger))

	// 3. Policy enforcement (check methods, paths, body size)
	proxy.OnRequest(ChaperoneCondition(registry)).DoFunc(policyHandler(registry, enforcer, logger))

	// 4. User-configurable strip handler (remove additional headers)
	proxy.OnRequest(ChaperoneCondition(registry)).DoFunc(stripHandler(registry, logger))

	// 5. Authentication injection
	proxy.OnRequest(ChaperoneCondition(registry)).DoFunc(authHandler(registry, secretRegistry, authRegistry, logger))

	// 6. HAR recording - request start
	proxy.OnRequest(ChaperoneCondition(registry)).DoFunc(recordRequestHandler(s.recorder))

	// Configure response handler for HAR recording
	proxy.OnResponse(ChaperoneRespCondition(registry)).DoFunc(recordResponseHandler(s.recorder))

	s.proxy = proxy

	// Create HTTP server with the proxy
	s.httpServer = &http.Server{
		Handler:           proxy,
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

// NewExamineProxy creates a passthrough MITM proxy for examining requests.
// It does NOT inject credentials or alter requests/responses.
// This mode is used by 'chaperone examine' to help users discover how authentication
// credentials are passed in requests.
func NewExamineProxy(cfg *config.Config, logger *slog.Logger, shutdownMgr *shutdown.Manager, certCache *mitm.CertCache, examineLogger *examine.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	s := &Server{
		config:      cfg,
		logger:      logger,
		shutdownMgr: shutdownMgr,
	}

	// Create goproxy server
	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = (cfg.Logging.Level == "debug")

	// Disable proxy for upstream connections to avoid proxy loops
	proxy.Tr.Proxy = nil

	// Create certificate store adapter
	certStore := NewGoproxyCertStore(certCache)

	// CONNECT handler: MITM ALL connections (passthrough but intercepted)
	proxy.OnRequest().HandleConnectFunc(examineConnectHandler(certStore, logger))

	// Request handler: Just log, don't modify
	proxy.OnRequest().DoFunc(examineRequestHandler(examineLogger))

	// Response handler: Log responses if configured
	proxy.OnResponse().DoFunc(examineResponseHandler(examineLogger))

	s.proxy = proxy

	// Create HTTP server with the proxy
	s.httpServer = &http.Server{
		Handler:           proxy,
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

// GetHAR returns the recorded HAR data as JSON bytes
func (s *Server) GetHAR() ([]byte, error) {
	if s.recorder == nil {
		return nil, fmt.Errorf("recorder not initialized")
	}
	return s.recorder.ToJSON()
}

// FindingCallback is called when a new auth finding is detected during init mode.
type FindingCallback func(host string, finding *chaperoneInit.Finding)

// NewInitProxy creates a MITM proxy for the init wizard.
// It analyzes requests to detect authentication patterns and policy constraints.
// The detector analyzes requests and the callback reports findings in real-time.
func NewInitProxy(cfg *config.Config, logger *slog.Logger, shutdownMgr *shutdown.Manager, certCache *mitm.CertCache, detector *chaperoneInit.Detector, evidence *chaperoneInit.Evidence, onFinding FindingCallback) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	s := &Server{
		config:      cfg,
		logger:      logger,
		shutdownMgr: shutdownMgr,
	}

	// Create goproxy server
	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = (cfg.Logging.Level == "debug")

	// Disable proxy for upstream connections to avoid proxy loops
	proxy.Tr.Proxy = nil

	// Create certificate store adapter
	certStore := NewGoproxyCertStore(certCache)

	// CONNECT handler: MITM ALL connections (like examine mode)
	proxy.OnRequest().HandleConnectFunc(initConnectHandler(certStore, logger))

	// Request handler: Analyze for auth patterns, then forward unmodified
	proxy.OnRequest().DoFunc(initRequestHandler(detector, evidence, onFinding))

	// Response handler: No-op (passthrough)
	proxy.OnResponse().DoFunc(initResponseHandler())

	s.proxy = proxy

	// Create HTTP server with the proxy
	s.httpServer = &http.Server{
		Handler:           proxy,
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
