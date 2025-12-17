package service

import (
	"fmt"
	"log/slog"
	"path"
	"strings"

	"github.com/bmf/chaperone/internal/errors"
)

// Enforcer implements the PolicyEnforcer interface.
type Enforcer struct {
	logger *slog.Logger
}

// NewPolicyEnforcer creates a new policy enforcer.
func NewPolicyEnforcer(logger *slog.Logger) *Enforcer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Enforcer{
		logger: logger,
	}
}

// CheckPath validates the request path against allowed patterns.
func (e *Enforcer) CheckPath(requestPath string, policy *Policy) error {
	if policy == nil {
		return nil // no policy = allow all
	}

	// Empty allowlist means allow all paths
	if len(policy.AllowedPaths) == 0 {
		return nil
	}

	// Check if path matches any allowed pattern
	for _, pattern := range policy.AllowedPaths {
		if matchPath(requestPath, pattern) {
			return nil
		}
	}

	// Path not allowed
	e.logger.Warn("path not allowed by policy",
		"path", requestPath,
		"allowed_paths", policy.AllowedPaths)

	return &errors.PolicyError{
		Rule:  "path",
		Cause: fmt.Errorf("path %s not allowed", requestPath),
	}
}

// CheckMethod validates the HTTP method is allowed.
func (e *Enforcer) CheckMethod(method string, policy *Policy) error {
	if policy == nil {
		return nil // no policy = allow all
	}

	// Empty allowlist means allow all methods
	if len(policy.AllowedMethods) == 0 {
		return nil
	}

	// Check if method is in allowed list
	for _, allowed := range policy.AllowedMethods {
		if method == allowed {
			return nil
		}
	}

	// Method not allowed
	e.logger.Warn("method not allowed by policy",
		"method", method,
		"allowed_methods", policy.AllowedMethods)

	return &errors.PolicyError{
		Rule:  "method",
		Cause: fmt.Errorf("method %s not allowed", method),
	}
}

// CheckBodySize validates the request body size is within limits.
func (e *Enforcer) CheckBodySize(size int64, policy *Policy) error {
	if policy == nil {
		return nil // no policy = allow all
	}

	// Zero or negative size in request means no Content-Length header (streaming)
	// We allow this - the handler will need to handle streaming appropriately
	if size < 0 {
		return nil
	}

	// Zero limit means no limit
	if policy.MaxBodyBytes == 0 {
		return nil
	}

	// Check if size exceeds limit
	if size > policy.MaxBodyBytes {
		e.logger.Warn("body size exceeds limit",
			"size", size,
			"max_bytes", policy.MaxBodyBytes)

		return &errors.PolicyError{
			Rule:  "body_size",
			Cause: fmt.Errorf("body size %d bytes too large (max %d bytes)", size, policy.MaxBodyBytes),
		}
	}

	return nil
}

// matchPath checks if a request path matches an allowed path pattern.
// Supports:
// - Exact matching: /v1/chat matches only /v1/chat
// - Wildcard matching: /v1/* matches /v1/chat, /v1/models, etc.
// Path matching is case-sensitive (HTTP paths are case-sensitive).
func matchPath(requestPath, pattern string) bool {
	// Exact match
	if requestPath == pattern {
		return true
	}

	// Wildcard match
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(requestPath, prefix)
	}

	// Use path.Match for more complex patterns if needed
	matched, err := path.Match(pattern, requestPath)
	if err != nil {
		return false
	}

	return matched
}
