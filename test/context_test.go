package test

import (
	"context"
	"errors"
	"testing"
	"time"

	chcontext "github.com/bmf/chaperone/internal/context"
)

// TestContextPropagation validates Phase 0.6: Context Propagation
//
// This test cannot be gamed because:
// 1. Uses real context.Context from standard library (not mocks)
// 2. Verifies actual cancellation signals propagate through context chains
// 3. Tests real timeout behavior with time.Sleep (measures actual wall-clock time)
// 4. Validates value propagation through derived contexts (real context.WithValue)
// 5. Confirms goroutines detect cancellation (uses real channels and select)
// 6. Checks parent/child context relationships with actual cancellation
// 7. Tests fail if context package is missing or values don't round-trip correctly
//
// An AI cannot fake this with stubs - the context propagation must actually work.

func TestContextPropagation(t *testing.T) {
	t.Run("context_creation", func(t *testing.T) {
		testContextCreation(t)
	})

	t.Run("request_id_storage_and_retrieval", func(t *testing.T) {
		testRequestIDStorageAndRetrieval(t)
	})

	t.Run("service_storage_and_retrieval", func(t *testing.T) {
		testServiceStorageAndRetrieval(t)
	})

	t.Run("hostname_storage_and_retrieval", func(t *testing.T) {
		testHostnameStorageAndRetrieval(t)
	})

	t.Run("client_id_storage_and_retrieval", func(t *testing.T) {
		testClientIDStorageAndRetrieval(t)
	})

	t.Run("multiple_values_on_same_context", func(t *testing.T) {
		testMultipleValuesOnSameContext(t)
	})

	t.Run("missing_values_return_empty", func(t *testing.T) {
		testMissingValuesReturnEmpty(t)
	})

	t.Run("value_propagation_through_derived_contexts", func(t *testing.T) {
		testValuePropagationThroughDerivedContexts(t)
	})

	t.Run("cancellation_propagates_to_children", func(t *testing.T) {
		testCancellationPropagatesToChildren(t)
	})

	t.Run("child_cancellation_does_not_affect_parent", func(t *testing.T) {
		testChildCancellationDoesNotAffectParent(t)
	})

	t.Run("timeout_behavior", func(t *testing.T) {
		testTimeoutBehavior(t)
	})

	t.Run("goroutine_respects_cancellation", func(t *testing.T) {
		testGoroutineRespectsCancellation(t)
	})
}

// testContextCreation verifies:
// - NewRequestContext creates a cancellable context
// - Returns non-nil context and cancel function
// - Returned context is not the background context
// - Cancel function can be called without panic
func testContextCreation(t *testing.T) {
	// Test 1: NewRequestContext returns valid context and cancel function
	ctx, cancel := chcontext.NewRequestContext()

	if ctx == nil {
		t.Fatal("FAIL: NewRequestContext() returned nil context - must return valid context")
	}

	if cancel == nil {
		t.Fatal("FAIL: NewRequestContext() returned nil cancel function - must return valid cancel function")
	}

	// Test 2: Context should not be the background context (should be derived)
	if ctx == context.Background() {
		t.Error("FAIL: NewRequestContext() returned background context - should return derived context")
	}

	// Test 3: Cancel function should be callable without panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("FAIL: Calling cancel() caused panic: %v", r)
		}
	}()
	cancel()

	// Test 4: After cancellation, context should report Done
	select {
	case <-ctx.Done():
		// Expected - context is cancelled
		t.Log("PASS: Context properly cancelled")
	case <-time.After(10 * time.Millisecond):
		t.Error("FAIL: Context.Done() did not signal after cancel() was called")
	}

	// Test 5: Context.Err() should return error after cancellation
	if ctx.Err() == nil {
		t.Error("FAIL: Context.Err() should return error after cancellation, got nil")
	}

	// Test 6: Error should be context.Canceled
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Errorf("FAIL: Context.Err() should be context.Canceled, got: %v", ctx.Err())
	}

	t.Log("PASS: NewRequestContext creates cancellable context correctly")
}

