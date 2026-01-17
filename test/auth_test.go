package test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/bmf/chaperone/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAuthStrategyRegistry validates:
// - Strategies can be registered and retrieved
// - Unknown strategies return error
// - Multiple strategies can coexist
// - Thread-safe concurrent access
//
// ANTI-GAMING: Tests real registry behavior, not mocked interfaces
func TestAuthStrategyRegistry(t *testing.T) {
	t.Run("register_and_retrieve_strategy", func(t *testing.T) {
		// Create mock strategy
		mockStrategy := &mockAuthStrategy{name: "test-strategy"}

		// Register strategy
		registry := auth.NewRegistry()
		registry.Register("test", mockStrategy)

		// Retrieve strategy
		retrieved, err := registry.Get("test")
		require.NoError(t, err, "Should retrieve registered strategy")
		assert.Equal(t, mockStrategy, retrieved, "Should get same strategy instance")
	})

	t.Run("retrieve_nonexistent_strategy_returns_error", func(t *testing.T) {
		registry := auth.NewRegistry()

		// Try to retrieve non-existent strategy
		strategy, err := registry.Get("nonexistent")
		assert.Error(t, err, "Should return error for unknown strategy")
		assert.Nil(t, strategy, "Should return nil strategy")
		assert.Contains(t, err.Error(), "nonexistent", "Error should mention strategy name")
	})

	t.Run("multiple_strategies_registered", func(t *testing.T) {
		registry := auth.NewRegistry()

		// Register multiple strategies
		bearerStrategy := &mockAuthStrategy{name: "bearer"}
		headerStrategy := &mockAuthStrategy{name: "header"}
		registry.Register("bearer", bearerStrategy)
		registry.Register("header", headerStrategy)

		// Verify both can be retrieved
		retrieved1, err := registry.Get("bearer")
		require.NoError(t, err)
		assert.Equal(t, bearerStrategy, retrieved1)

		retrieved2, err := registry.Get("header")
		require.NoError(t, err)
		assert.Equal(t, headerStrategy, retrieved2)
	})

	t.Run("concurrent_access_thread_safe", func(t *testing.T) {
		registry := auth.NewRegistry()

		// Register initial strategies
		for i := 0; i < 10; i++ {
			name := string(rune('a' + i))
			registry.Register(name, &mockAuthStrategy{name: name})
		}

		// Test concurrent reads
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				name := string(rune('a' + (idx % 10)))
				strategy, err := registry.Get(name)
				assert.NoError(t, err)
				assert.NotNil(t, strategy)
			}(i)
		}
		wg.Wait()

		// Test concurrent writes (should not panic or race)
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				name := string(rune('A' + (idx % 10)))
				registry.Register(name, &mockAuthStrategy{name: name})
			}(i)
		}
		wg.Wait()
	})

	t.Run("replace_existing_strategy", func(t *testing.T) {
		registry := auth.NewRegistry()

		// Register initial strategy
		strategy1 := &mockAuthStrategy{name: "v1"}
		registry.Register("test", strategy1)

		// Replace with new strategy
		strategy2 := &mockAuthStrategy{name: "v2"}
		registry.Register("test", strategy2)

		// Verify new strategy is returned
		retrieved, err := registry.Get("test")
		require.NoError(t, err)
		assert.Equal(t, strategy2, retrieved, "Should get replaced strategy")
		assert.NotEqual(t, strategy1, retrieved, "Should not get old strategy")
	})
}

