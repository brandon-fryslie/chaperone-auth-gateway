package config

import (
	"testing"
)

func TestMergeConfigs(t *testing.T) {
	tests := []struct {
		name string
		base *Config
		proj *Config
		want *Config
	}{
		{
			name: "empty project config",
			base: &Config{
				Server: ServerConfig{
					Address: "127.0.0.1",
					Port:    4010,
				},
			},
			proj: &Config{},
			want: &Config{
				Server: ServerConfig{
					Address: "127.0.0.1",
					Port:    4010,
				},
			},
		},
		{
			name: "server field-level override",
			base: &Config{
				Server: ServerConfig{
					Address: "127.0.0.1",
					Port:    4010,
				},
			},
			proj: &Config{
				Server: ServerConfig{
					Port: 8080,
				},
			},
			want: &Config{
				Server: ServerConfig{
					Address: "127.0.0.1", // Not overridden
					Port:    8080,        // Overridden
				},
			},
		},
		{
			name: "logging field-level override",
			base: &Config{
				Logging: LoggingConfig{
					Level:  "info",
					Format: "json",
					Output: "stdout",
				},
			},
			proj: &Config{
				Logging: LoggingConfig{
					Level: "debug",
				},
			},
			want: &Config{
				Logging: LoggingConfig{
					Level:  "debug", // Overridden
					Format: "json",  // Not overridden
					Output: "stdout",
				},
			},
		},
		{
			name: "audit field-level override",
			base: &Config{
				Audit: AuditConfig{
					Enabled: false,
					Path:    "",
				},
			},
			proj: &Config{
				Audit: AuditConfig{
					Enabled: true,
					Path:    "/var/log/audit.log",
				},
			},
			want: &Config{
				Audit: AuditConfig{
					Enabled: true,
					Path:    "/var/log/audit.log",
				},
			},
		},
		{
			name: "service-level replacement",
			base: &Config{
				Services: map[string]ServiceConfig{
					"openai": {
						HostPattern:   "api.openai.com",
						AuthStrategy:  "bearer",
						CredentialRef: "env:OPENAI_KEY",
					},
					"anthropic": {
						HostPattern:   "api.anthropic.com",
						AuthStrategy:  "header:x-api-key",
						CredentialRef: "env:ANTHROPIC_KEY",
					},
				},
			},
			proj: &Config{
				Services: map[string]ServiceConfig{
					"openai": {
						HostPattern:   "api.openai.com",
						AuthStrategy:  "bearer",
						CredentialRef: "file:/tmp/openai.key", // Different credential
						Placeholder:   "chap_test",            // New field
					},
				},
			},
			want: &Config{
				Services: map[string]ServiceConfig{
					"openai": {
						HostPattern:   "api.openai.com",
						AuthStrategy:  "bearer",
						CredentialRef: "file:/tmp/openai.key", // Completely replaced
						Placeholder:   "chap_test",
					},
					"anthropic": {
						HostPattern:   "api.anthropic.com",
						AuthStrategy:  "header:x-api-key",
						CredentialRef: "env:ANTHROPIC_KEY", // Unchanged
					},
				},
			},
		},
		{
			name: "project adds new service",
			base: &Config{
				Services: map[string]ServiceConfig{
					"openai": {
						HostPattern:   "api.openai.com",
						AuthStrategy:  "bearer",
						CredentialRef: "env:OPENAI_KEY",
					},
				},
			},
			proj: &Config{
				Services: map[string]ServiceConfig{
					"anthropic": {
						HostPattern:   "api.anthropic.com",
						AuthStrategy:  "header:x-api-key",
						CredentialRef: "env:ANTHROPIC_KEY",
					},
				},
			},
			want: &Config{
				Services: map[string]ServiceConfig{
					"openai": {
						HostPattern:   "api.openai.com",
						AuthStrategy:  "bearer",
						CredentialRef: "env:OPENAI_KEY",
					},
					"anthropic": {
						HostPattern:   "api.anthropic.com",
						AuthStrategy:  "header:x-api-key",
						CredentialRef: "env:ANTHROPIC_KEY",
					},
				},
			},
		},
		{
			name: "nil base services",
			base: &Config{
				Services: nil,
			},
			proj: &Config{
				Services: map[string]ServiceConfig{
					"openai": {
						HostPattern:   "api.openai.com",
						AuthStrategy:  "bearer",
						CredentialRef: "env:OPENAI_KEY",
					},
				},
			},
			want: &Config{
				Services: map[string]ServiceConfig{
					"openai": {
						HostPattern:   "api.openai.com",
						AuthStrategy:  "bearer",
						CredentialRef: "env:OPENAI_KEY",
					},
				},
			},
		},
		{
			name: "nil project services",
			base: &Config{
				Services: map[string]ServiceConfig{
					"openai": {
						HostPattern:   "api.openai.com",
						AuthStrategy:  "bearer",
						CredentialRef: "env:OPENAI_KEY",
					},
				},
			},
			proj: &Config{
				Services: nil,
			},
			want: &Config{
				Services: map[string]ServiceConfig{
					"openai": {
						HostPattern:   "api.openai.com",
						AuthStrategy:  "bearer",
						CredentialRef: "env:OPENAI_KEY",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeConfigs(tt.base, tt.proj)

			// Compare Server
			if got.Server != tt.want.Server {
				t.Errorf("Server = %+v, want %+v", got.Server, tt.want.Server)
			}

			// Compare Logging
			if got.Logging != tt.want.Logging {
				t.Errorf("Logging = %+v, want %+v", got.Logging, tt.want.Logging)
			}

			// Compare Audit
			if got.Audit != tt.want.Audit {
				t.Errorf("Audit = %+v, want %+v", got.Audit, tt.want.Audit)
			}

			// Compare Services
			if len(got.Services) != len(tt.want.Services) {
				t.Errorf("Services length = %d, want %d", len(got.Services), len(tt.want.Services))
			}

			for name, wantSvc := range tt.want.Services {
				gotSvc, ok := got.Services[name]
				if !ok {
					t.Errorf("Service %q not found in result", name)
					continue
				}

				if gotSvc.HostPattern != wantSvc.HostPattern {
					t.Errorf("Service %q HostPattern = %q, want %q", name, gotSvc.HostPattern, wantSvc.HostPattern)
				}
				if gotSvc.AuthStrategy != wantSvc.AuthStrategy {
					t.Errorf("Service %q AuthStrategy = %q, want %q", name, gotSvc.AuthStrategy, wantSvc.AuthStrategy)
				}
				if gotSvc.CredentialRef != wantSvc.CredentialRef {
					t.Errorf("Service %q CredentialRef = %q, want %q", name, gotSvc.CredentialRef, wantSvc.CredentialRef)
				}
				if gotSvc.Placeholder != wantSvc.Placeholder {
					t.Errorf("Service %q Placeholder = %q, want %q", name, gotSvc.Placeholder, wantSvc.Placeholder)
				}
			}
		})
	}
}
