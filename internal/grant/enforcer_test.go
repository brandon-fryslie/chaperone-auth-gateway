package grant

import (
	"testing"

	"github.com/bmf/chaperone/internal/config"
	"github.com/bmf/chaperone/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// approvedUniverse is one bounded pairing used across the accept/reject table:
// openai bearer, methods {GET,POST}, paths under /v1/*, body ≤ 1MB.
func approvedUniverse() []config.GrantableConfig {
	return []config.GrantableConfig{
		{
			CredentialRef:  "env:OPENAI_API_KEY",
			HostPattern:    "api.openai.com",
			AuthStrategy:   "bearer",
			AllowedMethods: []string{"GET", "POST"},
			AllowedPaths:   []string{"/v1/*"},
			MaxBodyBytes:   1048576,
		},
	}
}

// grant builds a proposed service refining the openai pairing, overridable per case.
func grant(mutate func(*service.Service)) *service.Service {
	s := &service.Service{
		Name:            "runtime-grant",
		CredentialRef:   "env:OPENAI_API_KEY",
		HostPattern:     "api.openai.com",
		AuthStrategyRef: "bearer",
		Policy: &service.Policy{
			AllowedMethods: []string{"GET"},
			AllowedPaths:   []string{"/v1/chat"},
			MaxBodyBytes:   1024,
		},
	}
	if mutate != nil {
		mutate(s)
	}
	return s
}

func newEnforcer(t *testing.T) *Enforcer {
	t.Helper()
	e, err := NewEnforcer(approvedUniverse())
	require.NoError(t, err)
	return e
}

func TestAuthorize_ExactRefinementAccepted(t *testing.T) {
	e := newEnforcer(t)
	assert.NoError(t, e.Authorize(grant(nil)))
}

func TestAuthorize_EqualToBoundAccepted(t *testing.T) {
	e := newEnforcer(t)
	g := grant(func(s *service.Service) {
		s.Policy.AllowedMethods = []string{"GET", "POST"}
		s.Policy.AllowedPaths = []string{"/v1/*"}
		s.Policy.MaxBodyBytes = 1048576
	})
	assert.NoError(t, e.Authorize(g))
}

func TestAuthorize_IdentityRejections(t *testing.T) {
	e := newEnforcer(t)
	cases := map[string]func(*service.Service){
		"unknown credential_ref": func(s *service.Service) { s.CredentialRef = "env:OTHER_KEY" },
		"credential superstring": func(s *service.Service) { s.CredentialRef = "env:OPENAI_API_KEY_2" },
		"unknown host":           func(s *service.Service) { s.HostPattern = "api.anthropic.com" },
		"host as subdomain":      func(s *service.Service) { s.HostPattern = "api.openai.com.evil.com" },
		"strategy mismatch":      func(s *service.Service) { s.AuthStrategyRef = "header:x-api-key" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Error(t, e.Authorize(grant(mutate)))
		})
	}
}

func TestAuthorize_MethodNarrowing(t *testing.T) {
	e := newEnforcer(t)
	t.Run("subset accepted", func(t *testing.T) {
		assert.NoError(t, e.Authorize(grant(func(s *service.Service) {
			s.Policy.AllowedMethods = []string{"POST"}
		})))
	})
	t.Run("method outside bound rejected", func(t *testing.T) {
		assert.Error(t, e.Authorize(grant(func(s *service.Service) {
			s.Policy.AllowedMethods = []string{"GET", "DELETE"}
		})))
	})
	t.Run("requesting all methods rejected", func(t *testing.T) {
		assert.Error(t, e.Authorize(grant(func(s *service.Service) {
			s.Policy.AllowedMethods = nil
		})))
	})
	t.Run("case mismatch rejected", func(t *testing.T) {
		assert.Error(t, e.Authorize(grant(func(s *service.Service) {
			s.Policy.AllowedMethods = []string{"get"}
		})))
	})
}

func TestAuthorize_PathNarrowing(t *testing.T) {
	e := newEnforcer(t)
	t.Run("sub-prefix accepted", func(t *testing.T) {
		assert.NoError(t, e.Authorize(grant(func(s *service.Service) {
			s.Policy.AllowedPaths = []string{"/v1/embeddings/*"}
		})))
	})
	t.Run("exact within prefix accepted", func(t *testing.T) {
		assert.NoError(t, e.Authorize(grant(func(s *service.Service) {
			s.Policy.AllowedPaths = []string{"/v1/chat"}
		})))
	})
	t.Run("sibling prefix rejected", func(t *testing.T) {
		assert.Error(t, e.Authorize(grant(func(s *service.Service) {
			s.Policy.AllowedPaths = []string{"/v2/*"}
		})))
	})
	t.Run("requesting all paths rejected", func(t *testing.T) {
		assert.Error(t, e.Authorize(grant(func(s *service.Service) {
			s.Policy.AllowedPaths = nil
		})))
	})
	t.Run("prefix-without-slash rejected", func(t *testing.T) {
		// "/v1" is not matched by the bound "/v1/*", so it is not a subset.
		assert.Error(t, e.Authorize(grant(func(s *service.Service) {
			s.Policy.AllowedPaths = []string{"/v1"}
		})))
	})
}

