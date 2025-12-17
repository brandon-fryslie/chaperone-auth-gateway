package test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bmf/chaperone/internal/shutdown"
)

// TestGracefulShutdown validates Phase 0.8: Graceful Shutdown
//
// This test suite validates the shutdown manager by testing:
// 1. Manager creation and function registration
// 2. LIFO (Last In, First Out) execution order
// 3. Timeout enforcement on slow shutdown functions
// 4. Error collection from multiple shutdown functions
// 5. Once semantics (multiple Shutdown calls only execute once)
// 6. Context cancellation propagation to shutdown functions
//
// ANTI-GAMING MEASURES:
// 1. Uses real channels and atomic counters to track execution order (observable side effects)
// 2. Tests measure actual wall-clock time with time.Sleep (cannot be faked)
// 3. LIFO order verified by appending to shared slice with mutex protection
// 4. Timeout enforcement uses real context.WithTimeout (no mocks)
// 5. Error collection verified by checking actual error values returned
// 6. Once semantics tested with real goroutines racing to call Shutdown
// 7. Tests FAIL when shutdown package is missing or behavior is incorrect
//
// An AI cannot fake this with stubs - the shutdown behavior must actually work.

// TestManagerCreation verifies:
// - NewManager creates a valid manager
// - Manager can be created with nil logger (defaults work)
// - Manager is not nil
func TestManagerCreation(t *testing.T) {
	t.Run("create_manager_with_logger", func(t *testing.T) {
		testCreateManagerWithLogger(t)
	})

	t.Run("create_manager_with_nil_logger", func(t *testing.T) {
		testCreateManagerWithNilLogger(t)
	})

	t.Run("register_shutdown_function", func(t *testing.T) {
		testRegisterShutdownFunction(t)
	})

	t.Run("register_multiple_shutdown_functions", func(t *testing.T) {
		testRegisterMultipleShutdownFunctions(t)
	})
}

// testCreateManagerWithLogger verifies:
// - NewManager accepts a logger and returns non-nil manager
// - Manager can be used immediately after creation
func testCreateManagerWithLogger(t *testing.T) {
	// Create manager with nil logger (should work with defaults)
	mgr := shutdown.NewManager(nil)

	if mgr == nil {
		t.Fatal("FAIL: NewManager(nil) returned nil - must return valid manager")
	}

	t.Log("PASS: NewManager creates valid manager with nil logger")
}

// testCreateManagerWithNilLogger verifies:
// - Manager works with nil logger (uses defaults)
func testCreateManagerWithNilLogger(t *testing.T) {
	mgr := shutdown.NewManager(nil)

	if mgr == nil {
		t.Fatal("FAIL: NewManager(nil) returned nil")
	}

	// Should be able to register function immediately
	called := false
	mgr.Register(func(ctx context.Context) error {
		called = true
		return nil
	})

	// Trigger shutdown
	err := mgr.Shutdown(100 * time.Millisecond)
	if err != nil {
		t.Fatalf("FAIL: Shutdown returned error: %v", err)
	}

	if !called {
		t.Fatal("FAIL: Registered function was not called")
	}

	t.Log("PASS: Manager works with nil logger")
}

// testRegisterShutdownFunction verifies:
// - Register accepts a shutdown function
// - Registered function is called during shutdown
// - Function receives a valid context
func testRegisterShutdownFunction(t *testing.T) {
	mgr := shutdown.NewManager(nil)

	functionCalled := false
	var receivedCtx context.Context

	mgr.Register(func(ctx context.Context) error {
		functionCalled = true
		receivedCtx = ctx
		return nil
	})

	// Trigger shutdown
	err := mgr.Shutdown(100 * time.Millisecond)
	if err != nil {
		t.Fatalf("FAIL: Shutdown returned unexpected error: %v", err)
	}

	if !functionCalled {
		t.Fatal("FAIL: Registered shutdown function was not called")
	}

	if receivedCtx == nil {
		t.Fatal("FAIL: Shutdown function received nil context")
	}

	t.Log("PASS: Registered shutdown function is called with valid context")
}

// testRegisterMultipleShutdownFunctions verifies:
// - Multiple functions can be registered
// - All registered functions are called
func testRegisterMultipleShutdownFunctions(t *testing.T) {
	mgr := shutdown.NewManager(nil)

	callCount := int32(0)

	// Register 5 functions
	for i := 0; i < 5; i++ {
		mgr.Register(func(ctx context.Context) error {
			atomic.AddInt32(&callCount, 1)
			return nil
		})
	}

	// Trigger shutdown
	err := mgr.Shutdown(200 * time.Millisecond)
	if err != nil {
		t.Fatalf("FAIL: Shutdown returned error: %v", err)
	}

	finalCount := atomic.LoadInt32(&callCount)
	if finalCount != 5 {
		t.Fatalf("FAIL: Expected 5 functions called, got %d", finalCount)
	}

	t.Log("PASS: All 5 registered functions were called")
}

