package test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bmf/chaperone/internal/config"
)

// TestConfigurationFramework validates Phase 0.4: Configuration Framework
//
// This test cannot be gamed because:
// 1. Loads actual TOML files from disk (not in-memory strings)
// 2. Validates real file I/O and parsing behavior
// 3. Checks actual validation logic with specific error conditions
// 4. Verifies default value application by inspecting actual struct fields
// 5. Tests multiple services configuration with real data structures
// 6. Confirms error handling with invalid/missing files
// 7. Validates complete configuration roundtrip (load -> validate -> defaults)
//
// An AI cannot fake this with stubs - the TOML parsing, validation,
// and default logic must actually work.

func TestConfigurationFramework(t *testing.T) {
	t.Run("load_minimal_config", func(t *testing.T) {
		testLoadMinimalConfig(t)
	})

	t.Run("load_full_config", func(t *testing.T) {
		testLoadFullConfig(t)
	})

	t.Run("load_missing_file", func(t *testing.T) {
		testLoadMissingFile(t)
	})

	t.Run("load_invalid_toml", func(t *testing.T) {
		testLoadInvalidTOML(t)
	})

	t.Run("validate_port_ranges", func(t *testing.T) {
		testValidatePortRanges(t)
	})

	t.Run("validate_log_levels", func(t *testing.T) {
		testValidateLogLevels(t)
	})

	t.Run("validate_service_host_pattern", func(t *testing.T) {
		testValidateServiceHostPattern(t)
	})

	t.Run("default_server_address", func(t *testing.T) {
		testDefaultServerAddress(t)
	})

	t.Run("default_server_port", func(t *testing.T) {
		testDefaultServerPort(t)
	})

	t.Run("default_logging_config", func(t *testing.T) {
		testDefaultLoggingConfig(t)
	})

	t.Run("multiple_services_load", func(t *testing.T) {
		testMultipleServicesLoad(t)
	})

	t.Run("service_fields_parsed", func(t *testing.T) {
		testServiceFieldsParsed(t)
	})

	t.Run("complete_config_workflow", func(t *testing.T) {
		testCompleteConfigWorkflow(t)
	})
}

// testLoadMinimalConfig verifies:
// - Load() can read a minimal TOML file
// - Config struct is populated with values from file
// - No errors on valid minimal config
func testLoadMinimalConfig(t *testing.T) {
	configPath := filepath.Join("fixtures", "configs", "minimal.toml")

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load() should succeed for minimal.toml, got error: %v", err)
	}

	if cfg == nil {
		t.Fatal("Load() returned nil config for valid file")
	}

	// Minimal config should have at least one service
	if len(cfg.Services) == 0 {
		t.Error("Minimal config should have at least one service defined")
	}

	t.Logf("Successfully loaded minimal config with %d services", len(cfg.Services))
}

// testLoadFullConfig verifies:
// - Load() can read a full TOML file with all fields
// - All configuration sections are populated
// - Complex configurations parse correctly
func testLoadFullConfig(t *testing.T) {
	configPath := filepath.Join("fixtures", "configs", "full.toml")

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load() should succeed for full.toml, got error: %v", err)
	}

	if cfg == nil {
		t.Fatal("Load() returned nil config for valid file")
	}

	// Full config should have Server configuration
	if cfg.Server.Port == 0 {
		t.Error("Full config should have Server.Port set")
	}

	if cfg.Server.Address == "" {
		t.Error("Full config should have Server.Address set")
	}

	// Full config should have Logging configuration
	if cfg.Logging.Level == "" {
		t.Error("Full config should have Logging.Level set")
	}

	if cfg.Logging.Format == "" {
		t.Error("Full config should have Logging.Format set")
	}

	if cfg.Logging.Output == "" {
		t.Error("Full config should have Logging.Output set")
	}

	// Full config should have Services
	if len(cfg.Services) == 0 {
		t.Error("Full config should have services defined")
	}

	t.Logf("Successfully loaded full config: Server=%s:%d, Logging=%s/%s, Services=%d",
		cfg.Server.Address, cfg.Server.Port, cfg.Logging.Level, cfg.Logging.Format, len(cfg.Services))
}

