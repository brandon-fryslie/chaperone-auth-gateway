# CLI Tests Documentation

## Overview

This document describes the functional tests for Phase 0.9: CLI Foundation (CHAP-8tw).

## Test File

**Location:** `/Users/bmf/code/chaperone-auth-gateway/test/cli_test.go`

## What These Tests Validate

### 1. Version Command (`TestCLIVersion`)

**User Workflow:** Developer runs `chaperone --version` to check installed version

**Validates:**
- Binary compiles successfully
- `--version` flag is recognized
- Version information is displayed
- Command exits with code 0 (success)

**Anti-Gaming:**
- Executes real compiled binary (not mocked)
- Captures actual stdout output
- Verifies version string present in output

**Cannot be satisfied by:**
- Stub functions returning hardcoded strings
- Mocked CLI framework
- Empty implementations

---

### 2. Help Command (`TestCLIHelp`)

**User Workflow:** Developer runs `chaperone --help` to learn available commands

**Validates:**
- `--help` flag is recognized
- Help output lists all commands (`init`, `run`)
- Help output shows usage information
- Command exits with code 0

**Anti-Gaming:**
- Executes real binary with `--help`
- Parses actual stdout for expected content
- Verifies all commands are documented

**Cannot be satisfied by:**
- Empty help text
- Incomplete command documentation

---

### 3. Init Command (`TestCLIInitCommand`)

**User Workflow:** Developer runs `chaperone init openai` to bootstrap configuration

**Sub-tests:**

#### 3.1: `init_openai`
**Validates:**
- `init openai` creates `chaperone.toml`
- File is created in current directory
- File contains OpenAI-specific configuration
- File is not empty

#### 3.2: `init_anthropic`
**Validates:**
- `init anthropic` creates `chaperone.toml`
- File contains Anthropic-specific configuration

#### 3.3: `init_without_service_shows_error`
**Validates:**
- Running `init` without service argument fails
- Error message is displayed
- Non-zero exit code

#### 3.4: `init_invalid_service_shows_error`
**Validates:**
- Running `init invalidservice` fails
- Error message mentions invalid service
- Non-zero exit code

#### 3.5: `init_creates_valid_toml`
**Validates:**
- Generated config is valid TOML syntax
- Contains section headers `[...]`
- Contains key=value pairs

**Anti-Gaming:**
- Creates temporary directory for each test
- Verifies actual file created on filesystem
- Reads file content and validates structure
- Uses real filesystem I/O

**Cannot be satisfied by:**
- Printing to stdout instead of creating file
- Creating empty file
- Creating invalid TOML

---

### 4. Run Command (`TestCLIRunCommand`)

**User Workflow:** Developer runs `chaperone run` to start the proxy server

**Sub-tests:**

#### 4.1: `run_loads_config`
**Validates:**
- `run` command accepts `--config` flag
- Command attempts to load configuration file
- (Server may not be implemented - testing CLI framework only)

#### 4.2: `run_without_config_shows_error`
**Validates:**
- Running `run` without config file fails
- Error message is displayed
- Non-zero exit code

#### 4.3: `run_with_invalid_config_shows_error`
**Validates:**
- Running `run` with malformed TOML fails
- Error mentions config or parsing problem
- Non-zero exit code

#### 4.4: `run_can_be_interrupted`
**Validates:**
- `run` command responds to SIGTERM/SIGINT
- Graceful shutdown occurs (uses Phase 0.8 shutdown manager)
- Process exits cleanly

**Anti-Gaming:**
- Creates real config files
- Executes real binary
- Tests timeout and signal handling
- Cannot fake graceful shutdown

**Cannot be satisfied by:**
- Ignoring config file
- Not implementing signal handling
- Immediate exit without cleanup

---

### 5. Template Validation (`TestCLITemplates`)

**User Workflow:** Developer expects templates to generate valid, service-specific configs

**Sub-tests:**

#### 5.1: `openai_template_works`
**Validates:**
- OpenAI template generates valid config
- Config references `openai` or `api.openai.com`
- Template produces working configuration

#### 5.2: `anthropic_template_works`
**Validates:**
- Anthropic template generates valid config
- Config references `anthropic` or `api.anthropic.com`
- Template produces working configuration

**Anti-Gaming:**
- Tests actual template output
- Verifies service-specific content
- Cannot use generic template for all services

