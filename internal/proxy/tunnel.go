package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/bmf/chaperone/internal/log"
	"github.com/bmf/chaperone/internal/mitm"
	"github.com/bmf/chaperone/internal/service"
)

// TunnelHandler implements http.Handler to handle CONNECT requests
// for establishing HTTPS tunnels.
type TunnelHandler struct {
	logger      *slog.Logger
	registry    service.ServiceRegistry // nil for transparent-only mode
	certCache   *mitm.CertCache         // nil for transparent-only mode
	mitmHandler *MITMHandler            // nil for transparent-only mode
}

// ServeHTTP handles incoming HTTP requests, specifically CONNECT requests.
func (h *TunnelHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Create context with request ID for logging
	ctx := log.WithRequestID(r.Context())

	// Only handle CONNECT method for tunneling
	if r.Method != http.MethodConnect {
		log.Info(ctx, "rejecting non-CONNECT request",
			"method", r.Method,
			"host", r.Host,
		)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	h.HandleCONNECT(ctx, w, r)
}

// HandleCONNECT processes a CONNECT request and establishes either a MITM
// connection or a transparent tunnel based on service configuration.
func (h *TunnelHandler) HandleCONNECT(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	// Check if we should MITM this domain
	if h.registry != nil && service.ShouldMITM(h.registry, r.Host) {
		h.handleMITM(ctx, w, r)
	} else {
		log.Debug(ctx, "using transparent tunnel for non-configured domain", "host", r.Host)
		h.handleTransparentTunnel(ctx, w, r)
	}
}

// handleMITM performs a man-in-the-middle TLS termination for configured services.
func (h *TunnelHandler) handleMITM(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	// Hijack the client connection
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		log.Error(ctx, "ResponseWriter does not support hijacking", fmt.Errorf("hijacking not supported"))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		log.Error(ctx, "failed to hijack connection", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer clientConn.Close() //nolint:errcheck // Best effort cleanup

	// Send 200 Connection Established response
	_, err = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	if err != nil {
		log.Error(ctx, "failed to send CONNECT response", err)
		return
	}

	// Get or generate certificate for this hostname
	cert, err := h.certCache.GetCertificate(r.Host)
	if err != nil {
		log.Error(ctx, "failed to get certificate for MITM", err, "host", r.Host)
		return
	}

	// Create TLS config for client connection
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{*cert},
		MinVersion:   tls.VersionTLS12,
	}

	// Wrap client connection with TLS
	tlsConn := tls.Server(clientConn, tlsConfig)

	// Set handshake timeout
	deadline := time.Now().Add(30 * time.Second)
	if err := tlsConn.SetDeadline(deadline); err != nil {
		log.Error(ctx, "failed to set TLS deadline", err)
		return
	}

	// Perform TLS handshake
	if err := tlsConn.Handshake(); err != nil {
		log.Error(ctx, "TLS handshake failed", err, "host", r.Host)
		return
	}

	// Clear deadline after handshake
	if err := tlsConn.SetDeadline(time.Time{}); err != nil {
		log.Error(ctx, "failed to clear TLS deadline", err)
		return
	}

	// Proxy HTTP requests through MITM handler
	// Pass the full r.Host (with port) so the handler can construct correct upstream URLs
	if err := h.mitmHandler.ProxyRequest(ctx, tlsConn, r.Host); err != nil {
		log.Debug(ctx, "MITM proxy ended", "error", err, "host", r.Host)
	}
}

// handleTransparentTunnel establishes a transparent TCP tunnel without TLS termination.
// This is the original Phase 1 behavior for non-configured domains.
func (h *TunnelHandler) handleTransparentTunnel(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	// Establish connection to upstream server
	upstreamConn, err := net.DialTimeout("tcp", r.Host, 10*time.Second)
	if err != nil {
		log.Error(ctx, "failed to connect to upstream", err,
			"host", r.Host,
		)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer upstreamConn.Close() //nolint:errcheck // Best effort cleanup

	// Hijack the client connection
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		log.Error(ctx, "ResponseWriter does not support hijacking", fmt.Errorf("hijacking not supported"))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		log.Error(ctx, "failed to hijack connection", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer clientConn.Close() //nolint:errcheck // Best effort cleanup

	// Send 200 Connection Established response
	_, err = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	if err != nil {
		log.Error(ctx, "failed to send CONNECT response", err)
		return
	}

	// Copy data bidirectionally between client and upstream
	errChan := make(chan error, 2)

	// Client -> Upstream
	go func() {
		_, err := io.Copy(upstreamConn, clientConn)
		errChan <- err
	}()

	// Upstream -> Client
	go func() {
		_, err := io.Copy(clientConn, upstreamConn)
		errChan <- err
	}()

	// Wait for one direction to complete
	err = <-errChan
	if err != nil && err != io.EOF {
		log.Debug(ctx, "tunnel copy error", "error", err)
	}

	log.Debug(ctx, "tunnel closed", "host", r.Host)
}