// testLoadMissingFile verifies:
// - Load() returns error for missing file
// - Error message is meaningful
func testLoadMissingFile(t *testing.T) {
	configPath := filepath.Join("fixtures", "configs", "does-not-exist.toml")

	cfg, err := config.Load(configPath)
	if err == nil {
		t.Fatal("Load() should return error for missing file")
	}

	if cfg != nil {
		t.Error("Load() should return nil config on error")
	}

	t.Logf("Load() correctly returned error for missing file: %v", err)
}

// testLoadInvalidTOML verifies:
// - Load() returns error for invalid TOML syntax
// - Error indicates parsing problem
func testLoadInvalidTOML(t *testing.T) {
	configPath := filepath.Join("fixtures", "configs", "invalid.toml")

	cfg, err := config.Load(configPath)
	if err == nil {
		t.Fatal("Load() should return error for invalid TOML")
	}

	if cfg != nil {
		t.Error("Load() should return nil config on error")
	}

	t.Logf("Load() correctly returned error for invalid TOML: %v", err)
}

// testValidatePortRanges verifies:
// - Validate() rejects port 0
// - Validate() rejects port > 65535
// - Validate() accepts valid ports (1-65535)
func testValidatePortRanges(t *testing.T) {
	// Test invalid port: 0
	cfg := &config.Config{
		Server: config.ServerConfig{
			Address: "127.0.0.1",
			Port:    0,
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() should reject port 0")
	} else {
		t.Logf("Validate() correctly rejected port 0: %v", err)
	}

	// Test invalid port: 70000
	cfg.Server.Port = 70000
	err = cfg.Validate()
	if err == nil {
		t.Error("Validate() should reject port > 65535")
	} else {
		t.Logf("Validate() correctly rejected port 70000: %v", err)
	}

	// Test invalid port: -1
	cfg.Server.Port = -1
	err = cfg.Validate()
	if err == nil {
		t.Error("Validate() should reject negative port")
	} else {
		t.Logf("Validate() correctly rejected port -1: %v", err)
	}

	// Test valid port: 4010
	cfg.Server.Port = 4010
	// Need to add a service with valid fields for complete validation
	cfg.Services = map[string]config.ServiceConfig{
		"test": {
			HostPattern: "api.example.com",
		},
	}
	cfg.Logging.Level = "info"

	err = cfg.Validate()
	if err != nil {
		t.Errorf("Validate() should accept valid port 4010, got error: %v", err)
	}

	// Test valid edge cases
	validPorts := []int{1, 80, 443, 8080, 65535}
	for _, port := range validPorts {
		cfg.Server.Port = port
		err = cfg.Validate()
		if err != nil {
			t.Errorf("Validate() should accept valid port %d, got error: %v", port, err)
		}
	}
}

// testValidateLogLevels verifies:
// - Validate() accepts valid log levels: debug, info, warn, error
// - Validate() rejects invalid log levels
func testValidateLogLevels(t *testing.T) {
	baseCfg := &config.Config{
		Server: config.ServerConfig{
			Address: "127.0.0.1",
			Port:    4010,
		},
		Services: map[string]config.ServiceConfig{
			"test": {
				HostPattern: "api.example.com",
			},
		},
	}

	// Test valid log levels
	validLevels := []string{"debug", "info", "warn", "error"}
	for _, level := range validLevels {
		cfg := *baseCfg
		cfg.Logging.Level = level
		err := cfg.Validate()
		if err != nil {
			t.Errorf("Validate() should accept log level '%s', got error: %v", level, err)
		}
	}

	// Test invalid log levels
	invalidLevels := []string{"trace", "fatal", "invalid", "INFO", "Debug", ""}
	for _, level := range invalidLevels {
		cfg := *baseCfg
		cfg.Logging.Level = level
		err := cfg.Validate()
		if err == nil {
			t.Errorf("Validate() should reject invalid log level '%s'", level)
		} else {
			t.Logf("Validate() correctly rejected log level '%s': %v", level, err)
		}
	}
}

// testValidateServiceHostPattern verifies:
// - Validate() rejects services with empty HostPattern
// - Validate() requires HostPattern for all services
func testValidateServiceHostPattern(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Address: "127.0.0.1",
			Port:    4010,
		},
		Logging: config.LoggingConfig{
			Level: "info",
		},
		Services: map[string]config.ServiceConfig{
			"test-service": {
				HostPattern: "", // Empty host pattern - should fail
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() should reject service with empty HostPattern")
	} else {
		t.Logf("Validate() correctly rejected empty HostPattern: %v", err)
	}

	// Test valid host pattern
	cfg.Services["test-service"] = config.ServiceConfig{
		HostPattern: "api.example.com",
	}

	err = cfg.Validate()
	if err != nil {
		t.Errorf("Validate() should accept service with valid HostPattern, got error: %v", err)
	}
}

// testDefaultServerAddress verifies:
// - SetDefaults() sets Server.Address to "127.0.0.1" when empty
// - SetDefaults() defaults to Unix socket mode (no address set)
// - SetDefaults() sets Server.Address to 127.0.0.1 when port is explicitly set
// - SetDefaults() does not override explicitly set Address
func testDefaultServerAddress(t *testing.T) {
	// Test: Default is Unix socket mode (no address/port)
	cfg := &config.Config{}
	cfg.SetDefaults()

	// With Unix socket default, Address is not set
	if cfg.Server.Socket != "/tmp/chaperone.sock" {
		t.Errorf("SetDefaults() should default to Unix socket mode, got socket: %s", cfg.Server.Socket)
	}

	// Test: When port is explicitly set, Address defaults to 127.0.0.1
	cfg = &config.Config{
		Server: config.ServerConfig{
			Port: 4010,
		},
	}
	cfg.SetDefaults()

	if cfg.Server.Address != "127.0.0.1" {
		t.Errorf("SetDefaults() should set Server.Address to '127.0.0.1' when port is set, got: %s", cfg.Server.Address)
	}

	// Test: Explicit value is not overridden
	cfg = &config.Config{
		Server: config.ServerConfig{
			Port:    4010,
			Address: "0.0.0.0",
		},
	}
	cfg.SetDefaults()

	if cfg.Server.Address != "0.0.0.0" {
		t.Errorf("SetDefaults() should not override explicit Server.Address, got: %s", cfg.Server.Address)
	}

	t.Logf("SetDefaults() correctly handled Server.Address")
}

// testDefaultServerPort verifies:
// - SetDefaults() defaults to Unix socket mode (port remains 0)
// - SetDefaults() does not override explicitly set Port
func testDefaultServerPort(t *testing.T) {
	// Test: Default is Unix socket mode (port remains 0)
	cfg := &config.Config{}
	cfg.SetDefaults()

	// With Unix socket default, Port remains 0
	if cfg.Server.Port != 0 {
		t.Errorf("SetDefaults() should leave Server.Port as 0 in socket mode, got: %d", cfg.Server.Port)
	}
	if cfg.Server.Socket != "/tmp/chaperone.sock" {
		t.Errorf("SetDefaults() should default to Unix socket mode, got socket: %s", cfg.Server.Socket)
	}

	// Test: Explicit value is not overridden
	cfg = &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
		},
	}
	cfg.SetDefaults()

	if cfg.Server.Port != 8080 {
		t.Errorf("SetDefaults() should not override explicit Server.Port, got: %d", cfg.Server.Port)
	}

	t.Logf("SetDefaults() correctly handled Server.Port")
}