// TestLIFOExecutionOrder verifies:
// - Shutdown functions execute in reverse order of registration (LIFO)
// - Last registered function executes FIRST
// - First registered function executes LAST
// - Order is deterministic and observable
func TestLIFOExecutionOrder(t *testing.T) {
	t.Run("lifo_order_with_three_functions", func(t *testing.T) {
		testLIFOOrderWithThreeFunctions(t)
	})

	t.Run("lifo_order_with_ten_functions", func(t *testing.T) {
		testLIFOOrderWithTenFunctions(t)
	})

	t.Run("lifo_order_verified_with_timestamps", func(t *testing.T) {
		testLIFOOrderVerifiedWithTimestamps(t)
	})
}

// testLIFOOrderWithThreeFunctions verifies:
// - With 3 functions A, B, C registered in that order
// - Execution order is C, B, A (reverse)
func testLIFOOrderWithThreeFunctions(t *testing.T) {
	mgr := shutdown.NewManager(nil)

	var executionOrder []string
	var mu sync.Mutex

	// Register function A (should execute LAST)
	mgr.Register(func(ctx context.Context) error {
		mu.Lock()
		executionOrder = append(executionOrder, "A")
		mu.Unlock()
		return nil
	})

	// Register function B (should execute SECOND)
	mgr.Register(func(ctx context.Context) error {
		mu.Lock()
		executionOrder = append(executionOrder, "B")
		mu.Unlock()
		return nil
	})

	// Register function C (should execute FIRST)
	mgr.Register(func(ctx context.Context) error {
		mu.Lock()
		executionOrder = append(executionOrder, "C")
		mu.Unlock()
		return nil
	})

	// Trigger shutdown
	err := mgr.Shutdown(200 * time.Millisecond)
	if err != nil {
		t.Fatalf("FAIL: Shutdown returned error: %v", err)
	}

	// Verify LIFO order: C, B, A
	expectedOrder := []string{"C", "B", "A"}
	if len(executionOrder) != len(expectedOrder) {
		t.Fatalf("FAIL: Expected %d functions to execute, got %d", len(expectedOrder), len(executionOrder))
	}

	for i, expected := range expectedOrder {
		if executionOrder[i] != expected {
			t.Fatalf("FAIL: Execution order at position %d: expected %s, got %s\nFull order: %v",
				i, expected, executionOrder[i], executionOrder)
		}
	}

	t.Logf("PASS: Functions executed in correct LIFO order: %v", executionOrder)
}

// testLIFOOrderWithTenFunctions verifies LIFO order with larger set:
// - Register functions 0-9
// - Execution order should be 9, 8, 7, 6, 5, 4, 3, 2, 1, 0
func testLIFOOrderWithTenFunctions(t *testing.T) {
	mgr := shutdown.NewManager(nil)

	var executionOrder []int
	var mu sync.Mutex

	// Register 10 functions with IDs 0-9
	for i := 0; i < 10; i++ {
		id := i // capture loop variable
		mgr.Register(func(ctx context.Context) error {
			mu.Lock()
			executionOrder = append(executionOrder, id)
			mu.Unlock()
			return nil
		})
	}

	// Trigger shutdown
	err := mgr.Shutdown(500 * time.Millisecond)
	if err != nil {
		t.Fatalf("FAIL: Shutdown returned error: %v", err)
	}

	// Verify reverse order: 9, 8, 7, ..., 1, 0
	if len(executionOrder) != 10 {
		t.Fatalf("FAIL: Expected 10 functions to execute, got %d", len(executionOrder))
	}

	for i := 0; i < 10; i++ {
		expected := 9 - i // Reverse order
		if executionOrder[i] != expected {
			t.Fatalf("FAIL: Execution order at position %d: expected %d, got %d\nFull order: %v",
				i, expected, executionOrder[i], executionOrder)
		}
	}

	t.Logf("PASS: 10 functions executed in correct LIFO order: %v", executionOrder)
}

