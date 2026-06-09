// Package service provides service configuration and policy management.
package service

// Service represents a managed API endpoint configuration.
type Service struct {
	Name            string // Service name from config
	HostPattern     string
	AuthStrategyRef string
	HeaderName      string // For "header" auth strategy - the header to set
	CredentialRef   string
	Placeholder     string // Token app sends that we replace
	Policy          *Policy
}

// ServiceRegistry manages service configurations.
// Implementations must be safe for concurrent use.
//
// The registry is the single owner of live service state; all mutation goes
// through this API, never a side channel ([LAW:no-shared-mutable-globals]).
// Register, Upsert, and Unregister are three distinct mutation behaviors,
// not one behavior behind a flag: they differ in what counts as an error.
type ServiceRegistry interface {
	// Register adds a NEW service. It returns an error if a service is already
	// registered for the same host pattern, so a duplicate host in static config
	// is a hard error ([LAW:no-silent-failure]). For runtime add-or-replace
	// (regranting a host while the proxy runs), use Upsert.
	Register(service *Service) error
	// Upsert adds a service or replaces the existing one for the same host
	// pattern. It never errors on an existing host: a runtime regrant is a normal
	// operation, not a misconfiguration. Used by the control plane to apply grants.
	Upsert(service *Service) error
	// Unregister removes the service registered for the given host pattern.
	// The pattern is matched exactly (after normalization), NOT by wildcard
	// resolution — you remove what you registered. Returns an error if no service
	// is registered for it. Used by the control plane to revoke a grant.
	Unregister(hostPattern string) error
	// Lookup finds a service by hostname.
	// Returns an error if the service is not found.
	Lookup(hostname string) (*Service, error)
	// ListAll returns all registered services.
	ListAll() []*Service
}