// testRequestIDStorageAndRetrieval verifies:
// - WithRequestID stores a request ID in context
// - RequestID retrieves the same ID
// - Empty context returns empty string
// - Multiple IDs can be stored in different contexts
func testRequestIDStorageAndRetrieval(t *testing.T) {
	// Test 1: WithRequestID stores ID, RequestID retrieves it
	ctx := context.Background()
	testID := "req-12345-abcde"

	ctxWithID := chcontext.WithRequestID(ctx, testID)
	if ctxWithID == nil {
		t.Fatal("FAIL: WithRequestID returned nil context")
	}

	retrievedID := chcontext.RequestID(ctxWithID)
	if retrievedID != testID {
		t.Fatalf("FAIL: RequestID round-trip failed. Set: %q, Got: %q", testID, retrievedID)
	}

	// Test 2: Empty context returns empty string
	emptyCtx := context.Background()
	emptyID := chcontext.RequestID(emptyCtx)
	if emptyID != "" {
		t.Errorf("FAIL: RequestID on empty context should return empty string, got: %q", emptyID)
	}

	// Test 3: Different contexts can have different IDs
	ctx1 := chcontext.WithRequestID(context.Background(), "id-1")
	ctx2 := chcontext.WithRequestID(context.Background(), "id-2")

	id1 := chcontext.RequestID(ctx1)
	id2 := chcontext.RequestID(ctx2)

	if id1 == id2 {
		t.Errorf("FAIL: Different contexts should have different IDs. Both got: %q", id1)
	}

	if id1 != "id-1" {
		t.Errorf("FAIL: Context 1 has wrong ID. Expected: id-1, Got: %q", id1)
	}

	if id2 != "id-2" {
		t.Errorf("FAIL: Context 2 has wrong ID. Expected: id-2, Got: %q", id2)
	}

	// Test 4: Overwriting request ID in derived context
	originalCtx := chcontext.WithRequestID(context.Background(), "original-id")
	overwrittenCtx := chcontext.WithRequestID(originalCtx, "new-id")

	originalID := chcontext.RequestID(originalCtx)
	newID := chcontext.RequestID(overwrittenCtx)

	if originalID != "original-id" {
		t.Errorf("FAIL: Original context ID changed. Expected: original-id, Got: %q", originalID)
	}

	if newID != "new-id" {
		t.Errorf("FAIL: Overwritten context ID wrong. Expected: new-id, Got: %q", newID)
	}

	t.Log("PASS: RequestID storage and retrieval works correctly")
}

// testServiceStorageAndRetrieval verifies:
// - WithService stores a service name in context
// - Service retrieves the same name
// - Empty context returns empty string
func testServiceStorageAndRetrieval(t *testing.T) {
	// Test 1: WithService stores name, Service retrieves it
	ctx := context.Background()
	serviceName := "openai-api"

	ctxWithService := chcontext.WithService(ctx, serviceName)
	if ctxWithService == nil {
		t.Fatal("FAIL: WithService returned nil context")
	}

	retrievedService := chcontext.Service(ctxWithService)
	if retrievedService != serviceName {
		t.Fatalf("FAIL: Service round-trip failed. Set: %q, Got: %q", serviceName, retrievedService)
	}

	// Test 2: Empty context returns empty string
	emptyCtx := context.Background()
	emptyService := chcontext.Service(emptyCtx)
	if emptyService != "" {
		t.Errorf("FAIL: Service on empty context should return empty string, got: %q", emptyService)
	}

	// Test 3: Different services in different contexts
	ctx1 := chcontext.WithService(context.Background(), "anthropic-api")
	ctx2 := chcontext.WithService(context.Background(), "slack-api")

	service1 := chcontext.Service(ctx1)
	service2 := chcontext.Service(ctx2)

	if service1 != "anthropic-api" {
		t.Errorf("FAIL: Context 1 service wrong. Expected: anthropic-api, Got: %q", service1)
	}

	if service2 != "slack-api" {
		t.Errorf("FAIL: Context 2 service wrong. Expected: slack-api, Got: %q", service2)
	}

	t.Log("PASS: Service storage and retrieval works correctly")
}

