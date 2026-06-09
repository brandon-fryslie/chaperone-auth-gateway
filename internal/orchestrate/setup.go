// Package orchestrate provides shared setup logic for inject and run modes.
package orchestrate

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/bmf/chaperone/internal/auth"
	"github.com/bmf/chaperone/internal/config"
	"github.com/bmf/chaperone/internal/log"
	"github.com/bmf/chaperone/internal/mitm"
	"github.com/bmf/chaperone/internal/secrets"
	"github.com/bmf/chaperone/internal/service"
)

// SetupConfig contains configuration for the setup process.
type SetupConfig struct {
	Config       *config.Config
	ServiceNames []string // Empty = all services, otherwise filter to these names
	CAKeyPath    string
	CACertPath   string
}

// SetupResult contains the initialized registries and components.
type SetupResult struct {
	ServiceRegistry service.ServiceRegistry
	SecretRegistry  *secrets.Registry
	AuthRegistry    *auth.Registry
	CertCache       *mitm.CertCache
}

// Setup initializes all registries and components based on configuration.
// This is the single source of truth for setup logic shared between inject and run modes.
func Setup(ctx context.Context, cfg SetupConfig, ca *mitm.CA, logger *slog.Logger) (*SetupResult, error) {
	// Create service registry
	registry := service.NewRegistry()

	// Load services from config
	serviceCount := 0
	headerStrategies := make(map[string]bool) // Track header strategies to register
	credentialRefs := make([]string, 0)       // Collect credential refs for preloading

	for name, svcCfg := range cfg.Config.Services {
		// Apply service filter if specified
		if len(cfg.ServiceNames) > 0 && !contains(cfg.ServiceNames, name) {
			continue
		}

		// Canonicalize the auth strategy ref (folds the combined "header:x-api-key"
		// and separate "header"+header_name spellings into one form).
		authStrategyRef := service.CanonicalAuthRef(svcCfg.AuthStrategy, svcCfg.HeaderName)
		if headerName, ok := service.HeaderNameFromRef(authStrategyRef); ok {
			headerStrategies[headerName] = true
		}

		// Collect credential refs for preloading
		if svcCfg.CredentialRef != "" {
			credentialRefs = append(credentialRefs, svcCfg.CredentialRef)
		}

		// Convert config.ServiceConfig to service.Service
		svc := &service.Service{
			Name:            name,
			HostPattern:     svcCfg.HostPattern,
			AuthStrategyRef: authStrategyRef,
			HeaderName:      svcCfg.HeaderName,
			CredentialRef:   svcCfg.CredentialRef,
			Placeholder:     svcCfg.Placeholder,
			Policy: &service.Policy{
				AllowedMethods: svcCfg.AllowedMethods,
				AllowedPaths:   svcCfg.AllowedPaths,
				MaxBodyBytes:   svcCfg.MaxBodyBytes,
				ClientGroups:   svcCfg.ClientGroups,
				Drop:           svcCfg.Drop,
				Strip:          svcCfg.Strip,
			},
		}

		// Apply policy defaults
		svc.Policy.ApplyDefaults()

		// Register service
		if err := registry.Register(svc); err != nil {
			log.Error(ctx, "failed to register service", err,
				"service", name,
				"host_pattern", svcCfg.HostPattern,
			)
			return nil, fmt.Errorf("failed to register service %s: %w", name, err)
		}

		serviceCount++
		log.Info(ctx, "registered service",
			"name", name,
			"host_pattern", svcCfg.HostPattern,
			"auth_strategy", authStrategyRef,
		)
	}

	// Validate service count if filter was applied
	if len(cfg.ServiceNames) > 0 && serviceCount == 0 {
		return nil, fmt.Errorf("no services found matching filter: %v", cfg.ServiceNames)
	}

	log.Info(ctx, "MITM enabled for configured services",
		"service_count", serviceCount,
	)

	// Initialize secret and auth registries
	secretRegistry := secrets.NewRegistry()
	authRegistry := auth.NewRegistry()

	// Register built-in secret providers from their single canonical source.
	secrets.RegisterBuiltins(secretRegistry)

	// Preload secrets at startup to avoid expensive lookups during request handling
	if len(credentialRefs) > 0 {
		if err := secretRegistry.PreloadSecrets(ctx, credentialRefs...); err != nil {
			return nil, fmt.Errorf("failed to preload secrets at startup: %w", err)
		}
		log.Info(ctx, "preloaded secrets at startup",
			"secret_count", len(credentialRefs),
		)
	}

	// Register built-in auth strategies
	authRegistry.Register("bearer", &auth.BearerStrategy{})

	// Register header strategies for each unique header name used in config
	for headerName := range headerStrategies {
		strategyKey := "header:" + headerName
		authRegistry.Register(strategyKey, &auth.HeaderStrategy{HeaderName: headerName})
		log.Debug(ctx, "registered header auth strategy",
			"strategy", strategyKey,
			"header_name", headerName,
		)
	}

	// Validate all services have registered auth strategies and credentials
	if serviceCount > 0 {
		if err := validateConfiguration(ctx, registry, authRegistry, secretRegistry); err != nil {
			return nil, fmt.Errorf("configuration validation failed: %w", err)
		}
	}

	// Create certificate cache
	certCache := mitm.NewCertCache(ca, logger)

	return &SetupResult{
		ServiceRegistry: registry,
		SecretRegistry:  secretRegistry,
		AuthRegistry:    authRegistry,
		CertCache:       certCache,
	}, nil
}

