package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/bmf/chaperone/internal/secrets"
)

// Config represents the complete application configuration.
type Config struct {
	Server    ServerConfig
	Services  map[string]ServiceConfig
	Logging   LoggingConfig
	Audit     AuditConfig
	Grantable []GrantableConfig `toml:"grantable"`
}

// ServerConfig contains HTTP server settings.
// Address is always 127.0.0.1 and Port is always 0 (OS-allocated).
type ServerConfig struct {
	Address string `toml:"address"`
	Port    int    `toml:"port"`
}

// RunConfig defines settings for spawning and managing child processes in run mode.
type RunConfig struct {
	Command   string   `toml:"command"`     // Command to execute (searches PATH)
	Args      []string `toml:"args"`        // Command arguments
	EnvFile   string   `toml:"env_file"`    // Path to .env file (KEY=value format)
	Stdin     string   `toml:"stdin"`       // "inherit", "file:/path", or "discard"
	Stdout    string   `toml:"stdout"`      // "inherit", "file:/path", or "discard"
	Stderr    string   `toml:"stderr"`      // "inherit", "file:/path", or "discard"
	CAEnvVars []string `toml:"ca_env_vars"` // CA cert environment variables to set (defaults to all standard vars)
}

// ServiceConfig defines settings for a managed API service.
type ServiceConfig struct {
	HostPattern    string     `toml:"host_pattern"`
	AuthStrategy   string     `toml:"auth_strategy"`
	HeaderName     string     `toml:"header_name"` // For "header" auth strategy
	CredentialRef  string     `toml:"credential_ref"`
	Placeholder    string     `toml:"placeholder"` // Token app sends that we replace
	AllowedMethods []string   `toml:"allowed_methods"`
	AllowedPaths   []string   `toml:"allowed_paths"`
	MaxBodyBytes   int64      `toml:"max_body_bytes"`
	ClientGroups   []string   `toml:"client_groups"`
	Drop           []string   `toml:"drop"`  // URL patterns to block (drops traffic)
	Strip          []string   `toml:"strip"` // Headers to strip from requests
	Run            *RunConfig `toml:"run"`   // Run mode configuration (optional)
}

// GrantableConfig declares one human-approved (credential ↔ host ↔ strategy)
// pairing that Claude may activate at runtime, plus the MAXIMAL policy bound a
// grant against it may request. It is the human-owned source of truth for the
// grantable universe; the grant enforcer derives its decisions from it and
// nowhere else.
//
// allowed_methods / allowed_paths / max_body_bytes describe the WIDEST scope a
// grant may ask for (empty / zero = unrestricted). A runtime grant may only
// NARROW within these — never widen.
type GrantableConfig struct {
	CredentialRef  string   `toml:"credential_ref"`
	HostPattern    string   `toml:"host_pattern"`
	AuthStrategy   string   `toml:"auth_strategy"`
	HeaderName     string   `toml:"header_name"` // For "header" auth strategy
	AllowedMethods []string `toml:"allowed_methods"`
	AllowedPaths   []string `toml:"allowed_paths"`
	MaxBodyBytes   int64    `toml:"max_body_bytes"`
}

// LoggingConfig contains logging settings.
type LoggingConfig struct {
	Level  string `toml:"level"`
	Format string `toml:"format"`
	Output string `toml:"output"`
}

// AuditConfig contains audit logging settings.
type AuditConfig struct {
	Enabled bool   `toml:"enabled"`
	Path    string `toml:"path"` // File path or "stdout"
}

// Load reads and parses a TOML configuration file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // Path is user-provided config file
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse TOML: %w", err)
	}

	return &cfg, nil
}

