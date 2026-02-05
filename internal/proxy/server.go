package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/bmf/chaperone/internal/audit"
	"github.com/bmf/chaperone/internal/auth"
	"github.com/bmf/chaperone/internal/config"
	"github.com/bmf/chaperone/internal/examine"
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
	auditLogger *audit.Logger
	mu          sync.Mutex
	started     bool
}

// createHTTPServer initializes the HTTP server with standard timeout configuration.
// This is the single source of truth for server timeouts across all proxy modes.
func (s *Server) createHTTPServer() {
	s.httpServer = &http.Server{
		Handler:           s.proxy,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       1 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		// WriteTimeout is intentionally omitted to support streaming responses
		// (SSE, GraphQL subscriptions, chunked transfer encoding)
	}
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
	s.createHTTPServer()

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

	// Create audit logger
	auditLogger, err := audit.NewLogger(audit.Config{
		Enabled: cfg.Audit.Enabled,
		Path:    cfg.Audit.Path,
	})
	if err != nil {
		logger.Warn("failed to create audit logger", "error", err)
		// Create disabled logger as fallback
		auditLogger = &audit.Logger{}
	}
	s.auditLogger = auditLogger

	// Register audit logger cleanup on shutdown
	if shutdownMgr != nil {
		shutdownMgr.Register(func(ctx context.Context) error {
			return auditLogger.Close()
		})
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
	proxy.OnRequest(ChaperoneCondition(registry)).DoFunc(dropHandler(registry, auditLogger, logger))

	// 2. SECURITY: Auto-strip ALL known auth headers (prevents credential leakage)
	// This is NOT configurable - it's a security measure that always runs
	proxy.OnRequest(ChaperoneCondition(registry)).DoFunc(securityStripAuthHandler(registry, auditLogger, logger))

	// 3. Policy enforcement (check methods, paths, body size)
	proxy.OnRequest(ChaperoneCondition(registry)).DoFunc(policyHandler(registry, enforcer, auditLogger, logger))

	// 4. User-configurable strip handler (remove additional headers)
	proxy.OnRequest(ChaperoneCondition(registry)).DoFunc(stripHandler(registry, logger))

	// 5. Authentication injection
	proxy.OnRequest(ChaperoneCondition(registry)).DoFunc(authHandler(registry, secretRegistry, authRegistry, auditLogger, logger))

	// 6. HAR recording - request start
	proxy.OnRequest(ChaperoneCondition(registry)).DoFunc(recordRequestHandler(s.recorder))

	// Configure response handler for HAR recording
	proxy.OnResponse(ChaperoneRespCondition(registry)).DoFunc(recordResponseHandler(s.recorder))

	s.proxy = proxy

	// Create HTTP server with the proxy
	s.createHTTPServer()

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
// If recorder is provided, it will capture traffic in HAR format.
func NewExamineProxy(cfg *config.Config, logger *slog.Logger, shutdownMgr *shutdown.Manager, certCache *mitm.CertCache, examineLogger *examine.Logger, rec *recorder.Recorder, sentinelChan chan struct{}) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	s := &Server{
		config:      cfg,
		logger:      logger,
		shutdownMgr: shutdownMgr,
		recorder:    rec,
	}

	// Create goproxy server
	proxy := goproxy.NewProxyHttpServer()
	// NEVER enable proxy.Verbose in examine mode - it writes to stdout and breaks
	// full-screen terminal applications. Use structured logging instead.
	proxy.Verbose = false

	// Disable proxy for upstream connections to avoid proxy loops
	proxy.Tr.Proxy = nil

	// Create certificate store adapter
	certStore := NewGoproxyCertStore(certCache)

	// Configure CONNECT handler - MITM ALL connections (no filtering)
	proxy.OnRequest().HandleConnectFunc(examine.ConnectHandler(certStore, logger))

	// Add request ID handler FIRST - provides correlation colors for all requests
	proxy.OnRequest().DoFunc(requestIDHandler())

	// Configure request handler - log ALL requests
	proxy.OnRequest().DoFunc(examine.RequestHandler(examineLogger, sentinelChan))

	// Configure response handler - log ALL responses
	proxy.OnResponse().DoFunc(examine.ResponseHandler(examineLogger))

	// Add HAR recording if recorder provided
	if rec != nil {
		proxy.OnRequest().DoFunc(recordRequestHandler(rec))
		proxy.OnResponse().DoFunc(recordResponseHandler(rec))
	}

	s.proxy = proxy

	// Create HTTP server with the proxy
	s.createHTTPServer()

	// Register shutdown function
	if shutdownMgr != nil {
		shutdownMgr.Register(s.Stop)
	}

	return s
}

// Start starts the proxy server.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("server already started")
	}

	var listener net.Listener
	var err error

	// Always use TCP mode on 127.0.0.1 with OS-allocated port
	addr := fmt.Sprintf("%s:%d", s.config.Server.Address, s.config.Server.Port)
	listener, err = net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to start listener: %w", err)
	}

	s.logger.Info("proxy server started", "address", addr)

	s.listener = listener
	s.started = true

	// Start serving in a goroutine
	go func() {
		if err := s.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			s.logger.Error("proxy server error", "error", err)
		}
	}()

	return nil
}

// Stop stops the proxy server gracefully.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return nil
	}

	s.logger.Info("stopping proxy server")

	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown server: %w", err)
	}

	s.started = false
	return nil
}

// Addr returns the listening address of the proxy server.
// Returns empty string if server is not started.
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return ""
}

// GetRecorder returns the HAR recorder (for testing/debugging).
func (s *Server) GetRecorder() *recorder.Recorder {
	return s.recorder
}
