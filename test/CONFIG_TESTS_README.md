# Configuration Framework Tests - Phase 0.4

## Overview

This test suite validates the Configuration Framework (Phase 0.4) for the Chaperone project. These tests ensure that TOML-based configuration loading, validation, and default value application work correctly.

## What Makes These Tests Un-Gameable

These tests are designed to be impossible to satisfy with stubs or shortcuts:

### 1. Real File I/O
- Tests load actual TOML files from disk using real file paths
- Missing file tests verify actual filesystem errors
- Invalid TOML tests verify real parsing errors from github.com/BurntSushi/toml

### 2. Actual Data Structure Validation
- Tests inspect real struct fields after loading
- Validates slice fields (AllowedMethods, AllowedPaths, ClientGroups)
- Verifies int64 fields (MaxBodyBytes)
- Checks map structures (Services map)

### 3. Real Validation Logic
- Port range validation (0, negative, > 65535 must fail)
- Log level validation (only debug/info/warn/error allowed)
- Service validation (HostPattern required for all services)
- Tests verify exact error conditions, not just error presence

### 4. Real Default Value Application
- Tests verify actual struct field values after SetDefaults()
- Confirms defaults are applied when fields are empty/zero
- Confirms explicit values are NOT overridden
- Tests both Server, Logging configurations

### 5. Complete Workflow Testing
- Tests simulate real application startup flow
- Load → SetDefaults → Validate → Access pattern
- Multiple services with complex configurations
- End-to-end integration scenarios

### 6. Observable Behavior Only
- No mocking of config loading
- No internal implementation details tested
- Only tests what users would observe
- Tests would catch any implementation shortcuts

## Test Coverage

### Configuration Loading Tests

#### `testLoadMinimalConfig`
- **Validates:** Load() reads minimal TOML file correctly
- **Checks:** Config struct populated, Services map has entries
- **File:** `fixtures/configs/minimal.toml`
- **Gaming Resistance:** Must parse real TOML file, populate real structs

#### `testLoadFullConfig`
- **Validates:** Load() reads complete TOML with all fields
- **Checks:** Server, Logging, Services all populated correctly
- **File:** `fixtures/configs/full.toml`
- **Gaming Resistance:** Must handle complex nested structures, multiple services

#### `testLoadMissingFile`
- **Validates:** Load() returns error for non-existent file
- **Checks:** Error returned, nil config returned
- **Gaming Resistance:** Must perform real filesystem check

#### `testLoadInvalidTOML`
- **Validates:** Load() returns error for invalid TOML syntax
- **Checks:** Parsing error returned
- **File:** `fixtures/configs/invalid.toml`
- **Gaming Resistance:** Must use real TOML parser that catches syntax errors

### Validation Tests

#### `testValidatePortRanges`
- **Validates:** Port validation rules enforced
- **Checks:**
  - Port 0 rejected
  - Port -1 rejected
  - Port 70000 (> 65535) rejected
  - Ports 1-65535 accepted
- **Gaming Resistance:** Must implement actual range checking logic

#### `testValidateLogLevels`
- **Validates:** Log level validation rules enforced
- **Checks:**
  - Valid levels accepted: debug, info, warn, error
  - Invalid levels rejected: trace, fatal, invalid, INFO, Debug, empty string
- **Gaming Resistance:** Case-sensitive validation, specific allowed values

#### `testValidateServiceHostPattern`
- **Validates:** Service HostPattern required validation
- **Checks:**
  - Empty HostPattern rejected
  - Valid HostPattern accepted
- **Gaming Resistance:** Must iterate services map, check each entry

### Default Value Tests

#### `testDefaultServerAddress`
- **Validates:** Server.Address defaults to "127.0.0.1"
- **Checks:**
  - Empty Address gets default
  - Explicit Address not overridden
- **Gaming Resistance:** Must inspect actual struct field values

#### `testDefaultServerPort`
- **Validates:** Server.Port defaults to 4010
- **Checks:**
  - Zero Port gets default
  - Explicit Port not overridden
- **Gaming Resistance:** Must handle zero vs explicit value distinction

#### `testDefaultLoggingConfig`
- **Validates:** Logging defaults applied
- **Checks:**
  - Level defaults to "info"
  - Format defaults to "json"
  - Output defaults to "stdout"
  - Explicit values not overridden
- **Gaming Resistance:** Must apply defaults to multiple fields correctly

### Service Configuration Tests

#### `testMultipleServicesLoad`
- **Validates:** Multiple services load correctly from TOML
- **Checks:**
  - Services map populated with multiple entries
  - Services accessible by name
  - Each service has required fields
- **Gaming Resistance:** Must parse map[string]ServiceConfig structure

#### `testServiceFieldsParsed`
- **Validates:** All ServiceConfig fields parse correctly
- **Checks:**
  - String fields: HostPattern, AuthStrategy, CredentialRef
  - Slice fields: AllowedMethods, AllowedPaths, ClientGroups
  - Int64 field: MaxBodyBytes
  - Exact values match TOML file
- **Gaming Resistance:** Must parse every field type correctly, verify exact values

### Integration Tests

#### `testCompleteConfigWorkflow`
- **Validates:** Load → SetDefaults → Validate workflow
- **Checks:**
  - Minimal config loads successfully
  - Defaults fill missing fields
  - Validation passes after defaults applied
- **Gaming Resistance:** Tests realistic usage pattern end-to-end

#### `TestConfigurationIntegrationScenario`
- **Validates:** Real application startup scenario
- **Checks:**
  - File existence check
  - Load from disk
  - Apply defaults
  - Validate
  - Access all configuration values