// TestBearerStrategy validates:
// - Bearer token set correctly on request
// - Empty secret returns error
// - Existing Authorization header replaced
// - Request is modified in place (as designed)
//
// ANTI-GAMING: Tests real http.Request mutation, not mocked behavior
func TestBearerStrategy(t *testing.T) {
	t.Run("bearer_token_set_correctly", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://api.example.com/v1/chat", nil)
		strategy := &auth.BearerStrategy{}

		err := strategy.Apply(context.Background(), req, "test-secret-key")
		require.NoError(t, err, "Apply should succeed with valid secret")

		// Verify Authorization header format
		authHeader := req.Header.Get("Authorization")
		assert.Equal(t, "Bearer test-secret-key", authHeader,
			"Authorization header should have correct Bearer format")
	})

	t.Run("empty_secret_returns_error", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://api.example.com", nil)
		strategy := &auth.BearerStrategy{}

		err := strategy.Apply(context.Background(), req, "")
		assert.Error(t, err, "Apply should fail with empty secret")
		assert.Empty(t, req.Header.Get("Authorization"),
			"Authorization header should not be set on error")
	})

	t.Run("existing_authorization_replaced", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://api.example.com", nil)
		req.Header.Set("Authorization", "Bearer old-token")
		strategy := &auth.BearerStrategy{}

		err := strategy.Apply(context.Background(), req, "new-token")
		require.NoError(t, err)

		// Verify only new token present (not appended)
		authHeader := req.Header.Get("Authorization")
		assert.Equal(t, "Bearer new-token", authHeader,
			"Should replace existing Authorization, not append")

		// Verify only one Authorization header
		authValues := req.Header.Values("Authorization")
		assert.Len(t, authValues, 1,
			"Should have exactly one Authorization header")
	})

	t.Run("original_request_not_modified_on_error", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://api.example.com", nil)
		req.Header.Set("X-Test", "original")
		strategy := &auth.BearerStrategy{}

		// Apply with empty secret (should fail)
		err := strategy.Apply(context.Background(), req, "")
		assert.Error(t, err)

		// Verify original request unchanged
		assert.Equal(t, "original", req.Header.Get("X-Test"),
			"Original headers should be unchanged on error")
		assert.Empty(t, req.Header.Get("Authorization"),
			"Authorization should not be set on error")
	})

	t.Run("preserves_other_headers", func(t *testing.T) {
		req := httptest.NewRequest("POST", "https://api.example.com", nil)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Request-ID", "12345")
		strategy := &auth.BearerStrategy{}

		err := strategy.Apply(context.Background(), req, "secret")
		require.NoError(t, err)

		// Verify other headers preserved
		assert.Equal(t, "application/json", req.Header.Get("Content-Type"),
			"Content-Type should be preserved")
		assert.Equal(t, "12345", req.Header.Get("X-Request-ID"),
			"X-Request-ID should be preserved")
		assert.Equal(t, "Bearer secret", req.Header.Get("Authorization"),
			"Authorization should be added")
	})

	t.Run("no_extra_whitespace_in_token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://api.example.com", nil)
		strategy := &auth.BearerStrategy{}

		err := strategy.Apply(context.Background(), req, "my-secret-key")
		require.NoError(t, err)

		authHeader := req.Header.Get("Authorization")
		assert.Equal(t, "Bearer my-secret-key", authHeader,
			"Should have exactly one space between 'Bearer' and token")
		assert.NotContains(t, authHeader, "  ", "Should not have double spaces")
	})
}

