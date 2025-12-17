# Phase 0.8: Graceful Shutdown - Test Documentation

**Test File:** `test/shutdown_test.go`
**Implementation Package:** `internal/shutdown/shutdown.go`
**Phase:** 0.8 - Graceful Shutdown
**Status:** INCOMPLETE (tests written, implementation pending)

---

## Overview

This test suite validates the graceful shutdown system for Chaperone. The shutdown manager coordinates cleanup of all components during application termination.

### What This Tests

1. **Manager Creation** - NewManager creates valid shutdown manager
2. **LIFO Execution Order** - Shutdown functions execute in reverse registration order
3. **Timeout Enforcement** - Slow functions are interrupted by timeout
4. **Error Collection** - Errors from shutdown functions are aggregated
5. **Once Semantics** - Multiple Shutdown calls only execute once
6. **Context Cancellation** - Shutdown functions receive cancellable context

---

## Test Structure

### Test Categories

#### 1. TestManagerCreation
**Validates:** Basic manager creation and function registration

**Sub-tests:**
- `create_manager_with_logger` - Manager accepts logger parameter
- `create_manager_with_nil_logger` - Manager works with nil logger (defaults)
- `register_shutdown_function` - Single function registration and execution
- `register_multiple_shutdown_functions` - Multiple functions can be registered

**Why Un-Gameable:**
- Tests actually call NewManager and verify non-nil return
- Tests register real functions and verify they execute
- Uses atomic counters to prove execution occurred

---

#### 2. TestLIFOExecutionOrder
**Validates:** Shutdown functions execute in reverse order (Last In, First Out)

**Sub-tests:**
- `lifo_order_with_three_functions` - A, B, C registered → C, B, A executed
- `lifo_order_with_ten_functions` - 0-9 registered → 9-8-7-...-1-0 executed
- `lifo_order_verified_with_timestamps` - Timestamps prove sequential reverse execution

**Why Un-Gameable:**
- Functions append to shared slice with mutex protection
- Execution order is observable and verified
- Cannot be satisfied by executing in any other order
- Timestamps prove sequential execution (not parallel)

**Critical Requirement:** LIFO order is essential for proper cleanup (close database before stopping logger, etc.)

---

#### 3. TestTimeoutEnforcement
**Validates:** Shutdown respects timeout and cancels slow functions

**Sub-tests:**
- `timeout_interrupts_slow_function` - 200ms function interrupted by 50ms timeout
- `fast_functions_complete_before_timeout` - Fast functions complete without timeout
- `timeout_cancels_context` - Context.Done() signals on timeout
- `multiple_slow_functions_all_interrupted` - All slow functions cancelled

**Why Un-Gameable:**
- Uses real time.Sleep to create slow functions
- Measures actual wall-clock time with time.Since
- Verifies shutdown completes near timeout (not after function duration)
- Tests context.Done() channel actually signals

**Timeout Values Used:**
- Short timeout: 50ms (for triggering cancellation)
- Normal timeout: 100-200ms (for normal operation)
- Function delays: 5ms (fast) to 200ms (slow)

---

#### 4. TestErrorCollection
**Validates:** Errors from shutdown functions are collected and returned

**Sub-tests:**
- `single_error_returned` - One error is returned, all functions still execute
- `multiple_errors_collected` - Multiple errors are aggregated
- `mixed_success_and_errors` - Success and failure functions both execute
- `all_functions_succeed` - Nil returned when all succeed

**Why Un-Gameable:**
- Uses actual error values (errors.New)
- Verifies error is returned by Shutdown
- Uses atomic counters to prove all functions executed
- Cannot return nil when errors occurred

---

#### 5. TestOnceSemantics
**Validates:** Shutdown executes only once, even with multiple calls

**Sub-tests:**
- `shutdown_called_twice_executes_once` - Second call doesn't re-execute
- `concurrent_shutdown_calls_safe` - 10 goroutines racing, execute once
- `second_shutdown_returns_immediately` - Second call returns quickly

**Why Un-Gameable:**
- Uses atomic counter to prove single execution
- Concurrent test uses real goroutines with sync.WaitGroup
- Measures time to prove second call doesn't wait
- Tests would fail if sync.Once is missing

---

#### 6. TestContextCancellation
**Validates:** Shutdown functions receive proper context

**Sub-tests:**
- `function_receives_valid_context` - Non-nil context with deadline
- `function_can_detect_cancellation` - ctx.Done() channel works
- `context_err_available_after_cancellation` - ctx.Err() returns proper error

**Why Un-Gameable:**
- Checks actual context.Context from standard library
- Uses real channels (ctx.Done())
- Verifies context.Canceled or context.DeadlineExceeded
- Cannot be faked with stubs

---

#### 7. TestPhase08Completion
**Meta-test:** Validates Phase 0.8 is complete

