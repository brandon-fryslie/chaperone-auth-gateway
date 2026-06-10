package proxy

import (
	"context"
	"crypto/x509"
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
	auditLogger audit.AuditLogger
	proxySecret string // Per-run secret for proxy authentication
	mu          sync.Mutex
	started     bool
}

// createHTTPServer initializes the HTTP server with standard timeout configuration.
// This is the single source of truth for server timeouts across all proxy modes.
func (s *Server) createHTTPServer() {
	s.httpServer = &http.Server{
		Handler:           s.proxy,
		ReadHeaderTimeout: 30 * time.Second, // Time to read request headers
		IdleTimeout:       10 * time.Minute, // Long idle timeout for streaming connections
		// ReadTimeout: Omitted - streaming requests can be very long
		// WriteTimeout: Omitted - streaming responses can last minutes
	}
}

// configureTransport sets up the outbound transport: trust policy first, then
// streaming-friendly connection tuning. It is the single owner of proxy.Tr —
// every constructor routes through here so no mode can ship goproxy's
// insecure default outbound TLS config. [LAW:single-enforcer]
func configureTransport(proxy *goproxy.ProxyHttpServer, upstreamCAs *x509.CertPool) {
	// Replace goproxy's default TLS client config (InsecureSkipVerify=true)
	// with the owned outbound-trust policy.
	proxy.Tr.TLSClientConfig = upstreamTLSConfig(upstreamCAs)

	// goproxy's HTTP/2 outbound leg bypasses proxy.Tr (and with it this trust
	// policy), so it must stay disabled. Pinned explicitly rather than relying
	// on the zero value. [LAW:no-silent-failure]
	proxy.AllowHTTP2 = false

	// Disable upstream proxy to avoid loops
	proxy.Tr.Proxy = nil

	// Configure for long-lived streaming connections (Claude API can stream for minutes)
	proxy.Tr.IdleConnTimeout = 10 * time.Minute      // Long idle timeout for streaming pauses
	proxy.Tr.ResponseHeaderTimeout = 5 * time.Minute // Long timeout for slow API responses
	proxy.Tr.ExpectContinueTimeout = 30 * time.Second
	proxy.Tr.DisableKeepAlives = false // Keep connections alive
	proxy.Tr.MaxIdleConns = 100        // Allow connection pooling
	proxy.Tr.MaxIdleConnsPerHost = 10  // Per-host connection pool
	proxy.Tr.MaxConnsPerHost = 0       // Unlimited concurrent connections per host
	proxy.Tr.ForceAttemptHTTP2 = false // Don't force HTTP/2 - causes client compatibility issues
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

	// Redirect goproxy's internal logging to our slog
	proxy.Logger = &slogAdapter{logger: logger}

	// Configure outbound trust + streaming transport (system roots; a
	// transparent proxy never originates TLS itself, but the policy is uniform)
	configureTransport(proxy, nil)

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

	// AuditLogger, if provided, is the audit sink the proxy writes to. Injecting it
	// lets the daemon share ONE audit trail across the proxy and the control plane
	// ([LAW:one-source-of-truth]); its lifecycle (Close) is owned by the injector.
	// If nil, the proxy creates and owns its own logger from cfg.Audit.
	AuditLogger audit.AuditLogger

	// UpstreamCAs is the trust anchor set for verifying upstream server
	// certificates on MITM'd connections. nil = system roots; non-nil = ONLY
	// these roots (pinning). Verification is always on — there is no way to
	// disable it. [LAW:types-are-the-program]
	UpstreamCAs *x509.CertPool
}

