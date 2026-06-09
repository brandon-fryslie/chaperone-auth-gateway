package grant

import (
	"fmt"
	"strings"

	"github.com/bmf/chaperone/internal/config"
	"github.com/bmf/chaperone/internal/service"
)

// Pairing is one human-approved binding plus the widest scope a grant against it
// may request. Identity (credential ↔ host ↔ strategy) must match a grant exactly;
// the grant's policy may only narrow within MaxBound.
//
// Identity fields are stored canonicalized — host lowercased/trimmed (matching the
// service registry) and auth folded via service.CanonicalAuthRef — so equality
// comparison is the whole identity test.
type Pairing struct {
	identity tripleKey
	MaxBound *service.Policy
}

// CredentialRef is the canonical credential pointer this pairing approves.
func (p *Pairing) CredentialRef() string { return p.identity.credentialRef }

// HostPattern is the normalized host pattern this pairing approves.
func (p *Pairing) HostPattern() string { return p.identity.hostPattern }

// AuthStrategy is the canonical auth strategy ref this pairing approves.
func (p *Pairing) AuthStrategy() string { return p.identity.authStrategy }

// tripleKey is the canonical identity of a pairing, usable as a map key.
type tripleKey struct {
	credentialRef string
	hostPattern   string
	authStrategy  string
}

func identityOf(credentialRef, hostPattern, authStrategy, headerName string) tripleKey {
	return tripleKey{
		credentialRef: credentialRef,
		hostPattern:   strings.ToLower(strings.TrimSpace(hostPattern)),
		authStrategy:  service.CanonicalAuthRef(authStrategy, headerName),
	}
}

// Enforcer is the single authority for "what is grantable". It holds the approved
// universe and authorizes a proposed grant iff its identity matches one pairing
// exactly and its scope narrows within that pairing's maximal bound.
type Enforcer struct {
	pairings map[tripleKey]*Pairing
}

// NewEnforcer builds the enforcer from the human-owned grantable config. It fails
// loudly if two pairings share an identity triple (the universe must be a single
// source of truth — an ambiguous pairing has no defined maximal bound) or if a
// maximal-bound path is outside the supported, proven-containable vocabulary.
func NewEnforcer(grantable []config.GrantableConfig) (*Enforcer, error) {
	pairings := make(map[tripleKey]*Pairing, len(grantable))
	for i := range grantable {
		g := &grantable[i]
		id := identityOf(g.CredentialRef, g.HostPattern, g.AuthStrategy, g.HeaderName)

		if _, dup := pairings[id]; dup {
			return nil, fmt.Errorf("grantable[%d]: duplicate pairing for credential_ref=%q host_pattern=%q auth_strategy=%q",
				i, g.CredentialRef, id.hostPattern, id.authStrategy)
		}

		bound := &service.Policy{
			AllowedMethods: g.AllowedMethods,
			AllowedPaths:   g.AllowedPaths,
			MaxBodyBytes:   g.MaxBodyBytes,
		}
		// Surface unsupported bound patterns at construction (startup), not at grant time.
		if err := service.Narrows(bound, bound); err != nil {
			return nil, fmt.Errorf("grantable[%d]: invalid maximal bound: %w", i, err)
		}

		pairings[id] = &Pairing{identity: id, MaxBound: bound}
	}
	return &Enforcer{pairings: pairings}, nil
}

// ListPairings returns the approved grantable universe so a control client can
// discover what it may ask for. The returned pairings expose only references and
// scope bounds (CredentialRef/HostPattern/AuthStrategy/MaxBound) — never a secret.
// Order is unspecified: the universe is a set keyed by identity triple, not a list.
func (e *Enforcer) ListPairings() []*Pairing {
	out := make([]*Pairing, 0, len(e.pairings))
	for _, p := range e.pairings {
		out = append(out, p)
	}
	return out
}

// Authorize returns nil if the proposed service is an authorized grant, or a
// specific error naming the constraint that failed. A grant is authorized iff its
// identity matches one approved pairing exactly, its policy narrows within that
// pairing's maximal bound, and it sets no operator-only field.
func (e *Enforcer) Authorize(proposed *service.Service) error {
	if proposed == nil {
		return fmt.Errorf("grant rejected: no service proposed")
	}

	if err := operatorOnlyFieldsEmpty(proposed); err != nil {
		return err
	}

	id := identityOf(proposed.CredentialRef, proposed.HostPattern, proposed.AuthStrategyRef, proposed.HeaderName)
	pairing, ok := e.pairings[id]
	if !ok {
		return fmt.Errorf("grant rejected: no approved pairing for credential_ref=%q host_pattern=%q auth_strategy=%q",
			proposed.CredentialRef, id.hostPattern, id.authStrategy)
	}

	if err := service.Narrows(proposed.Policy, pairing.MaxBound); err != nil {
		return fmt.Errorf("grant rejected: scope is not within the approved bound: %w", err)
	}
	return nil
}

// operatorOnlyFieldsEmpty rejects a grant that sets fields the human controls and
// Claude may not grant. These have inverted monotonicity (more entries = more
// restrictive), so they are not modeled as narrowable scope; they must be empty.
func operatorOnlyFieldsEmpty(s *service.Service) error {
	p := s.Policy
	if p == nil {
		return nil
	}
	switch {
	case len(p.ClientGroups) > 0:
		return fmt.Errorf("grant rejected: client_groups is operator-only and not grantable")
	case len(p.Drop) > 0:
		return fmt.Errorf("grant rejected: drop is operator-only and not grantable")
	case len(p.Strip) > 0:
		return fmt.Errorf("grant rejected: strip is operator-only and not grantable")
	}
	return nil
}
