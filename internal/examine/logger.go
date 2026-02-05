// Package examine provides functionality for discovering authentication patterns in HTTP traffic.
// This file contains core logging for requests and responses in examine mode.
package examine

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/bmf/chaperone/internal/defaults"
	"github.com/bmf/chaperone/internal/log"
)

// Config holds configuration options for examine mode logging.
type Config struct {
	// ShowBody controls whether request/response bodies are logged
	ShowBody bool
	// ShowParams controls whether query parameters are logged
	ShowParams bool
	// ShowCookies controls whether cookies are logged
	ShowCookies bool
	// ShowResponse controls whether responses are logged
	ShowResponse bool
	// MaxBodyBytes limits the number of body bytes to display (0 = unlimited)
	MaxBodyBytes int
	// AllHeaders disables header filtering heuristics
	AllHeaders bool
	// SentinelValue is the value to look for in headers to identify auth headers
	SentinelValue string
}

// Logger outputs examine-mode request/response information using structured logging.
type Logger struct {
	config      Config
	discovery   *DiscoveryTracker
	commandName string
}

// NewLogger creates a new examine-mode logger.
func NewLogger(config Config) *Logger {
	// Set sensible defaults
	if config.MaxBodyBytes == 0 {
		config.MaxBodyBytes = defaults.DefaultExamineBodyBytes
	}
	if config.SentinelValue == "" {
		config.SentinelValue = "chaperone-sentinel"
	}
	return &Logger{
		config:    config,
		discovery: NewDiscoveryTracker(),
	}
}

// SetCommandName sets the command name for config generation
func (l *Logger) SetCommandName(cmd string) {
	l.commandName = cmd
}

// GetCommandName returns the command name for config generation
func (l *Logger) GetCommandName() string {
	return l.commandName
}

// GetDiscoveryTracker returns the discovery tracker for this logger
func (l *Logger) GetDiscoveryTracker() *DiscoveryTracker {
	return l.discovery
}

// shouldLogHeader determines if a header should be logged based on configuration
func (l *Logger) shouldLogHeader(name, value string) bool {
	if l.config.AllHeaders {
		return true
	}
	return IsAuthRelevant(name, value)
}

// LogRequest logs a request in examine mode using structured logging.
// Returns true if the sentinel value was found in any header.
func (l *Logger) LogRequest(r *http.Request) bool {
	ctx := r.Context()
	foundSentinel := false

	// Build log arguments for request
	args := []any{
		"method", r.Method,
		"url", r.URL.String(),
		"host", r.Host,
	}

	// Query parameters (optional)
	if l.config.ShowParams && len(r.URL.Query()) > 0 {
		var params []string
		for key, values := range r.URL.Query() {
			for _, v := range values {
				params = append(params, key+"="+v)
			}
		}
		args = append(args, "params", params)
	}

	// Cookies (optional)
	if l.config.ShowCookies {
		cookies := r.Cookies()
		if len(cookies) > 0 {
			var cookieStrs []string
			for _, c := range cookies {
				cookieStrs = append(cookieStrs, c.Name+"="+c.Value)
			}
			args = append(args, "cookies", cookieStrs)
		}
	}

	// Request body (optional)
	if l.config.ShowBody && r.Body != nil {
		// Use TeeReader to copy ENTIRE body without consuming original stream
		var buf bytes.Buffer
		teeReader := io.TeeReader(r.Body, &buf)

		// Read ENTIRE body (not limited) to ensure proper restoration
		body, err := io.ReadAll(teeReader)
		r.Body.Close()

		if err != nil {
			args = append(args, "body_error", err.Error())
		} else if len(body) > 0 {
			// Truncate only what we LOG, not what we buffer
			bodyStr := string(body)
			if len(body) > l.config.MaxBodyBytes {
				bodyStr = string(body[:l.config.MaxBodyBytes]) + "... (truncated)"
			}
			args = append(args, "body", bodyStr)
		}

		// Replace body with FULL buffer copy for downstream handlers
		r.Body = io.NopCloser(&buf)
	}

	// Log the main request
	log.Info(ctx, "request", args...)

	// Log auth-relevant headers on separate lines
	for name, values := range r.Header {
		for _, v := range values {
			if l.shouldLogHeader(name, v) {
				if l.logAuthHeaderLine(ctx, name, v, r.URL.String()) {
					foundSentinel = true
				}
			}
		}
	}

	return foundSentinel
}