func TestAuthorize_BodySizeNarrowing(t *testing.T) {
	e := newEnforcer(t)
	t.Run("smaller accepted", func(t *testing.T) {
		assert.NoError(t, e.Authorize(grant(func(s *service.Service) {
			s.Policy.MaxBodyBytes = 1024
		})))
	})
	t.Run("equal accepted", func(t *testing.T) {
		assert.NoError(t, e.Authorize(grant(func(s *service.Service) {
			s.Policy.MaxBodyBytes = 1048576
		})))
	})
	t.Run("bigger rejected", func(t *testing.T) {
		assert.Error(t, e.Authorize(grant(func(s *service.Service) {
			s.Policy.MaxBodyBytes = 2097152
		})))
	})
	t.Run("unlimited rejected", func(t *testing.T) {
		assert.Error(t, e.Authorize(grant(func(s *service.Service) {
			s.Policy.MaxBodyBytes = 0
		})))
	})
}

func TestAuthorize_OperatorOnlyFieldsRejected(t *testing.T) {
	e := newEnforcer(t)
	cases := map[string]func(*service.Service){
		"client_groups": func(s *service.Service) { s.Policy.ClientGroups = []string{"team"} },
		"drop":          func(s *service.Service) { s.Policy.Drop = []string{"*.evil.com"} },
		"strip":         func(s *service.Service) { s.Policy.Strip = []string{"X-Trace"} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Error(t, e.Authorize(grant(mutate)))
		})
	}
}

func TestAuthorize_NilServiceRejected(t *testing.T) {
	e := newEnforcer(t)
	assert.Error(t, e.Authorize(nil))
}

func TestAuthorize_NilPolicyIsWidest(t *testing.T) {
	// A nil policy is the widest possible scope; against a bounded pairing it must
	// be rejected (it would widen methods/paths/body all at once).
	e := newEnforcer(t)
	assert.Error(t, e.Authorize(grant(func(s *service.Service) { s.Policy = nil })))
}

func TestAuthorize_UnboundedPairingAcceptsAnything(t *testing.T) {
	// When the human declares no bound, a grant may request any scope (including all).
	e, err := NewEnforcer([]config.GrantableConfig{{
		CredentialRef: "env:OPENAI_API_KEY",
		HostPattern:   "api.openai.com",
		AuthStrategy:  "bearer",
	}})
	require.NoError(t, err)

	assert.NoError(t, e.Authorize(grant(func(s *service.Service) { s.Policy = nil })))
	assert.NoError(t, e.Authorize(grant(func(s *service.Service) {
		s.Policy = &service.Policy{} // request-all on every dimension
	})))
}

func TestAuthorize_HostMatchIsCaseInsensitive(t *testing.T) {
	e := newEnforcer(t)
	assert.NoError(t, e.Authorize(grant(func(s *service.Service) {
		s.HostPattern = "API.OpenAI.com"
	})))
}

func TestAuthorize_HeaderStrategyFormatsAreEquivalent(t *testing.T) {
	// Pairing declared in combined format; grant arrives in separate-field format.
	e, err := NewEnforcer([]config.GrantableConfig{{
		CredentialRef: "env:SVC_KEY",
		HostPattern:   "api.example.com",
		AuthStrategy:  "header:X-API-Key",
	}})
	require.NoError(t, err)

	g := &service.Service{
		CredentialRef:   "env:SVC_KEY",
		HostPattern:     "api.example.com",
		AuthStrategyRef: "header",
		HeaderName:      "X-API-Key",
		Policy:          &service.Policy{},
	}
	assert.NoError(t, e.Authorize(g))
}

func TestNewEnforcer_DuplicatePairingRejected(t *testing.T) {
	dup := append(approvedUniverse(), approvedUniverse()...)
	_, err := NewEnforcer(dup)
	assert.Error(t, err)
}

func TestNewEnforcer_UnsupportedBoundPatternRejected(t *testing.T) {
	_, err := NewEnforcer([]config.GrantableConfig{{
		CredentialRef: "env:OPENAI_API_KEY",
		HostPattern:   "api.openai.com",
		AuthStrategy:  "bearer",
		AllowedPaths:  []string{"/v1/*/messages"}, // mid-path glob, not provably containable
	}})
	assert.Error(t, err)
}
