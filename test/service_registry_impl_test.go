package test

import (
	"testing"

	"github.com/bmf/chaperone/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegistryBasicOperations tests service registration and lookup
func TestRegistryBasicOperations(t *testing.T) {
	t.Parallel()

	t.Run("register_and_lookup", func(t *testing.T) {
		registry := service.NewRegistry()

		svc := &service.Service{
			HostPattern:     "api.example.com",
			AuthStrategyRef: "bearer",
			CredentialRef:   "env:TEST_KEY",
		}

		err := registry.Register(svc)
		require.NoError(t, err, "Service registration should succeed")

		found, err := registry.Lookup("api.example.com")
		require.NoError(t, err, "Service should be found")
		require.NotNil(t, found, "Found service should not be nil")
		assert.Equal(t, "api.example.com", found.HostPattern)
	})

	t.Run("case_insensitive_lookup", func(t *testing.T) {
		registry := service.NewRegistry()

		svc := &service.Service{
			HostPattern:     "api.example.com",
			AuthStrategyRef: "bearer",
			CredentialRef:   "env:TEST_KEY",
		}

		err := registry.Register(svc)
		require.NoError(t, err)

		// Test various case variants
		testCases := []string{
			"api.example.com",
			"API.EXAMPLE.COM",
			"Api.Example.Com",
		}

		for _, hostname := range testCases {
			found, err := registry.Lookup(hostname)
			require.NoError(t, err, "Service should be found for: %s", hostname)
			require.NotNil(t, found)
		}
	})

	t.Run("port_stripping", func(t *testing.T) {
		registry := service.NewRegistry()

		svc := &service.Service{
			HostPattern:     "api.example.com",
			AuthStrategyRef: "bearer",
			CredentialRef:   "env:TEST_KEY",
		}

		err := registry.Register(svc)
		require.NoError(t, err)

		testCases := []string{
			"api.example.com",
			"api.example.com:443",
			"api.example.com:8080",
		}

		for _, hostname := range testCases {
			_, err := registry.Lookup(hostname)
			require.NoError(t, err, "Service should be found for: %s", hostname)
		}
	})

	t.Run("missing_service", func(t *testing.T) {
		registry := service.NewRegistry()

		found, err := registry.Lookup("nonexistent.example.com")
		assert.Error(t, err)
		assert.Nil(t, found)
		assert.Contains(t, err.Error(), "service not found for hostname")
	})

	t.Run("duplicate_registration", func(t *testing.T) {
		registry := service.NewRegistry()

		svc1 := &service.Service{
			HostPattern:     "api.example.com",
			AuthStrategyRef: "bearer",
			CredentialRef:   "env:KEY1",
		}

		err := registry.Register(svc1)
		require.NoError(t, err)

		svc2 := &service.Service{
			HostPattern:     "api.example.com",
			AuthStrategyRef: "bearer",
			CredentialRef:   "env:KEY2",
		}

		err = registry.Register(svc2)
		require.Error(t, err, "Duplicate registration should fail")
		assert.Contains(t, err.Error(), "already registered")
	})

	t.Run("list_all", func(t *testing.T) {
		registry := service.NewRegistry()

		// Empty registry
		services := registry.ListAll()
		assert.Empty(t, services)

		// Add services
		svc1 := &service.Service{
			HostPattern:     "api.example.com",
			AuthStrategyRef: "bearer",
			CredentialRef:   "env:KEY1",
		}
		registry.Register(svc1)

		svc2 := &service.Service{
			HostPattern:     "api.other.com",
			AuthStrategyRef: "bearer",
			CredentialRef:   "env:KEY2",
		}
		registry.Register(svc2)

		services = registry.ListAll()
		assert.Len(t, services, 2)
	})
}

// TestRegistryRuntimeMutation tests the Upsert/Unregister seam that the control
// plane uses to apply and revoke runtime grants. Register stays strict; Upsert
// is add-or-replace; Unregister removes by exact host pattern.
func TestRegistryRuntimeMutation(t *testing.T) {
	t.Parallel()

	newSvc := func(host, ref string) *service.Service {
		return &service.Service{
			HostPattern:     host,
			AuthStrategyRef: "bearer",
			CredentialRef:   ref,
		}
	}

	t.Run("register_then_unregister_removes_eligibility", func(t *testing.T) {
		registry := service.NewRegistry()

		require.NoError(t, registry.Register(newSvc("api.example.com", "env:KEY1")))

		_, err := registry.Lookup("api.example.com")
		require.NoError(t, err, "service should be eligible after register")

		require.NoError(t, registry.Unregister("api.example.com"))

		_, err = registry.Lookup("api.example.com")
		assert.Error(t, err, "service should not be eligible after unregister")
		assert.Empty(t, registry.ListAll())
	})

	t.Run("upsert_adds_when_absent", func(t *testing.T) {
		registry := service.NewRegistry()

		require.NoError(t, registry.Upsert(newSvc("api.example.com", "env:KEY1")))

		found, err := registry.Lookup("api.example.com")
		require.NoError(t, err)
		assert.Equal(t, "env:KEY1", found.CredentialRef)
	})

	t.Run("upsert_regrants_existing_host_without_error", func(t *testing.T) {
		registry := service.NewRegistry()

		require.NoError(t, registry.Register(newSvc("api.example.com", "env:KEY1")))

		// A runtime regrant of the same host must succeed and replace the value,
		// unlike Register which would reject the duplicate.
		require.NoError(t, registry.Upsert(newSvc("api.example.com", "env:KEY2")))

		found, err := registry.Lookup("api.example.com")
		require.NoError(t, err)
		assert.Equal(t, "env:KEY2", found.CredentialRef, "upsert should replace the existing service")
		assert.Len(t, registry.ListAll(), 1, "regrant must not create a second entry")
	})

	t.Run("register_still_rejects_config_duplicate", func(t *testing.T) {
		registry := service.NewRegistry()

		require.NoError(t, registry.Register(newSvc("api.example.com", "env:KEY1")))

		err := registry.Register(newSvc("api.example.com", "env:KEY2"))
		require.Error(t, err, "Register must remain a hard error on duplicate host")
		assert.Contains(t, err.Error(), "already registered")
	})

	t.Run("unregister_absent_host_errors", func(t *testing.T) {
		registry := service.NewRegistry()

		err := registry.Unregister("nonexistent.example.com")
		require.Error(t, err, "revoking a host that was never granted must fail loudly")
		assert.Contains(t, err.Error(), "no service registered")
	})

	t.Run("unregister_matches_register_normalization", func(t *testing.T) {
		registry := service.NewRegistry()

		require.NoError(t, registry.Register(newSvc("API.Example.Com", "env:KEY1")))

		// Stored under the normalized key; a differently-cased pattern removes it.
		require.NoError(t, registry.Unregister("api.example.com"))
		assert.Empty(t, registry.ListAll())
	})

	t.Run("upsert_nil_errors", func(t *testing.T) {
		registry := service.NewRegistry()
		assert.Error(t, registry.Upsert(nil))
	})
}
