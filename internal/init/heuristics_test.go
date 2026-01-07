package init

import (
	"testing"
)

func TestCheckSentinel(t *testing.T) {
	tests := []struct {
		name       string
		headerName string
		value      string
		sentinel   string
		wantMatch  bool
	}{
		{
			name:       "exact match",
			headerName: "X-Custom-Key",
			value:      "test-sentinel-123",
			sentinel:   "test-sentinel-123",
			wantMatch:  true,
		},
		{
			name:       "no match",
			headerName: "X-Custom-Key",
			value:      "different-value",
			sentinel:   "test-sentinel-123",
			wantMatch:  false,
		},
		{
			name:       "no sentinel configured",
			headerName: "X-Custom-Key",
			value:      "any-value",
			sentinel:   "",
			wantMatch:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DetectorConfig{SentinelValue: tt.sentinel}
			finding := checkSentinel(tt.headerName, tt.value, config)

			if tt.wantMatch {
				if finding == nil {
					t.Error("expected sentinel match, got nil")
					return
				}
				if finding.Confidence != 1.0 {
					t.Errorf("expected confidence 1.0, got %f", finding.Confidence)
				}
				if finding.Heuristic != "sentinel_match" {
					t.Errorf("expected heuristic 'sentinel_match', got %s", finding.Heuristic)
				}
			} else {
				if finding != nil {
					t.Errorf("expected no match, got finding: %+v", finding)
				}
			}
		})
	}
}

func TestCheckKnownAuthHeader(t *testing.T) {
	tests := []struct {
		name       string
		headerName string
		value      string
		wantMatch  bool
	}{
		{
			name:       "Authorization header",
			headerName: "Authorization",
			value:      "Bearer token123",
			wantMatch:  true,
		},
		{
			name:       "X-API-Key header",
			headerName: "X-API-Key",
			value:      "sk-abc123",
			wantMatch:  true,
		},
		{
			name:       "case insensitive match",
			headerName: "AUTHORIZATION",
			value:      "Bearer token123",
			wantMatch:  true,
		},
		{
			name:       "not a known header",
			headerName: "X-Custom-Header",
			value:      "some-value",
			wantMatch:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			finding := checkKnownAuthHeader(tt.headerName, tt.value)

			if tt.wantMatch {
				if finding == nil {
					t.Error("expected known auth header match, got nil")
					return
				}
				if finding.Confidence != 0.9 {
					t.Errorf("expected confidence 0.9, got %f", finding.Confidence)
				}
				if finding.Heuristic != "known_auth_header" {
					t.Errorf("expected heuristic 'known_auth_header', got %s", finding.Heuristic)
				}
			} else {
				if finding != nil {
					t.Errorf("expected no match, got finding: %+v", finding)
				}
			}
		})
	}
}

func TestCheckAuthKeyword(t *testing.T) {
	tests := []struct {
		name       string
		headerName string
		value      string
		wantMatch  bool
	}{
		{
			name:       "contains 'token'",
			headerName: "X-Session-Token",
			value:      "abc123",
			wantMatch:  true,
		},
		{
			name:       "contains 'api-key'",
			headerName: "X-Custom-Api-Key",
			value:      "sk-123",
			wantMatch:  true,
		},
		{
			name:       "contains 'auth'",
			headerName: "X-Auth-Header",
			value:      "value",
			wantMatch:  true,
		},
		{
			name:       "contains 'secret'",
			headerName: "X-Secret",
			value:      "confidential",
			wantMatch:  true,
		},
		{
			name:       "no keyword match",
			headerName: "X-Custom-Header",
			value:      "value",
			wantMatch:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			finding := checkAuthKeyword(tt.headerName, tt.value)

			if tt.wantMatch {
				if finding == nil {
					t.Error("expected auth keyword match, got nil")
					return
				}
				if finding.Confidence != 0.7 {
					t.Errorf("expected confidence 0.7, got %f", finding.Confidence)
				}
				if finding.Heuristic != "auth_keyword" {
					t.Errorf("expected heuristic 'auth_keyword', got %s", finding.Heuristic)
				}
			} else {
				if finding != nil {
					t.Errorf("expected no match, got finding: %+v", finding)
				}
			}
		})
	}
}

