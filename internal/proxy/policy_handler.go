// Package proxy provides HTTP/HTTPS proxy handlers for MITM and credential injection.
// This file contains policy enforcement handlers (allowed methods, paths, body size, drop patterns).
package proxy

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/bmf/chaperone/internal/audit"
	"github.com/bmf/chaperone/internal/log"
	"github.com/bmf/chaperone/internal/service"
	"github.com/elazarl/goproxy"
)

// policyHandler creates a handler that enforces service policies.
func policyHandler(registry service.ServiceRegistry, enforcer *service.Enforcer, auditLogger audit.AuditLogger, logger *slog.Logger) func(*http.Request, *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	return func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		reqCtx := r.Context()

		svc, err := registry.Lookup(r.Host)
		if err != nil || svc.Policy == nil {
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
			logAudit(reqCtx, auditLogger, audit.Entry{
				Event:      audit.EventPolicyDenied,
				Service:    svc.Name,
				Host:       r.Host,
				Path:       r.URL.Path,
				Method:     r.Method,
				RequestID:  log.RequestID(reqCtx),
				ClientIP:   extractClientIP(r),
				Outcome:    "blocked",
				StatusCode: statusCode,
				Detail:     fmt.Sprintf("method %s not allowed", r.Method),
			})

			return r, goproxy.NewResponse(r, goproxy.ContentTypeText, statusCode, err.Error())
		}

		// Check path
		if err := enforcer.CheckPath(r.URL.Path, svc.Policy); err != nil {
			logger.Warn("policy violation - path not allowed", "error", err, "hostname", r.Host, "path", r.URL.Path)

			// AUDIT: Policy denied event
			logAudit(reqCtx, auditLogger, audit.Entry{
				Event:      audit.EventPolicyDenied,
				Service:    svc.Name,
				Host:       r.Host,
				Path:       r.URL.Path,
				Method:     r.Method,
				RequestID:  log.RequestID(reqCtx),
				ClientIP:   extractClientIP(r),
				Outcome:    "blocked",
				StatusCode: http.StatusForbidden,
				Detail:     fmt.Sprintf("path %s not allowed", r.URL.Path),
			})

			return r, goproxy.NewResponse(r, goproxy.ContentTypeText, http.StatusForbidden, err.Error())
		}

		// Check body size
		if err := enforcer.CheckBodySize(r.ContentLength, svc.Policy); err != nil {
			logger.Warn("policy violation - body too large", "error", err, "hostname", r.Host)

			// AUDIT: Policy denied event
			logAudit(reqCtx, auditLogger, audit.Entry{
				Event:      audit.EventPolicyDenied,
				Service:    svc.Name,
				Host:       r.Host,
				Path:       r.URL.Path,
				Method:     r.Method,
				RequestID:  log.RequestID(reqCtx),
				ClientIP:   extractClientIP(r),
				Outcome:    "blocked",
				StatusCode: http.StatusRequestEntityTooLarge,
				Detail:     fmt.Sprintf("body size %d exceeds limit", r.ContentLength),
			})

			return r, goproxy.NewResponse(r, goproxy.ContentTypeText, http.StatusRequestEntityTooLarge, err.Error())
		}

		log.Debug(reqCtx, "policy check passed", "hostname", r.Host)
		return r, nil
	}
}

// dropHandler creates a handler that blocks requests matching drop patterns.
func dropHandler(registry service.ServiceRegistry, auditLogger audit.AuditLogger, logger *slog.Logger) func(*http.Request, *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	return func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		reqCtx := r.Context()

		svc, err := registry.Lookup(r.Host)
		if err != nil || svc.Policy == nil || len(svc.Policy.Drop) == 0 {
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
				logAudit(reqCtx, auditLogger, audit.Entry{
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

				return r, goproxy.NewResponse(r, goproxy.ContentTypeText,
					http.StatusForbidden, "Request blocked by drop policy")
			}
		}

		return r, nil
	}
}

// stripHandler creates a handler that removes specified headers from requests.
func stripHandler(registry service.ServiceRegistry, logger *slog.Logger) func(*http.Request, *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	return func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		reqCtx := r.Context()

		svc, err := registry.Lookup(r.Host)
		if err != nil || svc.Policy == nil || len(svc.Policy.Strip) == 0 {
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