// logAuthHeaderLine logs a single auth header on its own line with proper formatting.
// Returns true if sentinel was found in this header.
func (l *Logger) logAuthHeaderLine(ctx context.Context, name, value, url string) bool {
	truncated := truncateMiddle(value, 8, 4)
	isStandardAuth := isStandardAuthHeader(name)
	foundSentinel := strings.Contains(value, l.config.SentinelValue)

	// Track the discovery
	l.discovery.TrackHeader(name, url, truncated, foundSentinel, isStandardAuth)

	// Only log if we haven't seen this header before
	if l.discovery.HasSeenHeader(name) {
		return foundSentinel
	}
	l.discovery.MarkHeaderSeen(name)

	// Format the detail message
	var detailText string
	var color string
	var showBg bool
	if foundSentinel {
		// For sentinel, don't show the value - just the header name
		detailText = "✓ SENTINEL FOUND in header: " + name
		color = "lightblue"
		showBg = true
	} else if isStandardAuth {
		detailText = "Found auth header: " + name + ": " + truncated
		color = "green"
		showBg = true
	} else {
		detailText = "Interesting header: " + name + "=" + truncated
		color = "orange"
		showBg = true
	}

	// Log as detail message with proper formatting
	log.Detail(ctx, "detail_text", detailText, "detail_color", color, "detail_bg", showBg)

	return foundSentinel
}

// LogResponse logs a response in examine mode using structured logging.
func (l *Logger) LogResponse(resp *http.Response) {
	if !l.config.ShowResponse || resp == nil {
		return
	}

	ctx := context.Background()
	if resp.Request != nil {
		ctx = resp.Request.Context()
	}

	args := []any{
		"status", resp.StatusCode,
	}

	// Add request info if available
	if resp.Request != nil {
		args = append(args,
			"method", resp.Request.Method,
			"host", resp.Request.Host,
			"path", resp.Request.URL.Path,
		)
	}

	// Collect auth-relevant headers with truncated values
	var authHeaders []string
	for name, values := range resp.Header {
		for _, v := range values {
			if l.shouldLogHeader(name, v) {
				// Truncate middle of value: show first 8 and last 4 chars
				truncated := truncateMiddle(v, 8, 4)
				authHeaders = append(authHeaders, name+": "+truncated)
			}
		}
	}
	if len(authHeaders) > 0 {
		args = append(args, "auth_headers", authHeaders)
	}

	// Cookies from Set-Cookie headers
	if l.config.ShowCookies {
		cookies := resp.Cookies()
		if len(cookies) > 0 {
			var cookieStrs []string
			for _, c := range cookies {
				info := c.Name + "=" + c.Value
				if c.Secure {
					info += " [Secure]"
				}
				if c.HttpOnly {
					info += " [HttpOnly]"
				}
				cookieStrs = append(cookieStrs, info)
			}
			args = append(args, "set_cookies", cookieStrs)
		}
	}

	// Response body (optional)
	if l.config.ShowBody && resp.Body != nil {
		// Use TeeReader to copy ENTIRE body without consuming original stream
		var buf bytes.Buffer
		teeReader := io.TeeReader(resp.Body, &buf)

		// Read ENTIRE body (not limited) to ensure proper restoration
		body, err := io.ReadAll(teeReader)
		resp.Body.Close()

		if err != nil {
			args = append(args, "body_error", err.Error())
		} else if len(body) > 0 {
			// Truncate only what we LOG, not what we buffer
			bodyStr := string(body)
			if len(body) > l.config.MaxBodyBytes {
				bodyStr = string(body[:l.config.MaxBodyBytes]) + "... (truncated)"
			}
			args = append(args, "body", bodyStr)
		}

		// Replace body with FULL buffer copy for downstream handlers
		resp.Body = io.NopCloser(&buf)
	}

	log.Info(ctx, "response", args...)
}

// FormatHeaders formats headers for display, filtering to auth-relevant ones.
// Note: This function always applies filtering (does not support AllHeaders mode).
// Use Logger.LogRequest/LogResponse for full configuration support.
func FormatHeaders(headers http.Header) string {
	var parts []string
	for name, values := range headers {
		for _, v := range values {
			if IsAuthRelevant(name, v) {
				parts = append(parts, name+": "+v)
			}
		}
	}
	if len(parts) == 0 {
		return "(none)"
	}
	return strings.Join(parts, ", ")
}

// truncateMiddle returns a string showing the start and end with "..." in middle
// e.g., "Bearer sk-1234567890abcdef" -> "Bearer s...cdef"
func truncateMiddle(s string, startChars, endChars int) string {
	if len(s) <= startChars+endChars+3 {
		return s // Too short to truncate
	}
	return s[:startChars] + "..." + s[len(s)-endChars:]
}

// isStandardAuthHeader checks if a header name is in the always-include list
// or matches common auth header patterns
func isStandardAuthHeader(name string) bool {
	nameLower := strings.ToLower(name)
	standardHeaders := []string{
		"authorization",
		"x-api-key",
		"x-auth-token",
		"x-access-token",
		"x-secret-token",
		"api-key",
		"api-token",
		"access-token",
		"x-token",
		"x-key",
	}
	for _, h := range standardHeaders {
		if nameLower == h {
			return true
		}
	}

	// Heuristic: if header name ends with "apikey" or "api-key" (case-insensitive), assume it's auth
	if strings.HasSuffix(nameLower, "apikey") || strings.HasSuffix(nameLower, "api-key") {
		return true
	}

	return false
}
