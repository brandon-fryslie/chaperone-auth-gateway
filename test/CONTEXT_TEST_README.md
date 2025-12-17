# Phase 0.6: Context Propagation - Test Documentation

## Overview

This document describes the functional tests for Phase 0.6: Context Propagation in the Chaperone project.

**Test File:** `/Users/bmf/code/chaperone-auth-gateway/test/context_test.go`
**Implementation Package:** `github.com/bmf/chaperone/internal/context`
**Lines of Test Code:** 1095
**Number of Test Cases:** 15
**Test Functions:** 3 (TestContextPropagation, TestContextPatterns, TestPhase06Completion)

## What These Tests Validate

### 1. Context Creation
- `NewRequestContext()` creates a cancellable context
- Returns both context and cancel function
- Context can be cancelled and signals Done channel
- Context reports proper error after cancellation

### 2. Value Storage and Retrieval
All value types support round-trip storage and retrieval:

- **Request ID** (`WithRequestID`, `RequestID`)
  - Stores unique request identifier
  - Empty context returns empty string
  - Multiple contexts can have different IDs

- **Service Name** (`WithService`, `Service`)
  - Stores service name for routing
  - Empty context returns empty string
  - Service names are isolated per context

- **Hostname** (`WithHostname`, `Hostname`)
  - Stores target hostname
  - Empty context returns empty string
  - Hostnames are isolated per context

- **Client ID** (`WithClientID`, `ClientID`)
  - Stores client identifier
  - Empty context returns empty string
  - Client IDs are isolated per context

### 3. Multiple Values on Same Context
- All four value types can coexist on a single context
- Setting values in different orders produces same result
- All values remain accessible after being set
- Values don't interfere with each other

### 4. Missing Values
- Retrieving values from empty context returns empty strings
- No panics occur when retrieving missing values
- Partial contexts (some values set) work correctly

### 5. Context Chaining
- Values propagate from parent to child contexts
- Standard library context derivation (WithTimeout, WithCancel) preserves values
- Multi-level derivation maintains all ancestor values
- Child contexts can add additional values
- Parent contexts are not modified by child additions

### 6. Cancellation Propagation
- Cancelling parent context cancels all children
- Cancelling parent context cancels all grandchildren
- All descendants detect parent cancellation via Done channel
- Context.Err() reports correct error after cancellation
- Values remain accessible even after cancellation

### 7. Parent Isolation
- Cancelling child does NOT cancel parent
- Cancelling child does NOT affect siblings
- Parent remains active after child cancellation
- Sibling contexts are independent

### 8. Timeout Behavior
- Context with timeout eventually times out
- Timeout occurs at approximately correct time
- Error is `context.DeadlineExceeded`
- Values remain accessible after timeout
- Actual wall-clock time is measured (cannot be faked)

### 9. Goroutine Cancellation
- Goroutines detect context cancellation
- Select with ctx.Done() works correctly
- Goroutines exit when context is cancelled
- Real concurrent behavior is tested (not simulated)

### 10. Context Patterns
- Functions accept context as first parameter
- Context values are accessible within functions
- Functions respect context cancellation
- Context chains through nested function calls
- Multiple goroutines can share same context

## Why These Tests Are Un-Gameable

### 1. Real Standard Library Usage
- Uses actual `context.Context` from Go standard library
- No mocks or test doubles for context behavior
- Cancellation signals are real Go channels
- Cannot be faked with hardcoded returns

### 2. Time-Based Validation
- Timeout tests measure actual wall-clock time with `time.Sleep`
- Verifies timeouts occur within expected range (15-100ms)
- Cannot be satisfied with constant returns
- Requires real timeout implementation

### 3. Concurrent Behavior
- Goroutines actually execute concurrently
- Uses real channels for synchronization
- Tests detect if goroutines don't exit on cancellation
- Cannot be faked with sequential code

