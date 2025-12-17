# Phase 0.1: Core Interfaces - Functional Tests

## Overview

This document describes the functional tests for Phase 0.1 (Core Interfaces) of the Chaperone project. These tests validate that all core interfaces are correctly defined with proper method signatures, documentation, and type compatibility.

## Test File

**Location:** `/Users/bmf/code/chaperone-auth-gateway/test/interfaces_test.go`

## What These Tests Validate

### 1. Interface Definitions

The tests use Go's AST (Abstract Syntax Tree) parser to verify that each interface is defined in the correct file with the correct methods:

#### SecretProvider (`internal/secrets/provider.go`)
```go
type SecretProvider interface {
    Fetch(ctx context.Context, ref string) (string, error)
}
```

#### AuthStrategy (`internal/auth/strategy.go`)
```go
type AuthStrategy interface {
    Apply(ctx context.Context, req *http.Request, secret string) error
}
```

#### ServiceRegistry (`internal/service/registry.go`)
```go
type ServiceRegistry interface {
    Register(service *Service) error
    Lookup(hostname string) (*Service, bool)
    ListAll() []*Service
}
```

#### PolicyEnforcer (`internal/service/policy.go`)
```go
type PolicyEnforcer interface {
    CheckPath(path string, policy *Policy) error
    CheckMethod(method string, policy *Policy) error
    CheckBodySize(size int64, policy *Policy) error
}
```

#### AuditLogger (`internal/audit/logger.go`)
```go
type AuditLogger interface {
    LogRequest(ctx context.Context, entry *RequestLog) error
}
```

### 2. Core Struct Definitions

Tests verify that the following structs are defined:

- **Service** (in `internal/service/registry.go` or `service.go`)
  - Should contain: host pattern, auth strategy reference, credential reference, policy

- **Policy** (in `internal/service/policy.go`)
  - Should contain: allowed methods, allowed paths, max body bytes, client groups

- **RequestLog** (in `internal/audit/logger.go`)
  - Should contain: timestamp, request_id, client_id, service, host, method, path, status, bytes, duration, policy result

### 3. Documentation (Godoc)

Tests verify that all interfaces have complete godoc comments explaining:
- What the interface does
- Concurrency safety considerations
- Error conditions
- Usage patterns

### 4. Type Compatibility

Tests verify that:
- Interfaces can be imported and used in type assertions
- Mock implementations can satisfy the interface contracts
- Method signatures compile correctly
- All necessary packages are imported (context, net/http, etc.)

### 5. Naming Conventions

Tests verify that all interface names:
- Are exported (start with capital letter)
- Follow Go naming conventions (camelCase, no underscores)
- Are descriptive and reasonable length

## Running the Tests

### Run All Interface Tests
```bash
cd /Users/bmf/code/chaperone-auth-gateway
go test -v ./test -run TestCoreInterfaces
```

### Run Specific Test Groups
```bash
# Test interface definitions
go test -v ./test -run TestCoreInterfaces/secret_provider_interface

# Test struct definitions
go test -v ./test -run TestCoreInterfaces/core_structs_defined

# Test documentation
go test -v ./test -run TestCoreInterfaces/interfaces_have_godoc

# Test compilation
go test -v ./test -run TestCoreInterfaces/interface_compilation
```

### Check Phase 0.1 Completion Status
```bash
go test -v ./test -run TestPhase0Completion
```

### Run All Interface-Related Tests
```bash
go test -v ./test -run TestInterface
```

## Expected Test Behavior

### When Interfaces Are NOT Implemented (Current State)

Tests will fail with clear error messages indicating what's missing:

```
--- FAIL: TestCoreInterfaces/secret_provider_interface (0.00s)
    interfaces_test.go:78: internal/secrets/provider.go does not exist - create it with SecretProvider interface

--- FAIL: TestCoreInterfaces/core_structs_defined/Service_struct (0.00s)
    interfaces_test.go:455: Service struct not found - define in internal/service/registry.go or service.go
```

### When Interfaces ARE Implemented Correctly

Tests will pass and provide informative logs:

```
--- PASS: TestCoreInterfaces/secret_provider_interface (0.00s)
    interfaces_test.go:151: SecretProvider interface found with methods: [Fetch]

--- PASS: TestCoreInterfaces/interfaces_have_godoc/SecretProvider (0.00s)
    interfaces_test.go:548: SecretProvider has documentation: 234 characters
```

## Why These Tests Are Un-Gameable

These tests cannot be faked or satisfied with stub implementations because:

1. **Real AST Parsing**: Tests use Go's built-in AST parser to read actual source code files, not runtime values
2. **Method Signature Verification**: Tests check exact parameter counts, types, and return values
3. **Compilation Verification**: Tests attempt to compile actual Go code that imports and uses the interfaces
4. **Documentation Verification**: Tests use `go/doc` to extract real documentation comments from source
5. **Multiple Validation Points**: Each interface is validated through:
   - File existence checks
   - AST parsing and type verification
   - Documentation extraction
   - Compilation tests
   - Import validation