---

### 6. Integration Scenario (`TestCLIIntegrationScenario`)

**User Workflow:** Complete workflow from help to running server

**Validates:**
1. `chaperone --help` succeeds
2. `chaperone init openai` creates config
3. Config file exists and is readable
4. `chaperone run` starts with generated config

**Anti-Gaming:**
- Executes commands in sequence
- Verifies state after each step
- Tests realistic user interaction
- All steps must succeed

**Cannot be satisfied by:**
- Skipping steps
- Not creating real files
- Not actually starting server

---

### 7. Edge Cases (`TestCLIEdgeCases`)

**Sub-tests:**

#### 7.1: `invalid_command_shows_error`
**Validates:**
- Unknown commands fail with error
- Error message is helpful

#### 7.2: `run_with_nonexistent_config_file`
**Validates:**
- Error when config file doesn't exist
- Clear error message

#### 7.3: `init_in_directory_with_existing_config`
**Validates:**
- Behavior when config already exists
- (Implementation-defined: error, overwrite, or backup)

**Anti-Gaming:**
- Tests error conditions
- Verifies proper error handling
- Cannot succeed by ignoring errors

---

### 8. Phase Completion Meta-Test (`TestPhase09Completion`)

**Purpose:** Validates all Phase 0.9 acceptance criteria are met

**Checks:**
1. ✓ Binary builds successfully
2. ✓ `chaperone --version` works
3. ✓ `chaperone --help` works
4. ✓ `chaperone init openai` creates config
5. ✓ `chaperone init anthropic` creates config
6. ✓ `chaperone run` loads config

**Fails if:**
- Any check does not pass
- Provides clear instructions for fixing

**Output:** Summary of passed/failed checks with actionable guidance

---

## Running the Tests

### Run all CLI tests:
```bash
cd /Users/bmf/code/chaperone-auth-gateway
go test ./test -run TestCLI -v
```

### Run specific test:
```bash
go test ./test -run TestCLIVersion -v
go test ./test -run TestCLIInitCommand -v
go test ./test -run TestPhase09Completion -v
```

### Run with race detector:
```bash
go test -race ./test -run TestCLI -v
```

---

## Expected Test Behavior

### BEFORE Implementation (Tests Fail):

All tests will **FAIL** with errors like:
- "cannot build chaperone binary" (cobra not installed)
- "unknown flag: --version" (CLI not implemented)
- "unknown command: init" (init command not implemented)
- "unknown command: run" (run command not implemented)

**This is expected!** These tests define what must be implemented.

### AFTER Implementation (Tests Pass):

All tests should **PASS**, indicating:
- ✅ CLI framework (cobra) is installed and configured
- ✅ Version and help flags work
- ✅ Init command creates configs from templates
- ✅ Run command loads configuration
- ✅ Error handling is appropriate
- ✅ Graceful shutdown works

---

## Implementation Requirements

To make these tests pass, implement:

### 1. Install Cobra
```bash
go get github.com/spf13/cobra
```

### 2. Create Root Command (`cmd/chaperone/main.go`)
- Initialize cobra root command
- Add `--version` flag
- Add `--help` flag (automatic with cobra)

### 3. Create Init Command (`cmd/chaperone/cmd/init.go`)
- Accept service name argument: `openai`, `anthropic`
- Read template from `templates/<service>.toml` or use embed
- Write template to `chaperone.toml` in current directory
- Handle errors: missing argument, invalid service, file exists

### 4. Create Run Command (`cmd/chaperone/cmd/run.go`)
- Accept `--config` flag (default: `chaperone.toml`)
- Load config using `internal/config.Load()`
- Set defaults: `config.SetDefaults()`
- Validate: `config.Validate()`
- Create logger using `internal/log`
- Create shutdown manager using `internal/shutdown`
- Register shutdown handlers
- (Server implementation not required for Phase 0.9)
- Wait for shutdown signal

### 5. Create Templates
- `templates/openai.toml` - OpenAI API configuration
- `templates/anthropic.toml` - Anthropic API configuration
- Templates should be valid TOML
- Templates should include comments for user guidance

---

## Test Traceability

### STATUS Gaps Addressed:
- CLI is not implemented (Phase 0.9 required before Phase 1)
- No user-facing interface for configuration

