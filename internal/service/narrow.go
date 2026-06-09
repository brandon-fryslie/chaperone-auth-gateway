package service

import (
	"fmt"
	"strings"
)

// Narrows reports whether requested is a refinement of (subset of, or equal to)
// bound: every request the requested policy would permit, bound also permits.
//
// It governs only the three bounded scope dimensions — methods, paths, body size
// — where an empty allowlist / zero limit means "widest" (allow all / unlimited).
// A nil policy is treated as the widest policy. It returns a specific error naming
// the dimension and values that widen, or nil if requested narrows within bound.
//
// Path patterns are restricted to the proven-containable vocabulary (exact or
// "<prefix>/*"); an unsupported glob in either argument is rejected rather than
// guessed, keeping the check sound ([LAW:no-silent-failure]).
func Narrows(requested, bound *Policy) error {
	req := orEmpty(requested)
	max := orEmpty(bound)

	if err := methodsNarrow(req.AllowedMethods, max.AllowedMethods); err != nil {
		return err
	}
	if err := pathsNarrow(req.AllowedPaths, max.AllowedPaths); err != nil {
		return err
	}
	return bodyNarrow(req.MaxBodyBytes, max.MaxBodyBytes)
}

func orEmpty(p *Policy) *Policy {
	if p == nil {
		return &Policy{}
	}
	return p
}

// methodsNarrow checks the requested method allowlist is a subset of the bound's.
// Empty = all methods; a non-empty bound with an empty request is a widening.
func methodsNarrow(req, bound []string) error {
	if len(bound) == 0 {
		return nil // bound allows all methods → any request narrows or equals
	}
	if len(req) == 0 {
		return fmt.Errorf("methods widen: request allows all methods, bound restricts to %v", bound)
	}
	allowed := make(map[string]struct{}, len(bound))
	for _, m := range bound {
		allowed[m] = struct{}{}
	}
	for _, m := range req {
		if _, ok := allowed[m]; !ok {
			return fmt.Errorf("methods widen: method %q is not within bound %v", m, bound)
		}
	}
	return nil
}

// pathsNarrow checks every requested path pattern is contained by some bound pattern.
// Empty = all paths; a non-empty bound with an empty request is a widening.
func pathsNarrow(req, bound []string) error {
	if len(bound) == 0 {
		return nil // bound allows all paths → any request narrows or equals
	}
	if len(req) == 0 {
		return fmt.Errorf("paths widen: request allows all paths, bound restricts to %v", bound)
	}

	boundPats := make([]pathPattern, len(bound))
	for i, b := range bound {
		pat, err := parsePathPattern(b)
		if err != nil {
			return fmt.Errorf("bound path %q: %w", b, err)
		}
		boundPats[i] = pat
	}

	for _, r := range req {
		inner, err := parsePathPattern(r)
		if err != nil {
			return fmt.Errorf("requested path %q: %w", r, err)
		}
		if !containedByAny(inner, boundPats) {
			return fmt.Errorf("paths widen: requested path %q is not within bound %v", r, bound)
		}
	}
	return nil
}

// bodyNarrow checks the requested body limit does not exceed the bound's, where 0
// means unlimited (widest).
func bodyNarrow(req, bound int64) error {
	if bound == 0 {
		return nil // bound is unlimited → any request narrows or equals
	}
	if req == 0 {
		return fmt.Errorf("body size widens: request is unlimited, bound is %d bytes", bound)
	}
	if req > bound {
		return fmt.Errorf("body size widens: request %d bytes exceeds bound %d bytes", req, bound)
	}
	return nil
}

// pathPattern is a path allowlist entry from the supported vocabulary: an exact
// path, or a prefix wildcard ("<prefix>/*" stored as prefix "<prefix>/").
type pathPattern struct {
	prefix   string // for wildcard: the literal prefix that paths must start with
	wildcard bool
}

// parsePathPattern parses an allowlist entry, rejecting any glob construct outside
// the proven-containable vocabulary (exact, or a single trailing "/*").
func parsePathPattern(s string) (pathPattern, error) {
	if wild, ok := strings.CutSuffix(s, "/*"); ok {
		body := wild + "/"
		if containsGlobMeta(body) {
			return pathPattern{}, fmt.Errorf("unsupported glob pattern (only exact paths and a trailing \"/*\" are supported)")
		}
		return pathPattern{prefix: body, wildcard: true}, nil
	}
	if containsGlobMeta(s) {
		return pathPattern{}, fmt.Errorf("unsupported glob pattern (only exact paths and a trailing \"/*\" are supported)")
	}
	return pathPattern{prefix: s, wildcard: false}, nil
}

func containsGlobMeta(s string) bool {
	return strings.ContainsAny(s, "*?[")
}

// containedByAny reports whether inner is contained by at least one of outers.
func containedByAny(inner pathPattern, outers []pathPattern) bool {
	for _, o := range outers {
		if contains(o, inner) {
			return true
		}
	}
	return false
}

// contains reports whether every path matching inner also matches outer.
func contains(outer, inner pathPattern) bool {
	if !outer.wildcard {
		// Exact outer matches only its own literal; only an equal exact inner is a subset.
		return !inner.wildcard && inner.prefix == outer.prefix
	}
	// Wildcard outer "<p>/*" covers every path beginning with "<p>/". An exact inner
	// is contained when it has that prefix; a wildcard inner "<q>/*" is contained
	// when "<q>/" has that prefix — both reduce to one HasPrefix on the literal.
	return strings.HasPrefix(inner.prefix, outer.prefix)
}
