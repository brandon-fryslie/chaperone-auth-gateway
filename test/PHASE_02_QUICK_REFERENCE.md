# Phase 0.2 Error Framework - Quick Reference

## Test Status: READY FOR IMPLEMENTATION

### Files Created
```
test/errors_test.go                    (870 lines, 26K)
test/ERROR_TESTS_README.md             (12K - detailed guide)
test/ERROR_TEST_VERIFICATION.md        (11K - verification results)
test/error_tests_summary.json          (7.8K - tracking data)
test/PHASE_02_QUICK_REFERENCE.md       (this file)
```

---

## Run Tests

### Check All Tests (will fail until implementation exists)
```bash
cd /Users/bmf/code/chaperone-auth-gateway
go test ./test -run TestError -v
```

### Check Completion Status
```bash
go test ./test -run TestPhase02Completion -v
```

### Expected Current Output
```
FAIL: compile error (undefined symbols)
```

### Expected After Implementation
```
✓✓✓ PASS: Phase 0.2 Error Handling Framework is COMPLETE ✓✓✓
```

---

## Implementation Checklist

Create `internal/errors/errors.go`:

### 1. Sentinel Errors (6)
```go
var (
    ErrSecretNotFound    = errors.New("secret not found")
    ErrPermissionDenied  = errors.New("permission denied")
    ErrTimeout           = errors.New("operation timeout")
    ErrPolicyViolation   = errors.New("policy violation")
    ErrInvalidConfig     = errors.New("invalid configuration")
    ErrUpstreamError     = errors.New("upstream error")
)
```

### 2. Structured Errors (3)
```go
type SecretError struct {
    Provider string
    Ref      string
    Cause    error
}
func (e *SecretError) Error() string { /* ... */ }
func (e *SecretError) Unwrap() error { return e.Cause }

type PolicyError struct {
    Service string
    Rule    string
    Cause   error
}
func (e *PolicyError) Error() string { /* ... */ }
func (e *PolicyError) Unwrap() error { return e.Cause }

type ConfigError struct {
    Field string
    Value interface{}
    Cause error
}
func (e *ConfigError) Error() string { /* ... */ }
func (e *ConfigError) Unwrap() error { return e.Cause }
```

### 3. HTTPStatus Function
```go
func HTTPStatus(err error) int {
    // Use errors.Is() to check through wrap chain
    // Map to: 502, 403, 504, 500, etc.
}
```

### 4. ClientMessage Function
```go
func ClientMessage(err error) string {
    // MUST NOT leak: providers, refs, services, paths
    // Return generic user-friendly messages
}
```

---

## Test Coverage

| Component | Tests | Validations |
|-----------|-------|-------------|
| Sentinel errors | 2 | 24 |
| Structured errors | 3 | 15 |
| HTTP status | 2 | 13 |
| Client messages | 2 | 8 |
| Integration | 2 | 8 |
| Completion | 1 | 6 |
| **TOTAL** | **12** | **74** |

---

## Critical Requirements

### Security (MUST NOT LEAK)
- ❌ Provider names (`keychain`, `vault`)
- ❌ Secret refs (`api-key-prod`)
- ❌ Service names (`openai-api`)
- ❌ File paths (`/etc/chaperone/...`)
- ❌ Stack traces
- ❌ Module paths

Tests explicitly check for these leaks!

### HTTP Status Mapping
| Error | Status |
|-------|--------|
| ErrSecretNotFound | 502 |
| ErrPermissionDenied | 403 |
| ErrTimeout | 504 |
| ErrPolicyViolation | 403 |
| ErrInvalidConfig | 500 |
| ErrUpstreamError | 502 |
| Unknown | 500 |

### Error Chaining
All structured errors MUST implement:
- `Error() string` - for error message
- `Unwrap() error` - for error chaining

This enables `errors.Is()` and `errors.As()` to work through chains.

---

## Verification Steps

### 1. Implementation Complete
```bash
go build ./internal/errors
# No errors
```

### 2. Tests Pass
```bash
go test ./test -run TestError -v
# All green
```

### 3. Race Detection
```bash
go test -race ./test -run TestError
# PASS
```

### 4. Completion Check
```bash
go test ./test -run TestPhase02Completion -v
# Shows: Phase 0.2 Completion Status: 6/6 checks passed
```

### 5. Coverage
```bash
go test -cover ./internal/errors
# Should be >= 90%
```

---

## Why These Tests Are Strong

1. **Compile-time enforcement**: Can't fake with stubs
2. **Behavioral validation**: Tests actual functionality
3. **Security checks**: Explicit leak detection
4. **Error chain validation**: Tests multi-level wrapping
5. **Edge case coverage**: Nil, empty strings, etc.

---

## Quick Test Reference

```bash
# Test individual components
go test ./test -run TestSentinelErrorsExist -v
go test ./test -run TestStructuredErrorTypes -v
go test ./test -run TestHTTPStatusMapping -v
go test ./test -run TestClientMessageSafety -v

# Test completion
go test ./test -run TestPhase02Completion -v

# All error tests
go test ./test -run TestError -v

# With race detection
go test -race ./test -run TestError
```

---

## Current Status

- ✅ Tests written (870 lines)
- ✅ Documentation complete
- ✅ Tests fail correctly (compile-time)
- ⏳ Implementation needed in `internal/errors/errors.go`
- ⏳ Tests must pass before Phase 0.3

---

## Next Phase

**Phase 0.3: Observability Foundation (Logging)**
- Blocked until Phase 0.2 complete
- Will use error types from Phase 0.2
- Will use HTTPStatus() and ClientMessage()

---

## Contact/Questions

See detailed documentation:
- `ERROR_TESTS_README.md` - Complete test guide
- `ERROR_TEST_VERIFICATION.md` - Verification results
- `error_tests_summary.json` - Structured tracking data
