// Package service provides service configuration and policy management.
package service

// Service represents a managed API endpoint configuration.
type Service struct {
	HostPattern     string
	AuthStrategyRef string
	HeaderName      string // For "header" auth strategy - the header to set
	CredentialRef   string
	Policy          *Policy
}

// ServiceRegistry manages service configurations.
// Implementations must be safe for concurrent use.
type ServiceRegistry interface {
	// Register adds or updates a service configuration.
	Register(service *Service) error
	// Lookup finds a service by hostname.
	Lookup(hostname string) (*Service, bool)
	// ListAll returns all registered services.
	ListAll() []*Service
}
