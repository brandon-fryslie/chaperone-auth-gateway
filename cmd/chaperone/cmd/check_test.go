package cmd

import (
	"os/user"
	"testing"

	"github.com/bmf/chaperone/internal/config"
)

func TestCheckPlaceholders_NilConfig(t *testing.T) {
	result := checkPlaceholders(nil)
	if result.allConfigured {
		t.Error("expected allConfigured to be false with nil config")
	}
	if result.missing != nil {
		t.Errorf("expected missing to be nil, got %v", result.missing)
	}
}

func TestCheckPlaceholders_EmptyServices(t *testing.T) {
	cfg := &config.Config{
		Services: map[string]config.ServiceConfig{},
	}
	result := checkPlaceholders(cfg)
	if result.allConfigured {
		t.Error("expected allConfigured to be false with empty services")
	}
	if result.missing != nil {
		t.Errorf("expected missing to be nil, got %v", result.missing)
	}
}

func TestCheckPlaceholders_AllConfigured(t *testing.T) {
	cfg := &config.Config{
		Services: map[string]config.ServiceConfig{
			"openai": {
				HostPattern:   "api.openai.com",
				Placeholder:   "chap_openai_xxx",
				AuthStrategy:  "bearer",
				CredentialRef: "env:OPENAI_API_KEY",
			},
			"anthropic": {
				HostPattern:   "api.anthropic.com",
				Placeholder:   "chap_anthropic_xxx",
				AuthStrategy:  "header:x-api-key",
				CredentialRef: "env:ANTHROPIC_API_KEY",
			},
		},
	}
	result := checkPlaceholders(cfg)
	if !result.allConfigured {
		t.Error("expected allConfigured to be true when all services have placeholders")
	}
	if len(result.missing) != 0 {
		t.Errorf("expected no missing services, got %v", result.missing)
	}
}

func TestCheckPlaceholders_MissingPlaceholders(t *testing.T) {
	cfg := &config.Config{
		Services: map[string]config.ServiceConfig{
			"openai": {
				HostPattern:   "api.openai.com",
				Placeholder:   "chap_openai_xxx",
				AuthStrategy:  "bearer",
				CredentialRef: "env:OPENAI_API_KEY",
			},
			"anthropic": {
				HostPattern:   "api.anthropic.com",
				Placeholder:   "", // Missing
				AuthStrategy:  "header:x-api-key",
				CredentialRef: "env:ANTHROPIC_API_KEY",
			},
			"github": {
				HostPattern: "api.github.com",
				// Placeholder not set
				AuthStrategy:  "bearer",
				CredentialRef: "env:GITHUB_TOKEN",
			},
		},
	}
	result := checkPlaceholders(cfg)
	if result.allConfigured {
		t.Error("expected allConfigured to be false when some services lack placeholders")
	}
	if len(result.missing) != 2 {
		t.Errorf("expected 2 missing services, got %d: %v", len(result.missing), result.missing)
	}
	// Check that missing contains the right services (order may vary)
	missingMap := make(map[string]bool)
	for _, name := range result.missing {
		missingMap[name] = true
	}
	if !missingMap["anthropic"] {
		t.Error("expected 'anthropic' in missing services")
	}
	if !missingMap["github"] {
		t.Error("expected 'github' in missing services")
	}
}

func TestIsDedicatedUser(t *testing.T) {
	// This test is environment-dependent
	// We can only verify it returns false for current user unless actually 'chaperone'
	currentUser, err := user.Current()
	if err != nil {
		t.Skip("cannot get current user, skipping test")
	}

	result := isDedicatedUser()

	if currentUser.Username == "chaperone" {
		if !result {
			t.Error("expected true when running as 'chaperone' user")
		}
	} else {
		if result {
			t.Errorf("expected false when running as '%s' (not 'chaperone')", currentUser.Username)
		}
	}
}

func TestGetCurrentUsername(t *testing.T) {
	username := getCurrentUsername()
	if username == "" {
		t.Error("expected non-empty username")
	}
	if username == "unknown" {
		// This is acceptable if user.Current() fails, but let's verify
		currentUser, err := user.Current()
		if err == nil {
			t.Errorf("expected real username, got 'unknown', but user.Current() succeeded with '%s'", currentUser.Username)
		}
	}
}

func TestJoinMax(t *testing.T) {
	tests := []struct {
		name     string
		items    []string
		max      int
		expected string
	}{
		{
			name:     "fewer than max",
			items:    []string{"one", "two"},
			max:      3,
			expected: "one, two",
		},
		{
			name:     "equal to max",
			items:    []string{"one", "two", "three"},
			max:      3,
			expected: "one, two, three",
		},
		{
			name:     "more than max",
			items:    []string{"one", "two", "three", "four", "five"},
			max:      3,
			expected: "one, two, three (+2 more)",
		},
		{
			name:     "empty list",
			items:    []string{},
			max:      3,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := joinMax(tt.items, tt.max)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}
