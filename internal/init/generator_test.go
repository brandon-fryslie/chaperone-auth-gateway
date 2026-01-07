package init

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/bmf/chaperone/internal/config"
	"github.com/bmf/chaperone/internal/service"
)

func TestInferAuthStrategy(t *testing.T) {
	tests := []struct {
		name       string
		headerName string
		want       string
	}{
		{
			name:       "Authorization header",
			headerName: "Authorization",
			want:       "bearer",
		},
		{
			name:       "authorization (lowercase)",
			headerName: "authorization",
			want:       "bearer",
		},
		{
			name:       "AUTHORIZATION (uppercase)",
			headerName: "AUTHORIZATION",
			want:       "bearer",
		},
		{
			name:       "X-API-Key",
			headerName: "X-API-Key",
			want:       "header:X-API-Key",
		},
		{
			name:       "X-Custom-Auth",
			headerName: "X-Custom-Auth",
			want:       "header:X-Custom-Auth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InferAuthStrategy(tt.headerName)
			if got != tt.want {
				t.Errorf("InferAuthStrategy(%q) = %q, want %q", tt.headerName, got, tt.want)
			}
		})
	}
}

func TestGenerateTOMLConfig(t *testing.T) {
	tests := []struct {
		name    string
		service *GeneratedService
		wantErr bool
	}{
		{
			name: "bearer strategy with policy",
			service: &GeneratedService{
				Name:          "openai",
				HostPattern:   "api.openai.com",
				AuthStrategy:  "bearer",
				CredentialRef: "keychain:chaperone/openai",
				Policy: &service.Policy{
					AllowedMethods: []string{"GET", "POST"},
					AllowedPaths:   []string{"/v1/*"},
					MaxBodyBytes:   1048576,
				},
			},
			wantErr: false,
		},
		{
			name: "header strategy without policy",
			service: &GeneratedService{
				Name:          "anthropic",
				HostPattern:   "api.anthropic.com",
				AuthStrategy:  "header:x-api-key",
				CredentialRef: "env:ANTHROPIC_API_KEY",
				Policy:        nil,
			},
			wantErr: false,
		},
		{
			name: "file credential",
			service: &GeneratedService{
				Name:          "custom",
				HostPattern:   "api.custom.com",
				AuthStrategy:  "header:X-Custom-Key",
				CredentialRef: "file:/Users/test/.config/chaperone/secrets/custom",
				Policy: &service.Policy{
					AllowedMethods: []string{"POST"},
					AllowedPaths:   []string{"/api/*"},
					MaxBodyBytes:   2097152,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateTOMLConfig(tt.service)

			// Verify the generated TOML is parseable
			var cfg config.Config
			// Wrap in [[services]] array for parsing
			fullTOML := "[server]\naddress = \"127.0.0.1\"\nport = 4010\n\n[logging]\nlevel = \"info\"\n\n" + got
			if err := toml.Unmarshal([]byte(fullTOML), &cfg); err != nil {
				if !tt.wantErr {
					t.Errorf("Generated TOML failed to parse: %v\nTOML:\n%s", err, got)
				}
				return
			}

			// Verify basic fields are present
			if !strings.Contains(got, tt.service.Name) {
				t.Errorf("Generated TOML missing service name %q", tt.service.Name)
			}
			if !strings.Contains(got, tt.service.HostPattern) {
				t.Errorf("Generated TOML missing host pattern %q", tt.service.HostPattern)
			}
			if !strings.Contains(got, tt.service.CredentialRef) {
				t.Errorf("Generated TOML missing credential ref %q", tt.service.CredentialRef)
			}

			// Verify policy section if present
			if tt.service.Policy != nil && len(tt.service.Policy.AllowedMethods) > 0 {
				if !strings.Contains(got, "[services.policy]") {
					t.Error("Generated TOML missing [services.policy] section")
				}
			}
		})
	}
}

func TestBuildGeneratedService(t *testing.T) {
	tests := []struct {
		name          string
		serviceName   string
		findings      *ServiceFindings
		topFinding    *Finding
		credentialRef string
		wantNil       bool
	}{
		{
			name:        "valid findings",
			serviceName: "openai",
			findings: &ServiceFindings{
				Host: "api.openai.com",
				AuthFindings: map[string]*Finding{
					"authorization": {
						HeaderName:  "Authorization",
						HeaderValue: "Bearer sk-xxx",
						Confidence:  0.9,
						Heuristic:   "known_auth_header",
					},
				},
				Policy: &PolicyFindings{
					Methods:      map[string]bool{"GET": true, "POST": true},
					Paths:        map[string]bool{"/v1/chat/completions": true, "/v1/models": true},
					MaxBodyBytes: 512000,
				},
			},
			topFinding: &Finding{
				HeaderName:  "Authorization",
				HeaderValue: "Bearer sk-xxx",
				Confidence:  0.9,
				Heuristic:   "known_auth_header",
			},
			credentialRef: "keychain:chaperone/openai",
			wantNil:       false,
		},
		{
			name:          "nil findings",
			serviceName:   "test",
			findings:      nil,
			topFinding:    nil,
			credentialRef: "env:TEST_KEY",
			wantNil:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildGeneratedService(tt.serviceName, tt.findings, tt.topFinding, tt.credentialRef)

			if tt.wantNil {
				if got != nil {
					t.Error("Expected nil GeneratedService, got non-nil")
				}
				return
			}

			if got == nil {
				t.Fatal("Expected non-nil GeneratedService, got nil")
			}

			if got.Name != tt.serviceName {
				t.Errorf("Name = %q, want %q", got.Name, tt.serviceName)
			}
			if got.HostPattern != tt.findings.Host {
				t.Errorf("HostPattern = %q, want %q", got.HostPattern, tt.findings.Host)
			}
			if got.CredentialRef != tt.credentialRef {
				t.Errorf("CredentialRef = %q, want %q", got.CredentialRef, tt.credentialRef)
			}

			// Verify auth strategy inference
			expectedStrategy := InferAuthStrategy(tt.topFinding.HeaderName)
			if got.AuthStrategy != expectedStrategy {
				t.Errorf("AuthStrategy = %q, want %q", got.AuthStrategy, expectedStrategy)
			}

			// Verify policy was built
			if tt.findings.Policy != nil && len(tt.findings.Policy.Methods) > 0 {
				if got.Policy == nil {
					t.Error("Expected Policy to be set, got nil")
				}
			}
		})
	}
}
