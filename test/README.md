# Chaperone Test Suite

This directory contains functional tests that validate the Chaperone project's scaffolding, infrastructure, and implementation.

## Test Categories

### Scaffolding Tests (`scaffolding_test.go`)

**Purpose:** Validate Phase 0.7 (Project Scaffolding) is complete and correct.

**What it tests:**
- Go module initialization (`go.mod` with correct module path)
- Complete directory structure (all required packages and test directories)
- Makefile with all required targets (`build`, `test`, `test-race`, `lint`, `fmt`, `clean`)
- Linting configuration (`.golangci.yml` exists)
- Basic compilation (`go build ./...` succeeds)
- Git ignore configuration
- Project documentation

**Why these tests are un-gameable:**
1. Tests verify actual filesystem structure, not mocks
2. Tests execute real Go toolchain commands
3. Tests run real Makefile targets
4. Tests validate actual compilation with Go compiler
5. An AI cannot satisfy these tests with stubs - the real build infrastructure must work

## Running Tests

### Run all tests:
```bash
cd /Users/bmf/code/chaperone-auth-gateway
go test ./test/...
```

### Run with verbose output:
```bash
go test -v ./test/...
```

### Run specific test:
```bash
go test -v ./test -run TestProjectScaffolding
```

### Run with coverage:
```bash
go test -cover ./test/...
```

## Expected Test Results

### Before Phase 0.7 is complete:
All tests in `scaffolding_test.go` will **FAIL** with errors like:
- "go.mod does not exist"
- "required directory does not exist"
- "Makefile does not exist"
- "cmd/chaperone/main.go does not exist"

**This is expected!** These tests define what Phase 0.7 must deliver.

### After Phase 0.7 is complete:
All tests in `scaffolding_test.go` should **PASS**, indicating:
- ✅ Go module initialized correctly
- ✅ Directory structure complete
- ✅ Makefile targets work
- ✅ Linting configured
- ✅ Project compiles

## Test Philosophy

These tests follow the **functional testing** approach:

1. **Real Execution:** Tests run actual commands (`go build`, `make`, `golangci-lint`)
2. **Observable Results:** Tests verify filesystem state and command outputs
3. **No Mocks:** Tests validate real infrastructure, not test doubles
4. **Un-Gameable:** Tests cannot be satisfied by shortcuts or stubs
5. **User-Centric:** Tests validate what a developer would actually do

## Test Structure

Each test follows this pattern:

```go
func testSomething(t *testing.T, projectRoot string) {
    // SETUP: Define what should exist

    // EXECUTE: Run real commands or check real filesystem

    // VERIFY: Assert on multiple observable outcomes:
    //   - Files exist
    //   - Commands succeed
    //   - Output is correct
    //   - State is valid

    // Report clear errors if anything is wrong
}
```

## Adding New Tests

When adding tests for new phases:

1. **Create test file:** `test/<phase>_test.go`
2. **Follow naming:** `TestPhaseX_<Feature>`
3. **Test real behavior:** No mocks for external systems
4. **Verify multiple outcomes:** File creation, state changes, side effects
5. **Document gaming resistance:** Explain why test can't be faked
6. **Add to this README:** Document what the test validates

## Gaming Resistance

These tests are structured to prevent shortcuts:

❌ **Cannot be satisfied by:**
- Creating empty files
- Stubbing functions
- Mocking the functionality being tested
- Hardcoding expected outputs

✅ **Must be satisfied by:**
- Real Go module with correct path
- Real directory structure with actual files
- Real Makefile that actually works
- Real linting configuration that runs
- Real code that actually compiles

## Troubleshooting

### Test fails: "go.mod does not exist"
**Solution:** Run `go mod init github.com/bmf/chaperone` in project root

### Test fails: "required directory does not exist"
**Solution:** Create missing directories with `mkdir -p <path>`

### Test fails: "Makefile does not exist"
**Solution:** Create Makefile with required targets

### Test fails: ".golangci.yml does not exist"
**Solution:** Create linting configuration file

### Test fails: "cmd/chaperone/main.go does not exist"
**Solution:** Create basic main.go:
```go
package main

import "fmt"

func main() {
    fmt.Println("chaperone")
}
```

### Test fails: "golangci-lint not found"
**Solution:** Install linter: `brew install golangci-lint`

### Test fails: "go build ./... failed"
**Solution:** Fix compilation errors in Go code

## Test Traceability

These tests validate the following planning artifacts:

### STATUS Gaps Addressed:
- Project is greenfield (0% complete) - tests validate starting point

### PLAN Items Validated:
- **CHAP-pmn (Phase 0.7):** Project Scaffolding
  - Go module initialization
  - Directory structure creation
  - Makefile creation
  - Linting configuration
  - Basic compilation

## Success Criteria

Phase 0.7 is complete when:

```bash
cd /Users/bmf/code/chaperone-auth-gateway
go test ./test/... -run TestProjectScaffolding
```

Returns:
```
PASS
ok      github.com/bmf/chaperone/test
```

All sub-tests should pass:
- ✅ go_module_initialized
- ✅ directory_structure_complete
- ✅ makefile_targets_work
- ✅ linting_configured
- ✅ basic_compilation

## Next Steps

After Phase 0.7 scaffolding tests pass:

1. Move to Phase 0.1: Core Interfaces
2. Move to Phase 0.2: Error Handling Framework
3. Move to Phase 0.3: Observability Foundation
4. Continue through Phase 0 before any feature work

Each phase should add its own functional tests that validate real behavior.