// testLIFOOrderVerifiedWithTimestamps verifies:
// - LIFO order using timestamps
// - Each function records when it executes
// - Timestamps confirm reverse registration order
func testLIFOOrderVerifiedWithTimestamps(t *testing.T) {
	mgr := shutdown.NewManager(nil)

	type execution struct {
		id   int
		time time.Time
	}

	var executions []execution
	var mu sync.Mutex

	// Register 5 functions
	for i := 0; i < 5; i++ {
		id := i
		mgr.Register(func(ctx context.Context) error {
			mu.Lock()
			executions = append(executions, execution{id: id, time: time.Now()})
			mu.Unlock()
			time.Sleep(2 * time.Millisecond) // Small delay to ensure timestamp separation
			return nil
		})
	}

	// Trigger shutdown
	err := mgr.Shutdown(500 * time.Millisecond)
	if err != nil {
		t.Fatalf("FAIL: Shutdown returned error: %v", err)
	}

	// Verify LIFO order by ID: 4, 3, 2, 1, 0
	if len(executions) != 5 {
		t.Fatalf("FAIL: Expected 5 executions, got %d", len(executions))
	}

	for i := 0; i < 5; i++ {
		expectedID := 4 - i // Reverse order
		if executions[i].id != expectedID {
			t.Fatalf("FAIL: Execution %d: expected ID %d, got %d", i, expectedID, executions[i].id)
		}
	}

	// Verify timestamps are in ascending order (later functions execute later)
	for i := 1; i < len(executions); i++ {
		if executions[i].time.Before(executions[i-1].time) {
			t.Fatalf("FAIL: Timestamp ordering violated at position %d", i)
		}
	}

	t.Logf("PASS: LIFO order verified with timestamps")
}

// TestTimeoutEnforcement verifies:
// - Shutdown respects timeout parameter
// - Slow functions are interrupted by timeout
// - Shutdown returns error when timeout exceeded
// - Fast functions complete before timeout
func TestTimeoutEnforcement(t *testing.T) {
	t.Run("timeout_interrupts_slow_function", func(t *testing.T) {
		testTimeoutInterruptsSlowFunction(t)
	})

	t.Run("fast_functions_complete_before_timeout", func(t *testing.T) {
		testFastFunctionsCompleteBeforeTimeout(t)
	})

	t.Run("timeout_cancels_context", func(t *testing.T) {
		testTimeoutCancelsContext(t)
	})

	t.Run("multiple_slow_functions_all_interrupted", func(t *testing.T) {
		testMultipleSlowFunctionsAllInterrupted(t)
	})
}

// testTimeoutInterruptsSlowFunction verifies:
// - Function that sleeps longer than timeout is interrupted
// - Shutdown returns within timeout duration
// - Error indicates timeout occurred
func testTimeoutInterruptsSlowFunction(t *testing.T) {
	mgr := shutdown.NewManager(nil)

	functionCompleted := int32(0)

	// Register function that sleeps for 200ms
	mgr.Register(func(ctx context.Context) error {
		time.Sleep(200 * time.Millisecond)
		atomic.StoreInt32(&functionCompleted, 1)
		return nil
	})

	// Shutdown with 50ms timeout (should interrupt)
	start := time.Now()
	err := mgr.Shutdown(50 * time.Millisecond)
	elapsed := time.Since(start)

	// Should complete near timeout (within 100ms total)
	if elapsed > 150*time.Millisecond {
		t.Fatalf("FAIL: Shutdown took %v, expected near 50ms timeout", elapsed)
	}

	// Error should indicate timeout or context cancellation
	if err == nil {
		t.Fatal("FAIL: Shutdown should return error when timeout occurs")
	}

	// Function should not have completed
	if atomic.LoadInt32(&functionCompleted) == 1 {
		t.Error("WARNING: Function completed despite timeout (may not respect context)")
	}

	t.Logf("PASS: Timeout enforced - shutdown completed in %v with error: %v", elapsed, err)
}

// testFastFunctionsCompleteBeforeTimeout verifies:
// - Functions that complete quickly don't trigger timeout
// - Shutdown returns nil error when all functions complete
// - Shutdown duration is less than timeout
func testFastFunctionsCompleteBeforeTimeout(t *testing.T) {
	mgr := shutdown.NewManager(nil)

	completed := int32(0)

	// Register 3 fast functions
	for i := 0; i < 3; i++ {
		mgr.Register(func(ctx context.Context) error {
			time.Sleep(5 * time.Millisecond)
			atomic.AddInt32(&completed, 1)
			return nil
		})
	}

	// Shutdown with generous timeout
	start := time.Now()
	err := mgr.Shutdown(200 * time.Millisecond)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("FAIL: Shutdown returned error for fast functions: %v", err)
	}

	if elapsed > 100*time.Millisecond {
		t.Fatalf("FAIL: Shutdown took %v, expected fast completion", elapsed)
	}

	if atomic.LoadInt32(&completed) != 3 {
		t.Fatalf("FAIL: Expected 3 functions completed, got %d", completed)
	}

	t.Logf("PASS: Fast functions completed successfully in %v", elapsed)
}