// testHostnameStorageAndRetrieval verifies:
// - WithHostname stores a hostname in context
// - Hostname retrieves the same hostname
// - Empty context returns empty string
func testHostnameStorageAndRetrieval(t *testing.T) {
	// Test 1: WithHostname stores hostname, Hostname retrieves it
	ctx := context.Background()
	hostname := "api.openai.com"

	ctxWithHostname := chcontext.WithHostname(ctx, hostname)
	if ctxWithHostname == nil {
		t.Fatal("FAIL: WithHostname returned nil context")
	}

	retrievedHostname := chcontext.Hostname(ctxWithHostname)
	if retrievedHostname != hostname {
		t.Fatalf("FAIL: Hostname round-trip failed. Set: %q, Got: %q", hostname, retrievedHostname)
	}

	// Test 2: Empty context returns empty string
	emptyCtx := context.Background()
	emptyHostname := chcontext.Hostname(emptyCtx)
	if emptyHostname != "" {
		t.Errorf("FAIL: Hostname on empty context should return empty string, got: %q", emptyHostname)
	}

	// Test 3: Different hostnames in different contexts
	ctx1 := chcontext.WithHostname(context.Background(), "api.anthropic.com")
	ctx2 := chcontext.WithHostname(context.Background(), "slack.com")

	hostname1 := chcontext.Hostname(ctx1)
	hostname2 := chcontext.Hostname(ctx2)

	if hostname1 != "api.anthropic.com" {
		t.Errorf("FAIL: Context 1 hostname wrong. Expected: api.anthropic.com, Got: %q", hostname1)
	}

	if hostname2 != "slack.com" {
		t.Errorf("FAIL: Context 2 hostname wrong. Expected: slack.com, Got: %q", hostname2)
	}

	t.Log("PASS: Hostname storage and retrieval works correctly")
}

// testClientIDStorageAndRetrieval verifies:
// - WithClientID stores a client identifier in context
// - ClientID retrieves the same identifier
// - Empty context returns empty string
func testClientIDStorageAndRetrieval(t *testing.T) {
	// Test 1: WithClientID stores ID, ClientID retrieves it
	ctx := context.Background()
	clientID := "client-abc-123"

	ctxWithClientID := chcontext.WithClientID(ctx, clientID)
	if ctxWithClientID == nil {
		t.Fatal("FAIL: WithClientID returned nil context")
	}

	retrievedClientID := chcontext.ClientID(ctxWithClientID)
	if retrievedClientID != clientID {
		t.Fatalf("FAIL: ClientID round-trip failed. Set: %q, Got: %q", clientID, retrievedClientID)
	}

	// Test 2: Empty context returns empty string
	emptyCtx := context.Background()
	emptyClientID := chcontext.ClientID(emptyCtx)
	if emptyClientID != "" {
		t.Errorf("FAIL: ClientID on empty context should return empty string, got: %q", emptyClientID)
	}

	// Test 3: Different client IDs in different contexts
	ctx1 := chcontext.WithClientID(context.Background(), "client-1")
	ctx2 := chcontext.WithClientID(context.Background(), "client-2")

	clientID1 := chcontext.ClientID(ctx1)
	clientID2 := chcontext.ClientID(ctx2)

	if clientID1 != "client-1" {
		t.Errorf("FAIL: Context 1 client ID wrong. Expected: client-1, Got: %q", clientID1)
	}

	if clientID2 != "client-2" {
		t.Errorf("FAIL: Context 2 client ID wrong. Expected: client-2, Got: %q", clientID2)
	}

	t.Log("PASS: ClientID storage and retrieval works correctly")
}

// testMultipleValuesOnSameContext verifies:
// - Multiple values can be stored on the same context
// - All values can be retrieved independently
// - Setting values in different orders works correctly
func testMultipleValuesOnSameContext(t *testing.T) {
	// Test 1: Add all values to one context
	ctx := context.Background()
	ctx = chcontext.WithRequestID(ctx, "req-123")
	ctx = chcontext.WithService(ctx, "test-service")
	ctx = chcontext.WithHostname(ctx, "test.example.com")
	ctx = chcontext.WithClientID(ctx, "client-456")

	// Verify all values are present
	if chcontext.RequestID(ctx) != "req-123" {
		t.Errorf("FAIL: RequestID not preserved. Got: %q", chcontext.RequestID(ctx))
	}

	if chcontext.Service(ctx) != "test-service" {
		t.Errorf("FAIL: Service not preserved. Got: %q", chcontext.Service(ctx))
	}

	if chcontext.Hostname(ctx) != "test.example.com" {
		t.Errorf("FAIL: Hostname not preserved. Got: %q", chcontext.Hostname(ctx))
	}

	if chcontext.ClientID(ctx) != "client-456" {
		t.Errorf("FAIL: ClientID not preserved. Got: %q", chcontext.ClientID(ctx))
	}

	// Test 2: Add values in different order
	ctx2 := context.Background()
	ctx2 = chcontext.WithHostname(ctx2, "host2.com")
	ctx2 = chcontext.WithClientID(ctx2, "client-789")
	ctx2 = chcontext.WithRequestID(ctx2, "req-456")
	ctx2 = chcontext.WithService(ctx2, "service2")

	if chcontext.RequestID(ctx2) != "req-456" {
		t.Errorf("FAIL: RequestID not preserved in different order. Got: %q", chcontext.RequestID(ctx2))
	}

	if chcontext.Service(ctx2) != "service2" {
		t.Errorf("FAIL: Service not preserved in different order. Got: %q", chcontext.Service(ctx2))
	}

	if chcontext.Hostname(ctx2) != "host2.com" {
		t.Errorf("FAIL: Hostname not preserved in different order. Got: %q", chcontext.Hostname(ctx2))
	}

	if chcontext.ClientID(ctx2) != "client-789" {
		t.Errorf("FAIL: ClientID not preserved in different order. Got: %q", chcontext.ClientID(ctx2))
	}

	t.Log("PASS: Multiple values can coexist on same context")
}

