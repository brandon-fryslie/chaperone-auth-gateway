package proxy

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
	"sync"

	"github.com/bmf/chaperone/internal/audit"
	"github.com/bmf/chaperone/internal/auth"
	"github.com/bmf/chaperone/internal/examine"
	chaperoneInit "github.com/bmf/chaperone/internal/init"
	"github.com/bmf/chaperone/internal/log"
	"github.com/bmf/chaperone/internal/recorder"
	"github.com/bmf/chaperone/internal/secrets"
	"github.com/bmf/chaperone/internal/service"
	"github.com/elazarl/goproxy"
)

// requestMetadata stores request-specific data for HAR recording and response logging.
type requestMetadata struct {
	startTime              time.Time
	request                *http.Request
	strippedHeaders        []string
	requestID              string // For correlating request/response log colors
	colorR, colorG, colorB int    // Pre-computed correlation color for this connection
}

// extractClientIP extracts the client IP from an HTTP request.
// Handles both IP:port format and bare IP addresses.
func extractClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr might not have port (shouldn't happen in normal flow)
		return r.RemoteAddr
	}
	return host
}

// connectHandler creates a CONNECT handler that decides whether to MITM or transparently tunnel.
func connectHandler(registry service.ServiceRegistry, certStore *GoproxyCertStore, logger *slog.Logger) func(string, *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
	return func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
		// Add request ID to context for logging
		reqCtx := log.WithRequestID(ctx.Req.Context())
		ctx.Req = ctx.Req.WithContext(reqCtx)

		// Store request ID and pre-generate correlation color for this connection
		// All requests through this CONNECT tunnel will use the same color
		requestID := log.RequestID(reqCtx)
		r, g, b := log.NextCorrelationColor()
		ctx.UserData = &requestMetadata{
			requestID: requestID,
			colorR:    r,
			colorG:    g,
			colorB:    b,
		}

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
func policyHandler(registry service.ServiceRegistry, enforcer *service.Enforcer, auditLogger *audit.Logger, logger *slog.Logger) func(*http.Request, *goproxy.ProxyCtx) (*http.Request, *http.Response) {
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

			// AUDIT: Policy denied event
			if auditLogger != nil {
				auditLogger.Log(audit.Entry{
					Event:      audit.EventPolicyDenied,
					Service:    svc.Name,
					Host:       r.Host,
					Path:       r.URL.Path,
					Method:     r.Method,
					RequestID:  log.RequestID(reqCtx),
					ClientIP:   extractClientIP(r),
					Outcome:    "blocked",
					StatusCode: statusCode,
					Detail:     fmt.Sprintf("method %s not allowed: %v", r.Method, err),
				})
			}

			return r, goproxy.NewResponse(r, goproxy.ContentTypeText, statusCode, err.Error())
		}

		// Check path
		if err := enforcer.CheckPath(r.URL.Path, svc.Policy); err != nil {
			logger.Warn("policy violation - path not allowed", "error", err, "hostname", r.Host, "path", r.URL.Path)

			// AUDIT: Policy denied event
			if auditLogger != nil {
				auditLogger.Log(audit.Entry{
					Event:      audit.EventPolicyDenied,
					Service:    svc.Name,
					Host:       r.Host,
					Path:       r.URL.Path,
					Method:     r.Method,
					RequestID:  log.RequestID(reqCtx),
					ClientIP:   extractClientIP(r),
					Outcome:    "blocked",
					StatusCode: http.StatusForbidden,
					Detail:     fmt.Sprintf("path %s not allowed: %v", r.URL.Path, err),
				})
			}

			return r, goproxy.NewResponse(r, goproxy.ContentTypeText, http.StatusForbidden, err.Error())
		}

		// Check body size
		if err := enforcer.CheckBodySize(r.ContentLength, svc.Policy); err != nil {
			logger.Warn("policy violation - body too large", "error", err, "hostname", r.Host)

			// AUDIT: Policy denied event
			if auditLogger != nil {
				auditLogger.Log(audit.Entry{
					Event:      audit.EventPolicyDenied,
					Service:    svc.Name,
					Host:       r.Host,
					Path:       r.URL.Path,
					Method:     r.Method,
					RequestID:  log.RequestID(reqCtx),
					ClientIP:   extractClientIP(r),
					Outcome:    "blocked",
					StatusCode: http.StatusRequestEntityTooLarge,
					Detail:     fmt.Sprintf("body size %d exceeds limit: %v", r.ContentLength, err),
				})
			}

			return r, goproxy.NewResponse(r, goproxy.ContentTypeText, http.StatusRequestEntityTooLarge, err.Error())
		}

		log.Debug(reqCtx, "policy check passed", "hostname", r.Host)
		return r, nil
	}
}

