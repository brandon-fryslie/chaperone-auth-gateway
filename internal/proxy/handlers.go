package proxy

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/bmf/chaperone/internal/auth"
	"github.com/bmf/chaperone/internal/examine"
	"github.com/bmf/chaperone/internal/log"
	"github.com/bmf/chaperone/internal/recorder"
	"github.com/bmf/chaperone/internal/secrets"
	"github.com/bmf/chaperone/internal/service"
	"github.com/elazarl/goproxy"
)

// requestMetadata stores request-specific data for HAR recording.
type requestMetadata struct {
	startTime time.Time
	request   *http.Request
}

// connectHandler creates a CONNECT handler that decides whether to MITM or transparently tunnel.
func connectHandler(registry service.ServiceRegistry, certStore *GoproxyCertStore, logger *slog.Logger) func(string, *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
	return func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
		// Add request ID to context for logging
		reqCtx := log.WithRequestID(ctx.Req.Context())
		ctx.Req = ctx.Req.WithContext(reqCtx)

		if service.ShouldMITM(registry, host) {
			log.Debug(reqCtx, "handling CONNECT request with MITM", "host", host)

			// Create TLS config function that generates certificates per hostname
			tlsConfigFunc := func(host string, ctx *goproxy.ProxyCtx) (*tls.Config, error) {
				cert, err := certStore.Fetch(host, nil)
				if err != nil {
					log.Error(reqCtx, "failed to get certificate for MITM", err, "host", host)
					return nil, err
				}

				log.Debug(reqCtx, "MITM TLS handshake successful", "host", host)
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

		log.Debug(reqCtx, "using transparent tunnel for non-configured domain", "host", host)
		return goproxy.OkConnect, host
	}
}

// policyHandler creates a handler that enforces service policies.
func policyHandler(registry service.ServiceRegistry, enforcer *service.Enforcer, logger *slog.Logger) func(*http.Request, *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	return func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		reqCtx := r.Context()

		svc, found := registry.Lookup(r.Host)
		if !found || svc.Policy == nil {
			return r, nil
		}

		// Check method
		if err := enforcer.CheckMethod(r.Method, svc.Policy); err != nil {
			logger.Warn("policy violation - method not allowed", "error", err, "hostname", r.Host)
			statusCode := http.StatusForbidden
			if strings.Contains(err.Error(), "too large") {
				statusCode = http.StatusRequestEntityTooLarge
			}
			return r, goproxy.NewResponse(r, goproxy.ContentTypeText, statusCode, err.Error())
		}

		// Check path
		if err := enforcer.CheckPath(r.URL.Path, svc.Policy); err != nil {
			logger.Warn("policy violation - path not allowed", "error", err, "hostname", r.Host, "path", r.URL.Path)
			return r, goproxy.NewResponse(r, goproxy.ContentTypeText, http.StatusForbidden, err.Error())
		}

		// Check body size
		if err := enforcer.CheckBodySize(r.ContentLength, svc.Policy); err != nil {
			logger.Warn("policy violation - body too large", "error", err, "hostname", r.Host)
			return r, goproxy.NewResponse(r, goproxy.ContentTypeText, http.StatusRequestEntityTooLarge, err.Error())
		}

		log.Debug(reqCtx, "policy check passed", "hostname", r.Host)
		return r, nil
	}
}

// dropHandler creates a handler that blocks requests matching drop patterns.
func dropHandler(registry service.ServiceRegistry, logger *slog.Logger) func(*http.Request, *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	return func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		reqCtx := r.Context()

		svc, found := registry.Lookup(r.Host)
		if !found || svc.Policy == nil || len(svc.Policy.Drop) == 0 {
			return r, nil
		}

		// Normalize hostname (strip port for matching)
		hostname := r.Host
		if host, _, err := net.SplitHostPort(hostname); err == nil {
			hostname = host
		}

		// Check if request matches any drop pattern
		for _, pattern := range svc.Policy.Drop {
			urlPattern := service.NewURLPattern(pattern)
			if urlPattern.Matches(hostname, r.URL.Path) {
				log.Info(reqCtx, "dropping request matching pattern",
					"pattern", pattern,
					"host", r.Host,
					"path", r.URL.Path)
				return r, goproxy.NewResponse(r, goproxy.ContentTypeText,
					http.StatusForbidden, "Request blocked by drop policy")
			}
		}

		return r, nil
	}
}

