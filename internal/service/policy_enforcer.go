package service

import (
	"fmt"
	"log/slog"

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

// CheckPath validates the request path against allowed patterns. Matching goes
// through pathPattern — the one matcher shared with drop rules and grant
// narrowing — so the request path is normalized (dot-segments resolved,
// duplicate slashes collapsed) and compared case-insensitively; "/v1/../admin"
// is judged as "/admin", never as "inside /v1". [LAW:single-enforcer]
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
		pp, err := parsePathPattern(pattern)
		if err != nil {
			// Patterns are validated at the config/grant boundary, so this
			// branch is unreachable in a correctly assembled daemon. If it
			// fires anyway, fail closed (the pattern allows nothing) and say
			// so on every request it dead-ends. [LAW:no-silent-failure]
			e.logger.Error("unmatchable allowed_paths pattern; treating as no match",
				"pattern", pattern, "error", err)
			continue
		}
		if pp.matches(requestPath) {
			return nil
		}
	}

	// Path not allowed
	e.logger.Warn("path not allowed by policy",
		"path", requestPath,
		"normalized_path", normalizePath(requestPath),
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