// dropHandler creates a handler that blocks requests matching drop patterns.
func dropHandler(registry service.ServiceRegistry, auditLogger *audit.Logger, logger *slog.Logger) func(*http.Request, *goproxy.ProxyCtx) (*http.Request, *http.Response) {
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

				// AUDIT: Request dropped event
				if auditLogger != nil {
					auditLogger.Log(audit.Entry{
						Event:      audit.EventRequestDropped,
						Service:    svc.Name,
						Host:       r.Host,
						Path:       r.URL.Path,
						Method:     r.Method,
						RequestID:  log.RequestID(reqCtx),
						ClientIP:   extractClientIP(r),
						Outcome:    "blocked",
						StatusCode: http.StatusForbidden,
						Detail:     fmt.Sprintf("matched drop pattern: %s", pattern),
					})
				}

				return r, goproxy.NewResponse(r, goproxy.ContentTypeText,
					http.StatusForbidden, "Request blocked by drop policy")
			}
		}

		return r, nil
	}
}

// knownAuthHeaders is the list of headers that commonly carry authentication credentials.
// These are automatically stripped from all outgoing requests for safety.  The problematic
// scenario being this:
//   - A user uses a tool that requires an API KEY, for example, Claude Code
//   - The user overrides the tool configuration to specify the url for a third party LLM provider (e.g., ANTHROPIC_BASE_URL=z.ai/anthropic)
//     In this situation, if the user runs Claude Code without specifying the env var ANTHROPIC_API_KEY and the user is logged
//     in to a Claude Code subscription, the user's subscription credentials will be sent to z.ai with no visible indication
//   - Although this is known/standard behavior and to be expected on some level, this is such an easy configuration mistake to make
//     that I think it is going to be extremely common.  I made the mistake several times when testing this application, even after knowing about it
//   - In this application, stripping existing auth shouldn't be a problem.  If this is problematic for your use case, please
//     let me know what your use case is and why the traffic needs to go through this proxy rather than bypassing it.
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
// EXCEPTION: If a service has a placeholder configured and the request contains
// that exact placeholder, the header is NOT stripped (so authHandler can verify it).
//
// WARNING: This is not configurable. If Chaperone is handling auth for a service,
// it strips ALL known auth headers (except placeholders) first, then injects the correct credentials.
func securityStripAuthHandler(registry service.ServiceRegistry, auditLogger *audit.Logger, logger *slog.Logger) func(*http.Request, *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	return func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		reqCtx := r.Context()

		// Only strip for configured services (where we're doing MITM)
		svc, found := registry.Lookup(r.Host)
		if !found {
			return r, nil
		}

		// Check each known auth header
		var strippedHeaders []string
		for _, knownHeader := range knownAuthHeaders {
			// Find all case variations of this header
			for actualHeader := range r.Header {
				if strings.ToLower(actualHeader) == knownHeader {
					value := r.Header.Get(actualHeader)

					// SECURITY FIX: Don't strip if this is a placeholder token
					if svc.Placeholder != "" && headerContainsPlaceholder(value, svc.Placeholder, svc.AuthStrategyRef) {
						// This header contains the placeholder - keep it for authHandler to verify
						continue
					}

					// Capture the value for logging (redacted)
					redactedValue := redactCredential(value)

					r.Header.Del(actualHeader)
					strippedHeaders = append(strippedHeaders,
						fmt.Sprintf("%s=%s", actualHeader, redactedValue))
				}
			}
		}

		// Store stripped headers in UserData for later logging (combined with inject message)
		if len(strippedHeaders) > 0 {
			if meta, ok := ctx.UserData.(*requestMetadata); ok {
				meta.strippedHeaders = strippedHeaders
			} else {
				// Create metadata if it doesn't exist
				ctx.UserData = &requestMetadata{
					startTime:       time.Now(),
					request:         r,
					strippedHeaders: strippedHeaders,
				}
			}

			// AUDIT: Auth header stripped event
			if auditLogger != nil {
				auditLogger.Log(audit.Entry{
					Event:     audit.EventAuthHeaderStripped,
					Service:   svc.Name,
					Host:      r.Host,
					Path:      r.URL.Path,
					Method:    r.Method,
					RequestID: log.RequestID(reqCtx),
					ClientIP:  extractClientIP(r),
					Outcome:   "success",
					Detail:    fmt.Sprintf("stripped headers: %s", strings.Join(strippedHeaders, ", ")),
				})
			}
		}

		return r, nil
	}
}

