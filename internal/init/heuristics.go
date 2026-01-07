package init

import (
	"regexp"
	"strings"
)

// knownAuthHeaders is the list of headers commonly used for authentication.
// This list is synchronized with internal/proxy/handlers.go:136-149.
var knownAuthHeaders = map[string]bool{
	"authorization":   true,
	"x-api-key":       true,
	"x-auth-token":    true,
	"api-key":         true,
	"apikey":          true,
	"x-access-token":  true,
	"x-token":         true,
	"token":           true,
	"bearer":          true,
	"x-session-token": true,
	"x-csrf-token":    true,
	"x-xsrf-token":    true,
}

// authKeywords are keywords in header names that suggest authentication.
var authKeywords = []string{
	"api-key",
	"apikey",
	"token",
	"auth",
	"secret",
	"bearer",
	"credential",
	"key",
}

// credentialValuePatterns are regex patterns that match common credential formats.
var credentialValuePatterns = []*regexp.Regexp{
	// Starts with common prefixes
	regexp.MustCompile(`^sk-[a-zA-Z0-9_-]+$`),       // OpenAI secret keys
	regexp.MustCompile(`^pk-[a-zA-Z0-9_-]+$`),       // Public keys
	regexp.MustCompile(`^key-[a-zA-Z0-9_-]+$`),      // Generic key- prefix
	regexp.MustCompile(`^Bearer\s+[a-zA-Z0-9_-]+$`), // Bearer token
	// Long alphanumeric strings (40+ chars, looks like base64)
	regexp.MustCompile(`^[a-zA-Z0-9+/=_-]{40,}$`),
}

// DetectAuth analyzes HTTP headers and returns all findings.
// Returns a slice of Finding objects with confidence scores.
func DetectAuth(headers map[string][]string, config DetectorConfig) []*Finding {
	var findings []*Finding

	for headerName, values := range headers {
		for _, value := range values {
			if value == "" {
				continue
			}

			// Try each heuristic in order of confidence
			if finding := checkSentinel(headerName, value, config); finding != nil {
				findings = append(findings, finding)
				continue
			}

			if finding := checkKnownAuthHeader(headerName, value); finding != nil {
				findings = append(findings, finding)
				continue
			}

			if finding := checkAuthKeyword(headerName, value); finding != nil {
				findings = append(findings, finding)
				continue
			}

			if finding := checkValuePattern(headerName, value); finding != nil {
				findings = append(findings, finding)
				continue
			}
		}
	}

	return findings
}

// checkSentinel checks if the header value matches the sentinel value (100% confidence).
func checkSentinel(headerName, value string, config DetectorConfig) *Finding {
	if config.SentinelValue == "" {
		return nil
	}

	if value == config.SentinelValue {
		return &Finding{
			HeaderName:  headerName,
			HeaderValue: value,
			Confidence:  1.0,
			Heuristic:   "sentinel_match",
		}
	}

	return nil
}

// checkKnownAuthHeader checks if the header is in the known auth headers list (90% confidence).
func checkKnownAuthHeader(headerName, value string) *Finding {
	headerLower := strings.ToLower(headerName)

	if knownAuthHeaders[headerLower] {
		return &Finding{
			HeaderName:  headerName,
			HeaderValue: value,
			Confidence:  0.9,
			Heuristic:   "known_auth_header",
		}
	}

	return nil
}

// checkAuthKeyword checks if the header name contains auth-related keywords (70% confidence).
func checkAuthKeyword(headerName, value string) *Finding {
	headerLower := strings.ToLower(headerName)

	for _, keyword := range authKeywords {
		if strings.Contains(headerLower, keyword) {
			return &Finding{
				HeaderName:  headerName,
				HeaderValue: value,
				Confidence:  0.7,
				Heuristic:   "auth_keyword",
			}
		}
	}

	return nil
}

// checkValuePattern checks if the value matches common credential patterns (60% confidence).
func checkValuePattern(headerName, value string) *Finding {
	for _, pattern := range credentialValuePatterns {
		if pattern.MatchString(value) {
			return &Finding{
				HeaderName:  headerName,
				HeaderValue: value,
				Confidence:  0.6,
				Heuristic:   "value_pattern",
			}
		}
	}

	return nil
}
