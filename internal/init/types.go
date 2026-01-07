package init

import "github.com/bmf/chaperone/internal/service"

// Finding represents a detected authentication pattern with confidence score.
type Finding struct {
	// HeaderName is the HTTP header where the credential was found (e.g., "Authorization")
	HeaderName string
	// HeaderValue is the detected credential value
	HeaderValue string
	// Confidence is a score from 0.0 to 1.0 indicating detection confidence
	// 1.0 = sentinel match (exact value match)
	// 0.9 = known auth header
	// 0.7 = auth keyword in name
	// 0.6 = credential value pattern
	Confidence float64
	// Heuristic describes which detection method found this (e.g., "sentinel_match", "known_auth_header")
	Heuristic string
}

// PolicyFindings represents detected policy constraints for a service.
type PolicyFindings struct {
	// Methods is the set of unique HTTP methods observed (GET, POST, etc.)
	Methods map[string]bool
	// Paths is the set of URL path patterns observed, generalized (e.g., /users/*)
	Paths map[string]bool
	// MaxBodyBytes is the largest Content-Length observed
	MaxBodyBytes int64
}

// ServiceFindings represents all findings for a single host.
type ServiceFindings struct {
	// Host is the hostname (e.g., "api.openai.com")
	Host string
	// AuthFindings contains all detected auth patterns for this host
	// Map key is the header name (lowercase for deduplication)
	AuthFindings map[string]*Finding
	// Policy contains detected policy constraints
	Policy *PolicyFindings
}

// NewServiceFindings creates an initialized ServiceFindings.
func NewServiceFindings(host string) *ServiceFindings {
	return &ServiceFindings{
		Host:         host,
		AuthFindings: make(map[string]*Finding),
		Policy: &PolicyFindings{
			Methods: make(map[string]bool),
			Paths:   make(map[string]bool),
		},
	}
}

// DetectorConfig configures the credential detector.
type DetectorConfig struct {
	// SentinelValue is an optional exact value to search for in headers
	// If set, any header with this value gets 100% confidence
	SentinelValue string
	// ExcludeHosts is a list of hosts to skip detection for
	ExcludeHosts []string
}

// GeneratedService represents a service configuration to be written to TOML.
type GeneratedService struct {
	// Name is the service name (user-provided, e.g., "openai")
	Name string
	// HostPattern is the hostname to match (e.g., "api.openai.com")
	HostPattern string
	// AuthStrategy is the strategy name (e.g., "bearer" or "header:X-API-Key")
	AuthStrategy string
	// CredentialRef is the reference to the stored credential
	// Format: "keychain:service/account", "file:/path", or "env:VAR_NAME"
	CredentialRef string
	// Policy contains the generated policy configuration
	Policy *service.Policy
}