// NewWithMITM creates a new proxy server with MITM capabilities.
// It accepts a service registry for domain matching and a certificate cache
// for dynamic certificate generation.
//
// proxySecret gates every path into the injecting pipeline and is required:
// a proxy that injects real credentials must never be reachable without
// authentication, so "gate absent" is not constructible — an empty secret is
// a loud error, never a silently ungated server. [LAW:no-silent-failure]
//
// Optional MITMOptions can be passed to customize behavior (mainly for testing).
func NewWithMITM(cfg *config.Config, logger *slog.Logger, shutdownMgr *shutdown.Manager, registry service.ServiceRegistry, certCache *mitm.CertCache, proxySecret string, opts ...*MITMOptions) (*Server, error) {
	if proxySecret == "" {
		return nil, fmt.Errorf("refusing to start credential-injecting proxy without a proxy access secret: generate one with GenerateProxySecret()")
	}
	if logger == nil {
		logger = slog.Default()
	}

	s := &Server{
		config:      cfg,
		logger:      logger,
		shutdownMgr: shutdownMgr,
		recorder:    recorder.NewRecorder(),
		proxySecret: proxySecret,
	}

	// Get options (if provided)
	var options *MITMOptions
	if len(opts) > 0 && opts[0] != nil {
		options = opts[0]
	}

	// Audit sink: prefer an injected logger (the daemon shares one trail across
	// proxy + control plane) and let its injector own Close. Only when none is
	// provided does the proxy create and own its own from cfg.Audit.
	var auditLogger audit.AuditLogger
	if options != nil && options.AuditLogger != nil {
		auditLogger = options.AuditLogger
	} else {
		owned, err := audit.NewLogger(audit.Config{
			Enabled: cfg.Audit.Enabled,
			Path:    cfg.Audit.Path,
		})
		if err != nil {
			logger.Warn("failed to create audit logger", "error", err)
			owned = &audit.Logger{} // disabled fallback
		}
		auditLogger = owned
		if shutdownMgr != nil {
			shutdownMgr.Register(func(ctx context.Context) error {
				return owned.Close()
			})
		}
	}
	s.auditLogger = auditLogger

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

	// Redirect goproxy's internal logging to our slog
	proxy.Logger = &slogAdapter{logger: logger}

	// Configure outbound trust + streaming transport
	var upstreamCAs *x509.CertPool
	if options != nil {
		upstreamCAs = options.UpstreamCAs
	}
	configureTransport(proxy, upstreamCAs)

	// Create certificate store adapter
	certStore := NewGoproxyCertStore(certCache)

	// Configure CONNECT handler (MITM vs transparent tunnel decision),
	// always wrapped by the proxy-auth gate. The gate is wiring, not a mode:
	// it exists on every constructed server. [LAW:dataflow-not-control-flow]
	proxy.OnRequest().HandleConnectFunc(proxyAuthConnectHandler(proxySecret, connectHandler(registry, certStore, logger)))

	// Configure request handlers in order:
	// 0. Proxy authentication (MUST be first - reject unauthorized requests)
	proxy.OnRequest().DoFunc(proxyAuthHandler(proxySecret))

	// 1. Request ID setup (so all handlers have request ID for logging)
	proxy.OnRequest(ChaperoneCondition(registry)).DoFunc(requestIDHandler())

	// 2. Drop handler (block requests matching drop patterns)
	proxy.OnRequest(ChaperoneCondition(registry)).DoFunc(dropHandler(registry, auditLogger, logger))

	// 3. SECURITY: Auto-strip ALL known auth headers (prevents credential leakage)
	// This is NOT configurable - it's a security measure that always runs
	proxy.OnRequest(ChaperoneCondition(registry)).DoFunc(securityStripAuthHandler(registry, auditLogger, logger))

	// 4. Policy enforcement (check methods, paths, body size)
	proxy.OnRequest(ChaperoneCondition(registry)).DoFunc(policyHandler(registry, enforcer, auditLogger, logger))

	// 5. User-configurable strip handler (remove additional headers)
	proxy.OnRequest(ChaperoneCondition(registry)).DoFunc(stripHandler(registry, logger))

	// 6. Authentication injection
	proxy.OnRequest(ChaperoneCondition(registry)).DoFunc(authHandler(registry, secretRegistry, authRegistry, auditLogger, logger))

	// 7. HAR recording - request start
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

	return s, nil
}

// NewExamineProxy creates a passthrough MITM proxy for examining requests.
// It does NOT inject credentials or alter requests/responses.
// This mode is used by 'chaperone examine' to help users discover how authentication
// credentials are passed in requests.
// If recorder is provided, it will capture traffic in HAR format.
//
// proxySecret is required: examine MITMs every host and sees live credentials
// in plaintext, so an ungated examine proxy is refused at construction just
// like an ungated injecting proxy. [LAW:no-silent-failure]
func NewExamineProxy(cfg *config.Config, logger *slog.Logger, shutdownMgr *shutdown.Manager, certCache *mitm.CertCache, examineLogger *examine.Logger, rec *recorder.Recorder, sentinelChan chan struct{}, proxySecret string) (*Server, error) {
	if proxySecret == "" {
		return nil, fmt.Errorf("refusing to start examine proxy without a proxy access secret: generate one with GenerateProxySecret()")
	}
	if logger == nil {
		logger = slog.Default()
	}

	s := &Server{
		config:      cfg,
		logger:      logger,
		shutdownMgr: shutdownMgr,
		recorder:    rec,
		proxySecret: proxySecret,
	}

	// Create goproxy server
	proxy := goproxy.NewProxyHttpServer()
	// NEVER enable proxy.Verbose in examine mode - it writes to stdout and breaks
	// full-screen terminal applications. Use structured logging instead.
	proxy.Verbose = false

	// Redirect goproxy's internal logging to our slog
	// This ensures WARN/ERROR messages go to log file, not stdout
	proxy.Logger = &slogAdapter{logger: logger}

	// Configure outbound trust + streaming transport (examine MITMs every
	// host, so its trust anchors are the system roots — pinning to one CA
	// would make a discovery tool that can talk to exactly one service)
	configureTransport(proxy, nil)

	// Create certificate store adapter
	certStore := NewGoproxyCertStore(certCache)

	// Configure CONNECT handler - MITM ALL connections (no filtering),
	// always wrapped by the proxy-auth gate. [LAW:dataflow-not-control-flow]
	proxy.OnRequest().HandleConnectFunc(proxyAuthConnectHandler(proxySecret, examine.ConnectHandler(certStore, logger)))

	// Proxy authentication handler runs FIRST on the request path
	proxy.OnRequest().DoFunc(proxyAuthHandler(proxySecret))

	// Add request ID handler - provides correlation colors for all requests
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

	return s, nil
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

// ProxyURL returns the proxy URL with embedded credentials for use in HTTP_PROXY.
// If no proxy secret is configured, returns a plain URL.
// Returns empty string if server is not started.
func (s *Server) ProxyURL() string {
	addr := s.Addr()
	if addr == "" {
		return ""
	}

	if s.proxySecret != "" {
		return "http://" + ProxyAuthUser + ":" + s.proxySecret + "@" + addr
	}
	return "http://" + addr
}

// GetRecorder returns the HAR recorder (for testing/debugging).
func (s *Server) GetRecorder() *recorder.Recorder {
	return s.recorder
}
