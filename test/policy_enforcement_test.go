package test

import (
	"testing"

	"github.com/bmf/chaperone/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPolicyEnforcementImplementation tests the policy enforcer
func TestPolicyEnforcementImplementation(t *testing.T) {
	t.Parallel()

	t.Run("method_enforcement", func(t *testing.T) {
		enforcer := service.NewPolicyEnforcer(nil)

		policy := &service.Policy{
			AllowedMethods: []string{"POST", "GET"},
		}

		// Allowed methods
		err := enforcer.CheckMethod("POST", policy)
		require.NoError(t, err)

		err = enforcer.CheckMethod("GET", policy)
		require.NoError(t, err)

		// Disallowed method
		err = enforcer.CheckMethod("DELETE", policy)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "method")
		assert.Contains(t, err.Error(), "DELETE")

		// Empty allowlist allows all
		emptyPolicy := &service.Policy{}
		err = enforcer.CheckMethod("DELETE", emptyPolicy)
		require.NoError(t, err)
	})

	t.Run("path_enforcement", func(t *testing.T) {
		enforcer := service.NewPolicyEnforcer(nil)

		policy := &service.Policy{
			AllowedPaths: []string{"/v1/*", "/admin/health"},
		}

		// Allowed paths
		err := enforcer.CheckPath("/v1/chat", policy)
		require.NoError(t, err)

		err = enforcer.CheckPath("/v1/models", policy)
		require.NoError(t, err)

		err = enforcer.CheckPath("/admin/health", policy)
		require.NoError(t, err)

		// Disallowed paths
		err = enforcer.CheckPath("/v2/chat", policy)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "path")

		err = enforcer.CheckPath("/admin/users", policy)
		require.Error(t, err)

		// Empty allowlist allows all
		emptyPolicy := &service.Policy{}
		err = enforcer.CheckPath("/admin/secret", emptyPolicy)
		require.NoError(t, err)
	})

	t.Run("body_size_enforcement", func(t *testing.T) {
		enforcer := service.NewPolicyEnforcer(nil)

		policy := &service.Policy{
			MaxBodyBytes: 1024, // 1KB
		}

		// Within limit
		err := enforcer.CheckBodySize(512, policy)
		require.NoError(t, err)

		// At limit
		err = enforcer.CheckBodySize(1024, policy)
		require.NoError(t, err)

		// Exceeds limit
		err = enforcer.CheckBodySize(2048, policy)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "too large")

		// Zero limit allows all
		zeroPolicy := &service.Policy{MaxBodyBytes: 0}
		err = enforcer.CheckBodySize(10000, zeroPolicy)
		require.NoError(t, err)

		// Negative size (no Content-Length) is allowed
		err = enforcer.CheckBodySize(-1, policy)
		require.NoError(t, err)
	})

	t.Run("nil_policy_allows_all", func(t *testing.T) {
		enforcer := service.NewPolicyEnforcer(nil)

		err := enforcer.CheckMethod("DELETE", nil)
		require.NoError(t, err)

		err = enforcer.CheckPath("/any/path", nil)
		require.NoError(t, err)

		err = enforcer.CheckBodySize(999999, nil)
		require.NoError(t, err)
	})

	t.Run("exact_path_matching", func(t *testing.T) {
		enforcer := service.NewPolicyEnforcer(nil)

		policy := &service.Policy{
			AllowedPaths: []string{"/v1/chat"},
		}

		// Exact match
		err := enforcer.CheckPath("/v1/chat", policy)
		require.NoError(t, err)

		// Not exact match
		err = enforcer.CheckPath("/v1/models", policy)
		require.Error(t, err)
	})

	t.Run("case_sensitive_path_matching", func(t *testing.T) {
		enforcer := service.NewPolicyEnforcer(nil)

		policy := &service.Policy{
			AllowedPaths: []string{"/v1/Chat"},
		}

		err := enforcer.CheckPath("/v1/Chat", policy)
		require.NoError(t, err)

		// Different case should not match
		err = enforcer.CheckPath("/v1/chat", policy)
		require.Error(t, err)
	})
}