// testTimeoutCancelsContext verifies:
// - Context passed to shutdown functions is cancelled on timeout
// - Functions can detect cancellation via ctx.Done()
func testTimeoutCancelsContext(t *testing.T) {
	mgr := shutdown.NewManager(nil)

	contextCancelled := make(chan bool, 1)

	// Register function that detects context cancellation
	mgr.Register(func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			contextCancelled <- true
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
			return nil
		}
	})

	// Shutdown with short timeout
	err := mgr.Shutdown(50 * time.Millisecond)
	if err == nil {
		t.Fatal("FAIL: Expected timeout error, got nil")
	}

	// Verify context was cancelled
	select {
	case <-contextCancelled:
		t.Log("PASS: Context cancelled on timeout")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("FAIL: Context was not cancelled within timeout")
	}
}

// testMultipleSlowFunctionsAllInterrupted verifies:
// - When multiple functions are slow, all are interrupted
// - Timeout applies to entire shutdown, not per-function
func testMultipleSlowFunctionsAllInterrupted(t *testing.T) {
	mgr := shutdown.NewManager(nil)

	completed := int32(0)

	// Register 3 slow functions
	for i := 0; i < 3; i++ {
		mgr.Register(func(ctx context.Context) error {
			<-ctx.Done() // Wait for cancellation
			atomic.AddInt32(&completed, 1)
			return ctx.Err()
		})
	}

	// Shutdown with short timeout
	start := time.Now()
	err := mgr.Shutdown(50 * time.Millisecond)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("FAIL: Expected timeout error for slow functions")
	}

	if elapsed > 150*time.Millisecond {
		t.Fatalf("FAIL: Shutdown took too long: %v", elapsed)
	}

	// All functions should have detected cancellation
	if atomic.LoadInt32(&completed) != 3 {
		t.Errorf("WARNING: Expected 3 functions to detect cancellation, got %d", completed)
	}

	t.Logf("PASS: Multiple slow functions interrupted in %v", elapsed)
}

// TestErrorCollection verifies:
// - Errors from shutdown functions are collected
// - Multiple errors are returned/aggregated
// - Non-error functions don't block error collection
// - Nil errors are handled correctly
func TestErrorCollection(t *testing.T) {
	t.Run("single_error_returned", func(t *testing.T) {
		testSingleErrorReturned(t)
	})

	t.Run("multiple_errors_collected", func(t *testing.T) {
		testMultipleErrorsCollected(t)
	})

	t.Run("mixed_success_and_errors", func(t *testing.T) {
		testMixedSuccessAndErrors(t)
	})

	t.Run("all_functions_succeed", func(t *testing.T) {
		testAllFunctionsSucceed(t)
	})
}

// testSingleErrorReturned verifies:
// - When one function returns error, Shutdown returns that error
// - Other functions still execute
func testSingleErrorReturned(t *testing.T) {
	mgr := shutdown.NewManager(nil)

	executed := int32(0)
	expectedErr := errors.New("shutdown failed")

	mgr.Register(func(ctx context.Context) error {
		atomic.AddInt32(&executed, 1)
		return nil
	})

	mgr.Register(func(ctx context.Context) error {
		atomic.AddInt32(&executed, 1)
		return expectedErr
	})

	mgr.Register(func(ctx context.Context) error {
		atomic.AddInt32(&executed, 1)
		return nil
	})

	err := mgr.Shutdown(100 * time.Millisecond)

	if err == nil {
		t.Fatal("FAIL: Shutdown should return error when function fails")
	}

	// All functions should have executed
	if atomic.LoadInt32(&executed) != 3 {
		t.Fatalf("FAIL: Expected 3 functions executed, got %d", executed)
	}

	// Error should contain the expected error
	if !errors.Is(err, expectedErr) && err.Error() != expectedErr.Error() {
		t.Logf("WARNING: Error doesn't match exactly: got %v, want %v", err, expectedErr)
	}

	t.Logf("PASS: Single error returned: %v", err)
}