// testMissingValuesReturnEmpty verifies:
// - Retrieving values from context without them returns empty strings
// - No panics occur when retrieving missing values
func testMissingValuesReturnEmpty(t *testing.T) {
	ctx := context.Background()

	// Test all retrieval functions return empty string on empty context
	if id := chcontext.RequestID(ctx); id != "" {
		t.Errorf("FAIL: RequestID on empty context should return empty, got: %q", id)
	}

	if svc := chcontext.Service(ctx); svc != "" {
		t.Errorf("FAIL: Service on empty context should return empty, got: %q", svc)
	}

	if host := chcontext.Hostname(ctx); host != "" {
		t.Errorf("FAIL: Hostname on empty context should return empty, got: %q", host)
	}

	if client := chcontext.ClientID(ctx); client != "" {
		t.Errorf("FAIL: ClientID on empty context should return empty, got: %q", client)
	}

	// Test context with only some values set
	partialCtx := chcontext.WithRequestID(context.Background(), "req-only")

	if chcontext.RequestID(partialCtx) != "req-only" {
		t.Errorf("FAIL: RequestID should be present, got: %q", chcontext.RequestID(partialCtx))
	}

	if svc := chcontext.Service(partialCtx); svc != "" {
		t.Errorf("FAIL: Service on partial context should return empty, got: %q", svc)
	}

	if host := chcontext.Hostname(partialCtx); host != "" {
		t.Errorf("FAIL: Hostname on partial context should return empty, got: %q", host)
	}

	if client := chcontext.ClientID(partialCtx); client != "" {
		t.Errorf("FAIL: ClientID on partial context should return empty, got: %q", client)
	}

	t.Log("PASS: Missing values return empty strings correctly")
}

// testValuePropagationThroughDerivedContexts verifies:
// - Values propagate from parent to child contexts
// - Child contexts can add additional values
// - Parent values remain accessible in derived contexts
func testValuePropagationThroughDerivedContexts(t *testing.T) {
	// Create parent context with request ID
	parent := chcontext.WithRequestID(context.Background(), "parent-req-id")

	// Derive child context with additional service name
	child := chcontext.WithService(parent, "child-service")

	// Test 1: Child should have both parent and child values
	if chcontext.RequestID(child) != "parent-req-id" {
		t.Errorf("FAIL: Child context lost parent RequestID. Got: %q", chcontext.RequestID(child))
	}

	if chcontext.Service(child) != "child-service" {
		t.Errorf("FAIL: Child context Service wrong. Got: %q", chcontext.Service(child))
	}

	// Test 2: Parent should not have child values
	if chcontext.Service(parent) != "" {
		t.Errorf("FAIL: Parent context should not have child Service. Got: %q", chcontext.Service(parent))
	}

	// Test 3: Multi-level derivation
	grandchild := chcontext.WithHostname(child, "grandchild-host")

	if chcontext.RequestID(grandchild) != "parent-req-id" {
		t.Errorf("FAIL: Grandchild lost parent RequestID. Got: %q", chcontext.RequestID(grandchild))
	}

	if chcontext.Service(grandchild) != "child-service" {
		t.Errorf("FAIL: Grandchild lost child Service. Got: %q", chcontext.Service(grandchild))
	}

	if chcontext.Hostname(grandchild) != "grandchild-host" {
		t.Errorf("FAIL: Grandchild Hostname wrong. Got: %q", chcontext.Hostname(grandchild))
	}

	// Test 4: Standard library context derivation preserves values
	parentWithID := chcontext.WithRequestID(context.Background(), "std-req-id")
	derivedWithTimeout, cancel := context.WithTimeout(parentWithID, 5*time.Second)
	defer cancel()

	if chcontext.RequestID(derivedWithTimeout) != "std-req-id" {
		t.Errorf("FAIL: Context derived with WithTimeout lost RequestID. Got: %q",
			chcontext.RequestID(derivedWithTimeout))
	}

	t.Log("PASS: Values propagate correctly through derived contexts")
}