An AI cannot:
- Create files that don't exist
- Make the Go parser see interfaces that aren't defined
- Make the Go compiler accept invalid code
- Generate documentation comments that don't exist in source

## Test Structure

Each test follows this pattern:

```go
func testXXXInterface(t *testing.T, projectRoot string) {
    // 1. Verify file exists
    filePath := filepath.Join(projectRoot, "internal/xxx/file.go")
    if _, err := os.Stat(filePath); os.IsNotExist(err) {
        t.Fatal("file does not exist - create it with XXX interface")
    }

    // 2. Parse the file using Go's AST parser
    fset := token.NewFileSet()
    f, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
    if err != nil {
        t.Fatalf("failed to parse file: %v", err)
    }

    // 3. Find the interface in the AST
    var foundInterface *ast.InterfaceType
    ast.Inspect(f, func(n ast.Node) bool {
        // ... find interface by name
    })

    // 4. Verify interface exists
    if foundInterface == nil {
        t.Fatal("interface not found")
    }

    // 5. Verify methods exist with correct signatures
    // ... detailed method signature checks
}
```

## Integration with Planning

These tests map directly to Phase 0.1 work items from `PLAN-2025-11-26-031437.md`:

- **CHAP-dhf**: Core Interfaces (Phase 0.1)
  - Status: Ready to start (dependencies: Phase 0.7 complete)
  - Priority: High
  - Complexity: Medium

The tests validate the completion criteria for Phase 0.1:
- ✓ All interfaces have complete godoc
- ✓ Concurrency safety documented
- ✓ Error types documented
- ✓ `go build` succeeds (interfaces compile)
- ✓ No implementations yet (tests only check interfaces, not implementations)

## Traceability

### STATUS Gaps Addressed
These tests address gaps from the STATUS report:
- 0% test coverage → Provides interface contract validation
- No interface definitions → Tests require interfaces to exist
- No type safety → Tests verify exact type signatures

### PLAN Items Validated
- **P0: Phase 0.1 (Core Interfaces)**: All tests map to Phase 0.1 requirements
- **Acceptance Criteria**:
  - All interfaces defined with godoc ✓
  - Interfaces compile ✓
  - No implementations yet ✓

## Next Steps

After interfaces are implemented and tests pass:

1. **Phase 0.2**: Error Handling Framework
   - Tests will verify error types work with these interfaces

2. **Phase 0.3**: Observability Foundation
   - Tests will verify logging works with context from these interfaces

3. **Phase 0.5**: Test Infrastructure
   - Will use these interfaces to create test helpers and mocks

4. **Phase 1+**: Implementations
   - Will implement these interfaces
   - Implementation tests will verify behavior (separate from these interface tests)

## Test Maintenance

These tests should:
- **NOT be modified** when implementations are added (they test interfaces, not implementations)
- **BE updated** if interface signatures change (with clear justification)
- **PASS throughout the project** once interfaces are defined
- **SERVE as documentation** for what interfaces exist and what they require

## Example Output

### Current State (Interfaces Not Implemented)
```
=== RUN   TestPhase0Completion/phase_0.1_checklist
    ✗ SecretProvider interface exists
    ✗ AuthStrategy interface exists
    ✗ ServiceRegistry interface exists
    ✗ PolicyEnforcer interface exists
    ✗ AuditLogger interface exists
    ✓ All interfaces have godoc
    ✓ Project builds successfully

    Phase 0.1 Completion Status: 2/7 checks passed

    To complete Phase 0.1, implement the missing interfaces:
      1. Define all interface types with correct method signatures
      2. Define core structs (Service, Policy, RequestLog)
      3. Add godoc comments to all interfaces
      4. Ensure 'go build ./...' succeeds
```

### Target State (Interfaces Implemented)
```
=== RUN   TestPhase0Completion/phase_0.1_checklist
    ✓ SecretProvider interface exists
    ✓ AuthStrategy interface exists
    ✓ ServiceRegistry interface exists
    ✓ PolicyEnforcer interface exists
    ✓ AuditLogger interface exists
    ✓ All interfaces have godoc
    ✓ Project builds successfully

    Phase 0.1 Completion Status: 7/7 checks passed

--- PASS: TestPhase0Completion (0.12s)
```

## Summary

These functional tests provide:
- **Un-gameable validation** of interface contracts
- **Clear failure messages** when interfaces are missing or incorrect
- **Documentation** of what Phase 0.1 requires
- **Traceability** to planning documents
- **Foundation** for future implementation tests

The tests use real Go tooling (AST parser, compiler, doc extractor) to validate real source code, making them impossible to fake or satisfy with stubs.