// knownAuthHeaders is the list of headers that commonly carry authentication credentials.
// These are automatically stripped from ALL requests to configured services to prevent
// credential leakage when applications send unintended auth headers.
var knownAuthHeaders = []string{
	"authorization",
	"x-api-key",
	"x-auth-token",
	"api-key",
	"apikey",
	"x-access-token",
	"x-token",
	"token",
	"bearer",
	"x-session-token",
	"x-csrf-token",
	"x-xsrf-token",
}

// securityStripAuthHandler creates a handler that ALWAYS strips known auth headers
// from requests to configured services. This is a security measure to prevent
// credential leakage when applications (like Claude Code) send unintended
// authentication headers to third-party providers.
//
// WARNING: This is not configurable. If Chaperone is handling auth for a service,
// it strips ALL known auth headers first, then injects the correct credentials.
func securityStripAuthHandler(registry service.ServiceRegistry, logger *slog.Logger) func(*http.Request, *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	return func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		// Only strip for configured services (where we're doing MITM)
		_, found := registry.Lookup(r.Host)
		if !found {
			return r, nil
		}

		// Check each known auth header
		var strippedHeaders []string
		for _, knownHeader := range knownAuthHeaders {
			// Find all case variations of this header
			for actualHeader := range r.Header {
				if strings.ToLower(actualHeader) == knownHeader {
					// Capture the value for the warning (redacted)
					value := r.Header.Get(actualHeader)
					redactedValue := redactCredential(value)

					r.Header.Del(actualHeader)
					strippedHeaders = append(strippedHeaders,
						fmt.Sprintf("%s=%s", actualHeader, redactedValue))
				}
			}
		}

		// Log a BIG warning if we stripped anything
		if len(strippedHeaders) > 0 {
			// Use slog directly for WARN level since our log package doesn't have Warn
			slog.Default().Warn(""+
				"████████████████████████████████████████████████████████████████████████████████\n"+
				"██  SECURITY: Stripped existing auth headers from request!                    ██\n"+
				"██  This prevents credential leakage to third-party services.                 ██\n"+
				"██  The client application sent credentials that were NOT from Chaperone.     ██\n"+
				"████████████████████████████████████████████████████████████████████████████████",
				"host", r.Host,
				"path", r.URL.Path,
				"stripped_headers", strippedHeaders,
			)
		}

		return r, nil
	}
}

// redactCredential returns a redacted version of a credential for logging.
func redactCredential(value string) string {
	if len(value) <= 8 {
		return "[REDACTED]"
	}
	// Show first 4 and last 4 characters
	return value[:4] + "..." + value[len(value)-4:]
}

// stripHandler creates a handler that removes specified headers from requests.
func stripHandler(registry service.ServiceRegistry, logger *slog.Logger) func(*http.Request, *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	return func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		reqCtx := r.Context()

		svc, found := registry.Lookup(r.Host)
		if !found || svc.Policy == nil || len(svc.Policy.Strip) == 0 {
			return r, nil
		}

		// Strip specified headers (case-insensitive)
		for _, headerName := range svc.Policy.Strip {
			headerNameLower := strings.ToLower(headerName)

			// Find all case variations of this header
			strippedAny := false
			for actualHeader := range r.Header {
				if strings.ToLower(actualHeader) == headerNameLower {
					r.Header.Del(actualHeader)
					strippedAny = true
				}
			}

			if strippedAny {
				log.Debug(reqCtx, "stripped header from request",
					"header", headerName,
					"host", r.Host,
					"path", r.URL.Path)
			}
		}

		return r, nil
	}
}