// Validate checks that the configuration is valid.
func (c *Config) Validate() error {
	// Address must always be 127.0.0.1
	if c.Server.Address != "127.0.0.1" {
		return fmt.Errorf("invalid address %q: must be 127.0.0.1", c.Server.Address)
	}

	// Port must always be 0 (OS-allocated)
	if c.Server.Port != 0 {
		return fmt.Errorf("invalid port %d: must be 0 (OS-allocated)", c.Server.Port)
	}

	// Validate log level (lowercase only)
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[c.Logging.Level] {
		return fmt.Errorf("invalid log level %q: must be debug, info, warn, or error", c.Logging.Level)
	}

	// Validate services have host patterns
	for name, svc := range c.Services {
		if svc.HostPattern == "" {
			return fmt.Errorf("service %q: host_pattern is required", name)
		}

		// Validate placeholder length (P1: Security requirement)
		if svc.Placeholder != "" && len(svc.Placeholder) < 8 {
			return fmt.Errorf("service %q: placeholder must be at least 8 characters for security (got %d)", name, len(svc.Placeholder))
		}

		// Validate RunConfig if present
		if svc.Run != nil {
			if svc.Run.Command == "" {
				return fmt.Errorf("service %q: run.command is required when run section is present", name)
			}
			// Validate stdout/stderr modes
			validModes := map[string]bool{"": true, "inherit": true, "discard": true}
			if !validModes[svc.Run.Stdout] && len(svc.Run.Stdout) > 0 && svc.Run.Stdout[:5] != "file:" {
				return fmt.Errorf("service %q: run.stdout must be 'inherit', 'discard', or 'file:/path'", name)
			}
			if !validModes[svc.Run.Stderr] && len(svc.Run.Stderr) > 0 && svc.Run.Stderr[:5] != "file:" {
				return fmt.Errorf("service %q: run.stderr must be 'inherit', 'discard', or 'file:/path'", name)
			}
		}
	}

	// Validate grantable pairings (the human-owned grant universe)
	for i := range c.Grantable {
		if err := c.Grantable[i].validate(i); err != nil {
			return err
		}
	}

	return nil
}

// validate checks a single grantable pairing's fields. index identifies it in
// error messages since pairings are an ordered list with no names.
//
// The credential_ref scheme is validated against secrets.IsKnownScheme — the
// single source of truth for which providers exist — so this validator can never
// drift from the set the registry actually registers ([LAW:one-source-of-truth]).
func (g *GrantableConfig) validate(index int) error {
	if strings.TrimSpace(g.HostPattern) == "" {
		return fmt.Errorf("grantable[%d]: host_pattern is required", index)
	}
	if strings.TrimSpace(g.AuthStrategy) == "" {
		return fmt.Errorf("grantable[%d]: auth_strategy is required", index)
	}
	if strings.TrimSpace(g.CredentialRef) == "" {
		return fmt.Errorf("grantable[%d]: credential_ref is required", index)
	}
	if !secrets.IsKnownScheme(g.CredentialRef) {
		return fmt.Errorf("grantable[%d]: credential_ref %q must point to a secret via a known provider scheme",
			index, g.CredentialRef)
	}
	if g.MaxBodyBytes < 0 {
		return fmt.Errorf("grantable[%d]: max_body_bytes must not be negative (got %d)", index, g.MaxBodyBytes)
	}
	return nil
}

// SetDefaults applies default values to missing fields.
func (c *Config) SetDefaults() {
	// Server defaults - always use 127.0.0.1:0 (OS-allocated port)
	c.Server.Address = "127.0.0.1"
	c.Server.Port = 0

	// Logging defaults
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	if c.Logging.Format == "" {
		c.Logging.Format = "json"
	}
	if c.Logging.Output == "" {
		c.Logging.Output = "stdout"
	}

	// RunConfig defaults
	for name := range c.Services {
		svc := c.Services[name]
		if svc.Run != nil {
			if svc.Run.Stdout == "" {
				svc.Run.Stdout = "inherit"
			}
			if svc.Run.Stderr == "" {
				svc.Run.Stderr = "inherit"
			}
			// Update the map entry (necessary because Services is a map of values, not pointers)
			c.Services[name] = svc
		}
	}
}
