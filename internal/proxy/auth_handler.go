// Package proxy provides HTTP/HTTPS proxy handlers for MITM and credential injection.
// This file contains authentication handlers (credential stripping and injection).
package proxy

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bmf/chaperone/internal/audit"
	"github.com/bmf/chaperone/internal/auth"
	"github.com/bmf/chaperone/internal/log"
	"github.com/bmf/chaperone/internal/secrets"
	"github.com/bmf/chaperone/internal/service"
	"github.com/elazarl/goproxy"
)

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
func securityStripAuthHandler(registry service.ServiceRegistry, auditLogger audit.AuditLogger, logger *slog.Logger) func(*http.Request, *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	return func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		reqCtx := r.Context()

		// Only strip for configured services (where we're doing MITM)
		svc, err := registry.Lookup(r.Host)
		if err != nil {
			return r, nil
		}

		// Check each known auth header
		var strippedHeaders []string
		for _, knownHeader := range auth.KnownAuthHeaders {
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
			logAudit(reqCtx, auditLogger, audit.Entry{
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

		return r, nil
	}
}

// authHandler creates a handler that injects authentication credentials.
func authHandler(registry service.ServiceRegistry, secretRegistry *secrets.Registry, authRegistry *auth.Registry, auditLogger audit.AuditLogger, logger *slog.Logger) func(*http.Request, *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	// Track warnings for services without placeholders (warn once per service)
	warnedServices := make(map[string]bool)
	var warnMutex sync.Mutex

	return func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		reqCtx := r.Context()

		svc, err := registry.Lookup(r.Host)
		if err != nil {
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
				logAudit(reqCtx, auditLogger, audit.Entry{
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
				"service", svc.Name,
				"ref", svc.CredentialRef)

			// AUDIT: Auth failure event
			logAudit(reqCtx, auditLogger, audit.Entry{
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

			return r, goproxy.NewResponse(r, goproxy.ContentTypeText,
				http.StatusServiceUnavailable, "Secret unavailable")
		}

		// Get authentication strategy
		strategy, err := authRegistry.Get(svc.AuthStrategyRef)
		if err != nil {
			log.Error(reqCtx, "unknown authentication strategy", err,
				"service", svc.Name,
				"strategy", svc.AuthStrategyRef)

			// AUDIT: Auth failure event
			logAudit(reqCtx, auditLogger, audit.Entry{
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

			return r, goproxy.NewResponse(r, goproxy.ContentTypeText,
				http.StatusBadGateway, "Authentication configuration error")
		}

		// Apply authentication to request
		if err := strategy.Apply(reqCtx, r, secret); err != nil {
			log.Error(reqCtx, "failed to apply authentication", err,
				"service", svc.Name,
				"strategy", svc.AuthStrategyRef)

			// AUDIT: Auth failure event
			logAudit(reqCtx, auditLogger, audit.Entry{
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

			return r, goproxy.NewResponse(r, goproxy.ContentTypeText,
				http.StatusBadGateway, "Authentication failed")
		}

		// AUDIT LOGGING - after successful injection
		logAudit(reqCtx, auditLogger, audit.Entry{
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