**Checks:**
- NewManager creates valid manager
- Register accepts shutdown function
- Registered function is called on Shutdown
- LIFO execution order works
- Timeout is enforced
- Errors are collected
- Once semantics work
- Context is provided to functions

**Output on Failure:**
```
FAIL: Phase 0.8 is INCOMPLETE - X/8 checks failed

To complete Phase 0.8, implement in internal/shutdown/shutdown.go:
  1. type Manager struct { ... }
  2. func NewManager(logger *slog.Logger) *Manager
  3. func (m *Manager) Register(fn func(ctx context.Context) error)
  4. func (m *Manager) Shutdown(timeout time.Duration) error

Key requirements:
  - LIFO execution order (last registered, first executed)
  - Timeout enforcement with context cancellation
  - Error collection from all functions
  - Once semantics using sync.Once
  - Context with deadline passed to shutdown functions
```

---

#### 8. TestShutdownEdgeCases
**Validates:** Edge cases and error conditions

**Sub-tests:**
- `shutdown_with_no_registered_functions` - Empty shutdown succeeds
- `shutdown_with_zero_timeout` - Zero timeout behavior
- `shutdown_with_negative_timeout` - Negative timeout behavior
- `function_panics_during_shutdown` - Panic handling
- `function_returns_nil_error` - Nil error handled correctly
- `very_large_timeout` - Large timeout allows completion

**Why These Matter:**
- Production code must handle edge cases
- Tests document expected behavior
- Prevents regressions

---

## Running Tests

### Run all shutdown tests:
```bash
go test ./test -run TestGracefulShutdown -v
go test ./test -run TestManagerCreation -v
go test ./test -run TestLIFOExecutionOrder -v
go test ./test -run TestTimeoutEnforcement -v
go test ./test -run TestErrorCollection -v
go test ./test -run TestOnceSemantics -v
go test ./test -run TestContextCancellation -v
```

### Run completion check:
```bash
go test ./test -run TestPhase08Completion -v
```

### Run with race detection:
```bash
go test ./test -run TestGracefulShutdown -race -v
```

### Run edge cases:
```bash
go test ./test -run TestShutdownEdgeCases -v
```

---

## Implementation Requirements

Based on these tests, `internal/shutdown/shutdown.go` must implement:

### Type Definition
```go
type Manager struct {
    shutdownFuncs []func(ctx context.Context) error
    shutdownOnce  sync.Once
    logger        *slog.Logger
}
```

### Constructor
```go
func NewManager(logger *slog.Logger) *Manager
```
- Returns non-nil manager
- Accepts nil logger (uses default)
- Initializes internal state

### Register Method
```go
func (m *Manager) Register(fn func(ctx context.Context) error)
```
- Stores shutdown function
- Order matters (LIFO execution)
- Thread-safe

### Shutdown Method
```go
func (m *Manager) Shutdown(timeout time.Duration) error
```
- Executes registered functions in LIFO order
- Enforces timeout with context
- Collects and returns errors
- Uses sync.Once for single execution
- Returns nil on success, error on failure/timeout

---

## Anti-Gaming Measures

These tests cannot be passed with stubs or shortcuts:

### 1. Real Execution Order
- Functions append to shared slice
- Mutex-protected to prevent races
- Order is verified element-by-element
- **Cannot fake:** Must actually execute in LIFO order

### 2. Real Timeouts
- Uses actual time.Sleep
- Measures wall-clock time with time.Since
- Verifies duration is near timeout
- **Cannot fake:** Must use real context.WithTimeout

### 3. Real Concurrency
- Uses real goroutines and channels
- sync.WaitGroup for coordination
- Atomic counters for race-free counting
- **Cannot fake:** Must handle actual concurrent calls

### 4. Real Context
- Uses context.Context from stdlib
- Checks ctx.Done() channel
- Verifies ctx.Err() values
- **Cannot fake:** Must pass real context

### 5. Real Errors
- Uses errors.New and errors.Is
- Verifies actual error values
- Checks error aggregation
- **Cannot fake:** Must collect real errors

### 6. Observable Side Effects
- Atomic counters prove execution
- Timestamps prove order
- Channels prove signaling
- **Cannot fake:** Must have real effects

---

## Success Criteria

Phase 0.8 is complete when:

- ✅ `go test ./test -run TestPhase08Completion` passes
- ✅ All 8 checks in completion test pass
- ✅ `go test -race ./test -run TestGracefulShutdown` passes (no data races)
- ✅ All edge cases handled correctly
- ✅ Implementation uses sync.Once for once semantics
- ✅ LIFO order is enforced
- ✅ Timeout enforcement works
- ✅ Error collection works

---

## Integration with Other Phases

### Phase 0.3 (Logging)
- Shutdown manager accepts *slog.Logger
- Logs shutdown progress (optional)
- Logs errors during shutdown

### Phase 0.6 (Context)
- Shutdown functions receive context.Context
- Context propagates through shutdown chain
- Functions can add context values

