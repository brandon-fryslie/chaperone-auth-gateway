package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// Config represents the complete application configuration.
type Config struct {
	Server   ServerConfig
	Services map[string]ServiceConfig
	Logging  LoggingConfig
}

// ServerConfig contains HTTP server settings.
type ServerConfig struct {
	Address string `toml:"address"`
	Port    int    `toml:"port"`
}

// ServiceConfig defines settings for a managed API service.
type ServiceConfig struct {
	HostPattern    string   `toml:"host_pattern"`
	AuthStrategy   string   `toml:"auth_strategy"`
	HeaderName     string   `toml:"header_name"` // For "header" auth strategy
	CredentialRef  string   `toml:"credential_ref"`
	AllowedMethods []string `toml:"allowed_methods"`
	AllowedPaths   []string `toml:"allowed_paths"`
	MaxBodyBytes   int64    `toml:"max_body_bytes"`
	ClientGroups   []string `toml:"client_groups"`
	Drop           []string `toml:"drop"`  // URL patterns to block (drops traffic)
	Strip          []string `toml:"strip"` // Headers to strip from requests
}

// LoggingConfig contains logging settings.
type LoggingConfig struct {
	Level  string `toml:"level"`
	Format string `toml:"format"`
	Output string `toml:"output"`
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
	// Validate port range (1-65535)
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid port %d: must be between 1 and 65535", c.Server.Port)
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
	}

	return nil
}

// SetDefaults applies default values to missing fields.
func (c *Config) SetDefaults() {
	// Server defaults
	if c.Server.Address == "" {
		c.Server.Address = "127.0.0.1"
	}
	if c.Server.Port == 0 {
		c.Server.Port = 4010
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
}
