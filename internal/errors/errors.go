package errors

import (
	"errors"
	"fmt"
	"net/http"
)

// Sentinel errors for common failure conditions.
var (
	ErrSecretNotFound   = errors.New("secret not found")
	ErrPermissionDenied = errors.New("permission denied")
	ErrTimeout          = errors.New("operation timeout")
	ErrPolicyViolation  = errors.New("policy violation")
	ErrInvalidConfig    = errors.New("invalid configuration")
	ErrUpstreamError    = errors.New("upstream error")
)

// SecretError provides context for secret retrieval failures.
type SecretError struct {
	Provider string
	Ref      string
	Cause    error
}

func (e *SecretError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("secret error: provider=%s, ref=%s: %v", e.Provider, e.Ref, e.Cause)
	}
	return fmt.Sprintf("secret error: provider=%s, ref=%s", e.Provider, e.Ref)
}

func (e *SecretError) Unwrap() error {
	return e.Cause
}

// PolicyError provides context for policy enforcement failures.
type PolicyError struct {
	Service string
	Rule    string
	Cause   error
}

func (e *PolicyError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("policy error: service=%s, rule=%s: %v", e.Service, e.Rule, e.Cause)
	}
	return fmt.Sprintf("policy error: service=%s, rule=%s", e.Service, e.Rule)
}

func (e *PolicyError) Unwrap() error {
	return e.Cause
}

// ConfigError provides context for configuration failures.
type ConfigError struct {
	Field string
	Value interface{}
	Cause error
}

func (e *ConfigError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("config error: field=%s, value=%v: %v", e.Field, e.Value, e.Cause)
	}
	return fmt.Sprintf("config error: field=%s, value=%v", e.Field, e.Value)
}

func (e *ConfigError) Unwrap() error {
	return e.Cause
}

// HTTPStatus returns the appropriate HTTP status code for an error.
func HTTPStatus(err error) int {
	if err == nil {
		return http.StatusInternalServerError // 500 - treat nil as error condition
	}

	switch {
	case errors.Is(err, ErrSecretNotFound):
		return http.StatusBadGateway // 502
	case errors.Is(err, ErrPermissionDenied):
		return http.StatusForbidden // 403
	case errors.Is(err, ErrTimeout):
		return http.StatusGatewayTimeout // 504
	case errors.Is(err, ErrPolicyViolation):
		return http.StatusForbidden // 403
	case errors.Is(err, ErrInvalidConfig):
		return http.StatusInternalServerError // 500
	case errors.Is(err, ErrUpstreamError):
		return http.StatusBadGateway // 502
	default:
		return http.StatusInternalServerError // 500
	}
}

// ClientMessage returns a safe, user-friendly error message.
// It MUST NOT leak:
// - Internal paths
// - Secret references
// - Provider names
// - Stack traces
func ClientMessage(err error) string {
	if err == nil {
		return "An unexpected error occurred"
	}

	switch {
	case errors.Is(err, ErrSecretNotFound):
		return "Authentication service error temporarily unavailable"
	case errors.Is(err, ErrPermissionDenied):
		return "Access forbidden or denied"
	case errors.Is(err, ErrTimeout):
		return "Request timeout occurred, please try again"
	case errors.Is(err, ErrPolicyViolation):
		return "Access forbidden or denied by policy"
	case errors.Is(err, ErrInvalidConfig):
		return "Service configuration error occurred"
	case errors.Is(err, ErrUpstreamError):
		return "Upstream service error occurred"
	default:
		return "An unexpected error occurred"
	}
}
