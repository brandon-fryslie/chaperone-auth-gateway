package service

// ShouldMITM determines if a domain should be intercepted for MITM based on service configuration.
// Returns true if the hostname matches a registered service, false otherwise.
// This function is safe for concurrent use when the registry is thread-safe.
func ShouldMITM(registry ServiceRegistry, hostname string) bool {
	if registry == nil {
		return false
	}

	// Lookup the service for this hostname
	_, err := registry.Lookup(hostname)
	return err == nil
}