// headerContainsPlaceholder checks if a header value contains the placeholder token.
// For bearer auth, it strips the "Bearer " prefix before comparing.
func headerContainsPlaceholder(headerValue string, placeholder string, authStrategy string) bool {
	checkValue := headerValue

	// For bearer tokens, extract the token part (strip "Bearer " prefix)
	if strings.HasPrefix(authStrategy, "bearer") {
		checkValue = strings.TrimPrefix(checkValue, "Bearer ")
		checkValue = strings.TrimSpace(checkValue)
	}

	return checkValue == placeholder
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
func authHandler(registry service.ServiceRegistry, secretRegistry *secrets.Registry, authRegistry *auth.Registry, auditLogger *audit.Logger, logger *slog.Logger) func(*http.Request, *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	// Track warnings for services without placeholders (warn once per service)
	warnedServices := make(map[string]bool)
	var warnMutex sync.Mutex

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

		// PLACEHOLDER CHECK
		if svc.Placeholder != "" {
			// Get the current auth header value
			headerName := "Authorization"
			if svc.HeaderName != "" {
				headerName = svc.HeaderName
			}

			currentValue := r.Header.Get(headerName)

			// For bearer tokens, extract the token part (strip "Bearer " prefix)
			if strings.HasPrefix(svc.AuthStrategyRef, "bearer") {
				currentValue = strings.TrimPrefix(currentValue, "Bearer ")
				currentValue = strings.TrimSpace(currentValue)
			}

			// Check if placeholder matches
			if currentValue != svc.Placeholder {
				// No match - pass through without injection
				log.Debug(reqCtx, "placeholder mismatch, passing through",
					"host", r.Host,
					"expected_prefix", svc.Placeholder[:min(8, len(svc.Placeholder))]+"...")

				// AUDIT: Placeholder mismatch event
				if auditLogger != nil {
					auditLogger.Log(audit.Entry{
						Event:     audit.EventPlaceholderMismatch,
						Service:   svc.Name,
						Host:      r.Host,
						Path:      r.URL.Path,
						Method:    r.Method,
						RequestID: log.RequestID(reqCtx),
						ClientIP:  extractClientIP(r),
						Outcome:   "pass_through",
						Detail:    "placeholder mismatch",
					})
				}

				return r, nil
			}
		} else {
			// No placeholder configured - warn once per service (backward compat)
			warnMutex.Lock()
			if !warnedServices[svc.Name] {
				log.Warn(reqCtx, "no placeholder configured - credentials will be injected but consider adding a placeholder for security",
					"service", svc.Name,
					"host", r.Host,
					"recommendation", "add 'placeholder = \"chap_...\"' to your service config")
				warnedServices[svc.Name] = true
			}
			warnMutex.Unlock()
		}

		// Fetch secret
		secret, err := secretRegistry.Fetch(reqCtx, svc.CredentialRef)
		if err != nil {
			log.Error(reqCtx, "failed to fetch secret", err,
				"service", svc.HostPattern,
				"ref", svc.CredentialRef)

			// AUDIT: Auth failure event
			if auditLogger != nil {
				auditLogger.Log(audit.Entry{
					Event:        audit.EventAuthFailure,
					Service:      svc.Name,
					Host:         r.Host,
					Path:         r.URL.Path,
					Method:       r.Method,
					AuthStrategy: svc.AuthStrategyRef,
					RequestID:    log.RequestID(reqCtx),
					ClientIP:     extractClientIP(r),
					Outcome:      "failure",
					StatusCode:   http.StatusServiceUnavailable,
					ErrorMessage: fmt.Sprintf("secret fetch failed: %v", err),
				})
			}

			return r, goproxy.NewResponse(r, goproxy.ContentTypeText,
				http.StatusServiceUnavailable, "Secret unavailable")
		}

		// Get authentication strategy
		strategy, err := authRegistry.Get(svc.AuthStrategyRef)
		if err != nil {
			log.Error(reqCtx, "unknown authentication strategy", err,
				"service", svc.HostPattern,
				"strategy", svc.AuthStrategyRef)

			// AUDIT: Auth failure event
			if auditLogger != nil {
				auditLogger.Log(audit.Entry{
					Event:        audit.EventAuthFailure,
					Service:      svc.Name,
					Host:         r.Host,
					Path:         r.URL.Path,
					Method:       r.Method,
					AuthStrategy: svc.AuthStrategyRef,
					RequestID:    log.RequestID(reqCtx),
					ClientIP:     extractClientIP(r),
					Outcome:      "failure",
					StatusCode:   http.StatusBadGateway,
					ErrorMessage: fmt.Sprintf("unknown auth strategy: %v", err),
				})
			}

			return r, goproxy.NewResponse(r, goproxy.ContentTypeText,
				http.StatusBadGateway, "Authentication configuration error")
		}

		// Apply authentication to request
		if err := strategy.Apply(reqCtx, r, secret); err != nil {
			log.Error(reqCtx, "failed to apply authentication", err,
				"service", svc.HostPattern,
				"strategy", svc.AuthStrategyRef)

			// AUDIT: Auth failure event
			if auditLogger != nil {
				auditLogger.Log(audit.Entry{
					Event:        audit.EventAuthFailure,
					Service:      svc.Name,
					Host:         r.Host,
					Path:         r.URL.Path,
					Method:       r.Method,
					AuthStrategy: svc.AuthStrategyRef,
					RequestID:    log.RequestID(reqCtx),
					ClientIP:     extractClientIP(r),
					Outcome:      "failure",
					StatusCode:   http.StatusBadGateway,
					ErrorMessage: fmt.Sprintf("auth strategy apply failed: %v", err),
				})
			}

			return r, goproxy.NewResponse(r, goproxy.ContentTypeText,
				http.StatusBadGateway, "Authentication failed")
		}

		// AUDIT LOGGING - after successful injection
		if auditLogger != nil {
			auditLogger.Log(audit.Entry{
				Event:        audit.EventCredentialInjected,
				Service:      svc.Name,
				Host:         r.Host,
				Path:         r.URL.Path,
				Method:       r.Method,
				AuthStrategy: svc.AuthStrategyRef,
				RequestID:    log.RequestID(reqCtx),
				ClientIP:     extractClientIP(r),
				Outcome:      "success",
			})
		}

		// Build log args - include stripped headers if any
		logArgs := []any{
			"credential_ref", svc.CredentialRef,
			"auth_strategy", svc.AuthStrategyRef,
			"path", r.URL.Path,
			"host", r.Host,
		}

		// Include stripped headers from earlier handler if any
		if meta, ok := ctx.UserData.(*requestMetadata); ok && len(meta.strippedHeaders) > 0 {
			logArgs = append(logArgs, "stripped_headers", meta.strippedHeaders)
		}

		log.Info(reqCtx, "injected credential", logArgs...)

		return r, nil
	}
}

