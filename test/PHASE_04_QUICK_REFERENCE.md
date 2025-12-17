# Phase 0.4 Configuration Framework - Quick Reference

## Test Files

### Main Test File
`/Users/bmf/code/chaperone-auth-gateway/test/config_test.go` (657 lines)

### Test Fixtures
- `/Users/bmf/code/chaperone-auth-gateway/test/fixtures/configs/minimal.toml`
- `/Users/bmf/code/chaperone-auth-gateway/test/fixtures/configs/full.toml`
- `/Users/bmf/code/chaperone-auth-gateway/test/fixtures/configs/invalid.toml`

### Documentation
- `/Users/bmf/code/chaperone-auth-gateway/test/CONFIG_TESTS_README.md` - Comprehensive test documentation
- `/Users/bmf/code/chaperone-auth-gateway/test/CONFIG_TEST_VERIFICATION.md` - Verification results
- `/Users/bmf/code/chaperone-auth-gateway/test/config_tests_summary.json` - Machine-readable summary

## Run Tests

```bash
cd /Users/bmf/code/chaperone-auth-gateway

# Run all configuration tests
go test -v -run TestConfiguration ./test/

# Run specific test
go test -v -run TestConfigurationFramework/load_minimal_config ./test/

# Run integration test
go test -v -run TestConfigurationIntegrationScenario ./test/
```

## Test Coverage

**14 Tests Total:**
- 4 Loading tests (minimal, full, missing, invalid)
- 3 Validation tests (ports, log levels, host patterns)
- 3 Default value tests (address, port, logging)
- 2 Service configuration tests (multiple services, field parsing)
- 2 Integration tests (workflow, scenario)

## Implementation Checklist

Create `/Users/bmf/code/chaperone-auth-gateway/internal/config/config.go`:

- [ ] Define `type Config struct`
- [ ] Define `type ServerConfig struct`
- [ ] Define `type ServiceConfig struct`
- [ ] Define `type LoggingConfig struct`
- [ ] Implement `func Load(path string) (*Config, error)`
- [ ] Implement `func (*Config) Validate() error`
- [ ] Implement `func (*Config) SetDefaults()`
- [ ] Add `github.com/BurntSushi/toml` dependency

## Current Status

✅ Tests written (657 lines)
✅ Fixtures created (3 files)
✅ Documentation complete
❌ Implementation not started (expected)
🔴 Tests failing with build errors (expected)

## What Makes These Tests Un-Gameable

1. Real file I/O - loads actual TOML files from disk
2. Real parsing - uses github.com/BurntSushi/toml library
3. Real validation - tests specific error conditions
4. Real defaults - verifies exact struct field values
5. Real workflow - complete Load → Defaults → Validate
6. Zero mocks/stubs - only real operations

## Key Test Scenarios

### Configuration Loading
```go
cfg, err := config.Load("fixtures/configs/minimal.toml")
// Must parse real TOML file
// Must populate actual Config struct
```

### Validation
```go
cfg.Server.Port = 0
err := cfg.Validate()
// Must return error for invalid port
```

### Defaults
```go
cfg := &config.Config{}
cfg.SetDefaults()
// Must set Server.Port = 4010
// Must set Server.Address = "127.0.0.1"
// Must set Logging.Level = "info"
```

### Integration
```go
cfg, _ := config.Load(path)  // Load from disk
cfg.SetDefaults()            // Apply defaults
cfg.Validate()               // Validate config
// Complete workflow must work
```

## Next Steps

1. Add TOML dependency: `go get github.com/BurntSushi/toml`
2. Create `internal/config/config.go`
3. Define all required types
4. Implement Load() with TOML parsing
5. Implement Validate() with rules
6. Implement SetDefaults() with defaults
7. Run tests until all pass