// testDefaultLoggingConfig verifies:
// - SetDefaults() sets Logging.Level to "info" when empty
// - SetDefaults() sets Logging.Format to "json" when empty
// - SetDefaults() sets Logging.Output to "stdout" when empty
// - SetDefaults() does not override explicitly set values
func testDefaultLoggingConfig(t *testing.T) {
	// Test all defaults are applied when empty
	cfg := &config.Config{}
	cfg.SetDefaults()

	if cfg.Logging.Level != "info" {
		t.Errorf("SetDefaults() should set Logging.Level to 'info', got: %s", cfg.Logging.Level)
	}

	if cfg.Logging.Format != "json" {
		t.Errorf("SetDefaults() should set Logging.Format to 'json', got: %s", cfg.Logging.Format)
	}

	if cfg.Logging.Output != "stdout" {
		t.Errorf("SetDefaults() should set Logging.Output to 'stdout', got: %s", cfg.Logging.Output)
	}

	// Test explicit values are not overridden
	cfg = &config.Config{
		Logging: config.LoggingConfig{
			Level:  "debug",
			Format: "text",
			Output: "stderr",
		},
	}
	cfg.SetDefaults()

	if cfg.Logging.Level != "debug" {
		t.Errorf("SetDefaults() should not override explicit Logging.Level, got: %s", cfg.Logging.Level)
	}

	if cfg.Logging.Format != "text" {
		t.Errorf("SetDefaults() should not override explicit Logging.Format, got: %s", cfg.Logging.Format)
	}

	if cfg.Logging.Output != "stderr" {
		t.Errorf("SetDefaults() should not override explicit Logging.Output, got: %s", cfg.Logging.Output)
	}

	t.Logf("SetDefaults() correctly handled Logging config")
}