// validateConfiguration checks that all configured services have valid configuration:
// - auth strategies exist
// - secret providers exist (for credential references)
// - host patterns are valid
// - policy configuration is valid
// This is called at startup to catch configuration errors early.
func validateConfiguration(ctx context.Context, registry service.ServiceRegistry, authRegistry *auth.Registry, secretRegistry *secrets.Registry) error {
	var validationErrors []string

	for _, svc := range registry.ListAll() {
		// Validate auth strategy
		strategyRef := svc.AuthStrategyRef
		if strategyRef == "" {
			strategyRef = "bearer" // Default strategy
		}

		if !authRegistry.Has(strategyRef) {
			errMsg := fmt.Sprintf("auth strategy %q not registered (host pattern: %s)", strategyRef, svc.HostPattern)
			validationErrors = append(validationErrors, errMsg)
			log.Error(ctx, "auth strategy not found",
				fmt.Errorf("%s", errMsg),
				"host_pattern", svc.HostPattern,
			)
		}

		// Validate secret provider (if credential reference exists)
		if svc.CredentialRef != "" {
			provider := parseSecretProvider(svc.CredentialRef)
			if provider == "" {
				errMsg := fmt.Sprintf("invalid credential_ref format %q (host pattern: %s)", svc.CredentialRef, svc.HostPattern)
				validationErrors = append(validationErrors, errMsg)
				log.Error(ctx, "invalid credential reference format",
					fmt.Errorf("%s", errMsg),
					"credential_ref", svc.CredentialRef,
					"host_pattern", svc.HostPattern,
				)
			} else if !secretRegistry.HasProvider(provider) {
				errMsg := fmt.Sprintf("secret provider %q not found for credential_ref %q (host pattern: %s)", provider, svc.CredentialRef, svc.HostPattern)
				validationErrors = append(validationErrors, errMsg)
				log.Error(ctx, "secret provider not found",
					fmt.Errorf("%s", errMsg),
					"provider", provider,
					"credential_ref", svc.CredentialRef,
					"host_pattern", svc.HostPattern,
				)
			}
		}

		// Validate host pattern (basic check - should be non-empty)
		if svc.HostPattern == "" {
			errMsg := "host pattern is empty"
			validationErrors = append(validationErrors, errMsg)
			log.Error(ctx, "invalid service configuration",
				fmt.Errorf("%s", errMsg),
				"service", svc,
			)
		}

		// Validate policy configuration
		if svc.Policy != nil {
			if svc.Policy.MaxBodyBytes < 0 {
				errMsg := fmt.Sprintf("max_body_bytes is negative (host pattern: %s)", svc.HostPattern)
				validationErrors = append(validationErrors, errMsg)
				log.Error(ctx, "invalid policy configuration",
					fmt.Errorf("%s", errMsg),
					"host_pattern", svc.HostPattern,
				)
			}
		}
	}

	if len(validationErrors) > 0 {
		return fmt.Errorf("configuration validation failed: %d error(s): %v", len(validationErrors), validationErrors)
	}

	log.Info(ctx, "configuration validation passed")
	return nil
}

// parseSecretProvider extracts the provider name from a secret reference.
// Returns empty string if format is invalid.
// Format: "provider:path"
func parseSecretProvider(ref string) string {
	idx := -1
	for i, r := range ref {
		if r == ':' {
			idx = i
			break
		}
	}
	if idx == -1 {
		return "" // Invalid format
	}
	return ref[:idx]
}

// contains checks if a string slice contains a specific string.
func contains(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
