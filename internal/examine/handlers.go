package examine

import (
	"crypto/tls"
	"log/slog"
	"net/http"
	"sync"

	"github.com/bmf/chaperone/internal/log"
	"github.com/elazarl/goproxy"
)

// GoproxyCertStore is the interface required for certificate generation.
// This is defined in the proxy package, but we accept it as an interface
// to avoid circular dependencies.
type GoproxyCertStore interface {
	Fetch(hostname string, gen func() (*tls.Certificate, error)) (*tls.Certificate, error)
}

// ConnectHandler creates a CONNECT handler that MITMs ALL connections for examine mode.
// Unlike the normal connectHandler, this always MITMs regardless of service registry configuration.
func ConnectHandler(certStore GoproxyCertStore, logger *slog.Logger) func(string, *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
	return func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
		// Add request ID to context for logging
		reqCtx := log.WithRequestID(ctx.Req.Context())
		ctx.Req = ctx.Req.WithContext(reqCtx)

		log.Debug(reqCtx, "examine: MITM connection", "host", host)

		// Create TLS config function that generates certificates per hostname
		tlsConfigFunc := func(host string, ctx *goproxy.ProxyCtx) (*tls.Config, error) {
			cert, err := certStore.Fetch(host, nil)
			if err != nil {
				log.Error(reqCtx, "failed to get certificate for MITM", err, "host", host)
				return nil, err
			}

			return &tls.Config{
				Certificates: []tls.Certificate{*cert},
				MinVersion:   tls.VersionTLS12,
			}, nil
		}

		return &goproxy.ConnectAction{
			Action:    goproxy.ConnectMitm,
			TLSConfig: tlsConfigFunc,
		}, host
	}
}

// RequestHandler creates a handler for examine mode that logs requests without modifying them.
// If sentinelChan is non-nil, it will be closed (once) when sentinel is detected.
func RequestHandler(examineLogger *Logger, sentinelChan chan struct{}) func(*http.Request, *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	var sentinelOnce sync.Once
	return func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		// Log the request to help user discover auth patterns
		foundSentinel := examineLogger.LogRequest(r)

		// Signal if sentinel was found
		if foundSentinel && sentinelChan != nil {
			sentinelOnce.Do(func() {
				close(sentinelChan)
			})
		}

		// Pass through unchanged - no modification
		return r, nil
	}
}

// ResponseHandler creates a handler for examine mode that logs responses without modifying them.
func ResponseHandler(examineLogger *Logger) func(*http.Response, *goproxy.ProxyCtx) *http.Response {
	return func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
		// Log the response to help user discover auth patterns
		examineLogger.LogResponse(resp)

		// Pass through unchanged - no modification
		return resp
	}
}