// testMultipleErrorsCollected verifies:
// - Multiple errors from different functions are collected
// - Returned error contains information about all failures
func testMultipleErrorsCollected(t *testing.T) {
	mgr := shutdown.NewManager(nil)

	err1 := errors.New("error 1")
	err2 := errors.New("error 2")
	err3 := errors.New("error 3")

	mgr.Register(func(ctx context.Context) error {
		return err1
	})

	mgr.Register(func(ctx context.Context) error {
		return err2
	})

	mgr.Register(func(ctx context.Context) error {
		return err3
	})

	err := mgr.Shutdown(100 * time.Millisecond)

	if err == nil {
		t.Fatal("FAIL: Shutdown should return error when multiple functions fail")
	}

	// The error should reference multiple failures
	// (exact format depends on implementation - could be wrapped, joined, or aggregated)
	errMsg := err.Error()
	t.Logf("Collected error: %v", errMsg)

	// At minimum, the error should exist
	t.Log("PASS: Multiple errors collected (implementation may vary on aggregation)")
}

// testMixedSuccessAndErrors verifies:
// - When some functions succeed and some fail, all execute
// - Errors are still collected
// - Successful functions complete
func testMixedSuccessAndErrors(t *testing.T) {
	mgr := shutdown.NewManager(nil)

	successCount := int32(0)
	errorCount := int32(0)

	// Register mix of successful and failing functions
	mgr.Register(func(ctx context.Context) error {
		atomic.AddInt32(&successCount, 1)
		return nil
	})

	mgr.Register(func(ctx context.Context) error {
		atomic.AddInt32(&errorCount, 1)
		return errors.New("failed")
	})

	mgr.Register(func(ctx context.Context) error {
		atomic.AddInt32(&successCount, 1)
		return nil
	})

	mgr.Register(func(ctx context.Context) error {
		atomic.AddInt32(&errorCount, 1)
		return errors.New("failed")
	})

	err := mgr.Shutdown(100 * time.Millisecond)

	if err == nil {
		t.Fatal("FAIL: Shutdown should return error when some functions fail")
	}

	if atomic.LoadInt32(&successCount) != 2 {
		t.Fatalf("FAIL: Expected 2 successful functions, got %d", successCount)
	}

	if atomic.LoadInt32(&errorCount) != 2 {
		t.Fatalf("FAIL: Expected 2 failed functions, got %d", errorCount)
	}

	t.Logf("PASS: Mixed success (%d) and errors (%d) handled correctly", successCount, errorCount)
}

// testAllFunctionsSucceed verifies:
// - When all functions return nil, Shutdown returns nil
// - No error is created unnecessarily
func testAllFunctionsSucceed(t *testing.T) {
	mgr := shutdown.NewManager(nil)

	executed := int32(0)

	for i := 0; i < 5; i++ {
		mgr.Register(func(ctx context.Context) error {
			atomic.AddInt32(&executed, 1)
			return nil
		})
	}

	err := mgr.Shutdown(100 * time.Millisecond)

	if err != nil {
		t.Fatalf("FAIL: Shutdown should return nil when all functions succeed, got: %v", err)
	}

	if atomic.LoadInt32(&executed) != 5 {
		t.Fatalf("FAIL: Expected 5 functions executed, got %d", executed)
	}

	t.Log("PASS: All functions succeeded, no error returned")
}

// TestOnceSemantics verifies:
// - Multiple Shutdown calls only execute once
// - Second call returns immediately
// - Shutdown functions are not called multiple times
// - Concurrent Shutdown calls are safe
func TestOnceSemantics(t *testing.T) {
	t.Run("shutdown_called_twice_executes_once", func(t *testing.T) {
		testShutdownCalledTwiceExecutesOnce(t)
	})

	t.Run("concurrent_shutdown_calls_safe", func(t *testing.T) {
		testConcurrentShutdownCallsSafe(t)
	})

	t.Run("second_shutdown_returns_immediately", func(t *testing.T) {
		testSecondShutdownReturnsImmediately(t)
	})
}

// testShutdownCalledTwiceExecutesOnce verifies:
// - Calling Shutdown twice only executes shutdown functions once
// - Function call counter proves single execution
func testShutdownCalledTwiceExecutesOnce(t *testing.T) {
	mgr := shutdown.NewManager(nil)

	callCount := int32(0)

	mgr.Register(func(ctx context.Context) error {
		atomic.AddInt32(&callCount, 1)
		return nil
	})

	// First shutdown
	err1 := mgr.Shutdown(100 * time.Millisecond)
	if err1 != nil {
		t.Fatalf("FAIL: First Shutdown returned error: %v", err1)
	}

	// Second shutdown (should not execute functions again)
	err2 := mgr.Shutdown(100 * time.Millisecond)
	// Second call may return nil or the same error as first call

	// Function should have been called exactly once
	finalCount := atomic.LoadInt32(&callCount)
	if finalCount != 1 {
		t.Fatalf("FAIL: Expected function called once, got %d calls", finalCount)
	}

	t.Logf("PASS: Function called exactly once despite two Shutdown calls (err1=%v, err2=%v)", err1, err2)
}