func TestCheckValuePattern(t *testing.T) {
	tests := []struct {
		name       string
		headerName string
		value      string
		wantMatch  bool
	}{
		{
			name:       "OpenAI secret key",
			headerName: "X-Custom",
			value:      "sk-abc123def456",
			wantMatch:  true,
		},
		{
			name:       "public key",
			headerName: "X-Custom",
			value:      "pk-xyz789",
			wantMatch:  true,
		},
		{
			name:       "generic key prefix",
			headerName: "X-Custom",
			value:      "key-abcdef123456",
			wantMatch:  true,
		},
		{
			name:       "Bearer token",
			headerName: "X-Custom",
			value:      "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			wantMatch:  true,
		},
		{
			name:       "long base64-ish string",
			headerName: "X-Custom",
			value:      "dGhpcyBpcyBhIHZlcnkgbG9uZyBzdHJpbmcgdGhhdCBsb29rcyBsaWtlIGJhc2U2NCBlbmNvZGVk",
			wantMatch:  true,
		},
		{
			name:       "short string",
			headerName: "X-Custom",
			value:      "short",
			wantMatch:  false,
		},
		{
			name:       "plain text",
			headerName: "X-Custom",
			value:      "application/json",
			wantMatch:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			finding := checkValuePattern(tt.headerName, tt.value)

			if tt.wantMatch {
				if finding == nil {
					t.Errorf("expected value pattern match for %q, got nil", tt.value)
					return
				}
				if finding.Confidence != 0.6 {
					t.Errorf("expected confidence 0.6, got %f", finding.Confidence)
				}
				if finding.Heuristic != "value_pattern" {
					t.Errorf("expected heuristic 'value_pattern', got %s", finding.Heuristic)
				}
			} else {
				if finding != nil {
					t.Errorf("expected no match for %q, got finding: %+v", tt.value, finding)
				}
			}
		})
	}
}

func TestDetectAuth(t *testing.T) {
	tests := []struct {
		name            string
		headers         map[string][]string
		config          DetectorConfig
		wantFindings    int
		wantTopConfidence float64
	}{
		{
			name: "sentinel takes precedence",
			headers: map[string][]string{
				"Authorization":  {"Bearer token123"},
				"X-Custom-Key":   {"sentinel-value-xyz"},
			},
			config: DetectorConfig{SentinelValue: "sentinel-value-xyz"},
			wantFindings: 2,
			wantTopConfidence: 1.0, // Sentinel
		},
		{
			name: "known auth header",
			headers: map[string][]string{
				"Authorization": {"Bearer token123"},
				"Content-Type":  {"application/json"},
			},
			config: DetectorConfig{},
			wantFindings: 1,
			wantTopConfidence: 0.9, // Known header
		},
		{
			name: "auth keyword in name",
			headers: map[string][]string{
				"X-Session-Token": {"abc123"},
				"Content-Type":    {"application/json"},
			},
			config: DetectorConfig{},
			wantFindings: 1, // Known header "token" (has higher confidence, so continue prevents keyword match)
			wantTopConfidence: 0.9, // Known header "token"
		},
		{
			name: "value pattern only",
			headers: map[string][]string{
				"X-Custom": {"sk-abc123def456"},
			},
			config: DetectorConfig{},
			wantFindings: 1,
			wantTopConfidence: 0.6, // Value pattern
		},
		{
			name: "no matches",
			headers: map[string][]string{
				"Content-Type": {"application/json"},
				"Accept":       {"*/*"},
			},
			config: DetectorConfig{},
			wantFindings: 0,
			wantTopConfidence: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := DetectAuth(tt.headers, tt.config)

			if len(findings) != tt.wantFindings {
				t.Errorf("expected %d findings, got %d", tt.wantFindings, len(findings))
			}

			if tt.wantFindings > 0 {
				// Find the top confidence
				topConfidence := 0.0
				for _, f := range findings {
					if f.Confidence > topConfidence {
						topConfidence = f.Confidence
					}
				}
				if topConfidence != tt.wantTopConfidence {
					t.Errorf("expected top confidence %f, got %f", tt.wantTopConfidence, topConfidence)
				}
			}
		})
	}
}
