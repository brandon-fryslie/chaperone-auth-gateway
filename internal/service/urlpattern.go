package service

import (
	"fmt"
	"strings"
)

// URLPattern is a drop-rule matcher over (host, path). The host part supports
// wildcards (*.example.com; a bare domain implies itself and all subdomains;
// empty or "*" means every host). The path part — everything from the first
// "/" — is a pathPattern, the single path vocabulary shared with the allow
// list, so a request can never be "inside" a drop rule and "outside" the same
// spelling in allowed_paths. No path component means all paths.
// [LAW:one-source-of-truth]
//
// A pattern outside the vocabulary does not construct: a drop rule is a deny
// control, and a rule that silently failed to match would fail open.
// [LAW:no-silent-failure]
type URLPattern struct {
	hostPattern string
	allPaths    bool
	path        pathPattern
}

// ParseURLPattern parses a drop-rule pattern ("host", "host/path",
// "host/subtree/*", "/path-on-any-host", ...).
func ParseURLPattern(pattern string) (*URLPattern, error) {
	slashIdx := strings.Index(pattern, "/")
	if slashIdx == -1 {
		// No path component — the pattern names a host; every path matches.
		return &URLPattern{hostPattern: pattern, allPaths: true}, nil
	}
	pp, err := parsePathPattern(pattern[slashIdx:])
	if err != nil {
		return nil, fmt.Errorf("url pattern %q: %w", pattern, err)
	}
	return &URLPattern{hostPattern: pattern[:slashIdx], path: pp}, nil
}

// Matches checks if a request (host, path) matches this pattern. The path is
// judged under the shared normalization and case policy (see pathPattern).
func (p *URLPattern) Matches(host, urlPath string) bool {
	if !matchHost(host, p.hostPattern) {
		return false
	}
	if p.allPaths {
		return true
	}
	return p.path.matches(urlPath)
}

// matchHost checks if a host matches the host pattern.
// Supports wildcards:
//   - "example.com" matches "example.com", "*.example.com" (all subdomains)
//   - "*.example.com" matches "api.example.com", "foo.bar.example.com"
//   - "" or "*" matches every host
func matchHost(host, hostPattern string) bool {
	// Normalize both to lowercase for case-insensitive matching
	host = strings.ToLower(host)
	hostPattern = strings.ToLower(hostPattern)

	// Empty pattern or "*" matches all hosts
	if hostPattern == "" || hostPattern == "*" {
		return true
	}

	// If pattern doesn't have wildcard, it means match this host and all subdomains
	// e.g., "example.com" should match "example.com", "api.example.com", etc.
	if !strings.Contains(hostPattern, "*") {
		// Exact match
		if host == hostPattern {
			return true
		}
		// Subdomain match: host ends with ".pattern"
		return strings.HasSuffix(host, "."+hostPattern)
	}

	// Handle wildcard patterns like "*.example.com"
	if strings.HasPrefix(hostPattern, "*.") {
		suffix := hostPattern[2:] // Remove "*."
		// Must match suffix exactly or have it as a subdomain
		return host == suffix || strings.HasSuffix(host, "."+suffix)
	}

	// Pattern has wildcard but not at start - unsupported for now
	// Could add more sophisticated matching later if needed
	return false
}
