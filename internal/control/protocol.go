package control

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/bmf/chaperone/internal/service"
)

// Wire paths for the control plane. Server (mux) and client (request line) read
// these constants from one place so the two ends cannot drift ([LAW:one-source-of-truth]).
const (
	PathGrant         = "/grant"
	PathRevoke        = "/revoke"
	PathList          = "/list"
	PathListGrantable = "/list-grantable"
)

// GrantRequest is what a control client asks the daemon to activate. It carries
// only REFERENCES (credential_ref is a pointer like "env:NAME", never a secret
// value) and the SCOPE the grant requests. It deliberately omits the operator-only
// policy fields (client_groups / drop / strip): those are not Claude's to set, so
// the wire type makes them unrepresentable rather than accepting-then-rejecting
// ([LAW:types-are-the-program], [LAW:effects-at-boundaries]). The enforcer remains
// the authoritative single check.
type GrantRequest struct {
	// Name is an optional display label for the active grant. It is not part of
	// the trust-boundary identity; if empty the host pattern is used.
	Name string `json:"name,omitempty"`

	HostPattern   string `json:"host_pattern"`
	CredentialRef string `json:"credential_ref"`
	AuthStrategy  string `json:"auth_strategy"`
	HeaderName    string `json:"header_name,omitempty"`
	Placeholder   string `json:"placeholder,omitempty"`

	// Scope the grant requests. Empty / zero means "widest" and must still narrow
	// within the pairing's maximal bound (the enforcer decides).
	AllowedMethods []string `json:"allowed_methods,omitempty"`
	AllowedPaths   []string `json:"allowed_paths,omitempty"`
	MaxBodyBytes   int64    `json:"max_body_bytes,omitempty"`
}

// toService builds the *service.Service the grant payload represents, with the
// auth ref canonicalized exactly as the config→Service bridge does so identity
// matching against the enforcer's universe is apples-to-apples. The policy carries
// only the grantable scope fields; operator-only fields are left empty by
// construction (the request type cannot carry them).
func (g GrantRequest) toService() *service.Service {
	name := g.Name
	if name == "" {
		name = g.HostPattern
	}
	return &service.Service{
		Name:            name,
		HostPattern:     g.HostPattern,
		AuthStrategyRef: service.CanonicalAuthRef(g.AuthStrategy, g.HeaderName),
		HeaderName:      g.HeaderName,
		CredentialRef:   g.CredentialRef,
		Placeholder:     g.Placeholder,
		Policy: &service.Policy{
			AllowedMethods: g.AllowedMethods,
			AllowedPaths:   g.AllowedPaths,
			MaxBodyBytes:   g.MaxBodyBytes,
		},
	}
}

// RevokeRequest names the active grant to remove by its host pattern (the
// registry's identity key). Matched exactly after normalization.
type RevokeRequest struct {
	HostPattern string `json:"host_pattern"`
}

// PolicyView is the scope of an active grant or grantable bound, references only.
type PolicyView struct {
	AllowedMethods []string `json:"allowed_methods,omitempty"`
	AllowedPaths   []string `json:"allowed_paths,omitempty"`
	MaxBodyBytes   int64    `json:"max_body_bytes,omitempty"`
}

func policyView(p *service.Policy) PolicyView {
	if p == nil {
		return PolicyView{}
	}
	return PolicyView{
		AllowedMethods: p.AllowedMethods,
		AllowedPaths:   p.AllowedPaths,
		MaxBodyBytes:   p.MaxBodyBytes,
	}
}

// ServiceView is one live, injection-eligible service. credential_ref is the
// pointer; no secret value is ever present.
type ServiceView struct {
	Name          string     `json:"name"`
	HostPattern   string     `json:"host_pattern"`
	CredentialRef string     `json:"credential_ref"`
	AuthStrategy  string     `json:"auth_strategy"`
	HeaderName    string     `json:"header_name,omitempty"`
	Placeholder   string     `json:"placeholder,omitempty"`
	Scope         PolicyView `json:"scope"`
}

// PairingView is one approved grantable pairing: the universe a client may ask
// from. References and the maximal scope bound only.
type PairingView struct {
	CredentialRef string     `json:"credential_ref"`
	HostPattern   string     `json:"host_pattern"`
	AuthStrategy  string     `json:"auth_strategy"`
	MaxBound      PolicyView `json:"max_bound"`
}

// GrantResult is the response to a successful grant.
type GrantResult struct {
	Service ServiceView `json:"service"`
}

// RevokeResult is the response to a revoke. Revoked reports whether a grant was
// actually present and removed; an absent host is a soft success (idempotent
// DELETE-style), so Revoked=false distinguishes "removed" from "nothing to remove".
type RevokeResult struct {
	HostPattern string `json:"host_pattern"`
	Revoked     bool   `json:"revoked"`
}

// ListResult is the live active set (everything currently injection-eligible).
type ListResult struct {
	Services []ServiceView `json:"services"`
}

// ListGrantableResult is the approved universe.
type ListGrantableResult struct {
	Pairings []PairingView `json:"pairings"`
}

// errorBody is the JSON envelope for any non-2xx response. It carries the failing
// constraint's message verbatim so the client can surface it without re-deciding
// ([LAW:single-enforcer]).
type errorBody struct {
	Error string `json:"error"`
}

// writeJSON encodes v as the response body with the given status. This is the
// single encoder both success and error paths route through.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError sends a non-2xx response carrying err's message verbatim.
func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, errorBody{Error: err.Error()})
}

// decodeJSON reads a JSON request body into v, rejecting unknown fields so a
// client cannot smuggle operator-only policy fields past the wire type.
func decodeJSON(r io.Reader, v any) error {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	return nil
}