### PLAN Items Validated:
- **CHAP-8tw (Phase 0.9):** CLI Foundation
  - Version and help commands
  - Init command with templates
  - Run command with config loading
  - Integration with Phase 0 components (logging, config, shutdown)

---

## Anti-Gaming Architecture

These tests are structured to prevent shortcuts:

### ❌ Cannot Be Satisfied By:

1. **Printing instead of creating files**
   - Tests verify files exist on filesystem

2. **Stub implementations**
   - Tests execute real compiled binary
   - Tests verify actual command execution

3. **Hardcoded outputs**
   - Tests verify behavior with different inputs
   - Tests verify file content matches expectations

4. **Ignoring errors**
   - Tests verify error conditions fail appropriately
   - Tests verify error messages are present

5. **Mocking the CLI framework**
   - Tests build and execute real binary
   - Tests cannot pass without real cobra implementation

### ✅ Must Be Satisfied By:

1. **Real binary compilation**
   - Uses `go build` to create actual executable

2. **Real file I/O**
   - Creates files on filesystem
   - Reads and validates file content

3. **Real command execution**
   - Uses `exec.Command()` to run binary
   - Captures stdout/stderr
   - Verifies exit codes

4. **Real template processing**
   - Templates exist and are valid TOML
   - Generated configs are usable

5. **Real integration with Phase 0**
   - Uses `internal/config` for loading
   - Uses `internal/log` for logging
   - Uses `internal/shutdown` for graceful shutdown

---

## Success Criteria

Phase 0.9 is complete when:

```bash
cd /Users/bmf/code/chaperone-auth-gateway
go test ./test -run TestPhase09Completion -v
```

Returns:
```
✓ chaperone binary builds successfully
✓ chaperone --version works
✓ chaperone --help works
✓ chaperone init openai creates config
✓ chaperone init anthropic creates config
✓ chaperone run loads config

Phase 0.9 Completion Status: 6/6 checks passed

✓✓✓ PASS: Phase 0.9 CLI Foundation is COMPLETE ✓✓✓
PASS
```

---

## Next Steps After Phase 0.9

Once all CLI tests pass:

1. ✅ Phase 0.9 (CLI Foundation) - COMPLETE
2. → **Phase 1:** Basic Proxy Server
   - Implement HTTP proxy server
   - Implement CONNECT tunnel handler
   - Integration tests with real HTTPS traffic

---

## Troubleshooting

### Test fails: "cannot build chaperone binary"
**Solution:** Ensure `cmd/chaperone/main.go` compiles:
```bash
go build ./cmd/chaperone
```

### Test fails: "unknown flag: --version"
**Solution:** Add version flag to root command:
```go
rootCmd.Flags().BoolP("version", "v", false, "Print version information")
```

### Test fails: "unknown command: init"
**Solution:** Create init command and register with root command

### Test fails: "chaperone init openai' should create chaperone.toml"
**Solution:** Implement file writing in init command:
```go
template, err := os.ReadFile("templates/openai.toml")
os.WriteFile("chaperone.toml", template, 0644)
```

### Test fails: "cobra not installed"
**Solution:**
```bash
go get github.com/spf13/cobra
go mod tidy
```

---

## Test Maintenance

When adding new commands:

1. Add test function to `cli_test.go`
2. Follow existing test patterns
3. Test both success and error cases
4. Add to `TestPhase09Completion` checks
5. Document in this README

When modifying commands:

1. Update affected tests
2. Ensure backward compatibility tests still pass
3. Add new tests for new features
4. Update this documentation

---

## Quality Metrics

**Test Coverage Target:** 80%+ of CLI code

**Coverage Areas:**
- ✅ Command execution
- ✅ Flag parsing
- ✅ File I/O operations
- ✅ Error handling
- ✅ Integration with Phase 0 components
- ✅ User workflows

**Test Count:** 8 major test functions, 20+ sub-tests

**Execution Time:** ~5-10 seconds (includes binary compilation)

---

## Conclusion

These tests validate that the CLI provides a functional, user-friendly interface to Chaperone. They enforce real behavior through binary execution, file I/O verification, and integration validation. Tests cannot be satisfied by stubs or shortcuts - the CLI must actually work as a real user would expect.

**The tests are the specification. Make them pass, and Phase 0.9 is complete.**