// requestIDHandler creates a handler that ensures request ID and correlation color are set in context.
// This should run FIRST so all subsequent handlers have access to these for logging.
func requestIDHandler() func(*http.Request, *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	return func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		var requestID string
		var colorR, colorG, colorB int

		// Get request ID and color from UserData (set in connectHandler)
		if meta, ok := ctx.UserData.(*requestMetadata); ok {
			requestID = meta.requestID
			colorR, colorG, colorB = meta.colorR, meta.colorG, meta.colorB
		}

		// If no request ID, create one
		if requestID == "" {
			reqCtx := log.WithRequestID(r.Context())
			requestID = log.RequestID(reqCtx)
		}

		// If no color, generate one
		if colorR == 0 && colorG == 0 && colorB == 0 {
			colorR, colorG, colorB = log.NextCorrelationColor()
		}

		// Store in UserData for response handler if not already there
		if meta, ok := ctx.UserData.(*requestMetadata); ok {
			meta.requestID = requestID
			meta.colorR, meta.colorG, meta.colorB = colorR, colorG, colorB
		} else {
			ctx.UserData = &requestMetadata{
				requestID: requestID,
				colorR:    colorR,
				colorG:    colorG,
				colorB:    colorB,
			}
		}

		// Update request context with request ID and color for logging
		reqCtx := log.WithRequestIDValue(r.Context(), requestID)
		reqCtx = log.WithCorrelationColor(reqCtx, colorR, colorG, colorB)
		r = r.WithContext(reqCtx)

		return r, nil
	}
}

