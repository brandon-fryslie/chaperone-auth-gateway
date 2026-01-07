package examine

import (
	"fmt"
	"io"
	"net/http"
	"strings"
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

// Logger outputs examine-mode request/response information in a human-readable format.
type Logger struct {
	output io.Writer
	config Config
}

// NewLogger creates a new examine-mode logger that writes to the specified writer.
func NewLogger(w io.Writer, config Config) *Logger {
	// Set sensible defaults
	if config.MaxBodyBytes == 0 {
		config.MaxBodyBytes = 4096 // 4KB default
	}
	return &Logger{
		output: w,
		config: config,
	}
}

// LogRequest logs a request in examine mode, focusing on information that could contain authentication.
// Respects the logger's configuration to control what is displayed.
func (l *Logger) LogRequest(r *http.Request) {
	fmt.Fprintf(l.output, "\n%s\n", strings.Repeat("=", 80))
	fmt.Fprintf(l.output, "REQUEST: %s %s\n", r.Method, r.URL.String())
	fmt.Fprintf(l.output, "%s\n", strings.Repeat("-", 80))

	// Headers that may contain auth
	fmt.Fprintln(l.output, "Headers (potentially containing auth):")
	hasAuthHeaders := false
	for name, values := range r.Header {
		if IsAuthRelevant(name) {
			hasAuthHeaders = true
			for _, v := range values {
				fmt.Fprintf(l.output, "  %s: %s\n", name, v)
			}
		}
	}
	if !hasAuthHeaders {
		fmt.Fprintln(l.output, "  (none)")
	}

	// Query parameters (optional)
	if l.config.ShowParams {
		fmt.Fprintln(l.output, "\nQuery Parameters:")
		if len(r.URL.Query()) > 0 {
			for key, values := range r.URL.Query() {
				for _, v := range values {
					fmt.Fprintf(l.output, "  %s: %s\n", key, v)
				}
			}
		} else {
			fmt.Fprintln(l.output, "  (none)")
		}
	}

	// Cookies (optional)
	if l.config.ShowCookies {
		fmt.Fprintln(l.output, "\nCookies:")
		cookies := r.Cookies()
		if len(cookies) > 0 {
			for _, c := range cookies {
				fmt.Fprintf(l.output, "  %s: %s\n", c.Name, c.Value)
			}
		} else {
			fmt.Fprintln(l.output, "  (none)")
		}
	}

	// Request body (optional)
	if l.config.ShowBody && r.Body != nil {
		fmt.Fprintln(l.output, "\nRequest Body:")
		body, err := io.ReadAll(io.LimitReader(r.Body, int64(l.config.MaxBodyBytes)))
		if err != nil {
			fmt.Fprintf(l.output, "  (error reading body: %v)\n", err)
		} else if len(body) == 0 {
			fmt.Fprintln(l.output, "  (empty)")
		} else {
			truncated := len(body) == l.config.MaxBodyBytes
			fmt.Fprintf(l.output, "  %s", string(body))
			if truncated {
				fmt.Fprintln(l.output, "\n  (truncated at max body bytes)")
			} else {
				fmt.Fprintln(l.output)
			}
		}
		// Note: Body is consumed, but in examine mode we're just logging, not forwarding
	}

	fmt.Fprintf(l.output, "%s\n", strings.Repeat("=", 80))
}

// LogResponse logs a response in examine mode, focusing on information that could contain authentication.
// Respects the logger's configuration to control what is displayed.
func (l *Logger) LogResponse(resp *http.Response) {
	if !l.config.ShowResponse {
		return
	}

	fmt.Fprintf(l.output, "\n%s\n", strings.Repeat("=", 80))
	fmt.Fprintf(l.output, "RESPONSE: %d %s\n", resp.StatusCode, resp.Status)
	fmt.Fprintf(l.output, "%s\n", strings.Repeat("-", 80))

	// Headers that may contain auth
	fmt.Fprintln(l.output, "Headers (potentially containing auth):")
	hasAuthHeaders := false
	for name, values := range resp.Header {
		if IsAuthRelevant(name) {
			hasAuthHeaders = true
			for _, v := range values {
				fmt.Fprintf(l.output, "  %s: %s\n", name, v)
			}
		}
	}
	if !hasAuthHeaders {
		fmt.Fprintln(l.output, "  (none)")
	}

	// Cookies from Set-Cookie headers
	if l.config.ShowCookies {
		fmt.Fprintln(l.output, "\nSet-Cookie Headers:")
		cookies := resp.Cookies()
		if len(cookies) > 0 {
			for _, c := range cookies {
				fmt.Fprintf(l.output, "  %s: %s", c.Name, c.Value)
				if c.MaxAge != 0 {
					fmt.Fprintf(l.output, " (MaxAge: %d)", c.MaxAge)
				}
				if c.Secure {
					fmt.Fprint(l.output, " [Secure]")
				}
				if c.HttpOnly {
					fmt.Fprint(l.output, " [HttpOnly]")
				}
				fmt.Fprintln(l.output)
			}
		} else {
			fmt.Fprintln(l.output, "  (none)")
		}
	}

	// Response body (optional)
	if l.config.ShowBody && resp.Body != nil {
		fmt.Fprintln(l.output, "\nResponse Body:")
		body, err := io.ReadAll(io.LimitReader(resp.Body, int64(l.config.MaxBodyBytes)))
		if err != nil {
			fmt.Fprintf(l.output, "  (error reading body: %v)\n", err)
		} else if len(body) == 0 {
			fmt.Fprintln(l.output, "  (empty)")
		} else {
			truncated := len(body) == l.config.MaxBodyBytes
			fmt.Fprintf(l.output, "  %s", string(body))
			if truncated {
				fmt.Fprintln(l.output, "\n  (truncated at max body bytes)")
			} else {
				fmt.Fprintln(l.output)
			}
		}
		// Note: Body is consumed, but in examine mode we're just logging, not forwarding
	}

	fmt.Fprintf(l.output, "%s\n", strings.Repeat("=", 80))
}
