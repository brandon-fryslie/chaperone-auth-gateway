package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"

	apierrors "github.com/bmf/chaperone/internal/errors"
	"github.com/bmf/chaperone/internal/auth"
	"github.com/bmf/chaperone/internal/client"
	"github.com/bmf/chaperone/internal/log"
	"github.com/bmf/chaperone/internal/secrets"
	"github.com/bmf/chaperone/internal/service"
)

// MITMHandler handles HTTP request proxying in MITM mode.
// It reads HTTP requests from a TLS connection, enforces policies,
// injects authentication, and forwards them to upstream servers.
type MITMHandler struct {
	client         *client.Client
	logger         *slog.Logger
	registry       service.ServiceRegistry
	enforcer       *service.Enforcer
	secretRegistry *secrets.Registry
	authRegistry   *auth.Registry
}

// NewMITMHandler creates a new MITM handler.
func NewMITMHandler(
	c *client.Client,
	logger *slog.Logger,
	registry service.ServiceRegistry,
	enforcer *service.Enforcer,
	secretRegistry *secrets.Registry,
	authRegistry *auth.Registry,
) *MITMHandler {
	if logger == nil {
		logger = slog.Default()
	}
	if c == nil {
		c = client.NewClient(logger)
	}
	if enforcer == nil {
		enforcer = service.NewPolicyEnforcer(logger)
	}

	return &MITMHandler{
		client:         c,
		logger:         logger,
		registry:       registry,
		enforcer:       enforcer,
		secretRegistry: secretRegistry,
		authRegistry:   authRegistry,
	}
}

// ProxyRequest handles the complete MITM request flow:
// 1. Read HTTP request from client TLS connection
// 2. Lookup service and enforce policy
// 3. Inject authentication
// 4. Forward to upstream
// 5. Stream response back to client
func (h *MITMHandler) ProxyRequest(ctx context.Context, clientConn *tls.Conn, hostname string) error {
	// Create buffered reader for HTTP request parsing
	reader := bufio.NewReader(clientConn)

	for {
		// Read HTTP request from client
		req, err := http.ReadRequest(reader)
		if err != nil {
			if err == io.EOF {
				// Client closed connection - normal termination
				return nil
			}
			// Check if this is a connection close error
			if strings.Contains(err.Error(), "use of closed network connection") {
				return nil
			}
			log.Debug(ctx, "failed to read request from client", "error", err)
			return fmt.Errorf("failed to read request: %w", err)
		}

		// Add context to request
		req = req.WithContext(ctx)

		// Lookup service for this hostname
		svc, found := h.registry.Lookup(hostname)
		if !found {
			// This shouldn't happen since we only MITM configured services
			log.Error(ctx, "service not found for MITM'd hostname", fmt.Errorf("service lookup failed"), "hostname", hostname)
			h.sendErrorResponse(clientConn, http.StatusInternalServerError, "Service configuration error")
			continue
		}

		// Enforce policy
		if err := h.enforcePolicy(ctx, svc, req); err != nil {
			h.logger.Warn("policy violation", "error", err, "hostname", hostname)

			// Determine status code based on error type
			statusCode := http.StatusForbidden
			if policyErr, ok := err.(*apierrors.PolicyError); ok {
				if strings.Contains(policyErr.Error(), "too large") {
					statusCode = http.StatusRequestEntityTooLarge
				}
			}

			h.sendErrorResponse(clientConn, statusCode, err.Error())
			continue
		}

		// Forward request to upstream (with authentication)
		if err := h.forwardRequest(ctx, clientConn, req, hostname, svc); err != nil {
			log.Error(ctx, "failed to forward request", err, "hostname", hostname)
			h.sendErrorResponse(clientConn, http.StatusBadGateway, "Failed to proxy request")
			continue
		}

		// Check if connection should be kept alive
		if req.Close || req.Header.Get("Connection") == "close" {
			return nil
		}
	}
}