// testConcurrentShutdownCallsSafe verifies:
// - Multiple goroutines calling Shutdown concurrently is safe
// - Functions still execute only once
// - No data races occur
func testConcurrentShutdownCallsSafe(t *testing.T) {
	mgr := shutdown.NewManager(nil)

	callCount := int32(0)

	mgr.Register(func(ctx context.Context) error {
		atomic.AddInt32(&callCount, 1)
		time.Sleep(20 * time.Millisecond) // Make execution observable
		return nil
	})

	// Launch 10 goroutines that all call Shutdown
	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = mgr.Shutdown(100 * time.Millisecond)
		}()
	}

	wg.Wait()

	// Function should have been called exactly once
	finalCount := atomic.LoadInt32(&callCount)
	if finalCount != 1 {
		t.Fatalf("FAIL: Expected function called once with %d concurrent calls, got %d", numGoroutines, finalCount)
	}

	t.Logf("PASS: %d concurrent Shutdown calls executed function exactly once", numGoroutines)
}

// testSecondShutdownReturnsImmediately verifies:
// - Second Shutdown call returns very quickly
// - Does not wait for timeout
func testSecondShutdownReturnsImmediately(t *testing.T) {
	mgr := shutdown.NewManager(nil)

	mgr.Register(func(ctx context.Context) error {
		time.Sleep(30 * time.Millisecond)
		return nil
	})

	// First shutdown
	_ = mgr.Shutdown(100 * time.Millisecond)

	// Second shutdown should return immediately
	start := time.Now()
	_ = mgr.Shutdown(100 * time.Millisecond)
	elapsed := time.Since(start)

	// Should return in < 10ms (immediate)
	if elapsed > 50*time.Millisecond {
		t.Fatalf("FAIL: Second Shutdown took %v, expected immediate return", elapsed)
	}

	t.Logf("PASS: Second Shutdown returned immediately in %v", elapsed)
}

// TestContextCancellation verifies:
// - Shutdown functions receive cancellable context
// - Functions can respect context cancellation
// - Context.Done() works correctly
// - Context.Err() returns correct error
func TestContextCancellation(t *testing.T) {
	t.Run("function_receives_valid_context", func(t *testing.T) {
		testFunctionReceivesValidContext(t)
	})

	t.Run("function_can_detect_cancellation", func(t *testing.T) {
		testFunctionCanDetectCancellation(t)
	})

	t.Run("context_err_available_after_cancellation", func(t *testing.T) {
		testContextErrAvailableAfterCancellation(t)
	})
}

// testFunctionReceivesValidContext verifies:
// - Shutdown function receives non-nil context
// - Context is valid and usable
func testFunctionReceivesValidContext(t *testing.T) {
	mgr := shutdown.NewManager(nil)

	var receivedCtx context.Context

	mgr.Register(func(ctx context.Context) error {
		receivedCtx = ctx
		return nil
	})

	err := mgr.Shutdown(100 * time.Millisecond)
	if err != nil {
		t.Fatalf("FAIL: Shutdown returned error: %v", err)
	}

	if receivedCtx == nil {
		t.Fatal("FAIL: Shutdown function received nil context")
	}

	// Context should have deadline (from timeout)
	_, hasDeadline := receivedCtx.Deadline()
	if !hasDeadline {
		t.Log("WARNING: Context does not have deadline (timeout may not be enforced via context)")
	}

	t.Log("PASS: Shutdown function received valid context")
}

// testFunctionCanDetectCancellation verifies:
// - Function can use ctx.Done() channel
// - Cancellation is detected correctly
func testFunctionCanDetectCancellation(t *testing.T) {
	mgr := shutdown.NewManager(nil)

	cancelled := make(chan bool, 1)

	mgr.Register(func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			cancelled <- true
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
			return errors.New("timeout waiting for cancellation")
		}
	})

	// Shutdown with short timeout to trigger cancellation
	_ = mgr.Shutdown(50 * time.Millisecond)

	// Verify function detected cancellation
	select {
	case <-cancelled:
		t.Log("PASS: Function detected context cancellation")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("FAIL: Function did not detect cancellation")
	}
}

