package secrets

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Registry manages secret providers by name.
// It is safe for concurrent use.
// Secrets are cached in memory after the first fetch to avoid repeated lookups.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]SecretProvider
	cache     map[string]string // cache of fetched secrets: "provider:path" -> secret value
}

// NewRegistry creates a new secret provider registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]SecretProvider),
		cache:     make(map[string]string),
	}
}

// Register adds or replaces a secret provider.
// If a provider with the same name already exists, it will be replaced.
func (r *Registry) Register(name string, provider SecretProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[name] = provider
}

// Fetch retrieves a secret using the specified provider and path.
// The ref parameter should be in the format "provider:path":
//   - "env:MY_API_KEY" → use env provider, path="MY_API_KEY"
//   - "file:/path/to/key" → use file provider, path="/path/to/key"
//   - "keychain:service/account" → use keychain provider, path="service/account"
//
// Secrets are cached in memory after the first fetch to avoid repeated lookups.
//
// Returns an error if:
//   - The ref format is invalid (missing colon)
//   - The provider is not registered
//   - The provider fails to fetch the secret
func (r *Registry) Fetch(ctx context.Context, ref string) (string, error) {
	if ref == "" {
		return "", fmt.Errorf("empty secret reference")
	}

	// Check cache first
	r.mu.RLock()
	if cached, found := r.cache[ref]; found {
		r.mu.RUnlock()
		return cached, nil
	}
	r.mu.RUnlock()

	// Parse provider and path from ref
	idx := strings.Index(ref, ":")
	if idx == -1 {
		return "", fmt.Errorf("invalid secret reference format: missing colon separator (expected 'provider:path')")
	}

	providerName := ref[:idx]
	path := ref[idx+1:]

	if path == "" {
		return "", fmt.Errorf("invalid secret reference: empty path (expected 'provider:path')")
	}

	// Get provider
	r.mu.RLock()
	provider, found := r.providers[providerName]
	r.mu.RUnlock()

	if !found {
		return "", fmt.Errorf("secret provider not found: %s", providerName)
	}

	// Fetch secret
	secret, err := provider.Fetch(ctx, path)
	if err != nil {
		return "", err
	}

	// Cache the secret for future use
	r.mu.Lock()
	r.cache[ref] = secret
	r.mu.Unlock()

	return secret, nil
}

// ResolvedValues returns a snapshot of every secret value this registry has
// resolved so far. It exists so the recording redactor can scrub the values
// the process actually holds — including credentials first fetched for a
// runtime grant — without a second store of secrets ever being minted.
// [LAW:one-source-of-truth]
func (r *Registry) ResolvedValues() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	values := make([]string, 0, len(r.cache))
	for _, v := range r.cache {
		values = append(values, v)
	}
	return values
}

// HasProvider checks if a secret provider is registered.
func (r *Registry) HasProvider(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, found := r.providers[name]
	return found
}

// PreloadSecrets fetches and caches secrets at startup to avoid repeated lookups during request handling.
// This is useful for credentials that require expensive operations (e.g., keychain lookups).
// If any secret fails to load, returns an error containing all failed secrets.
func (r *Registry) PreloadSecrets(ctx context.Context, refs ...string) error {
	var errors []error
	successCount := 0

	for _, ref := range refs {
		if _, err := r.Fetch(ctx, ref); err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", ref, err))
		} else {
			successCount++
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to preload %d of %d secrets (successful: %d): %v",
			len(errors), len(refs), successCount, errors)
	}

	return nil
}
