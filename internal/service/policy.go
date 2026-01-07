// Package service provides service configuration and policy management.
package service

// Policy defines access control rules for a service.
type Policy struct {
	AllowedMethods []string
	AllowedPaths   []string
	MaxBodyBytes   int64
	ClientGroups   []string
	Drop           []string // URL patterns to block (drops traffic)
	Strip          []string // Headers to strip from requests
}

// PolicyEnforcer validates requests against service policies.
// Implementations must be safe for concurrent use.
type PolicyEnforcer interface {
	// CheckPath validates the request path against allowed patterns.
	CheckPath(path string, policy *Policy) error
	// CheckMethod validates the HTTP method is allowed.
	CheckMethod(method string, policy *Policy) error
	// CheckBodySize validates the request body size is within limits.
	CheckBodySize(size int64, policy *Policy) error
}