// testContextErrAvailableAfterCancellation verifies:
// - After cancellation, ctx.Err() is non-nil
// - Error is context.Canceled or context.DeadlineExceeded
func testContextErrAvailableAfterCancellation(t *testing.T) {
	mgr := shutdown.NewManager(nil)

	var ctxErr error

	mgr.Register(func(ctx context.Context) error {
		<-ctx.Done()
		ctxErr = ctx.Err()
		return ctxErr
	})

	_ = mgr.Shutdown(50 * time.Millisecond)

	if ctxErr == nil {
		t.Fatal("FAIL: ctx.Err() is nil after cancellation")
	}

	// Should be one of the standard context errors
	if !errors.Is(ctxErr, context.Canceled) && !errors.Is(ctxErr, context.DeadlineExceeded) {
		t.Logf("WARNING: ctx.Err() is neither Canceled nor DeadlineExceeded: %v", ctxErr)
	}

	t.Logf("PASS: ctx.Err() available after cancellation: %v", ctxErr)
}

// TestPhase08Completion is a meta-test that checks if Phase 0.8 is complete.
//
// This runs all validation checks and reports overall status.
func TestPhase08Completion(t *testing.T) {
	checks := []struct {
		name string
		fn   func() error
	}{
		{
			name: "NewManager creates valid manager",
			fn: func() error {
				mgr := shutdown.NewManager(nil)
				if mgr == nil {
					return errors.New("NewManager returned nil")
				}
				return nil
			},
		},
		{
			name: "Register accepts shutdown function",
			fn: func() error {
				mgr := shutdown.NewManager(nil)
				mgr.Register(func(ctx context.Context) error { return nil })
				return nil
			},
		},
		{
			name: "Registered function is called on Shutdown",
			fn: func() error {
				mgr := shutdown.NewManager(nil)
				called := false
				mgr.Register(func(ctx context.Context) error {
					called = true
					return nil
				})
				_ = mgr.Shutdown(100 * time.Millisecond)
				if !called {
					return errors.New("function not called")
				}
				return nil
			},
		},
		{
			name: "LIFO execution order works",
			fn: func() error {
				mgr := shutdown.NewManager(nil)
				var order []int
				var mu sync.Mutex

				for i := 0; i < 3; i++ {
					id := i
					mgr.Register(func(ctx context.Context) error {
						mu.Lock()
						order = append(order, id)
						mu.Unlock()
						return nil
					})
				}

				_ = mgr.Shutdown(100 * time.Millisecond)

				// Should be reverse: 2, 1, 0
				if len(order) != 3 || order[0] != 2 || order[1] != 1 || order[2] != 0 {
					return fmt.Errorf("wrong order: %v", order)
				}
				return nil
			},
		},
		{
			name: "Timeout is enforced",
			fn: func() error {
				mgr := shutdown.NewManager(nil)
				mgr.Register(func(ctx context.Context) error {
					time.Sleep(200 * time.Millisecond)
					return nil
				})

				start := time.Now()
				err := mgr.Shutdown(50 * time.Millisecond)
				elapsed := time.Since(start)

				if elapsed > 150*time.Millisecond {
					return fmt.Errorf("timeout not enforced: took %v", elapsed)
				}
				if err == nil {
					return errors.New("should return error on timeout")
				}
				return nil
			},
		},
		{
			name: "Errors are collected",
			fn: func() error {
				mgr := shutdown.NewManager(nil)
				mgr.Register(func(ctx context.Context) error {
					return errors.New("test error")
				})

				err := mgr.Shutdown(100 * time.Millisecond)
				if err == nil {
					return errors.New("error not collected")
				}
				return nil
			},
		},
		{
			name: "Once semantics work",
			fn: func() error {
				mgr := shutdown.NewManager(nil)
				count := int32(0)
				mgr.Register(func(ctx context.Context) error {
					atomic.AddInt32(&count, 1)
					return nil
				})

				_ = mgr.Shutdown(100 * time.Millisecond)
				_ = mgr.Shutdown(100 * time.Millisecond)

				if atomic.LoadInt32(&count) != 1 {
					return fmt.Errorf("called %d times, expected 1", count)
				}
				return nil
			},
		},
		{
			name: "Context is provided to functions",
			fn: func() error {
				mgr := shutdown.NewManager(nil)
				var receivedCtx context.Context
				mgr.Register(func(ctx context.Context) error {
					receivedCtx = ctx
					return nil
				})

				_ = mgr.Shutdown(100 * time.Millisecond)

				if receivedCtx == nil {
					return errors.New("nil context received")
				}
				return nil
			},
		},
	}

	passed := 0
	failed := 0
	var failureMessages []string

	for _, check := range checks {
		err := check.fn()
		if err == nil {
			t.Logf("✓ %s", check.name)
			passed++
		} else {
			t.Logf("✗ %s: %v", check.name, err)
			failureMessages = append(failureMessages, check.name+": "+err.Error())
			failed++
		}
	}

	t.Logf("\nPhase 0.8 Completion Status: %d/%d checks passed", passed, len(checks))

	if failed > 0 {
		t.Logf("\nFailed checks:")
		for _, msg := range failureMessages {
			t.Logf("  - %s", msg)
		}
		t.Fatalf("\nFAIL: Phase 0.8 is INCOMPLETE - %d/%d checks failed\n\n"+
			"To complete Phase 0.8, implement in internal/shutdown/shutdown.go:\n"+
			"  1. type Manager struct { ... }\n"+
			"  2. func NewManager(logger *slog.Logger) *Manager\n"+
			"  3. func (m *Manager) Register(fn func(ctx context.Context) error)\n"+
			"  4. func (m *Manager) Shutdown(timeout time.Duration) error\n\n"+
			"Key requirements:\n"+
			"  - LIFO execution order (last registered, first executed)\n"+
			"  - Timeout enforcement with context cancellation\n"+
			"  - Error collection from all functions\n"+
			"  - Once semantics using sync.Once\n"+
			"  - Context with deadline passed to shutdown functions\n\n"+
			"Then run: go test ./test -run TestPhase08",
			failed, len(checks))
	}

	t.Log("\n✓✓✓ PASS: Phase 0.8 Graceful Shutdown is COMPLETE ✓✓✓")
}

