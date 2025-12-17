package client

import (
	"crypto/tls"
	"crypto/x509"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Client is an HTTP client configured for upstream HTTPS connections.
type Client struct {
	httpClient *http.Client
	logger     *slog.Logger
}

// NewClient creates a new upstream HTTP client with proper TLS configuration.
// The client uses the system certificate pool for TLS validation.
// IMPORTANT: This client does NOT use any proxy for outbound connections
// to avoid proxy loops when Chaperone itself is running as a proxy.
func NewClient(logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}

	// Get system certificate pool for proper TLS validation
	certPool, err := x509.SystemCertPool()
	if err != nil {
		// Fallback to empty pool if system pool unavailable
		logger.Warn("failed to load system cert pool, using empty pool", "error", err)
		certPool = x509.NewCertPool()
	}

	// Create TLS config with system trust store
	tlsConfig := &tls.Config{
		RootCAs:    certPool,
		MinVersion: tls.VersionTLS12,
	}

	// Create transport with proper timeouts and connection pooling
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSClientConfig:       tlsConfig,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		MaxIdleConns:          1000,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     true, // Enable HTTP/2
		// IMPORTANT: Disable proxy for outbound connections to avoid proxy loops
		Proxy: nil,
	}

	return &Client{
		httpClient: &http.Client{
			Transport: transport,
			// No timeout on the client - let the caller control via context
		},
		logger: logger,
	}
}

// NewClientWithHTTPClient creates a Client wrapping an existing http.Client.
// This is primarily useful for testing with custom TLS configurations.
func NewClientWithHTTPClient(httpClient *http.Client, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}
	if httpClient == nil {
		// Fall back to creating a default client
		return NewClient(logger)
	}

	return &Client{
		httpClient: httpClient,
		logger:     logger,
	}
}

// Do executes an HTTP request using the upstream client.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	return c.httpClient.Do(req)
}

// HTTPClient returns the underlying http.Client for advanced use cases.
func (c *Client) HTTPClient() *http.Client {
	return c.httpClient
}
