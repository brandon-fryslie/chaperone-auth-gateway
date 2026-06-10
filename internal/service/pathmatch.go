package service

import (
	"fmt"
	"path"
	"strings"
)

// pathPattern is THE path-pattern vocabulary for every path-gated policy
// decision in the codebase — the allow list (CheckPath), the drop list
// (URLPattern), and grant-scope containment (Narrows) all parse and match
// through this one type, so a path can never classify differently across the
// three controls. [LAW:one-source-of-truth] [LAW:single-enforcer]
//
// The vocabulary is deliberately small and proven containable: an exact path
// ("/v1/chat"), or a subtree prefix ("<prefix>/*", matching every path BELOW
// the prefix at any depth — "/v1/*" matches "/v1/chat" and "/v1/chat/x" but
// not "/v1" itself; "/*" alone matches everything). Anything else (mid-pattern
// globs, "**", "?") is a parse error: a pattern whose containment can't be
// decided would make grant narrowing unsound. [LAW:types-are-the-program]
//
// MATCH SEMANTICS — the one documented policy shared by allow and drop:
//   - The request path is normalized before comparison: leading "/" ensured,
//     dot-segments resolved, duplicate slashes collapsed, trailing slash
//     dropped (path.Clean). The matcher judges the path the way an RFC-3986
//     upstream router will, so "/v1/../admin" is "/admin", never "inside /v1".
//   - Comparison is case-insensitive. The proxy cannot know whether the
//     upstream routes case-sensitively; a security classifier must give every
//     spelling a loose upstream would route identically the same verdict.
//     Case-folding slightly widens allow (fail-open toward the legitimate
//     host only) and strictly strengthens drop (no case bypass) — the safe
//     trade in both directions. Host matching is already case-insensitive.
//   - Patterns must be written in canonical form (leading "/", already clean);
//     a non-canonical pattern is rejected at parse so one intent has exactly
//     one spelling in config.
type pathPattern struct {
	// prefix is normalized and case-folded at parse. For a wildcard pattern it
	// carries the trailing "/" ("/v1/*" → "/v1/"), making the subtree check a
	// single HasPrefix that cannot match the sibling "/v10/...".
	prefix   string
	wildcard bool
}

// parsePathPattern parses one allowlist/droplist entry, rejecting anything
// outside the vocabulary loudly rather than guessing. [LAW:no-silent-failure]
func parsePathPattern(s string) (pathPattern, error) {
	if !strings.HasPrefix(s, "/") {
		return pathPattern{}, fmt.Errorf("path pattern must start with %q", "/")
	}
	if wild, ok := strings.CutSuffix(s, "/*"); ok {
		if wild == "" {
			return pathPattern{prefix: "/", wildcard: true}, nil
		}
		if containsGlobMeta(wild) {
			return pathPattern{}, fmt.Errorf("unsupported glob pattern (only exact paths and a trailing %q are supported)", "/*")
		}
		if wild == "/" || wild != path.Clean(wild) {
			// "//*" would build the prefix "//", which no normalized path can
			// carry — a dead pattern, not a pattern meaning "/*".
			return pathPattern{}, fmt.Errorf("path pattern is not in canonical form (write %q)", path.Clean(wild)+"/*")
		}
		return pathPattern{prefix: strings.ToLower(wild) + "/", wildcard: true}, nil
	}
	if containsGlobMeta(s) {
		return pathPattern{}, fmt.Errorf("unsupported glob pattern (only exact paths and a trailing %q are supported)", "/*")
	}
	if s != path.Clean(s) {
		return pathPattern{}, fmt.Errorf("path pattern is not in canonical form (write %q)", path.Clean(s))
	}
	return pathPattern{prefix: strings.ToLower(s), wildcard: false}, nil
}

func containsGlobMeta(s string) bool {
	return strings.ContainsAny(s, "*?[")
}

// normalizePath reduces a request path to the form an RFC-3986 router resolves
// it to: rooted, dot-segments removed, duplicate slashes collapsed, trailing
// slash dropped (root stays "/"). Matching anything else would let the proxy
// and the upstream disagree about which resource a path names.
func normalizePath(requestPath string) string {
	if !strings.HasPrefix(requestPath, "/") {
		requestPath = "/" + requestPath
	}
	return path.Clean(requestPath)
}

// matches reports whether the (raw, as-received) request path is named by this
// pattern under the documented normalization and case policy.
func (p pathPattern) matches(requestPath string) bool {
	norm := strings.ToLower(normalizePath(requestPath))
	if p.wildcard {
		return strings.HasPrefix(norm, p.prefix)
	}
	return norm == p.prefix
}

// contains reports whether every path matching inner also matches p. Prefixes
// are normalized and folded at parse, so containment is decided over exactly
// the representation matching uses — narrowing can never approve a grant the
// runtime matcher would judge differently.
func (p pathPattern) contains(inner pathPattern) bool {
	if !p.wildcard {
		// Exact outer matches only its own literal; only an equal exact inner is a subset.
		return !inner.wildcard && inner.prefix == p.prefix
	}
	// Wildcard outer "<p>/*" covers every path beginning with "<p>/". An exact inner
	// is contained when it has that prefix; a wildcard inner "<q>/*" is contained
	// when "<q>/" has that prefix — both reduce to one HasPrefix on the literal.
	return strings.HasPrefix(inner.prefix, p.prefix)
}

// ValidatePathPatterns checks that every entry parses in the path-pattern
// vocabulary. It is the rule the entry boundaries (config load, grant
// authorization) apply so an unmatchable pattern fails loudly at the door
// instead of becoming a dead allowlist entry at request time.
func ValidatePathPatterns(patterns []string) error {
	for _, p := range patterns {
		if _, err := parsePathPattern(p); err != nil {
			return fmt.Errorf("path pattern %q: %w", p, err)
		}
	}
	return nil
}