// recordRequestHandler creates a handler that captures request start time for HAR recording.
func recordRequestHandler(rec *recorder.Recorder) func(*http.Request, *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	return func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		// Get or create metadata
		if meta, ok := ctx.UserData.(*requestMetadata); ok {
			meta.request = r.Clone(r.Context())
			if meta.startTime.IsZero() {
				meta.startTime = time.Now()
			}
		} else {
			ctx.UserData = &requestMetadata{
				startTime: time.Now(),
				request:   r.Clone(r.Context()),
				requestID: log.RequestID(r.Context()),
			}
		}

		return r, nil
	}
}

// recordResponseHandler creates a handler that completes HAR entry with response data.
// It also logs the response status code for the request.
func recordResponseHandler(rec *recorder.Recorder) func(*http.Response, *goproxy.ProxyCtx) *http.Response {
	return func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
		if ctx.UserData == nil {
			return resp
		}

		meta, ok := ctx.UserData.(*requestMetadata)
		if !ok || meta.request == nil {
			return resp
		}

		// Record the request and response
		endTime := time.Now()
		recordResponse := rec.RecordRequest(meta.request, meta.startTime)
		recordResponse(resp, nil, endTime)

		// Log request completion with response status
		// Use stored request ID for log correlation with the request line
		reqCtx := meta.request.Context()
		duration := endTime.Sub(meta.startTime)

		logArgs := []any{
			"method", meta.request.Method,
			"host", meta.request.Host,
			"path", meta.request.URL.Path,
			"status", resp.StatusCode,
			"duration_ms", duration.Milliseconds(),
		}

		// Include stripped headers if any
		if len(meta.strippedHeaders) > 0 {
			logArgs = append(logArgs, "stripped_headers", meta.strippedHeaders)
		}

		log.Info(reqCtx, "request completed", logArgs...)

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

// initConnectHandler creates a CONNECT handler that MITMs ALL connections for init mode.
// Similar to examineConnectHandler, this always MITMs to analyze traffic for auth discovery.
func initConnectHandler(certStore *GoproxyCertStore, logger *slog.Logger) func(string, *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
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

// initRequestHandler creates a handler for init mode that analyzes requests for auth patterns.
func initRequestHandler(detector *chaperoneInit.Detector, evidence *chaperoneInit.Evidence, onFinding FindingCallback) func(*http.Request, *goproxy.ProxyCtx) (*http.Request, *http.Response) {
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

// initResponseHandler creates a no-op response handler for init mode.
func initResponseHandler() func(*http.Response, *goproxy.ProxyCtx) *http.Response {
	return func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
		return resp
	}
}
