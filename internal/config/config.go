package config

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/BurntSushi/toml"
)

// Config represents the complete application configuration.
type Config struct {
	Server   ServerConfig
	Services map[string]ServiceConfig
	Logging  LoggingConfig
	Audit    AuditConfig
}

// ServerConfig contains HTTP server settings.
type ServerConfig struct {
	Address string `toml:"address"`
	Port    int    `toml:"port"`
	Socket  string `toml:"socket"` // Unix socket path (mutually exclusive with Address/Port)
}

// RunConfig defines settings for spawning and managing child processes in run mode.
type RunConfig struct {
	Command    string   `toml:"command"`     // Command to execute (searches PATH)
	Args       []string `toml:"args"`        // Command arguments
	EnvFile    string   `toml:"env_file"`    // Path to .env file (KEY=value format)
	SocketPath string   `toml:"socket_path"` // Override default socket path
	Stdout     string   `toml:"stdout"`      // "inherit", "file:/path", or "discard"
	Stderr     string   `toml:"stderr"`      // "inherit", "file:/path", or "discard"
}

// ServiceConfig defines settings for a managed API service.
type ServiceConfig struct {
	HostPattern    string   `toml:"host_pattern"`
	AuthStrategy   string   `toml:"auth_strategy"`
	HeaderName     string   `toml:"header_name"` // For "header" auth strategy
	CredentialRef  string   `toml:"credential_ref"`
	Placeholder    string   `toml:"placeholder"` // Token app sends that we replace
	AllowedMethods []string `toml:"allowed_methods"`
	AllowedPaths   []string `toml:"allowed_paths"`
	MaxBodyBytes   int64    `toml:"max_body_bytes"`
	ClientGroups   []string `toml:"client_groups"`
	Drop           []string `toml:"drop"`  // URL patterns to block (drops traffic)
	Strip          []string `toml:"strip"` // Headers to strip from requests
	Run            *RunConfig `toml:"run"` // Run mode configuration (optional)
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
	// Validate socket vs port configuration
	if c.Server.Socket != "" && c.Server.Port != 0 {
		// Both socket and port specified - socket takes precedence, log warning
		slog.Warn("both socket and port configured, socket takes precedence",
			"socket", c.Server.Socket,
			"port", c.Server.Port,
		)
	}

	// Validate port range (1-65535) only if socket is not configured
	if c.Server.Socket == "" {
		if c.Server.Port < 1 || c.Server.Port > 65535 {
			return fmt.Errorf("invalid port %d: must be between 1 and 65535", c.Server.Port)
		}
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

	return nil
}

// SetDefaults applies default values to missing fields.
func (c *Config) SetDefaults() {
	// Server defaults
	// Default to Unix socket mode if neither socket nor port is configured
	if c.Server.Socket == "" && c.Server.Port == 0 {
		// Default: Unix socket mode for better security
		c.Server.Socket = "/tmp/chaperone.sock"
	}

	// If port is explicitly set but socket is not, use TCP mode
	if c.Server.Port != 0 && c.Server.Socket == "" {
		if c.Server.Address == "" {
			c.Server.Address = "127.0.0.1"
		}
	}

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