// testMultipleServicesLoad verifies:
// - Config can load multiple services from TOML
// - Each service is accessible by name
// - Service map is populated correctly
func testMultipleServicesLoad(t *testing.T) {
	configPath := filepath.Join("fixtures", "configs", "full.toml")

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if len(cfg.Services) < 2 {
		t.Errorf("Full config should have at least 2 services, got: %d", len(cfg.Services))
	}

	// Verify services are accessible by name
	expectedServices := []string{"github", "api"}
	for _, serviceName := range expectedServices {
		service, exists := cfg.Services[serviceName]
		if !exists {
			t.Errorf("Expected service '%s' not found in config", serviceName)
			continue
		}

		if service.HostPattern == "" {
			t.Errorf("Service '%s' should have HostPattern set", serviceName)
		}

		t.Logf("Service '%s': HostPattern=%s", serviceName, service.HostPattern)
	}
}

// testServiceFieldsParsed verifies:
// - All ServiceConfig fields parse correctly from TOML
// - String fields, slice fields, and int64 fields work
// - HostPattern, AuthStrategy, CredentialRef parse correctly
// - AllowedMethods, AllowedPaths (slices) parse correctly
// - MaxBodyBytes, ClientGroups parse correctly
func testServiceFieldsParsed(t *testing.T) {
	configPath := filepath.Join("fixtures", "configs", "full.toml")

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Check the "github" service which should have all fields
	githubService, exists := cfg.Services["github"]
	if !exists {
		t.Fatal("Expected 'github' service not found in full config")
	}

	// Verify string fields
	if githubService.HostPattern != "api.github.com" {
		t.Errorf("github.HostPattern incorrect, got: %s", githubService.HostPattern)
	}

	if githubService.AuthStrategy != "bearer" {
		t.Errorf("github.AuthStrategy incorrect, got: %s", githubService.AuthStrategy)
	}

	if githubService.CredentialRef != "github_token" {
		t.Errorf("github.CredentialRef incorrect, got: %s", githubService.CredentialRef)
	}

	// Verify slice fields
	if len(githubService.AllowedMethods) == 0 {
		t.Error("github.AllowedMethods should not be empty")
	}

	expectedMethods := []string{"GET", "POST", "PATCH", "DELETE"}
	if len(githubService.AllowedMethods) != len(expectedMethods) {
		t.Errorf("github.AllowedMethods length incorrect, expected %d, got: %d",
			len(expectedMethods), len(githubService.AllowedMethods))
	}

	for i, method := range expectedMethods {
		if i >= len(githubService.AllowedMethods) {
			break
		}
		if githubService.AllowedMethods[i] != method {
			t.Errorf("github.AllowedMethods[%d] incorrect, expected %s, got: %s",
				i, method, githubService.AllowedMethods[i])
		}
	}

	if len(githubService.AllowedPaths) == 0 {
		t.Error("github.AllowedPaths should not be empty")
	}

	// Verify int64 field
	if githubService.MaxBodyBytes <= 0 {
		t.Errorf("github.MaxBodyBytes should be positive, got: %d", githubService.MaxBodyBytes)
	}

	expectedMaxBodyBytes := int64(10485760) // 10MB
	if githubService.MaxBodyBytes != expectedMaxBodyBytes {
		t.Errorf("github.MaxBodyBytes incorrect, expected %d, got: %d",
			expectedMaxBodyBytes, githubService.MaxBodyBytes)
	}

	// Verify ClientGroups
	if len(githubService.ClientGroups) == 0 {
		t.Error("github.ClientGroups should not be empty")
	}

	t.Logf("Successfully parsed all github service fields: HostPattern=%s, AuthStrategy=%s, Methods=%d, Paths=%d, MaxBodyBytes=%d, Groups=%d",
		githubService.HostPattern, githubService.AuthStrategy, len(githubService.AllowedMethods),
		len(githubService.AllowedPaths), githubService.MaxBodyBytes, len(githubService.ClientGroups))
}

