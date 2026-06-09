package control

import (
	"fmt"
	"log/slog"

	"github.com/bmf/chaperone/internal/audit"
	"github.com/bmf/chaperone/internal/grant"
	"github.com/bmf/chaperone/internal/service"
)

// API is the control-plane logic: the four operations a client may invoke, over
// the three collaborators that own live state. It computes typed results and
// applies exactly two effects — registry mutation and an audit write — at this
// single boundary ([LAW:effects-at-boundaries]). It resolves no secrets and
// re-decides no policy: grant authorization is delegated wholesale to the
// enforcer ([LAW:single-enforcer]).
type API struct {
	enforcer *grant.Enforcer
	registry service.ServiceRegistry
	audit    audit.AuditLogger
	logger   *slog.Logger
}

// NewAPI builds the control API over its collaborators. All three are required:
// a nil enforcer would mean "nothing is grantable" expressed as a crash instead
// of a value, and a nil registry/audit would silently drop state or trail.
func NewAPI(enforcer *grant.Enforcer, registry service.ServiceRegistry, auditLogger audit.AuditLogger, logger *slog.Logger) (*API, error) {
	switch {
	case enforcer == nil:
		return nil, fmt.Errorf("control: enforcer is required")
	case registry == nil:
		return nil, fmt.Errorf("control: service registry is required")
	case auditLogger == nil:
		return nil, fmt.Errorf("control: audit logger is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &API{enforcer: enforcer, registry: registry, audit: auditLogger, logger: logger}, nil
}

// logAudit writes an audit entry and surfaces a write failure to the operational
// log. A grant trail that silently fails to record is a security gap, not a minor
// IO hiccup — it must be loud ([LAW:no-silent-failure]).
func (a *API) logAudit(entry audit.Entry) {
	if err := a.audit.Log(entry); err != nil {
		a.logger.Error("audit log write failed", "event", entry.Event, "error", err)
	}
}

// errKind categorizes a control failure so the transport can map it to an outcome
// contract (4xx = the request is wrong; 5xx = the daemon failed). The client
// surfaces the message verbatim regardless of kind.
type errKind int

const (
	kindRejected   errKind = iota // enforcer refused: deterministic, client's fault
	kindBadRequest                // malformed input
	kindInternal                  // daemon-side failure applying an authorized op
)

type apiError struct {
	kind errKind
	err  error
}

func (e *apiError) Error() string { return e.err.Error() }
func (e *apiError) Unwrap() error { return e.err }

func rejected(err error) *apiError   { return &apiError{kind: kindRejected, err: err} }
func badRequest(err error) *apiError { return &apiError{kind: kindBadRequest, err: err} }
func internal(err error) *apiError   { return &apiError{kind: kindInternal, err: err} }

// Grant authorizes a proposed pairing through the enforcer and, only if accepted,
// upserts it into the live registry so the host becomes injection-eligible at
// once. Every outcome — accepted or rejected — is audited with the reference and
// scope, never a secret.
func (a *API) Grant(req GrantRequest) (GrantResult, error) {
	svc := req.toService()

	// Trust boundary: the enforcer is the single authority. Authorize on the RAW
	// policy — it reads empty/zero scope as "widest"; defaulting first would lie
	// about what was asked.
	if err := a.enforcer.Authorize(svc); err != nil {
		a.logAudit(audit.Entry{
			Event:        audit.EventGrantRejected,
			Service:      svc.Name,
			Host:         svc.HostPattern,
			AuthStrategy: svc.AuthStrategyRef,
			ClientIP:     "unix-socket",
			Outcome:      "blocked",
			ErrorMessage: err.Error(),
			Detail:       grantDetail(svc),
		})
		return GrantResult{}, rejected(err)
	}

	// Accepted. Apply the proxy's normal policy defaults so the stored, enforced
	// scope matches a statically-configured service (a strict narrowing of an
	// unbounded field — safe past the boundary check).
	svc.Policy.ApplyDefaults()

	if err := a.registry.Upsert(svc); err != nil {
		return GrantResult{}, internal(fmt.Errorf("apply grant: %w", err))
	}

	a.logAudit(audit.Entry{
		Event:        audit.EventGrantApplied,
		Service:      svc.Name,
		Host:         svc.HostPattern,
		AuthStrategy: svc.AuthStrategyRef,
		ClientIP:     "unix-socket",
		Outcome:      "success",
		Detail:       grantDetail(svc),
	})

	return GrantResult{Service: serviceView(svc)}, nil
}

// Revoke removes the active grant for a host pattern. Absence is a soft success
// (idempotent, DELETE-style): Unregister's only failure mode is "no such host",
// so a non-nil error means nothing was there to remove, not a swallowed fault.
func (a *API) Revoke(req RevokeRequest) (RevokeResult, error) {
	if req.HostPattern == "" {
		return RevokeResult{}, badRequest(fmt.Errorf("revoke: host_pattern is required"))
	}

	err := a.registry.Unregister(req.HostPattern)
	revoked := err == nil

	outcome := "success"
	detail := "grant revoked"
	if !revoked {
		detail = "no active grant for host"
	}
	a.logAudit(audit.Entry{
		Event:    audit.EventGrantRevoked,
		Host:     req.HostPattern,
		ClientIP: "unix-socket",
		Outcome:  outcome,
		Detail:   detail,
	})

	return RevokeResult{HostPattern: req.HostPattern, Revoked: revoked}, nil
}

// List returns the live active set: every service currently injection-eligible
// (static config and runtime grants alike — one type, two sources). References
// only; no secret value is present in any view.
func (a *API) List() ListResult {
	all := a.registry.ListAll()
	views := make([]ServiceView, 0, len(all))
	for _, svc := range all {
		views = append(views, serviceView(svc))
	}
	return ListResult{Services: views}
}

// ListGrantable returns the human-approved universe a client may ask from.
func (a *API) ListGrantable() ListGrantableResult {
	pairings := a.enforcer.ListPairings()
	views := make([]PairingView, 0, len(pairings))
	for _, p := range pairings {
		views = append(views, PairingView{
			CredentialRef: p.CredentialRef(),
			HostPattern:   p.HostPattern(),
			AuthStrategy:  p.AuthStrategy(),
			MaxBound:      policyView(p.MaxBound),
		})
	}
	return ListGrantableResult{Pairings: views}
}

func serviceView(svc *service.Service) ServiceView {
	return ServiceView{
		Name:          svc.Name,
		HostPattern:   svc.HostPattern,
		CredentialRef: svc.CredentialRef,
		AuthStrategy:  svc.AuthStrategyRef,
		HeaderName:    svc.HeaderName,
		Placeholder:   svc.Placeholder,
		Scope:         policyView(svc.Policy),
	}
}

// grantDetail summarizes the reference and scope for the audit trail. The
// credential_ref is a pointer (env:/file:/keychain:), safe to record; a secret
// value is never resolved here.
func grantDetail(svc *service.Service) string {
	p := svc.Policy
	if p == nil {
		return fmt.Sprintf("credential_ref=%s scope=unrestricted", svc.CredentialRef)
	}
	return fmt.Sprintf("credential_ref=%s methods=%v paths=%v max_body_bytes=%d",
		svc.CredentialRef, p.AllowedMethods, p.AllowedPaths, p.MaxBodyBytes)
}
