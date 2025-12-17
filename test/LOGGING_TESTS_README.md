# Logging Framework Functional Tests

## Overview

This document describes the functional tests for **Phase 0.3: Observability Foundation** of the Chaperone project. These tests validate the structured logging framework built on Go's `log/slog` package.

**Test File:** `/Users/bmf/code/chaperone-auth-gateway/test/logging_test.go`

## What These Tests Validate

### 1. Request ID Generation and Propagation

**Tests:** `testRequestIDGenerationAndPropagation`

**Validates:**
- `WithRequestID(ctx)` creates a new context with a unique request ID
- `RequestID(ctx)` extracts the ID from context
- Request IDs are unique across 100+ calls
- Empty context returns empty string

**Why Un-Gameable:**
- Generates 100 IDs and verifies uniqueness using a map
- Cannot be satisfied by returning constant values
- Tests actual context value propagation, not mocks

### 2. Structured Logging Functions

**Tests:** `testStructuredLoggingFunctions`

**Validates:**
- `Info(ctx, msg, ...args)` logs info-level messages with context
- `Error(ctx, msg, err, ...args)` logs errors with error field
- `Debug(ctx, msg, ...args)` logs debug-level messages
- All functions include request_id from context
- All output is valid JSON

**Why Un-Gameable:**
- Captures actual log output to `bytes.Buffer`
- Parses JSON to verify structure (fails if not valid JSON)
- Verifies specific fields exist in parsed output
- Checks error is included in Error() output

### 3. JSON Output Format

**Tests:** `testJSONOutputFormat`

**Validates:**
- All logs are valid, parseable JSON
- Required fields present: `time`, `level`, `msg`
- `request_id` appears when context has request ID
- Timestamp is in valid RFC3339 format

**Why Un-Gameable:**
- Uses `json.Unmarshal()` - fails if output isn't valid JSON
- Parses timestamp with `time.Parse()` - fails if format wrong
- Verifies multiple log entries, not just one

### 4. Field Redaction (Security Critical)

**Tests:** `testFieldRedaction`

**Validates:**
- Authorization headers are redacted in logs
- Secret fields are masked
- Sensitive values never appear in plain text
- Redaction is case-insensitive

**Why Un-Gameable:**
- Logs actual sensitive values (tokens, passwords)
- Searches output for the literal sensitive strings
- Test FAILS if sensitive value appears unredacted
- Cannot be satisfied by simply not logging the field

### 5. Duration Logging

**Tests:** `testLogDuration`

**Validates:**
- `LogDuration(ctx, operation, start)` logs operation name and duration
- Duration is measured in milliseconds
- Duration is positive and reasonable (not negative or absurdly large)
- Duration matches actual elapsed time

**Why Un-Gameable:**
- Uses real `time.Sleep(10ms)` to create measurable duration
- Verifies duration is >= 5ms and < 1000ms (cannot hardcode)
- Parses JSON to extract numeric duration field
- Tests real time measurement, not stub values

### 6. Context-Aware Logging

**Tests:** `testContextAwareLogging`

