package service

import (
	"strings"

	"github.com/bmf/chaperone/internal/errors"
)

// Validate checks if the Service configuration is valid.
func (s *Service) Validate() error {
	if s == nil {
		return &errors.ConfigError{
			Field: "service",
			Value: nil,
			Cause: errors.ErrInvalidConfig,
		}
	}

	if strings.TrimSpace(s.HostPattern) == "" {
		return &errors.ConfigError{
			Field: "HostPattern",
			Value: s.HostPattern,
			Cause: errors.ErrInvalidConfig,
		}
	}

	if strings.TrimSpace(s.AuthStrategyRef) == "" {
		return &errors.ConfigError{
			Field: "AuthStrategy",
			Value: s.AuthStrategyRef,
			Cause: errors.ErrInvalidConfig,
		}
	}

	if strings.TrimSpace(s.CredentialRef) == "" {
		return &errors.ConfigError{
			Field: "CredentialRef",
			Value: s.CredentialRef,
			Cause: errors.ErrInvalidConfig,
		}
	}

	// Validate the policy if present
	if s.Policy != nil {
		if err := s.Policy.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// Validate checks if the Policy configuration is valid.
func (p *Policy) Validate() error {
	if p == nil {
		return nil // nil policy is valid (no restrictions)
	}

	if p.MaxBodyBytes < 0 {
		return &errors.ConfigError{
			Field: "MaxBodyBytes",
			Value: p.MaxBodyBytes,
			Cause: errors.ErrInvalidConfig,
		}
	}

	return nil
}

// ApplyDefaults sets default values for unset policy fields.
func (p *Policy) ApplyDefaults() {
	if p == nil {
		return
	}

	// Default MaxBodyBytes to 10MB if not set
	if p.MaxBodyBytes == 0 {
		p.MaxBodyBytes = 10 * 1024 * 1024 // 10MB
	}
}
