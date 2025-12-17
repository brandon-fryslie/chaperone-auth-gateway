# CLI Test Suite - Phase 0.9 Summary

## Deliverables

### Test File
**File:** `/Users/bmf/code/chaperone-auth-gateway/test/cli_test.go`
**Lines:** 956 lines
**Test Functions:** 8 major test functions
**Sub-tests:** 20+ individual test cases

### Template Files
1. `/Users/bmf/code/chaperone-auth-gateway/templates/openai.toml` - OpenAI API template
2. `/Users/bmf/code/chaperone-auth-gateway/templates/anthropic.toml` - Anthropic API template

### Documentation
1. `/Users/bmf/code/chaperone-auth-gateway/test/CLI_TESTS_README.md` - Comprehensive test documentation
2. This summary file

---

## Test Coverage

### 1. Version Command
- **Test:** `TestCLIVersion`
- **Validates:** `chaperone --version` displays version information
- **Current Status:** ❌ FAILING (implementation needed)

### 2. Help Command
- **Test:** `TestCLIHelp`
- **Validates:** `chaperone --help` shows command documentation
- **Current Status:** ❌ FAILING (implementation needed)

### 3. Init Command
- **Test:** `TestCLIInitCommand` (5 sub-tests)
- **Validates:**
  - `chaperone init openai` creates valid config
  - `chaperone init anthropic` creates valid config
  - Error handling for missing/invalid arguments
  - Generated files are valid TOML
- **Current Status:** ❌ FAILING (implementation needed)

### 4. Run Command
- **Test:** `TestCLIRunCommand` (4 sub-tests)
- **Validates:**
  - `chaperone run` loads configuration
  - Error handling for missing/invalid config
  - Graceful shutdown on signals
- **Current Status:** ❌ FAILING (implementation needed)

### 5. Template Validation
- **Test:** `TestCLITemplates` (2 sub-tests)
- **Validates:**
  - Templates exist for openai and anthropic
  - Templates generate valid, service-specific configs
- **Current Status:** ✅ Templates exist, tests ready

### 6. Integration Scenario
- **Test:** `TestCLIIntegrationScenario`
- **Validates:** Complete user workflow from help → init → run
- **Current Status:** ❌ FAILING (implementation needed)

### 7. Edge Cases
- **Test:** `TestCLIEdgeCases` (3 sub-tests)
- **Validates:** Error handling for invalid inputs
- **Current Status:** ❌ FAILING (implementation needed)

### 8. Phase Completion Meta-Test
- **Test:** `TestPhase09Completion`
- **Validates:** All acceptance criteria met
- **Current Status:** ❌ 0/6 checks passing

---

## Running the Tests

### All CLI tests:
```bash
cd /Users/bmf/code/chaperone-auth-gateway
go test ./test -run TestCLI -v
```

### Completion meta-test:
```bash
go test ./test -run TestPhase09Completion -v
```

### Specific test:
```bash
go test ./test -run TestCLIVersion -v
go test ./test -run TestCLIInitCommand -v
```

---

## Test Execution Output

Current output from meta-test:

```
✗ chaperone binary builds successfully: exit status 1
✗ chaperone --version works: exit status 1
✗ chaperone --help works: exit status 1
✗ chaperone init openai creates config: exit status 1
✗ chaperone init anthropic creates config: exit status 1
✗ chaperone run loads config: exit status 1

Phase 0.9 Completion Status: 0/6 checks passed
```

**This is expected!** Tests define the behavior that needs to be implemented.

---

## What Each Test Validates (Gaming Resistance)

### TestCLIVersion
**Cannot be satisfied by:**
- Printing hardcoded string to stdout (test expects command to parse --version flag)
- Empty implementation (test verifies version info present)

**Must be satisfied by:**
- Real CLI framework with flag parsing
- Actual version information display

### TestCLIHelp
**Cannot be satisfied by:**
- Empty help text (test checks for command names)
- Incomplete documentation (test verifies all commands listed)

**Must be satisfied by:**
- Real help generation (cobra automatic)
- Documentation for all commands

### TestCLIInitCommand
**Cannot be satisfied by:**
- Creating empty files (test reads and validates content)
- Generic template for all services (test checks service-specific content)
- Printing to stdout (test verifies file exists on filesystem)

**Must be satisfied by:**
- Real file I/O creating chaperone.toml
- Service-specific templates
- Valid TOML generation

### TestCLIRunCommand
**Cannot be satisfied by:**
- Ignoring config file (test verifies config loading attempted)
- Not handling signals (test sends SIGTERM and waits for shutdown)
- Fake config validation (uses real internal/config package)

**Must be satisfied by:**
- Real config loading using Phase 0.4 config package
- Real signal handling using Phase 0.8 shutdown manager
- Actual error messages for invalid configs

### TestCLITemplates
**Cannot be satisfied by:**
- Same template for all services (test checks service-specific references)
- Invalid TOML (test verifies structure)

**Must be satisfied by:**
- Separate templates for openai and anthropic
- Templates containing service-specific configuration