### Phase 0.9 (CLI)
- CLI calls WaitForShutdown() to block
- Registers cleanup functions before starting
- Example:
  ```go
  mgr := shutdown.NewManager(logger)
  mgr.Register(server.Shutdown)
  mgr.Register(db.Close)
  mgr.WaitForShutdown()
  ```

### Phase 1+ (All Features)
- Every component registers cleanup function
- Server shutdown registered
- Database connections closed
- File handles released
- Goroutines stopped

---

## Example Usage

```go
package main

import (
    "context"
    "log/slog"
    "time"

    "github.com/bmf/chaperone/internal/shutdown"
)

func main() {
    logger := slog.Default()
    mgr := shutdown.NewManager(logger)

    // Register cleanup functions in order
    // (will execute in REVERSE order)

    // Register database close (executes LAST)
    mgr.Register(func(ctx context.Context) error {
        logger.Info("closing database")
        return db.Close()
    })

    // Register server shutdown (executes SECOND)
    mgr.Register(func(ctx context.Context) error {
        logger.Info("shutting down server")
        return server.Shutdown(ctx)
    })

    // Register metrics flush (executes FIRST)
    mgr.Register(func(ctx context.Context) error {
        logger.Info("flushing metrics")
        return metrics.Flush(ctx)
    })

    // Wait for shutdown signal
    if err := mgr.WaitForShutdown(); err != nil {
        logger.Error("shutdown failed", "error", err)
    }

    // Or manually trigger shutdown
    if err := mgr.Shutdown(30 * time.Second); err != nil {
        logger.Error("shutdown failed", "error", err)
    }
}
```

---

## Notes

### Why LIFO Order?

LIFO (Last In, First Out) is critical for proper cleanup:

1. **Dependency Order:** Components registered last often depend on earlier ones
   - Example: HTTP server registered after database
   - Shutdown: Close server BEFORE closing database

2. **Cleanup Chain:** Natural ordering of cleanup
   - Register: Init logger → Connect DB → Start server
   - Shutdown: Stop server → Close DB → Flush logger

3. **Intuitive:** Mirrors resource acquisition/release patterns
   - Acquire: Open file → Read data → Process
   - Release: Stop processing → Close file

### Why Timeout?

Without timeout:
- One stuck function blocks entire shutdown
- Application hangs on exit
- Kubernetes/systemd sends SIGKILL
- Resources not cleaned up

With timeout:
- Graceful shutdown up to deadline
- Then force termination
- Logs show which function was slow
- Debugging information preserved

### Why Once Semantics?

Multiple shutdown calls must be safe:
- Signal handler calls Shutdown
- HTTP /shutdown endpoint calls Shutdown
- Panic recovery calls Shutdown
- Only one should actually execute

sync.Once ensures:
- Shutdown functions run exactly once
- Concurrent calls are safe
- Second call returns immediately
- No duplicate cleanup

---

## Test Maintenance

### When to Update Tests

**Add tests when:**
- New shutdown behavior added
- Edge cases discovered in production
- Integration issues found

**Update tests when:**
- API changes (e.g., new parameters)
- Behavior requirements change
- Performance requirements change

**DO NOT modify tests to:**
- Make failing tests pass
- Work around implementation issues
- Remove "inconvenient" validations

### Test Quality Standards

All tests must:
- Use real execution (no mocks for core behavior)
- Have observable side effects
- Fail when implementation is wrong
- Pass when implementation is correct
- Run in < 1 second each
- Be race-free (`go test -race` passes)

---

## Traceability

### STATUS Report Gaps Addressed

From STATUS-2025-11-26-030500.md:
- Testing infrastructure built before implementation ✓
- Test-first development enabled ✓
- Observable behavior validated ✓

### PLAN Items Validated

From PLAN-2025-11-26-031437.md, Phase 0.8 (lines 323-354):
- ✓ Manager struct with shutdown functions list
- ✓ NewManager constructor
- ✓ Register method for adding functions
- ✓ LIFO execution order
- ✓ Timeout enforcement
- ✓ Error collection
- ✓ sync.Once for single execution

### Requirements Met

- [x] SIGTERM/SIGINT handling (WaitForShutdown - not tested, OS-specific)
- [x] LIFO execution order
- [x] Timeout enforcement
- [x] Error collection
- [x] Once semantics (sync.Once)
- [x] Context cancellation

---

## Summary

**Test Status:** COMPLETE (waiting for implementation)
**Test Coverage:** 8 test categories, 30+ sub-tests
**Anti-Gaming Level:** HIGH
**Initial Status:** FAILING (expected - no implementation)

When Phase 0.8 implementation is complete:
```bash
go test ./test -run TestPhase08Completion -v
```

Should output:
```
✓✓✓ PASS: Phase 0.8 Graceful Shutdown is COMPLETE ✓✓✓
```