// testCompleteConfigWorkflow verifies:
// - Load -> SetDefaults -> Validate workflow works end-to-end
// - Config loaded from file can be validated after defaults applied
// - Real-world usage pattern works correctly
func testCompleteConfigWorkflow(t *testing.T) {
	// Load minimal config
	configPath := filepath.Join("fixtures", "configs", "minimal.toml")

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Apply defaults
	cfg.SetDefaults()

	// Verify defaults were applied (Unix socket mode by default)
	if cfg.Server.Socket == "" && cfg.Server.Port == 0 {
		t.Error("SetDefaults() should have set Server.Socket or Server.Port")
	}

	if cfg.Logging.Level == "" {
		t.Error("SetDefaults() should have set Logging.Level")
	}

	// Validate (skip port validation in socket mode)
	err = cfg.Validate()
	if err != nil {
		t.Errorf("Validate() should pass after SetDefaults(), got error: %v", err)
	}

	t.Logf("Complete workflow succeeded: Load -> SetDefaults -> Validate")
	if cfg.Server.Socket != "" {
		t.Logf("Final config: Socket=%s, Logging=%s/%s/%s, Services=%d",
			cfg.Server.Socket, cfg.Logging.Level,
			cfg.Logging.Format, cfg.Logging.Output, len(cfg.Services))
	} else {
		t.Logf("Final config: Server=%s:%d, Logging=%s/%s/%s, Services=%d",
			cfg.Server.Address, cfg.Server.Port, cfg.Logging.Level,
			cfg.Logging.Format, cfg.Logging.Output, len(cfg.Services))
	}
}

// TestConfigurationIntegrationScenario tests a realistic configuration scenario
//
// This test simulates a real application startup:
// 1. Load config file specified by user
// 2. Apply defaults for missing fields
// 3. Validate configuration
// 4. Access configuration values
//
// This cannot be gamed - all operations must work with real files and data.
func TestConfigurationIntegrationScenario(t *testing.T) {
	// Simulate application startup with config file
	configPath := filepath.Join("fixtures", "configs", "full.toml")

	// Check file exists (real filesystem check)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatalf("Config file does not exist: %s", configPath)
	}

	// Step 1: Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load configuration: %v", err)
	}

	t.Logf("Step 1: Loaded configuration from %s", configPath)

	// Step 2: Apply defaults
	cfg.SetDefaults()
	t.Log("Step 2: Applied default values")

	// Step 3: Validate configuration
	err = cfg.Validate()
	if err != nil {
		t.Fatalf("Configuration validation failed: %v", err)
	}
	t.Log("Step 3: Configuration validated successfully")

	// Step 4: Access configuration values (simulate application usage)
	serverAddr := cfg.Server.Address
	serverPort := cfg.Server.Port
	logLevel := cfg.Logging.Level
	serviceCount := len(cfg.Services)

	if serverAddr == "" {
		t.Error("Server address should not be empty after defaults and validation")
	}

	if serverPort == 0 {
		t.Error("Server port should not be 0 after defaults and validation")
	}

	if logLevel == "" {
		t.Error("Log level should not be empty after defaults and validation")
	}

	if serviceCount == 0 {
		t.Error("Should have at least one service configured")
	}

	t.Logf("Step 4: Configuration ready for use")
	t.Logf("  Server: %s:%d", serverAddr, serverPort)
	t.Logf("  Logging: %s format=%s output=%s", logLevel, cfg.Logging.Format, cfg.Logging.Output)
	t.Logf("  Services: %d configured", serviceCount)

	// Step 5: Access individual service configuration
	for serviceName, service := range cfg.Services {
		if service.HostPattern == "" {
			t.Errorf("Service '%s' has empty HostPattern", serviceName)
		}

		t.Logf("  Service '%s': host=%s auth=%s methods=%d",
			serviceName, service.HostPattern, service.AuthStrategy, len(service.AllowedMethods))
	}

	t.Log("Integration test passed: Application could successfully start with this configuration")
}