### TestCLIIntegrationScenario
**Cannot be satisfied by:**
- Skipping steps (test executes all steps in sequence)
- Not creating files (test verifies files exist after each step)

**Must be satisfied by:**
- Complete workflow from help → init → run
- Real file creation and command execution

---

## Implementation Guidance

To make tests pass:

### Step 1: Install Cobra
```bash
go get github.com/spf13/cobra@latest
go mod tidy
```

### Step 2: Create CLI Structure
```
cmd/chaperone/
├── main.go           # Entry point, initializes root command
├── cmd/
│   ├── root.go       # Root command with --version, --help
│   ├── init.go       # Init command implementation
│   └── run.go        # Run command implementation
```

### Step 3: Implement Root Command (cmd/chaperone/cmd/root.go)
```go
var rootCmd = &cobra.Command{
    Use:   "chaperone",
    Short: "Chaperone authentication gateway",
}

func Execute() {
    if err := rootCmd.Execute(); err != nil {
        os.Exit(1)
    }
}

func init() {
    rootCmd.Version = "0.9.0"
}
```

### Step 4: Implement Init Command (cmd/chaperone/cmd/init.go)
```go
var initCmd = &cobra.Command{
    Use:   "init [service]",
    Short: "Initialize configuration from template",
    Args:  cobra.ExactArgs(1),
    RunE:  runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
    service := args[0]
    // Read template from templates/<service>.toml
    // Write to chaperone.toml
    // Handle errors
}
```

### Step 5: Implement Run Command (cmd/chaperone/cmd/run.go)
```go
var runCmd = &cobra.Command{
    Use:   "run",
    Short: "Start the proxy server",
    RunE:  runServer,
}

var configFile string

func init() {
    runCmd.Flags().StringVar(&configFile, "config", "chaperone.toml", "Config file path")
}

func runServer(cmd *cobra.Command, args []string) error {
    // Load config using internal/config.Load(configFile)
    // Create logger using internal/log
    // Create shutdown manager using internal/shutdown
    // (Server not implemented yet - just load and validate config)
}
```

### Step 6: Update main.go
```go
package main

import "github.com/bmf/chaperone/cmd/chaperone/cmd"

func main() {
    cmd.Execute()
}
```

---

## Success Criteria (from CHAP-8tw)

Phase 0.9 is complete when all these work:

- [x] ✅ Templates exist for openai and anthropic
- [ ] ❌ `chaperone --version` works
- [ ] ❌ `chaperone --help` shows usage
- [ ] ❌ `chaperone init openai` creates config file
- [ ] ❌ `chaperone run` loads config successfully
- [ ] ❌ Test coverage >= 80%

---

## Test Patterns Used

### Pattern 1: Binary Execution Testing
```go
// Build real binary
binaryPath := buildChaperoneBinary(t)

// Execute real command
cmd := exec.Command(binaryPath, "--version")
output, _ := cmd.Output()

// Verify real output
assert.Contains(t, string(output), "version")
```

### Pattern 2: Filesystem Verification
```go
// Create temp directory
tmpDir := t.TempDir()

// Execute command that should create file
cmd := exec.Command(binary, "init", "openai")
cmd.Dir = tmpDir
cmd.Run()

// Verify file exists on real filesystem
assert.FileExists(t, filepath.Join(tmpDir, "chaperone.toml"))

// Verify file content
content, _ := os.ReadFile(configPath)
assert.Contains(t, string(content), "openai")
```

### Pattern 3: Signal Handling Testing
```go
// Start long-running command
cmd := exec.Command(binary, "run")
cmd.Start()

// Send real signal
cmd.Process.Signal(os.Interrupt)

// Verify graceful shutdown
select {
case <-done:
    // Clean shutdown
case <-timeout:
    t.Fatal("Did not respond to signal")
}
```

---

## Traceability

### STATUS Gaps Addressed
- CLI is not implemented (greenfield project)
- No user-facing interface

### PLAN Items Validated
- **CHAP-8tw (Phase 0.9):** CLI Foundation
  - ✅ Version and help commands defined in tests
  - ✅ Init command with templates defined in tests
  - ✅ Run command with config loading defined in tests
  - ✅ Integration with Phase 0 components tested

---

## Next Steps

1. **Implement CLI** following guidance above
2. **Run tests** to verify implementation
3. **Iterate** until all tests pass
4. **Verify completion** with `go test ./test -run TestPhase09Completion`
5. **Proceed to Phase 1** (Basic Proxy Server)

---

## Summary

**Tests Added:** 8 test functions, 20+ sub-tests
**Workflows Covered:**
- Version display
- Help documentation
- Configuration initialization (OpenAI, Anthropic)
- Server startup with config loading
- Error handling for invalid inputs
- Signal-based graceful shutdown

**Initial Status:** All tests failing (expected - no implementation yet)
**Gaming Resistance:** HIGH - tests execute real binary, verify filesystem, validate output
**Commit:** Tests ready for implementation phase

**The tests define the contract. Implement to make them pass.**