**Validates:**
- `FromContext(ctx)` returns a logger with context fields
- Logger includes request_id from context
- Logger works with empty context (doesn't crash)

**Why Un-Gameable:**
- Uses returned logger to actually log messages
- Captures and parses output
- Verifies request_id appears in logs from context logger

### 7. Log Level Configuration

**Tests:** `testLogLevelConfiguration`

**Validates:**
- `SetLevel(level)` configures minimum log level
- Debug logs suppressed when level is Info
- Info logs appear when level is Info
- Error logs always appear

**Why Un-Gameable:**
- Calls `SetLevel()` and immediately tests effect
- Verifies output buffer is empty/non-empty
- Tests multiple level configurations
- Cannot be satisfied by ignoring SetLevel() calls

### 8. Integration Scenario

**Tests:** `TestLoggingIntegrationScenario`

**Validates:**
- Complete request flow with multiple log statements
- All logs share same request_id
- Authorization headers redacted in realistic scenario
- Duration logging works in context of full workflow

**Why Un-Gameable:**
- Simulates real request processing workflow
- Parses and validates 5+ log entries
- Verifies request_id consistency across all entries
- Tests interaction between all logging features together

## How to Run Tests

### Run All Logging Tests

```bash
cd /Users/bmf/code/chaperone-auth-gateway
go test -v ./test -run TestLogging
```

### Run Specific Test

```bash
go test -v ./test -run TestLoggingFramework/request_id_generation
go test -v ./test -run TestLoggingFramework/json_output_format
go test -v ./test -run TestLoggingIntegrationScenario
```

### Run with Race Detection

```bash
go test -race -v ./test -run TestLogging
```

## Expected Behavior

### Before Implementation (Phase 0.3 incomplete)

Tests should **FAIL** with compilation errors:

```
undefined: log.WithRequestID
undefined: log.RequestID
undefined: log.SetLogger
undefined: log.Info
undefined: log.Error
undefined: log.Debug
undefined: log.LogDuration
undefined: log.FromContext
undefined: log.SetLevel
```

This confirms the tests are validating real functionality.

### After Implementation (Phase 0.3 complete)

All tests should **PASS**:

```
=== RUN   TestLoggingFramework
=== RUN   TestLoggingFramework/request_id_generation_and_propagation
=== RUN   TestLoggingFramework/structured_logging_functions
=== RUN   TestLoggingFramework/json_output_format
=== RUN   TestLoggingFramework/field_redaction
=== RUN   TestLoggingFramework/log_duration
=== RUN   TestLoggingFramework/context_aware_logging
=== RUN   TestLoggingFramework/log_level_configuration
--- PASS: TestLoggingFramework (0.02s)
=== RUN   TestLoggingIntegrationScenario
--- PASS: TestLoggingIntegrationScenario (0.01s)
PASS
```

## Gaming Resistance Analysis

### Why These Tests Cannot Be Gamed

1. **Real Output Capture**: Uses `bytes.Buffer` to capture actual log output, not mocks
2. **JSON Parsing**: Uses `json.Unmarshal()` which fails if output isn't valid JSON
3. **Time Measurement**: Uses `time.Sleep()` and verifies real elapsed time
4. **Uniqueness Verification**: Generates 100+ IDs and checks for duplicates with map
5. **Security Validation**: Logs actual secrets and verifies they don't appear in output
6. **Multiple Assertions**: Each test verifies multiple aspects of output
7. **Integration Testing**: Tests interaction between features, not isolated units
8. **No Mocks**: Tests use real `slog.Logger`, real context values, real time measurements

### What Would Happen If Implementation Cheated

- **Hardcoded request IDs**: Uniqueness test would fail (100 IDs, map check)
- **Non-JSON output**: `json.Unmarshal()` would fail
- **Missing fields**: JSON parsing would detect absent required fields
- **Fake timestamps**: `time.Parse()` would fail if format invalid
- **No redaction**: Test would find literal secret values in output
- **Fake duration**: Sleep for 10ms but log says 0ms → range check fails
- **Ignoring SetLevel**: Debug logs would appear when they shouldn't

## Traceability

### STATUS Gaps Addressed

From `STATUS-2025-11-26-030500.md`:
- **Gap:** "Observability missing from foundation - Logging added as afterthought (Priority 2!)"
- **Address:** These tests ensure logging is fundamental, not afterthought

### PLAN Items Validated

From `PLAN-2025-11-26-031437.md` lines 184-214:

| Acceptance Criteria | Test Coverage |
|---------------------|---------------|
| Logger outputs valid JSON | `testJSONOutputFormat` |
| Request IDs are unique | `testRequestIDGenerationAndPropagation` (100 IDs) |
| Request ID propagates through context | `testContextAwareLogging`, Integration test |
| Authorization headers redacted | `testFieldRedaction` |
| Secrets never logged | `testFieldRedaction` |

### Implementation Contract

These tests define the contract that `internal/log/logger.go` must fulfill:

```go
// Required Functions
func WithRequestID(ctx context.Context) context.Context
func RequestID(ctx context.Context) string
func FromContext(ctx context.Context) *slog.Logger
func Info(ctx context.Context, msg string, args ...any)
func Error(ctx context.Context, msg string, err error, args ...any)
func Debug(ctx context.Context, msg string, args ...any)
func LogDuration(ctx context.Context, operation string, start time.Time)
func SetLogger(logger *slog.Logger)
func SetLevel(level slog.Level)
```

## Test Maintenance

### When to Update Tests

- **New logging features**: Add new test functions
- **Changed log format**: Update JSON parsing assertions
- **New redaction rules**: Add test cases to `testFieldRedaction`
- **Performance requirements**: Add duration benchmarks

### When NOT to Change Tests

- **Don't** change tests to make them pass when implementation is wrong
- **Don't** add mocks to bypass real validation
- **Don't** reduce assertions because they're "too strict"
- **Don't** skip tests that are failing

## Success Criteria

Phase 0.3 is **COMPLETE** when:

1. ✅ All tests in `logging_test.go` pass
2. ✅ No compilation errors
3. ✅ Test coverage >= 85% for `internal/log/` package
4. ✅ No secrets appear in test output (verified by redaction tests)
5. ✅ All logs are valid JSON (verified by parsing tests)

## Test Philosophy

These tests embody the principle: **"Make it impossible to cheat."**

- If logging doesn't work, tests fail
- If redaction is broken, tests fail
- If JSON is malformed, tests fail
- If request IDs aren't unique, tests fail

There is no way to satisfy these tests without building a real, working logging framework.