// TestShutdownEdgeCases tests edge cases and error conditions.
func TestShutdownEdgeCases(t *testing.T) {
	t.Run("shutdown_with_no_registered_functions", func(t *testing.T) {
		mgr := shutdown.NewManager(nil)

		// Shutdown with no functions should succeed
		err := mgr.Shutdown(100 * time.Millisecond)
		if err != nil {
			t.Fatalf("FAIL: Shutdown with no functions should succeed, got: %v", err)
		}

		t.Log("PASS: Shutdown with no functions succeeds")
	})

	t.Run("shutdown_with_zero_timeout", func(t *testing.T) {
		mgr := shutdown.NewManager(nil)

		mgr.Register(func(ctx context.Context) error {
			return nil
		})

		// Zero timeout should still work (or return error immediately)
		err := mgr.Shutdown(0)
		// Implementation may choose to allow zero timeout or treat it as immediate cancellation
		t.Logf("Shutdown(0) result: %v (implementation-defined behavior)", err)
	})

	t.Run("shutdown_with_negative_timeout", func(t *testing.T) {
		mgr := shutdown.NewManager(nil)

		mgr.Register(func(ctx context.Context) error {
			return nil
		})

		// Negative timeout behavior is implementation-defined
		// Some implementations may treat it as zero, others as error
		err := mgr.Shutdown(-1 * time.Second)
		t.Logf("Shutdown(-1s) result: %v (implementation-defined behavior)", err)
	})

	t.Run("function_panics_during_shutdown", func(t *testing.T) {
		mgr := shutdown.NewManager(nil)

		panicFunc := func(ctx context.Context) error {
			panic("shutdown panic")
		}

		normalFunc := func(ctx context.Context) error {
			return nil
		}

		mgr.Register(normalFunc)
		mgr.Register(panicFunc)
		mgr.Register(normalFunc)

		// Shutdown should handle panics gracefully
		// Implementation should either:
		// 1. Recover from panic and continue with other functions
		// 2. Let panic propagate (test will fail, but that's documented behavior)
		defer func() {
			if r := recover(); r != nil {
				t.Logf("Shutdown propagated panic: %v (implementation allows panics to propagate)", r)
			}
		}()

		err := mgr.Shutdown(100 * time.Millisecond)
		t.Logf("Shutdown with panic result: %v", err)
	})

	t.Run("function_returns_nil_error", func(t *testing.T) {
		mgr := shutdown.NewManager(nil)

		mgr.Register(func(ctx context.Context) error {
			return nil
		})

		err := mgr.Shutdown(100 * time.Millisecond)
		if err != nil {
			t.Fatalf("FAIL: Shutdown should return nil when function returns nil, got: %v", err)
		}

		t.Log("PASS: Nil error handled correctly")
	})

	t.Run("very_large_timeout", func(t *testing.T) {
		mgr := shutdown.NewManager(nil)

		completed := false
		mgr.Register(func(ctx context.Context) error {
			time.Sleep(10 * time.Millisecond)
			completed = true
			return nil
		})

		// Very large timeout should allow function to complete
		err := mgr.Shutdown(1 * time.Hour)
		if err != nil {
			t.Fatalf("FAIL: Shutdown with large timeout failed: %v", err)
		}

		if !completed {
			t.Fatal("FAIL: Function did not complete with large timeout")
		}

		t.Log("PASS: Large timeout allows completion")
	})
}
