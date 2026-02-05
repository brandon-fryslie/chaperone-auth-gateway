package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// LoadWithMerge loads and merges user and project configurations.
// User config: ~/.config/chaperone/chaperone.toml
// Project config: .chaperone.toml (current directory)
//
// Merge strategy:
// - Services: Service-level replacement (project service replaces user service with same name)
// - Server/Logging/Audit: Field-level override (project fields override user fields)
func LoadWithMerge() (*Config, error) {
	// Load user config (optional)
	userConfig, err := loadUserConfig()
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load user config: %w", err)
	}

	// Load project config (optional)
	projectConfig, err := loadProjectConfig()
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load project config: %w", err)
	}

	// If neither exists, return error
	if userConfig == nil && projectConfig == nil {
		return nil, fmt.Errorf("no configuration found (checked ~/.config/chaperone/chaperone.toml and .chaperone.toml)")
	}

	// Start with user config (or empty if not found)
	merged := &Config{}
	if userConfig != nil {
		merged = userConfig
	}

	// If no project config, just return user config
	if projectConfig == nil {
		return merged, nil
	}

	// Merge project config into user config
	return mergeConfigs(merged, projectConfig), nil
}

// loadUserConfig loads the user-level configuration.
func loadUserConfig() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	path := filepath.Join(home, ".config", "chaperone", "chaperone.toml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, err
	}

	return Load(path)
}

// loadProjectConfig loads the project-level configuration.
func loadProjectConfig() (*Config, error) {
	path := ".chaperone.toml"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, err
	}

	return Load(path)
}

// mergeConfigs merges project config into base config.
// Services: Service-level replacement (project service replaces base service entirely)
// Server/Logging/Audit: Field-level override (non-zero project values override base)
func mergeConfigs(base, project *Config) *Config {
	result := *base // Shallow copy

	// Merge server config (field-level)
	if project.Server.Address != "" {
		result.Server.Address = project.Server.Address
	}
	if project.Server.Port != 0 {
		result.Server.Port = project.Server.Port
	}

	// Merge logging config (field-level)
	if project.Logging.Level != "" {
		result.Logging.Level = project.Logging.Level
	}
	if project.Logging.Format != "" {
		result.Logging.Format = project.Logging.Format
	}
	if project.Logging.Output != "" {
		result.Logging.Output = project.Logging.Output
	}

	// Merge audit config (field-level)
	// Note: Enabled is a bool, so we check if project explicitly set it
	// For simplicity, we'll always use project's value if present
	// This is safe because bool defaults to false
	if project.Audit.Enabled {
		result.Audit.Enabled = project.Audit.Enabled
	}
	if project.Audit.Path != "" {
		result.Audit.Path = project.Audit.Path
	}

	// Merge services (service-level replacement)
	// Start with base services
	if result.Services == nil {
		result.Services = make(map[string]ServiceConfig)
	}
	if base.Services != nil {
		for name, svc := range base.Services {
			result.Services[name] = svc
		}
	}

	// Replace with project services (complete replacement per service)
	if project.Services != nil {
		for name, svc := range project.Services {
			result.Services[name] = svc
		}
	}

	return &result
}