// testCancellationPropagatesToChildren verifies:
// - Cancelling parent context cancels all children
// - Child contexts detect parent cancellation
// - Done channel signals on parent cancellation
func testCancellationPropagatesToChildren(t *testing.T) {
	// Create parent context with cancel
	parent, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()

	// Add request ID to make it traceable
	parent = chcontext.WithRequestID(parent, "cancel-test-id")

	// Create child context
	child := chcontext.WithService(parent, "test-service")

	// Create grandchild
	grandchild, grandchildCancel := context.WithCancel(child)
	defer grandchildCancel()

	// Before cancellation, no context should be done
	select {
	case <-parent.Done():
		t.Fatal("FAIL: Parent context Done before cancel")
	case <-child.Done():
		t.Fatal("FAIL: Child context Done before cancel")
	case <-grandchild.Done():
		t.Fatal("FAIL: Grandchild context Done before cancel")
	default:
		// Expected - no context is done yet
	}

	// Cancel parent
	parentCancel()

	// All contexts should be done
	select {
	case <-parent.Done():
		// Expected
	case <-time.After(10 * time.Millisecond):
		t.Error("FAIL: Parent context not done after cancel")
	}

	select {
	case <-child.Done():
		// Expected
	case <-time.After(10 * time.Millisecond):
		t.Error("FAIL: Child context not done after parent cancel")
	}

	select {
	case <-grandchild.Done():
		// Expected
	case <-time.After(10 * time.Millisecond):
		t.Error("FAIL: Grandchild context not done after parent cancel")
	}

	// All contexts should report error
	if parent.Err() == nil {
		t.Error("FAIL: Parent Err() should not be nil after cancel")
	}

	if child.Err() == nil {
		t.Error("FAIL: Child Err() should not be nil after parent cancel")
	}

	if grandchild.Err() == nil {
		t.Error("FAIL: Grandchild Err() should not be nil after parent cancel")
	}

	// Values should still be accessible even after cancellation
	if chcontext.RequestID(child) != "cancel-test-id" {
		t.Error("FAIL: RequestID lost after cancellation")
	}

	if chcontext.Service(child) != "test-service" {
		t.Error("FAIL: Service lost after cancellation")
	}

	t.Log("PASS: Cancellation propagates correctly to children")
}

// testChildCancellationDoesNotAffectParent verifies:
// - Cancelling child context does NOT cancel parent
// - Parent remains active after child cancellation
// - Sibling contexts are not affected
func testChildCancellationDoesNotAffectParent(t *testing.T) {
	// Create parent context
	parent, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()

	parent = chcontext.WithRequestID(parent, "parent-id")

	// Create two child contexts
	child1, child1Cancel := context.WithCancel(parent)
	defer child1Cancel()
	child1 = chcontext.WithService(child1, "service-1")

	child2, child2Cancel := context.WithCancel(parent)
	defer child2Cancel()
	child2 = chcontext.WithService(child2, "service-2")

	// Cancel child1
	child1Cancel()

	// child1 should be done
	select {
	case <-child1.Done():
		// Expected
	case <-time.After(10 * time.Millisecond):
		t.Error("FAIL: Child1 not done after cancel")
	}

	// Parent should NOT be done
	select {
	case <-parent.Done():
		t.Error("FAIL: Parent done after child cancel")
	default:
		// Expected
	}

	// child2 should NOT be done
	select {
	case <-child2.Done():
		t.Error("FAIL: Child2 done after sibling cancel")
	default:
		// Expected
	}

	// Parent should not have error
	if parent.Err() != nil {
		t.Errorf("FAIL: Parent Err() should be nil, got: %v", parent.Err())
	}

	// child2 should not have error
	if child2.Err() != nil {
		t.Errorf("FAIL: Child2 Err() should be nil, got: %v", child2.Err())
	}

	// Values should still be accessible
	if chcontext.RequestID(parent) != "parent-id" {
		t.Error("FAIL: Parent RequestID affected by child cancel")
	}

	if chcontext.Service(child2) != "service-2" {
		t.Error("FAIL: Child2 Service affected by sibling cancel")
	}

	t.Log("PASS: Child cancellation does not affect parent")
}

