package init

import (
	"crypto/tls"
	"log/slog"
	"net"
	"net/http"

	"github.com/bmf/chaperone/internal/log"
	"github.com/elazarl/goproxy"
)

// GoproxyCertStore is the interface required for certificate generation.
// This is defined in the proxy package, but we accept it as an interface
// to avoid circular dependencies.
type GoproxyCertStore interface {
	Fetch(hostname string, gen func() (*tls.Certificate, error)) (*tls.Certificate, error)
}

// FindingCallback is called when a new finding is discovered in init mode.
type FindingCallback func(host string, finding *Finding)

// ConnectHandler creates a CONNECT handler that MITMs ALL connections for init mode.
// Similar to examine mode, this always MITMs to analyze traffic for auth discovery.
func ConnectHandler(certStore GoproxyCertStore, logger *slog.Logger) func(string, *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
	return func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
		reqCtx := log.WithRequestID(ctx.Req.Context())
		ctx.Req = ctx.Req.WithContext(reqCtx)

		log.Debug(reqCtx, "init: MITM connection", "host", host)

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

// RequestHandler creates a handler for init mode that analyzes requests for auth patterns.
func RequestHandler(detector *Detector, evidence *Evidence, onFinding FindingCallback) func(*http.Request, *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	return func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		// Get hosts before analysis to track new findings
		hostsBefore := len(evidence.GetAllHosts())

		// Analyze request for auth patterns
		detector.AnalyzeRequest(r)

		// Check if this is a new finding we should report
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}

		// If we found something new for this host, report it
		if onFinding != nil {
			topFinding := evidence.GetTopFinding(host)
			if topFinding != nil && len(evidence.GetAllHosts()) > hostsBefore {
				onFinding(host, topFinding)
			}
		}

		// Pass through unchanged
		return r, nil
	}
}

// ResponseHandler creates a no-op response handler for init mode.
func ResponseHandler() func(*http.Response, *goproxy.ProxyCtx) *http.Response {
	return func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
		return resp
	}
}
