// Package proxy provides HTTP/HTTPS proxy handlers for MITM and credential injection.
// This file contains utility functions shared across handlers.
package proxy

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/bmf/chaperone/internal/audit"
	"github.com/bmf/chaperone/internal/log"
	"github.com/elazarl/goproxy"
)

// logAudit writes an audit entry and surfaces a write failure to the
// operational log. An audit trail that fails to record without anyone
// noticing is a security gap, not a minor IO hiccup — the failure must be
// loud. [LAW:no-silent-failure] [LAW:single-enforcer]
func logAudit(ctx context.Context, auditLogger audit.AuditLogger, entry audit.Entry) {
	if err := auditLogger.Log(entry); err != nil {
		log.Error(ctx, "audit log write failed", err, "event", entry.Event)
	}
}

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