// testTimeoutBehavior verifies:
// - Context with timeout eventually times out
// - Timeout cancellation is detected correctly
// - Error is context.DeadlineExceeded
// - Values remain accessible after timeout
func testTimeoutBehavior(t *testing.T) {
	// Create context with very short timeout
	parent := chcontext.WithRequestID(context.Background(), "timeout-test-id")
	ctx, cancel := context.WithTimeout(parent, 20*time.Millisecond)
	defer cancel()

	ctx = chcontext.WithService(ctx, "timeout-service")

	// Context should not be done immediately
	select {
	case <-ctx.Done():
		t.Error("FAIL: Context done immediately (timeout should not be instant)")
	default:
		// Expected
	}

	// Wait for timeout to occur
	start := time.Now()
	<-ctx.Done()
	elapsed := time.Since(start)

	// Should have taken approximately 20ms
	if elapsed < 15*time.Millisecond {
		t.Errorf("FAIL: Context timed out too quickly: %v (expected ~20ms)", elapsed)
	}

	if elapsed > 100*time.Millisecond {
		t.Errorf("FAIL: Context timeout took too long: %v (expected ~20ms)", elapsed)
	}

	// Error should be DeadlineExceeded
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Errorf("FAIL: Expected context.DeadlineExceeded, got: %v", ctx.Err())
	}

	// Values should still be accessible
	if chcontext.RequestID(ctx) != "timeout-test-id" {
		t.Error("FAIL: RequestID lost after timeout")
	}

	if chcontext.Service(ctx) != "timeout-service" {
		t.Error("FAIL: Service lost after timeout")
	}

	t.Logf("PASS: Timeout behavior correct (timed out after %v)", elapsed)
}

// testGoroutineRespectsCancellation verifies:
// - Goroutines can detect context cancellation
// - Select statement with ctx.Done() works correctly
// - Goroutine exits when context is cancelled
// - Real concurrent behavior is tested
func testGoroutineRespectsCancellation(t *testing.T) {
	ctx, cancel := chcontext.NewRequestContext()
	ctx = chcontext.WithService(ctx, "goroutine-test")

	// Channel to signal goroutine completion
	done := make(chan bool, 1)
	cancelled := make(chan bool, 1)

	// Start goroutine that should respect cancellation
	go func() {
		defer func() { done <- true }()

		// Simulate work with cancellation check
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				// Context cancelled - exit gracefully
				cancelled <- true
				return
			case <-ticker.C:
				// Continue working
			}
		}
	}()

	// Let goroutine run for a bit
	time.Sleep(15 * time.Millisecond)

	// Cancel context
	cancel()

	// Goroutine should detect cancellation and exit
	select {
	case <-cancelled:
		// Expected - goroutine detected cancellation
	case <-time.After(50 * time.Millisecond):
		t.Error("FAIL: Goroutine did not detect cancellation within 50ms")
	}

	// Goroutine should complete
	select {
	case <-done:
		// Expected
	case <-time.After(50 * time.Millisecond):
		t.Error("FAIL: Goroutine did not exit after cancellation")
	}

	// Context should report cancelled error
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Errorf("FAIL: Expected context.Canceled, got: %v", ctx.Err())
	}

	t.Log("PASS: Goroutine respects cancellation correctly")
}

// TestContextPatterns validates documented usage patterns for Phase 0.6
//
// These tests verify the patterns that should be followed throughout the codebase:
// - Every public function accepts ctx as first parameter
// - Every HTTP handler creates context from request
// - Every outbound call passes context
// - Every goroutine respects cancellation
func TestContextPatterns(t *testing.T) {
	t.Run("function_accepts_context_first_parameter", func(t *testing.T) {
		testFunctionAcceptsContextFirstParameter(t)
	})

	t.Run("context_chain_through_function_calls", func(t *testing.T) {
		testContextChainThroughFunctionCalls(t)
	})

	t.Run("multiple_goroutines_share_context", func(t *testing.T) {
		testMultipleGoroutinesShareContext(t)
	})
}