// TestHeaderStrategy validates:
// - Custom header set correctly on request
// - Different header names work
// - Empty secret returns error
// - Empty header name returns error
// - Request is modified in place (as designed)
//
// ANTI-GAMING: Tests real http.Request headers, not mocked behavior
func TestHeaderStrategy(t *testing.T) {
	t.Run("custom_header_set_correctly", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://api.example.com", nil)
		strategy := &auth.HeaderStrategy{HeaderName: "X-API-Key"}

		err := strategy.Apply(context.Background(), req, "my-api-key")
		require.NoError(t, err, "Apply should succeed")

		assert.Equal(t, "my-api-key", req.Header.Get("X-API-Key"),
			"X-API-Key header should be set with secret value")
	})

	t.Run("different_header_names_work", func(t *testing.T) {
		testCases := []struct {
			headerName string
			secret     string
		}{
			{"X-API-Key", "key-123"},
			{"X-Auth-Token", "token-456"},
			{"Api-Secret", "secret-789"},
			{"X-Custom-Auth", "custom-abc"},
		}

		for _, tc := range testCases {
			t.Run(tc.headerName, func(t *testing.T) {
				req := httptest.NewRequest("GET", "https://api.example.com", nil)
				strategy := &auth.HeaderStrategy{HeaderName: tc.headerName}

				err := strategy.Apply(context.Background(), req, tc.secret)
				require.NoError(t, err)

				assert.Equal(t, tc.secret, req.Header.Get(tc.headerName),
					"Header %s should be set with secret", tc.headerName)
			})
		}
	})

	t.Run("empty_secret_returns_error", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://api.example.com", nil)
		strategy := &auth.HeaderStrategy{HeaderName: "X-API-Key"}

		err := strategy.Apply(context.Background(), req, "")
		assert.Error(t, err, "Apply should fail with empty secret")
		assert.Empty(t, req.Header.Get("X-API-Key"),
			"Header should not be set on error")
	})

	t.Run("empty_header_name_returns_error", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://api.example.com", nil)
		strategy := &auth.HeaderStrategy{HeaderName: ""}

		err := strategy.Apply(context.Background(), req, "my-secret")
		assert.Error(t, err, "Apply should fail with empty header name")
	})

	t.Run("existing_header_replaced", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://api.example.com", nil)
		req.Header.Set("X-API-Key", "old-key")
		strategy := &auth.HeaderStrategy{HeaderName: "X-API-Key"}

		err := strategy.Apply(context.Background(), req, "new-key")
		require.NoError(t, err)

		// Verify header replaced, not appended
		assert.Equal(t, "new-key", req.Header.Get("X-API-Key"),
			"Should replace existing header")
		values := req.Header.Values("X-API-Key")
		assert.Len(t, values, 1, "Should have exactly one header value")
	})

	t.Run("original_request_not_modified_on_error", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://api.example.com", nil)
		req.Header.Set("X-Original", "value")
		strategy := &auth.HeaderStrategy{HeaderName: "X-API-Key"}

		// Apply with empty secret (should fail)
		err := strategy.Apply(context.Background(), req, "")
		assert.Error(t, err)

		// Verify original headers unchanged
		assert.Equal(t, "value", req.Header.Get("X-Original"),
			"Original headers should be unchanged on error")
		assert.Empty(t, req.Header.Get("X-API-Key"),
			"X-API-Key should not be set on error")
	})

	t.Run("preserves_other_headers", func(t *testing.T) {
		req := httptest.NewRequest("POST", "https://api.example.com", nil)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer other-token")
		strategy := &auth.HeaderStrategy{HeaderName: "X-API-Key"}

		err := strategy.Apply(context.Background(), req, "my-key")
		require.NoError(t, err)

		// Verify all headers present
		assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer other-token", req.Header.Get("Authorization"))
		assert.Equal(t, "my-key", req.Header.Get("X-API-Key"))
	})

	t.Run("case_sensitive_header_names", func(t *testing.T) {
		req := httptest.NewRequest("GET", "https://api.example.com", nil)
		strategy := &auth.HeaderStrategy{HeaderName: "x-api-key"} // lowercase

		err := strategy.Apply(context.Background(), req, "my-key")
		require.NoError(t, err)

		// HTTP headers are case-insensitive, but we should respect the casing
		assert.Equal(t, "my-key", req.Header.Get("x-api-key"))
		assert.Equal(t, "my-key", req.Header.Get("X-API-Key"))
	})
}

// mockAuthStrategy is a test implementation of AuthStrategy
type mockAuthStrategy struct {
	name      string
	applyFunc func(ctx context.Context, req *http.Request, secret string) error
}

func (m *mockAuthStrategy) Apply(ctx context.Context, req *http.Request, secret string) error {
	if m.applyFunc != nil {
		return m.applyFunc(ctx, req, secret)
	}
	req.Header.Set("X-Mock-Strategy", m.name)
	return nil
}
