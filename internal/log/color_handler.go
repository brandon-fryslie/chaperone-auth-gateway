package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// ANSI escape codes
const (
	reset = "\033[0m"
	bold  = "\033[1m"
	dim   = "\033[2m"

	// Standard 16 colors for non-HTTP messages
	red     = "\033[31m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	blue    = "\033[34m"
	magenta = "\033[35m"
	cyan    = "\033[36m"
	gray    = "\033[90m"
)

// 24-bit color formatting
func fg24(r, g, b int) string { return fmt.Sprintf("\033[38;2;%d;%d;%dm", r, g, b) }
func bg24(r, g, b int) string { return fmt.Sprintf("\033[48;2;%d;%d;%dm", r, g, b) }

// darken returns a darker version of the color for use as background
// Uses 1/3 intensity for better visibility while still being distinct
func darken(r, g, b int) (int, int, int) {
	return r / 3, g / 3, b / 3
}

// getColorFromAttrs extracts the pre-computed correlation color from log attributes.
// Returns foreground color, matching dark background, and ok flag.
func getColorFromAttrs(attrs map[string]any) (fg string, bg string, ok bool) {
	r := getInt(attrs, "color_r")
	g := getInt(attrs, "color_g")
	b := getInt(attrs, "color_b")
	if r == 0 && g == 0 && b == 0 {
		return "", "", false
	}
	dr, dg, db := darken(r, g, b)
	return fg24(r, g, b), bg24(dr, dg, db), true
}

// TextHandler outputs human-readable colorized logs for terminal display.
// HTTP requests and responses use 24-bit color correlation.
type TextHandler struct {
	out   io.Writer
	opts  *slog.HandlerOptions
	attrs []slog.Attr
	group string
}

// NewTextHandler creates a new colorized text handler for terminal output
func NewTextHandler(out io.Writer, opts *slog.HandlerOptions) *TextHandler {
	if out == nil {
		out = os.Stdout
	}
	return &TextHandler{out: out, opts: opts}
}

func (h *TextHandler) Enabled(_ context.Context, level slog.Level) bool {
	minLevel := slog.LevelInfo
	if h.opts != nil && h.opts.Level != nil {
		minLevel = h.opts.Level.Level()
	}
	return level >= minLevel
}

func (h *TextHandler) Handle(_ context.Context, r slog.Record) error {
	// Collect all attributes first to detect message type
	attrs := make(map[string]any)
	for _, attr := range h.attrs {
		attrs[attr.Key] = attr.Value.Any()
	}
	r.Attrs(func(attr slog.Attr) bool {
		attrs[attr.Key] = attr.Value.Any()
		return true
	})

	var b strings.Builder

	// Timestamp - dim gray, compact
	b.WriteString(dim)
	b.WriteString(gray)
	b.WriteString(r.Time.Format("15:04:05"))
	b.WriteString(reset)
	b.WriteString(" ")

	// Format based on message type
	switch r.Message {
	case "request":
		h.formatExamineRequest(&b, attrs)
	case "response":
		h.formatExamineResponse(&b, attrs)
	case "injected credential":
		h.formatInjectRequest(&b, attrs)
		// If there are stripped headers, add a warning line
		if stripped := getStringSlice(attrs, "stripped_headers"); len(stripped) > 0 {
			b.WriteString("\n")
			// Timestamp for warning line
			b.WriteString(dim)
			b.WriteString(gray)
			b.WriteString(r.Time.Format("15:04:05"))
			b.WriteString(reset)
			b.WriteString(" ")
			h.formatStrippedWarning(&b, attrs)
		}
	case "request completed":
		h.formatInjectResponse(&b, attrs)
	case "stripped auth headers from request":
		// Skip - this is now handled as part of inject request
		return nil
	case "detail":
		// Generic detail message formatted with request correlation tag
		h.formatDetail(&b, r.Message, attrs)
	default:
		h.formatGeneric(&b, r.Message, r.Level, attrs)
	}

	b.WriteString("\n")
	_, err := io.WriteString(h.out, b.String())
	return err
}

// formatExamineRequest: [req:a3f2] GET https://api.example.com/v1/chat | auth: Authorization
func (h *TextHandler) formatExamineRequest(b *strings.Builder, attrs map[string]any) {
	method := getString(attrs, "method")
	url := getString(attrs, "url")
	host := getString(attrs, "host")
	requestID := getString(attrs, "request_id")
	colorFg, colorBg, ok := getColorFromAttrs(attrs)
	if !ok {
		// This should not happen anymore with requestIDHandler in place
		b.WriteString(red)
		b.WriteString("[ERR: no color]")
		b.WriteString(reset)
		b.WriteString(" ")
	} else {
		// [req:xxxx] tag with colored background - last 4 chars of request ID for correlation
		shortID := requestID
		if len(shortID) > 4 {
			shortID = shortID[len(shortID)-4:]
		}
		b.WriteString(colorBg)
		b.WriteString(colorFg)
		b.WriteString(bold)
		b.WriteString("[req:")
		b.WriteString(shortID)
		b.WriteString("]")
		b.WriteString(reset)
		b.WriteString(" ")
	}

	// Method
	b.WriteString(method)
	b.WriteString(" ")

	// URL or host
	if url != "" {
		b.WriteString(url)
	} else if host != "" {
		b.WriteString(host)
	}

	// Auth headers if present - values are already truncated by examine logger
	if authHeaders := getStringSlice(attrs, "auth_headers"); len(authHeaders) > 0 {
		b.WriteString(dim)
		b.WriteString(" | ")
		b.WriteString(reset)
		b.WriteString(yellow)
		b.WriteString(bold)

		for i, header := range authHeaders {
			if i > 0 {
				b.WriteString(", ")
			}

			// Parse "HeaderName: truncated_value" format
			parts := strings.SplitN(header, ": ", 2)
			if len(parts) == 2 {
				headerName := parts[0]
				headerValue := parts[1] // Already truncated

				// Show header name in bold
				b.WriteString(headerName)
				b.WriteString(reset)
				b.WriteString(dim)
				b.WriteString("=")
				b.WriteString(reset)
				b.WriteString(yellow)

				// Show truncated value
				b.WriteString(headerValue)
			} else {
				// Fallback if format is unexpected
				b.WriteString(header)
			}
		}
		b.WriteString(reset)
	}

	// Query parameters if present
	if params := getStringSlice(attrs, "params"); len(params) > 0 {
		b.WriteString("\n    ")
		b.WriteString(dim)
		b.WriteString("params: ")
		b.WriteString(reset)
		b.WriteString(cyan)
		for i, param := range params {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(param)
		}
		b.WriteString(reset)
	}

	// Cookies if present
	if cookies := getStringSlice(attrs, "cookies"); len(cookies) > 0 {
		b.WriteString("\n    ")
		b.WriteString(dim)
		b.WriteString("cookies: ")
		b.WriteString(reset)
		b.WriteString(magenta)
		for i, cookie := range cookies {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(cookie)
		}
		b.WriteString(reset)
	}

	// Request body if present (already truncated by logger)
	if body := getString(attrs, "body"); body != "" {
		b.WriteString("\n    ")
		b.WriteString(dim)
		b.WriteString("body: ")
		b.WriteString(reset)
		b.WriteString(gray)
		// Body may have newlines - indent them
		indented := strings.ReplaceAll(body, "\n", "\n    ")
		b.WriteString(indented)
		b.WriteString(reset)
	}

	// Body error if present
	if bodyErr := getString(attrs, "body_error"); bodyErr != "" {
		b.WriteString("\n    ")
		b.WriteString(dim)
		b.WriteString("body_error: ")
		b.WriteString(reset)
		b.WriteString(red)
		b.WriteString(bodyErr)
		b.WriteString(reset)
	}
}

// formatExamineResponse: [res:a3f2] 200 GET /v1/chat @ api.example.com
func (h *TextHandler) formatExamineResponse(b *strings.Builder, attrs map[string]any) {
	status := getInt(attrs, "status")
	method := getString(attrs, "method")
	path := getString(attrs, "path")
	host := getString(attrs, "host")
	requestID := getString(attrs, "request_id")
	colorFg, colorBg, ok := getColorFromAttrs(attrs)
	if !ok {
		// This should not happen anymore with requestIDHandler in place
		b.WriteString(red)
		b.WriteString("[ERR: no color]")
		b.WriteString(reset)
		b.WriteString(" ")
	} else {
		// [res:xxxx] tag with colored background - last 4 chars of request ID for correlation
		shortID := requestID
		if len(shortID) > 4 {
			shortID = shortID[len(shortID)-4:]
		}
		b.WriteString(colorBg)
		b.WriteString(colorFg)
		b.WriteString(bold)
		b.WriteString("[res:")
		b.WriteString(shortID)
		b.WriteString("]")
		b.WriteString(reset)
		b.WriteString(" ")
	}

	// Status code with semantic color
	b.WriteString(h.statusColor(status))
	b.WriteString(bold)
	fmt.Fprintf(b, "%d", status)
	b.WriteString(reset)
	b.WriteString(" ")

	// Method and path
	b.WriteString(method)
	b.WriteString(" ")
	b.WriteString(path)

	// Host
	if host != "" {
		b.WriteString(dim)
		b.WriteString(" @ ")
		b.WriteString(reset)
		b.WriteString(host)
	}

	// Auth headers in response if present
	if authHeaders := getStringSlice(attrs, "auth_headers"); len(authHeaders) > 0 {
		b.WriteString("\n    ")
		b.WriteString(dim)
		b.WriteString("auth_headers: ")
		b.WriteString(reset)
		b.WriteString(yellow)
		for i, header := range authHeaders {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(header)
		}
		b.WriteString(reset)
	}

	// Set-Cookie headers if present
	if setCookies := getStringSlice(attrs, "set_cookies"); len(setCookies) > 0 {
		b.WriteString("\n    ")
		b.WriteString(dim)
		b.WriteString("set_cookies: ")
		b.WriteString(reset)
		b.WriteString(magenta)
		for i, cookie := range setCookies {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(cookie)
		}
		b.WriteString(reset)
	}

	// Response body if present (already truncated by logger)
	if body := getString(attrs, "body"); body != "" {
		b.WriteString("\n    ")
		b.WriteString(dim)
		b.WriteString("body: ")
		b.WriteString(reset)
		b.WriteString(gray)
		// Body may have newlines - indent them
		indented := strings.ReplaceAll(body, "\n", "\n    ")
		b.WriteString(indented)
		b.WriteString(reset)
	}

	// Body error if present
	if bodyErr := getString(attrs, "body_error"); bodyErr != "" {
		b.WriteString("\n    ")
		b.WriteString(dim)
		b.WriteString("body_error: ")
		b.WriteString(reset)
		b.WriteString(red)
		b.WriteString(bodyErr)
		b.WriteString(reset)
	}
}

// formatInjectRequest: [req:a3f2] /v1/chat @ api.openai.com (bearer) [stripped: Authorization]
func (h *TextHandler) formatInjectRequest(b *strings.Builder, attrs map[string]any) {
	strategy := getString(attrs, "auth_strategy")
	host := getString(attrs, "host")
	path := getString(attrs, "path")
	requestID := getString(attrs, "request_id")
	colorFg, colorBg, ok := getColorFromAttrs(attrs)
	if !ok {
		b.WriteString(red)
		b.WriteString(bold)
		b.WriteString("[ERROR: missing correlation color in inject request log - check requestIDHandler]")
		b.WriteString(reset)
		return
	}

	// [req:xxxx] tag with colored background - last 4 chars of request ID for correlation
	shortID := requestID
	if len(shortID) > 4 {
		shortID = shortID[len(shortID)-4:]
	}
	b.WriteString(colorBg)
	b.WriteString(colorFg)
	b.WriteString(bold)
	b.WriteString("[req:")
	b.WriteString(shortID)
	b.WriteString("]")
	b.WriteString(reset)
	b.WriteString(" ")

	// Path and host
	b.WriteString(path)
	b.WriteString(dim)
	b.WriteString(" @ ")
	b.WriteString(reset)
	b.WriteString(host)

	// Auth strategy
	b.WriteString(" ")
	b.WriteString(green)
	b.WriteString("(")
	b.WriteString(strategy)
	b.WriteString(")")
	b.WriteString(reset)
}

// formatStrippedWarning: [req:a3f2] WARN stripped headers: Authorization=Bear...7wAA - you may be sending credentials to a 3rd party
func (h *TextHandler) formatStrippedWarning(b *strings.Builder, attrs map[string]any) {
	stripped := getStringSlice(attrs, "stripped_headers")
	if len(stripped) == 0 {
		return
	}
	requestID := getString(attrs, "request_id")
	colorFg, colorBg, ok := getColorFromAttrs(attrs)
	if !ok {
		return
	}

	// [req:xxxx] tag with colored background
	shortID := requestID
	if len(shortID) > 4 {
		shortID = shortID[len(shortID)-4:]
	}
	b.WriteString(colorBg)
	b.WriteString(colorFg)
	b.WriteString(bold)
	b.WriteString("[req:")
	b.WriteString(shortID)
	b.WriteString("]")
	b.WriteString(reset)
	b.WriteString(" ")

	// Warning message
	b.WriteString(yellow)
	b.WriteString(bold)
	b.WriteString("stripped: ")
	b.WriteString(strings.Join(stripped, ", "))
	b.WriteString(reset)
	b.WriteString(dim)
	b.WriteString(" - you may be sending credentials to a 3rd party")
	b.WriteString(reset)
}

// formatInjectResponse: [res:a3f2] 200 in 145ms
func (h *TextHandler) formatInjectResponse(b *strings.Builder, attrs map[string]any) {
	status := getInt(attrs, "status")
	duration := getInt(attrs, "duration_ms")
	requestID := getString(attrs, "request_id")
	colorFg, colorBg, ok := getColorFromAttrs(attrs)
	if !ok {
		b.WriteString(red)
		b.WriteString(bold)
		b.WriteString("[ERROR: missing correlation color in inject response log - check recordResponseHandler]")
		b.WriteString(reset)
		return
	}

	// [res:xxxx] tag with colored background - last 4 chars of request ID for correlation
	shortID := requestID
	if len(shortID) > 4 {
		shortID = shortID[len(shortID)-4:]
	}
	b.WriteString(colorBg)
	b.WriteString(colorFg)
	b.WriteString(bold)
	b.WriteString("[res:")
	b.WriteString(shortID)
	b.WriteString("]")
	b.WriteString(reset)
	b.WriteString(" ")

	// Status code with semantic color
	b.WriteString(h.statusColor(status))
	b.WriteString(bold)
	fmt.Fprintf(b, "%d", status)
	b.WriteString(reset)

	// Duration
	if duration > 0 {
		b.WriteString(dim)
		b.WriteString(" in ")
		b.WriteString(reset)
		if duration > 1000 {
			b.WriteString(yellow)
		} else {
			b.WriteString(green)
		}
		fmt.Fprintf(b, "%dms", duration)
		b.WriteString(reset)
	}
}

// formatDetail: [req:xxxx] message with optional formatting from attrs
// Uses "detail_text" attribute for the message content to display
// Supports optional "detail_color" for text color (green, yellow, etc.)
// Supports optional "detail_bg" for background (true = show background)
func (h *TextHandler) formatDetail(b *strings.Builder, msg string, attrs map[string]any) {
	requestID := getString(attrs, "request_id")
	detailText := getString(attrs, "detail_text")
	detailColor := getString(attrs, "detail_color")
	showBg := getBool(attrs, "detail_bg")
	colorFg, colorBg, ok := getColorFromAttrs(attrs)

	if !ok {
		// Fallback if no color - just show text without tag
		b.WriteString(detailText)
		return
	}

	// [req:xxxx] tag with colored background - last 4 chars of request ID for correlation
	shortID := requestID
	if len(shortID) > 4 {
		shortID = shortID[len(shortID)-4:]
	}
	b.WriteString(colorBg)
	b.WriteString(colorFg)
	b.WriteString(bold)
	b.WriteString("[req:")
	b.WriteString(shortID)
	b.WriteString("]")
	b.WriteString(reset)
	b.WriteString(" ")

	// Apply color with optional background if specified
	switch detailColor {
	case "green":
		if showBg {
			b.WriteString(bg24(40, 100, 40)) // Dark green background
			b.WriteString(fg24(150, 255, 150))
			b.WriteString(bold)
		} else {
			b.WriteString(green)
		}
	case "orange":
		if showBg {
			b.WriteString(bg24(100, 60, 20))  // Deep dark orange background
			b.WriteString(fg24(255, 180, 80)) // Bright orange text
			b.WriteString(bold)
		} else {
			b.WriteString(fg24(255, 140, 0)) // Deep orange
		}
	case "blue":
		if showBg {
			b.WriteString(bg24(30, 50, 100))
			b.WriteString(fg24(150, 200, 255))
			b.WriteString(bold)
		} else {
			b.WriteString(blue)
		}
	case "lightblue":
		if showBg {
			b.WriteString(bg24(20, 40, 80))    // Dark blue background
			b.WriteString(fg24(150, 200, 255)) // Light blue text
			b.WriteString(bold)
		} else {
			b.WriteString(fg24(150, 200, 255)) // Light blue
		}
	}

	b.WriteString(detailText)
	if detailColor != "" {
		b.WriteString(reset)
	}
}

// formatGeneric: message | key=value key=value
func (h *TextHandler) formatGeneric(b *strings.Builder, msg string, level slog.Level, attrs map[string]any) {
	// Level badge
	switch level {
	case slog.LevelDebug:
		b.WriteString(gray)
		b.WriteString("DBG")
		b.WriteString(reset)
	case slog.LevelInfo:
		b.WriteString(green)
		b.WriteString("INF")
		b.WriteString(reset)
	case slog.LevelWarn:
		b.WriteString(yellow)
		b.WriteString(bold)
		b.WriteString("WRN")
		b.WriteString(reset)
	case slog.LevelError:
		b.WriteString(red)
		b.WriteString(bold)
		b.WriteString("ERR")
		b.WriteString(reset)
	}
	b.WriteString(" ")

	// Message
	b.WriteString(msg)

	// Skip if no attrs
	if len(attrs) == 0 {
		return
	}

	// Add separator
	b.WriteString(dim)
	b.WriteString(" | ")
	b.WriteString(reset)

	// Key=value pairs
	first := true
	for k, v := range attrs {
		if !first {
			b.WriteString(" ")
		}
		first = false

		b.WriteString(dim)
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(reset)

		// Format value based on key
		switch k {
		case "error":
			b.WriteString(red)
			fmt.Fprintf(b, "%v", v)
			b.WriteString(reset)
		case "host", "path", "url", "address":
			b.WriteString(blue)
			fmt.Fprintf(b, "%v", v)
			b.WriteString(reset)
		case "status", "port":
			b.WriteString(cyan)
			fmt.Fprintf(b, "%v", v)
			b.WriteString(reset)
		default:
			fmt.Fprintf(b, "%v", v)
		}
	}
}

func (h *TextHandler) statusColor(status int) string {
	switch {
	case status >= 500:
		return red
	case status >= 400:
		return yellow
	case status >= 300:
		return cyan
	case status >= 200:
		return green
	default:
		return ""
	}
}

func (h *TextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &TextHandler{
		out:   h.out,
		opts:  h.opts,
		attrs: append(h.attrs, attrs...),
		group: h.group,
	}
}

func (h *TextHandler) WithGroup(name string) slog.Handler {
	newGroup := name
	if h.group != "" {
		newGroup = h.group + "." + name
	}
	return &TextHandler{
		out:   h.out,
		opts:  h.opts,
		attrs: h.attrs,
		group: newGroup,
	}
}

// Helper functions to extract typed values from attrs map
func getString(attrs map[string]any, key string) string {
	if v, ok := attrs[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func getInt(attrs map[string]any, key string) int {
	if v, ok := attrs[key]; ok {
		switch n := v.(type) {
		case int:
			return n
		case int64:
			return int(n)
		case float64:
			return int(n)
		}
	}
	return 0
}

func getStringSlice(attrs map[string]any, key string) []string {
	if v, ok := attrs[key]; ok {
		if s, ok := v.([]string); ok {
			return s
		}
	}
	return nil
}

func getBool(attrs map[string]any, key string) bool {
	if v, ok := attrs[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// LogfmtHandler is a slog.Handler that outputs logs in logfmt format (key=value pairs)
type LogfmtHandler struct {
	out   io.Writer
	opts  *slog.HandlerOptions
	attrs []slog.Attr
	group string
}

// NewLogfmtHandler creates a new logfmt slog handler
func NewLogfmtHandler(out io.Writer, opts *slog.HandlerOptions) *LogfmtHandler {
	if out == nil {
		out = os.Stdout
	}
	return &LogfmtHandler{out: out, opts: opts}
}

func (h *LogfmtHandler) Enabled(_ context.Context, level slog.Level) bool {
	minLevel := slog.LevelInfo
	if h.opts != nil && h.opts.Level != nil {
		minLevel = h.opts.Level.Level()
	}
	return level >= minLevel
}

func (h *LogfmtHandler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder

	b.WriteString("level=")
	b.WriteString(r.Level.String())

	b.WriteString(" time=")
	b.WriteString(r.Time.Format("2006-01-02T15:04:05.000Z07:00"))

	b.WriteString(" msg=")
	b.WriteString(quoteLogfmt(r.Message))

	for _, attr := range h.attrs {
		h.writeAttr(&b, attr)
	}

	r.Attrs(func(attr slog.Attr) bool {
		h.writeAttr(&b, attr)
		return true
	})

	b.WriteString("\n")
	_, err := io.WriteString(h.out, b.String())
	return err
}

func (h *LogfmtHandler) writeAttr(b *strings.Builder, attr slog.Attr) {
	if attr.Key == "" {
		return
	}
	b.WriteString(" ")
	if h.group != "" {
		b.WriteString(h.group)
		b.WriteString(".")
	}
	b.WriteString(attr.Key)
	b.WriteString("=")
	b.WriteString(quoteLogfmt(attr.Value.String()))
}

func (h *LogfmtHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &LogfmtHandler{
		out:   h.out,
		opts:  h.opts,
		attrs: append(h.attrs, attrs...),
		group: h.group,
	}
}

func (h *LogfmtHandler) WithGroup(name string) slog.Handler {
	newGroup := name
	if h.group != "" {
		newGroup = h.group + "." + name
	}
	return &LogfmtHandler{
		out:   h.out,
		opts:  h.opts,
		attrs: h.attrs,
		group: newGroup,
	}
}

func quoteLogfmt(s string) string {
	if strings.ContainsAny(s, " \t\n\"=") {
		return fmt.Sprintf("%q", s)
	}
	return s
}