// authHandler creates a handler that injects authentication credentials.
func authHandler(registry service.ServiceRegistry, secretRegistry *secrets.Registry, authRegistry *auth.Registry, logger *slog.Logger) func(*http.Request, *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	return func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		reqCtx := r.Context()

		svc, found := registry.Lookup(r.Host)
		if !found {
			return r, nil
		}

		// Skip auth if registries not configured
		if secretRegistry == nil || authRegistry == nil {
			return r, nil
		}

		// Fetch secret
		secret, err := secretRegistry.Fetch(reqCtx, svc.CredentialRef)
		if err != nil {
			log.Error(reqCtx, "failed to fetch secret", err,
				"service", svc.HostPattern,
				"ref", svc.CredentialRef)
			return r, goproxy.NewResponse(r, goproxy.ContentTypeText,
				http.StatusServiceUnavailable, "Secret unavailable")
		}

		// Get authentication strategy
		strategy, err := authRegistry.Get(svc.AuthStrategyRef)
		if err != nil {
			log.Error(reqCtx, "unknown authentication strategy", err,
				"service", svc.HostPattern,
				"strategy", svc.AuthStrategyRef)
			return r, goproxy.NewResponse(r, goproxy.ContentTypeText,
				http.StatusBadGateway, "Authentication configuration error")
		}

		// Apply authentication to request
		if err := strategy.Apply(reqCtx, r, secret); err != nil {
			log.Error(reqCtx, "failed to apply authentication", err,
				"service", svc.HostPattern,
				"strategy", svc.AuthStrategyRef)
			return r, goproxy.NewResponse(r, goproxy.ContentTypeText,
				http.StatusBadGateway, "Authentication failed")
		}

		log.Info(reqCtx, "injected credential",
			"credential_ref", svc.CredentialRef,
			"auth_strategy", svc.AuthStrategyRef,
			"path", r.URL.Path,
			"host", r.Host)

		return r, nil
	}
}

// recordRequestHandler creates a handler that captures request start time for HAR recording.
func recordRequestHandler(rec *recorder.Recorder) func(*http.Request, *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	return func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		// Store request metadata for response handler
		ctx.UserData = &requestMetadata{
			startTime: time.Now(),
			request:   r.Clone(r.Context()),
		}

		return r, nil
	}
}

// recordResponseHandler creates a handler that completes HAR entry with response data.
func recordResponseHandler(rec *recorder.Recorder) func(*http.Response, *goproxy.ProxyCtx) *http.Response {
	return func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
		if ctx.UserData == nil {
			return resp
		}

		meta, ok := ctx.UserData.(*requestMetadata)
		if !ok {
			return resp
		}

		// Record the request and response
		endTime := time.Now()
		recordResponse := rec.RecordRequest(meta.request, meta.startTime)
		recordResponse(resp, nil, endTime)

		return resp
	}
}

// examineConnectHandler creates a CONNECT handler that MITMs ALL connections for examine mode.
// Unlike the normal connectHandler, this always MITMs regardless of service registry configuration.
func examineConnectHandler(certStore *GoproxyCertStore, logger *slog.Logger) func(string, *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
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

// examineRequestHandler creates a handler for examine mode that logs requests without modifying them.
func examineRequestHandler(examineLogger *examine.Logger) func(*http.Request, *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	return func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		// Log the request to help user discover auth patterns
		examineLogger.LogRequest(r)

		// Pass through unchanged - no modification
		return r, nil
	}
}

// examineResponseHandler creates a handler for examine mode that logs responses without modifying them.
func examineResponseHandler(examineLogger *examine.Logger) func(*http.Response, *goproxy.ProxyCtx) *http.Response {
	return func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
		// Log the response to help user discover auth patterns
		examineLogger.LogResponse(resp)

		// Pass through unchanged - no modification
		return resp
	}
}