// testFunctionAcceptsContextFirstParameter verifies:
// - Simulated public function accepts context as first parameter
// - Context values are accessible within the function
// - Function respects context cancellation
func testFunctionAcceptsContextFirstParameter(t *testing.T) {
	// Simulate a public function that accepts context
	processRequest := func(ctx context.Context, data string) (string, error) {
		// Check if context is cancelled before processing
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		// Access context values
		requestID := chcontext.RequestID(ctx)
		service := chcontext.Service(ctx)

		// Do some work
		result := "processed: " + data + " (req=" + requestID + ", svc=" + service + ")"
		return result, nil
	}

	// Test 1: Function works with valid context
	ctx := chcontext.WithRequestID(context.Background(), "func-test-id")
	ctx = chcontext.WithService(ctx, "test-service")

	result, err := processRequest(ctx, "test-data")
	if err != nil {
		t.Fatalf("FAIL: Function returned error: %v", err)
	}

	if result == "" {
		t.Error("FAIL: Function returned empty result")
	}

	if chcontext.RequestID(ctx) != "func-test-id" {
		t.Error("FAIL: Context values not accessible in function")
	}

	// Test 2: Function respects cancellation
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = processRequest(cancelCtx, "test-data")
	if err == nil {
		t.Error("FAIL: Function should return error when context is cancelled")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("FAIL: Expected context.Canceled error, got: %v", err)
	}

	t.Log("PASS: Function pattern (ctx as first parameter) works correctly")
}

// testContextChainThroughFunctionCalls verifies:
// - Context propagates through chain of function calls
// - Values remain accessible at each level
// - Cancellation propagates through the chain
func testContextChainThroughFunctionCalls(t *testing.T) {
	// Simulate nested function calls
	level3 := func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if chcontext.RequestID(ctx) == "" {
			return errors.New("missing request ID at level 3")
		}
		return nil
	}

	level2 := func(ctx context.Context) error {
		ctx = chcontext.WithService(ctx, "level-2-service")
		return level3(ctx)
	}

	level1 := func(ctx context.Context) error {
		ctx = chcontext.WithHostname(ctx, "level-1-host")
		return level2(ctx)
	}

	// Test 1: Context propagates through all levels
	rootCtx := chcontext.WithRequestID(context.Background(), "chain-test-id")

	err := level1(rootCtx)
	if err != nil {
		t.Fatalf("FAIL: Error in function chain: %v", err)
	}

	// Test 2: Cancellation propagates through chain
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancelCtx = chcontext.WithRequestID(cancelCtx, "cancel-chain-id")

	cancel() // Cancel before calling chain

	err = level1(cancelCtx)
	if err == nil {
		t.Error("FAIL: Function chain should detect cancellation")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("FAIL: Expected context.Canceled, got: %v", err)
	}

	t.Log("PASS: Context chains through function calls correctly")
}

// testMultipleGoroutinesShareContext verifies:
// - Multiple goroutines can share the same context
// - All goroutines detect cancellation
// - Values are accessible in all goroutines
func testMultipleGoroutinesShareContext(t *testing.T) {
	ctx, cancel := chcontext.NewRequestContext()
	ctx = chcontext.WithRequestID(ctx, "multi-goroutine-id")
	ctx = chcontext.WithService(ctx, "multi-goroutine-service")

	numGoroutines := 5
	done := make(chan bool, numGoroutines)

	// Start multiple goroutines sharing the same context
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			// Verify context values are accessible
			if chcontext.RequestID(ctx) != "multi-goroutine-id" {
				t.Errorf("FAIL: Goroutine %d cannot access RequestID", id)
			}

			// Wait for cancellation
			<-ctx.Done()

			// Signal completion
			done <- true
		}(i)
	}

	// Let goroutines start
	time.Sleep(10 * time.Millisecond)

	// Cancel shared context
	cancel()

	// All goroutines should complete
	completedCount := 0
	timeout := time.After(100 * time.Millisecond)

	for completedCount < numGoroutines {
		select {
		case <-done:
			completedCount++
		case <-timeout:
			t.Fatalf("FAIL: Only %d/%d goroutines completed after cancellation",
				completedCount, numGoroutines)
		}
	}

	t.Logf("PASS: All %d goroutines detected cancellation correctly", numGoroutines)
}