### 4. Value Propagation Chains
- Tests multi-level context derivation (parent → child → grandchild)
- Verifies values propagate through entire chain
- Confirms parent isolation (child changes don't affect parent)
- Requires correct use of context.WithValue

### 5. Negative Testing
- Tests empty contexts return empty strings (not panics)
- Verifies nil safety throughout
- Checks edge cases (empty strings, nil causes, etc.)
- Cannot be satisfied with panic or crashes

### 6. Integration with Standard Library
- Tests context.WithTimeout preserves custom values
- Tests context.WithCancel works with custom values
- Verifies proper integration with Go context package
- Requires correct context.WithValue key handling

### 7. Cancellation Signal Flow
- Parent cancellation must reach all descendants
- Child cancellation must NOT reach parent or siblings
- Uses real Done channels and select statements
- Cannot be faked - signals must actually propagate

## API Requirements

To pass these tests, implement these functions in `internal/context/context.go`:

```go
// NewRequestContext creates a new context for an incoming request
func NewRequestContext() (context.Context, context.CancelFunc)

// WithRequestID attaches a request ID to context
func WithRequestID(ctx context.Context, id string) context.Context

// RequestID extracts the request ID from context
func RequestID(ctx context.Context) string

// WithService attaches the service name to context
func WithService(ctx context.Context, service string) context.Context

// Service extracts the service name from context
func Service(ctx context.Context) string

// WithHostname attaches the target hostname to context
func WithHostname(ctx context.Context, hostname string) context.Context

// Hostname extracts the target hostname from context
func Hostname(ctx context.Context) string

// WithClientID attaches the client identifier to context
func WithClientID(ctx context.Context, clientID string) context.Context

// ClientID extracts the client identifier from context
func ClientID(ctx context.Context) string
```

## Running the Tests

### Run All Context Tests
```bash
cd /Users/bmf/code/chaperone-auth-gateway
go test ./test -run TestContext -v
```

### Run Specific Test
```bash
go test ./test -run TestContextPropagation -v
go test ./test -run TestContextPatterns -v
go test ./test -run TestPhase06Completion -v
```

### Expected Behavior

**Before Implementation:**
- Tests will FAIL to compile (undefined: chcontext.NewRequestContext, etc.)
- This is CORRECT - tests enforce that implementation exists

**After Implementation:**
- All tests should PASS
- TestPhase06Completion will report which checks failed
- No warnings or errors should appear

## Test Coverage

The tests validate:
- **9 functions** (NewRequestContext + 8 value functions)
- **15 test scenarios** (context creation, 4 value types, chaining, cancellation, etc.)
- **10 completion checks** in meta-test
- **~50 individual assertions** across all tests

## Traceability to Plan

These tests validate **PLAN-2025-11-26-031437.md Phase 0.6: Context Propagation (CHAP-5e3)**:

**From PLAN lines 291-319:**
- ✅ NewRequestContext creates cancellable context
- ✅ WithRequestID/RequestID round-trip
- ✅ WithService/Service round-trip
- ✅ WithHostname/Hostname round-trip
- ✅ WithClientID/ClientID round-trip (added beyond plan)
- ✅ Context helpers work
- ✅ Values store and retrieve correctly
- ✅ Timeout behavior works
- ✅ Cancellation propagates correctly

**Additional Coverage Beyond Plan:**
- Multiple values on same context
- Value propagation through derived contexts
- Parent/child isolation
- Goroutine cancellation detection
- Context patterns (function signatures, chains)

## Anti-Gaming Measures Summary

| Test Aspect | Anti-Gaming Measure |
|------------|---------------------|
| Context Creation | Uses real context.WithCancel, verifies Done channel |
| Value Storage | Round-trip through context.WithValue, checks retrieval |
| Cancellation | Real goroutines, real channels, actual signal propagation |
| Timeout | Measures wall-clock time with time.Sleep |
| Goroutines | Concurrent execution, cannot fake with sequential code |
| Chaining | Multi-level derivation, parent isolation enforced |
| Edge Cases | Nil safety, empty contexts, missing values |

## Success Criteria

Phase 0.6 is complete when:
- ✅ All 15 test cases pass
- ✅ TestPhase06Completion reports 10/10 checks passed
- ✅ No compilation errors
- ✅ go test ./test -run TestPhase06 exits 0
- ✅ Test coverage >= 85% (per PLAN validation requirement)

## Implementation Hints

### Context Keys
Use unexported types for context keys to avoid collisions:
```go
type contextKey int

const (
    requestIDKey contextKey = iota
    serviceKey
    hostnameKey
    clientIDKey
)
```

### Value Storage
```go
func WithRequestID(ctx context.Context, id string) context.Context {
    return context.WithValue(ctx, requestIDKey, id)
}

func RequestID(ctx context.Context) string {
    v := ctx.Value(requestIDKey)
    if v == nil {
        return ""
    }
    return v.(string)
}
```

### Context Creation
```go
func NewRequestContext() (context.Context, context.CancelFunc) {
    return context.WithCancel(context.Background())
}
```

## Failure Modes

### Common Mistakes These Tests Catch

1. **Using strings as context keys**
   - Tests will pass but collision risk exists
   - Better: use unexported type

2. **Panicking on missing values**
   - Tests verify empty string return
   - Must handle nil from ctx.Value()

3. **Not deriving from parent context**
   - Tests verify value propagation
   - Must use parent as base for WithValue

4. **Type assertion without nil check**
   - Tests verify empty context handling
   - Must check if ctx.Value() returns nil

5. **Creating new background context instead of deriving**
   - Tests verify cancellation propagation
   - Must derive from parent to inherit cancellation

## Test Maintenance

### When to Update Tests
- API changes (new value types, new functions)
- New context patterns discovered
- Edge cases found in production
- Security requirements change

### What NOT to Change
- Core validation logic (round-trip, cancellation, timeout)
- Anti-gaming measures (real goroutines, real time)
- Negative tests (missing values, empty contexts)

## Related Tests

- **Phase 0.3:** `test/logging_test.go` - Uses request ID from context
- **Phase 0.4:** `test/config_test.go` - May use context in future
- **Phase 0.2:** `test/errors_test.go` - Error handling integrates with context

Context propagation is foundational - all subsequent phases will use these context helpers.