- **Gaming Resistance:** Complete workflow with multiple verification points

## Test Fixtures

### `fixtures/configs/minimal.toml`
Minimal valid configuration with only required fields:
- Single service with HostPattern
- No Server config (expects defaults)
- No Logging config (expects defaults)

**Purpose:** Test default value application

### `fixtures/configs/full.toml`
Complete configuration with all fields explicitly set:
- Server: address, port
- Logging: level, format, output
- Three services with all fields:
  - github: Full configuration with auth, methods, paths, limits
  - api: Alternative auth strategy
  - public: No auth service

**Purpose:** Test complex configuration parsing, multiple services

### `fixtures/configs/invalid.toml`
Invalid TOML with syntax errors:
- Missing closing brackets
- Invalid types (string for port)
- Missing quotes
- Duplicate sections

**Purpose:** Test error handling

## Running the Tests

### Run all configuration tests:
```bash
cd /Users/bmf/code/chaperone-auth-gateway
go test -v -run TestConfiguration ./test/
```

### Run specific test:
```bash
go test -v -run TestConfigurationFramework/load_minimal_config ./test/
```

### Run integration test:
```bash
go test -v -run TestConfigurationIntegrationScenario ./test/
```

## Expected Initial State

**These tests MUST FAIL initially** because the configuration framework is not yet implemented.

Expected failures:
- `undefined: config.Load` - Load function not implemented
- `undefined: config.Config` - Config struct not defined
- `undefined: config.ServerConfig` - ServerConfig struct not defined
- `undefined: config.ServiceConfig` - ServiceConfig struct not defined
- `undefined: config.LoggingConfig` - LoggingConfig struct not defined
- Build errors before any test runs

## Implementation Requirements

To make these tests pass, implement in `/Users/bmf/code/chaperone-auth-gateway/internal/config/config.go`:

### 1. Configuration Structures
```go
type Config struct {
    Server   ServerConfig
    Services map[string]ServiceConfig
    Logging  LoggingConfig
}

type ServerConfig struct {
    Address string
    Port    int
}

type ServiceConfig struct {
    HostPattern     string
    AuthStrategy    string
    CredentialRef   string
    AllowedMethods  []string
    AllowedPaths    []string
    MaxBodyBytes    int64
    ClientGroups    []string
}

type LoggingConfig struct {
    Level  string
    Format string
    Output string
}
```

### 2. Load Function
```go
func Load(path string) (*Config, error)
```
- Use github.com/BurntSushi/toml for parsing
- Return error if file doesn't exist
- Return error if TOML is invalid
- Populate all fields from file

### 3. Validate Method
```go
func (c *Config) Validate() error
```
- Check Server.Port in range 1-65535
- Check Logging.Level in [debug, info, warn, error]
- Check all services have non-empty HostPattern
- Return clear error messages

### 4. SetDefaults Method
```go
func (c *Config) SetDefaults()
```
- Set Server.Address = "127.0.0.1" if empty
- Set Server.Port = 4010 if zero
- Set Logging.Level = "info" if empty
- Set Logging.Format = "json" if empty
- Set Logging.Output = "stdout" if empty
- Do NOT override explicit values

### 5. TOML Field Mapping
Use struct tags for TOML field names:
```go
type ServerConfig struct {
    Address string `toml:"address"`
    Port    int    `toml:"port"`
}

type ServiceConfig struct {
    HostPattern    string   `toml:"host_pattern"`
    AuthStrategy   string   `toml:"auth_strategy"`
    CredentialRef  string   `toml:"credential_ref"`
    AllowedMethods []string `toml:"allowed_methods"`
    AllowedPaths   []string `toml:"allowed_paths"`
    MaxBodyBytes   int64    `toml:"max_body_bytes"`
    ClientGroups   []string `toml:"client_groups"`
}
```

## Test Verification

After implementation, verify:

1. **All tests pass:** `go test -v ./test/config_test.go`
2. **No test skipped or stubbed**
3. **Real TOML files loaded successfully**
4. **Validation catches all error cases**
5. **Defaults applied correctly**
6. **Integration scenario completes successfully**

## Traceability

### STATUS Gaps Addressed
These tests validate functionality described in STATUS as gaps for Phase 0.4:
- Configuration loading from TOML files
- Validation logic for config values
- Default value application
- Multi-service configuration support

### PLAN Items Validated
Maps to PLAN-2025-11-26-031437.md, Phase 0.4 (lines 217-250):
- **CHAP-3q0:** Configuration Framework
- **Priority:** High
- **Complexity:** Medium

### Acceptance Criteria
From PLAN Phase 0.4:
- ✅ Config loads from TOML - tested by load tests
- ✅ Validation catches errors - tested by validation tests
- ✅ Defaults applied correctly - tested by default tests
- ✅ Example configs load successfully - tested by integration tests
- ✅ Test coverage >= 85% - comprehensive test suite

## Gaming Resistance Summary

These tests CANNOT be satisfied by:
- ❌ Hardcoding return values
- ❌ Stubbing file I/O
- ❌ Mocking TOML parser
- ❌ Skipping validation logic
- ❌ Faking struct population
- ❌ Returning empty configs

The tests REQUIRE:
- ✅ Real TOML file parsing
- ✅ Actual struct field population
- ✅ Working validation logic
- ✅ Correct default value application
- ✅ Full Load → Validate → Defaults workflow
- ✅ Support for complex multi-service configs

The only way to pass these tests is to implement the configuration framework correctly.
