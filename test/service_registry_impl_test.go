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
