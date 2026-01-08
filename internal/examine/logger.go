package examine

import (
	"context"
	"io"
	"net/http"
	"strings"

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
}

// Logger outputs examine-mode request/response information using structured logging.
type Logger struct {
	config Config
}

// NewLogger creates a new examine-mode logger.
// The io.Writer parameter is kept for backward compatibility but is no longer used.
func NewLogger(_ io.Writer, config Config) *Logger {
	// Set sensible defaults
	if config.MaxBodyBytes == 0 {
		config.MaxBodyBytes = 4096 // 4KB default
	}
	return &Logger{
		config: config,
	}
}

// LogRequest logs a request in examine mode using structured logging.
func (l *Logger) LogRequest(r *http.Request) {
	ctx := r.Context()

	// Build log arguments
	args := []any{
		"method", r.Method,
		"url", r.URL.String(),
		"host", r.Host,
	}

	// Collect auth-relevant headers
	var authHeaders []string
	for name, values := range r.Header {
		if IsAuthRelevant(name) {
			for _, v := range values {
				authHeaders = append(authHeaders, name+": "+v)
			}
		}
	}
	if len(authHeaders) > 0 {
		args = append(args, "auth_headers", authHeaders)
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
		body, err := io.ReadAll(io.LimitReader(r.Body, int64(l.config.MaxBodyBytes)))
		if err != nil {
			args = append(args, "body_error", err.Error())
		} else if len(body) > 0 {
			truncated := len(body) == l.config.MaxBodyBytes
			bodyStr := string(body)
			if truncated {
				bodyStr += "... (truncated)"
			}
			args = append(args, "body", bodyStr)
		}
	}

	log.Info(ctx, "request", args...)
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

	// Collect auth-relevant headers
	var authHeaders []string
	for name, values := range resp.Header {
		if IsAuthRelevant(name) {
			for _, v := range values {
				authHeaders = append(authHeaders, name+": "+v)
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
		body, err := io.ReadAll(io.LimitReader(resp.Body, int64(l.config.MaxBodyBytes)))
		if err != nil {
			args = append(args, "body_error", err.Error())
		} else if len(body) > 0 {
			truncated := len(body) == l.config.MaxBodyBytes
			bodyStr := string(body)
			if truncated {
				bodyStr += "... (truncated)"
			}
			args = append(args, "body", bodyStr)
		}
	}

	log.Info(ctx, "response", args...)
}

// FormatHeaders formats headers for display, filtering to auth-relevant ones.
func FormatHeaders(headers http.Header) string {
	var parts []string
	for name, values := range headers {
		if IsAuthRelevant(name) {
			for _, v := range values {
				parts = append(parts, name+": "+v)
			}
		}
	}
	if len(parts) == 0 {
		return "(none)"
	}
	return strings.Join(parts, ", ")
}