// TestPhase06Completion is a meta-test that checks if Phase 0.6 is complete.
//
// This runs all validation checks and reports overall status.
func TestPhase06Completion(t *testing.T) {
	checks := []struct {
		name string
		fn   func() error
	}{
		{
			name: "NewRequestContext creates cancellable context",
			fn: func() error {
				ctx, cancel := chcontext.NewRequestContext()
				defer cancel()
				if ctx == nil {
					return errors.New("NewRequestContext returned nil")
				}
				return nil
			},
		},
		{
			name: "WithRequestID/RequestID round-trip works",
			fn: func() error {
				ctx := chcontext.WithRequestID(context.Background(), "test-id")
				if chcontext.RequestID(ctx) != "test-id" {
					return errors.New("RequestID round-trip failed")
				}
				return nil
			},
		},
		{
			name: "WithService/Service round-trip works",
			fn: func() error {
				ctx := chcontext.WithService(context.Background(), "test-service")
				if chcontext.Service(ctx) != "test-service" {
					return errors.New("Service round-trip failed")
				}
				return nil
			},
		},
		{
			name: "WithHostname/Hostname round-trip works",
			fn: func() error {
				ctx := chcontext.WithHostname(context.Background(), "test-host")
				if chcontext.Hostname(ctx) != "test-host" {
					return errors.New("Hostname round-trip failed")
				}
				return nil
			},
		},
		{
			name: "WithClientID/ClientID round-trip works",
			fn: func() error {
				ctx := chcontext.WithClientID(context.Background(), "test-client")
				if chcontext.ClientID(ctx) != "test-client" {
					return errors.New("ClientID round-trip failed")
				}
				return nil
			},
		},
		{
			name: "Multiple values coexist on same context",
			fn: func() error {
				ctx := context.Background()
				ctx = chcontext.WithRequestID(ctx, "req")
				ctx = chcontext.WithService(ctx, "svc")
				ctx = chcontext.WithHostname(ctx, "host")
				ctx = chcontext.WithClientID(ctx, "client")

				if chcontext.RequestID(ctx) != "req" ||
					chcontext.Service(ctx) != "svc" ||
					chcontext.Hostname(ctx) != "host" ||
					chcontext.ClientID(ctx) != "client" {
					return errors.New("multiple values not preserved")
				}
				return nil
			},
		},
		{
			name: "Missing values return empty strings",
			fn: func() error {
				ctx := context.Background()
				if chcontext.RequestID(ctx) != "" ||
					chcontext.Service(ctx) != "" ||
					chcontext.Hostname(ctx) != "" ||
					chcontext.ClientID(ctx) != "" {
					return errors.New("missing values do not return empty")
				}
				return nil
			},
		},
		{
			name: "Values propagate through derived contexts",
			fn: func() error {
				parent := chcontext.WithRequestID(context.Background(), "parent-id")
				child := chcontext.WithService(parent, "child-svc")
				if chcontext.RequestID(child) != "parent-id" {
					return errors.New("parent value not propagated to child")
				}
				return nil
			},
		},
		{
			name: "Cancellation propagates to children",
			fn: func() error {
				parent, cancel := context.WithCancel(context.Background())
				child := chcontext.WithService(parent, "test")
				cancel()
				select {
				case <-child.Done():
					return nil
				case <-time.After(10 * time.Millisecond):
					return errors.New("cancellation not propagated")
				}
			},
		},
		{
			name: "Child cancellation does not affect parent",
			fn: func() error {
				parent, parentCancel := context.WithCancel(context.Background())
				defer parentCancel()
				_, childCancel := context.WithCancel(parent)
				childCancel()
				select {
				case <-parent.Done():
					return errors.New("parent cancelled when child cancelled")
				default:
					return nil
				}
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

	t.Logf("\nPhase 0.6 Completion Status: %d/%d checks passed", passed, len(checks))

	if failed > 0 {
		t.Logf("\nFailed checks:")
		for _, msg := range failureMessages {
			t.Logf("  - %s", msg)
		}
		t.Fatalf("\nFAIL: Phase 0.6 is INCOMPLETE - %d/%d checks failed\n\n"+
			"To complete Phase 0.6, implement in internal/context/context.go:\n"+
			"  1. NewRequestContext() (context.Context, context.CancelFunc)\n"+
			"  2. WithRequestID(ctx context.Context, id string) context.Context\n"+
			"  3. RequestID(ctx context.Context) string\n"+
			"  4. WithService(ctx context.Context, service string) context.Context\n"+
			"  5. Service(ctx context.Context) string\n"+
			"  6. WithHostname(ctx context.Context, hostname string) context.Context\n"+
			"  7. Hostname(ctx context.Context) string\n"+
			"  8. WithClientID(ctx context.Context, clientID string) context.Context\n"+
			"  9. ClientID(ctx context.Context) string\n\n"+
			"Then run: go test ./test -run TestPhase06",
			failed, len(checks))
	}

	t.Log("\n✓✓✓ PASS: Phase 0.6 Context Propagation is COMPLETE ✓✓✓")
}
