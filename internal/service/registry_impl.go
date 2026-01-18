package service

import (
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/bmf/chaperone/internal/errors"
)

// Registry is a thread-safe implementation of ServiceRegistry.
type Registry struct {
	services map[string]*Service
	mu       sync.RWMutex
}

// NewRegistry creates a new service registry.
func NewRegistry() *Registry {
	return &Registry{
		services: make(map[string]*Service),
	}
}

// Register adds a service to the registry.
// Returns an error if a service with the same hostname already exists.
func (r *Registry) Register(service *Service) error {
	if service == nil {
		return &errors.ConfigError{
			Field: "service",
			Value: nil,
			Cause: errors.ErrInvalidConfig,
		}
	}

	// Validate the service first
	if err := service.Validate(); err != nil {
		return err
	}

	// Normalize the hostname (lowercase)
	hostname := strings.ToLower(strings.TrimSpace(service.HostPattern))

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if service already exists
	if _, exists := r.services[hostname]; exists {
		return &errors.ConfigError{
			Field: "HostPattern",
			Value: hostname,
			Cause: fmt.Errorf("service for %s already registered", hostname),
		}
	}

	// Register the service
	r.services[hostname] = service

	return nil
}

// Lookup finds a service by hostname.
// It supports exact and wildcard matches (e.g., "*.example.com").
// Hostname lookup is case-insensitive and strips ports.
func (r *Registry) Lookup(hostname string) (*Service, error) {
	// Normalize the hostname
	normalized := normalizeHostname(hostname)

	r.mu.RLock()
	defer r.mu.RUnlock()

	// 1. Exact match
	if service, found := r.services[normalized]; found {
		return service, nil
	}

	// 2. Wildcard match
	for pattern, service := range r.services {
		if isWildcardMatch(pattern, normalized) {
			return service, nil
		}
	}

	return nil, fmt.Errorf("service not found for hostname: %s", hostname)
}

// isWildcardMatch checks if a hostname matches a wildcard pattern.
// e.g., pattern="*.example.com", hostname="api.example.com" -> true
func isWildcardMatch(pattern, hostname string) bool {
	if !strings.HasPrefix(pattern, "*.") {
		return false
	}
	// a wildcard match requires at least one subdomain label
	if !strings.Contains(hostname, ".") {
		return false
	}

	wildcardDomain := strings.TrimPrefix(pattern, "*.")
	return strings.HasSuffix(hostname, "."+wildcardDomain) && len(hostname) > len("."+wildcardDomain) && !strings.Contains(strings.TrimSuffix(hostname, "."+wildcardDomain), ".")
}

// ListAll returns all registered services.
func (r *Registry) ListAll() []*Service {
	r.mu.RLock()
	defer r.mu.RUnlock()

	services := make([]*Service, 0, len(r.services))
	for _, svc := range r.services {
		services = append(services, svc)
	}

	return services
}

// normalizeHostname converts a hostname to lowercase and strips the port if present.
func normalizeHostname(hostname string) string {
	// Trim spaces
	hostname = strings.TrimSpace(hostname)

	// Try to split host and port
	host, _, err := net.SplitHostPort(hostname)
	if err != nil {
		// If SplitHostPort fails, it's probably a hostname without a port
		// Just use the original hostname
		return strings.ToLower(hostname)
	}

	// Successfully split - return the host part (lowercase)
	return strings.ToLower(host)
}
