package examine

import (
	"context"
	"io"
	"net/http"
	"runtime"
	"strings"
	"sync"

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

// AuthHeaderDiscovery tracks a discovered auth header
type AuthHeaderDiscovery struct {
	HeaderName        string
	URL               string
	TruncatedValue    string
	FoundSentinel     bool
	IsStandardAuthKey bool
}

// DiscoveryTracker collects auth header discoveries across all requests
type DiscoveryTracker struct {
	mu          sync.Mutex
	discoveries map[string]*AuthHeaderDiscovery // key: headerName
}

// NewDiscoveryTracker creates a new discovery tracker
func NewDiscoveryTracker() *DiscoveryTracker {
	return &DiscoveryTracker{
		discoveries: make(map[string]*AuthHeaderDiscovery),
	}
}

// TrackHeader records a discovered auth header
func (dt *DiscoveryTracker) TrackHeader(headerName, url, truncatedValue string, foundSentinel, isStandardAuth bool) {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	key := strings.ToLower(headerName)
	if _, exists := dt.discoveries[key]; !exists {
		dt.discoveries[key] = &AuthHeaderDiscovery{
			HeaderName:        headerName,
			URL:               url,
			TruncatedValue:    truncatedValue,
			FoundSentinel:     foundSentinel,
			IsStandardAuthKey: isStandardAuth,
		}
	}
}

// GetDiscoveries returns all discovered headers (sorted for consistency)
func (dt *DiscoveryTracker) GetDiscoveries() []*AuthHeaderDiscovery {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	result := make([]*AuthHeaderDiscovery, 0, len(dt.discoveries))
	for _, disc := range dt.discoveries {
		result = append(result, disc)
	}
	return result
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
		detailText = "Possible auth in header: " + name + "=" + truncated
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

// PrintSummaryReport prints a summary of all discovered auth headers at exit
func (l *Logger) PrintSummaryReport(ctx context.Context) {
	discoveries := l.discovery.GetDiscoveries()
	if len(discoveries) == 0 {
		log.Info(ctx, "No auth headers discovered during examination")
		return
	}

	// Log summary as a detail message
	log.Detail(ctx, "detail_text", "=== Auth Discovery Summary ===", "detail_color", "blue")

	// Separate discoveries by type
	sentinelHeaders := make([]*AuthHeaderDiscovery, 0)
	standardHeaders := make([]*AuthHeaderDiscovery, 0)
	possibleHeaders := make([]*AuthHeaderDiscovery, 0)

	for _, disc := range discoveries {
		if disc.FoundSentinel {
			sentinelHeaders = append(sentinelHeaders, disc)
		} else if disc.IsStandardAuthKey {
			standardHeaders = append(standardHeaders, disc)
		} else {
			possibleHeaders = append(possibleHeaders, disc)
		}
	}

	// Log sentinel matches
	if len(sentinelHeaders) > 0 {
		log.Detail(ctx, "detail_text", "✓ Sentinel value found in:", "detail_color", "green")
		for _, disc := range sentinelHeaders {
			log.Detail(ctx, "detail_text", "  - "+disc.HeaderName+" sent to "+disc.URL)
		}
	}

	// Log standard auth headers
	if len(standardHeaders) > 0 {
		log.Detail(ctx, "detail_text", "✓ Standard auth headers found:", "detail_color", "green")
		for _, disc := range standardHeaders {
			log.Detail(ctx, "detail_text", "  - "+disc.HeaderName+" sent to "+disc.URL)
		}
	}

	// Log possible headers
	if len(possibleHeaders) > 0 {
		log.Detail(ctx, "detail_text", "? Possible auth headers:", "detail_color", "yellow")
		for _, disc := range possibleHeaders {
			log.Detail(ctx, "detail_text", "  - "+disc.HeaderName+" sent to "+disc.URL)
		}
	}

	// Log example config
	l.printExampleConfig(ctx, sentinelHeaders, standardHeaders, possibleHeaders)
}

// printExampleConfig generates an example config block based on discoveries
func (l *Logger) printExampleConfig(ctx context.Context, sentinels, standards, possible []*AuthHeaderDiscovery) {
	log.Detail(ctx, "detail_text", "=== Example Config Block ===", "detail_color", "blue")

	var exampleHeader *AuthHeaderDiscovery

	if len(sentinels) > 0 {
		exampleHeader = sentinels[0]
	} else if len(standards) > 0 {
		exampleHeader = standards[0]
	} else if len(possible) > 0 {
		exampleHeader = possible[0]
	} else {
		log.Detail(ctx, "detail_text", "# No auth headers discovered. Check --all-headers flag or examine your requests.")
		return
	}

	// Extract hostname from URL
	hostPattern := ExtractHostFromURL(exampleHeader.URL)

	// Determine service name
	serviceName := "myservice"
	if l.commandName != "" {
		serviceName = l.commandName
	}

	// Generate example config
	strategy := GuessAuthStrategy(exampleHeader.HeaderName)

	log.Detail(ctx, "detail_text", "")
	log.Detail(ctx, "detail_text", "[services."+serviceName+"]")
	log.Detail(ctx, "detail_text", "host_pattern = \""+hostPattern+"\"")
	log.Detail(ctx, "detail_text", "auth_strategy = \""+strategy+"\"")

	if IsOSMacOS() {
		// macOS: Show keychain command and credential_ref
		credentialName := "chaperone/" + serviceName
		keychainCmd := "security add-generic-password -s \"" + credentialName + "\" -a \"\" -w \"<YOUR_API_KEY>\""
		log.Detail(ctx, "detail_text", "")
		log.Detail(ctx, "detail_text", "# to add to MacOS keychain, run: "+keychainCmd+" # MacOS Only")
		log.Detail(ctx, "detail_text", "# credential_ref = \"keychain:"+credentialName+"\" # MacOS Only")
	}

	// Always show env and file options
	log.Detail(ctx, "detail_text", "# credential_ref = \"env:YOUR_API_KEY\"")
	log.Detail(ctx, "detail_text", "# credential_ref = \"file:/path/to/secret\"")
}

// ExtractHostFromURL extracts the hostname from a URL
func ExtractHostFromURL(urlStr string) string {
	// Simple extraction - just get the host part
	// e.g., "https://api.openai.com/v1/chat" -> "api.openai.com"
	if urlStr == "" {
		return "api.example.com"
	}

	// Find the scheme
	schemeEnd := strings.Index(urlStr, "://")
	if schemeEnd == -1 {
		return "api.example.com"
	}

	urlStr = urlStr[schemeEnd+3:] // Skip "://"

	// Find the end of the host (port, path, etc.)
	hostEnd := strings.IndexAny(urlStr, ":/?")
	if hostEnd == -1 {
		return urlStr
	}
	return urlStr[:hostEnd]
}

// IsOSMacOS checks if the current OS is macOS
func IsOSMacOS() bool {
	return runtime.GOOS == "darwin"
}

// GuessAuthStrategy determines likely auth strategy based on header name
func GuessAuthStrategy(headerName string) string {
	nameLower := strings.ToLower(headerName)
	if nameLower == "authorization" {
		return "bearer"
	}
	return "header:" + headerName
}