// enforcePolicy checks if the request is allowed by the service policy.
func (h *MITMHandler) enforcePolicy(ctx context.Context, svc *service.Service, req *http.Request) error {
	policy := svc.Policy
	if policy == nil {
		// No policy means all requests allowed
		return nil
	}

	// Check method
	if err := h.enforcer.CheckMethod(req.Method, policy); err != nil {
		return err
	}

	// Check path
	if err := h.enforcer.CheckPath(req.URL.Path, policy); err != nil {
		return err
	}

	// Check body size
	if err := h.enforcer.CheckBodySize(req.ContentLength, policy); err != nil {
		return err
	}

	return nil
}

// forwardRequest forwards an HTTP request to the upstream server and streams the response back.
// It injects authentication credentials before forwarding.
// The hostname parameter includes the port from the original CONNECT request.
func (h *MITMHandler) forwardRequest(ctx context.Context, clientConn net.Conn, req *http.Request, hostname string, svc *service.Service) error {
	// Build absolute URL for upstream request using the original hostname (with port)
	upstreamURL := fmt.Sprintf("https://%s%s", hostname, req.URL.RequestURI())

	// Create new request for upstream
	upstreamReq, err := http.NewRequestWithContext(ctx, req.Method, upstreamURL, req.Body)
	if err != nil {
		return fmt.Errorf("failed to create upstream request: %w", err)
	}

	// Copy headers from client request
	upstreamReq.Header = make(http.Header)
	for k, v := range req.Header {
		// Skip hop-by-hop headers
		if isHopByHopHeader(k) {
			continue
		}
		upstreamReq.Header[k] = v
	}

	// Inject authentication if registries are available
	if h.secretRegistry != nil && h.authRegistry != nil {
		// Fetch secret
		secret, err := h.secretRegistry.Fetch(ctx, svc.CredentialRef)
		if err != nil {
			log.Error(ctx, "failed to fetch secret", err,
				"service", svc.HostPattern,
				"ref", svc.CredentialRef)
			h.sendErrorResponse(clientConn, http.StatusServiceUnavailable, "Secret unavailable")
			return fmt.Errorf("secret fetch failed: %w", err)
		}

		// Get authentication strategy
		strategy, err := h.authRegistry.Get(svc.AuthStrategyRef)
		if err != nil {
			log.Error(ctx, "unknown authentication strategy", err,
				"service", svc.HostPattern,
				"strategy", svc.AuthStrategyRef)
			h.sendErrorResponse(clientConn, http.StatusBadGateway, "Authentication configuration error")
			return fmt.Errorf("strategy not found: %w", err)
		}

		// Apply authentication to upstream request
		if err := strategy.Apply(ctx, upstreamReq, secret); err != nil {
			log.Error(ctx, "failed to apply authentication", err,
				"service", svc.HostPattern,
				"strategy", svc.AuthStrategyRef)
			h.sendErrorResponse(clientConn, http.StatusBadGateway, "Authentication failed")
			return fmt.Errorf("authentication application failed: %w", err)
		}
	}

	// Make request to upstream
	resp, err := h.client.Do(upstreamReq)
	if err != nil {
		return fmt.Errorf("upstream request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // Best effort cleanup

	// Write response to client using standard HTTP response writing
	// This ensures proper handling of headers, chunked encoding, and other HTTP details
	if err := resp.Write(clientConn); err != nil {
		return fmt.Errorf("failed to write response to client: %w", err)
	}

	return nil
}

// sendErrorResponse sends an HTTP error response to the client.
func (h *MITMHandler) sendErrorResponse(conn net.Conn, statusCode int, message string) {
	resp := &http.Response{
		StatusCode: statusCode,
		Status:     http.StatusText(statusCode),
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(message)),
	}
	resp.Header.Set("Content-Type", "text/plain")
	resp.Header.Set("Connection", "close")
	resp.ContentLength = int64(len(message))

	// Write response (ignore errors - connection might be closed)
	_ = resp.Write(conn)
}

// isHopByHopHeader returns true if the header is a hop-by-hop header.
// These headers are specific to a single connection and should not be forwarded.
func isHopByHopHeader(header string) bool {
	hopByHop := []string{
		"Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te",
		"Trailers",
		"Transfer-Encoding",
		"Upgrade",
	}

	headerLower := strings.ToLower(header)
	for _, h := range hopByHop {
		if strings.ToLower(h) == headerLower {
			return true
		}
	}
	return false
}
