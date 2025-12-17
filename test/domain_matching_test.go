package test

import (
	"testing"

	"github.com/bmf/chaperone/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDomainMatching tests the ShouldMITM function
func TestDomainMatchingImplementation(t *testing.T) {
	t.Parallel()

	t.Run("configured_domain_returns_true", func(t *testing.T) {
		registry := service.NewRegistry()

		svc := &service.Service{
			HostPattern:     "api.openai.com",
			AuthStrategyRef: "bearer",
			CredentialRef:   "env:OPENAI_API_KEY",
		}
		err := registry.Register(svc)
		require.NoError(t, err)

		assert.True(t, service.ShouldMITM(registry, "api.openai.com"))
	})

	t.Run("non_configured_domain_returns_false", func(t *testing.T) {
		registry := service.NewRegistry()

		svc := &service.Service{
			HostPattern:     "api.openai.com",
			AuthStrategyRef: "bearer",
			CredentialRef:   "env:OPENAI_API_KEY",
		}
		registry.Register(svc)

		assert.False(t, service.ShouldMITM(registry, "www.google.com"))
	})

	t.Run("case_insensitive_matching", func(t *testing.T) {
		registry := service.NewRegistry()

		svc := &service.Service{
			HostPattern:     "api.openai.com",
			AuthStrategyRef: "bearer",
			CredentialRef:   "env:OPENAI_API_KEY",
		}
		registry.Register(svc)

		assert.True(t, service.ShouldMITM(registry, "API.OPENAI.COM"))
		assert.True(t, service.ShouldMITM(registry, "Api.OpenAI.Com"))
	})

	t.Run("port_handling", func(t *testing.T) {
		registry := service.NewRegistry()

		svc := &service.Service{
			HostPattern:     "api.openai.com",
			AuthStrategyRef: "bearer",
			CredentialRef:   "env:OPENAI_API_KEY",
		}
		registry.Register(svc)

		assert.True(t, service.ShouldMITM(registry, "api.openai.com:443"))
		assert.True(t, service.ShouldMITM(registry, "api.openai.com:8080"))
	})

	t.Run("wildcard_domain_matching", func(t *testing.T) {
		registry := service.NewRegistry()

		svc := &service.Service{
			HostPattern:     "*.example.com",
			AuthStrategyRef: "bearer",
			CredentialRef:   "env:EXAMPLE_API_KEY",
		}
		err := registry.Register(svc)
		require.NoError(t, err)

		assert.True(t, service.ShouldMITM(registry, "api.example.com"))
		assert.True(t, service.ShouldMITM(registry, "www.example.com"))
		assert.False(t, service.ShouldMITM(registry, "example.com"))
		assert.False(t, service.ShouldMITM(registry, "test.v1.example.com"))
	})

	t.Run("nil_registry_returns_false", func(t *testing.T) {
		assert.False(t, service.ShouldMITM(nil, "api.example.com"))
	})
}
