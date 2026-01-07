package service

import (
	"path"
	"strings"
)

// URLPattern represents a URL pattern matcher that supports:
// - Host patterns with wildcards: *.example.com, example.com
// - Path patterns with * (single level) and ** (any level)
// - No protocol = all protocols
// - No subdomain = all subdomains (*.host)
// - No path = all paths (/**/*)
type URLPattern struct {
	pattern string
}

// NewURLPattern creates a new URL pattern matcher.
func NewURLPattern(pattern string) *URLPattern {
	return &URLPattern{pattern: pattern}
}

// Matches checks if a URL (host + path) matches this pattern.
// url should be in the format "host/path" (no protocol)
func (p *URLPattern) Matches(host, urlPath string) bool {
	// Split pattern into host and path parts
	hostPattern, pathPattern := p.parsePattern()

	// Check host match
	if !p.matchHost(host, hostPattern) {
		return false
	}

	// Check path match
	return p.matchPath(urlPath, pathPattern)
}

// parsePattern splits the pattern into host and path components.
// Examples:
//   - "example.com" -> ("example.com", "")
//   - "*.example.com" -> ("*.example.com", "")
//   - "example.com/api" -> ("example.com", "/api")
//   - "*.example.com/**/index.html" -> ("*.example.com", "/**/index.html")
func (p *URLPattern) parsePattern() (host, path string) {
	// Find first slash to separate host from path
	slashIdx := strings.Index(p.pattern, "/")
	if slashIdx == -1 {
		// No path component - pattern is just a host
		return p.pattern, ""
	}

	return p.pattern[:slashIdx], p.pattern[slashIdx:]
}

// matchHost checks if a host matches the host pattern.
// Supports wildcards:
//   - "example.com" matches "example.com", "*.example.com" (all subdomains)
//   - "*.example.com" matches "api.example.com", "foo.bar.example.com"
//   - "api.example.com" matches "api.example.com" exactly
func (p *URLPattern) matchHost(host, hostPattern string) bool {
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

// matchPath checks if a URL path matches the path pattern.
// Supports:
//   - "" (empty) = match all paths
//   - "/api" = exact match
//   - "/api/*" = match /api/anything (single level)
//   - "/api/**" or "/api/**/*" = match /api/anything at any depth
//   - "/**/index.html" = match index.html at any depth
func (p *URLPattern) matchPath(urlPath, pathPattern string) bool {
	// Empty pattern matches all paths
	if pathPattern == "" {
		return true
	}

	// Normalize paths - ensure they start with "/"
	if !strings.HasPrefix(urlPath, "/") {
		urlPath = "/" + urlPath
	}
	if !strings.HasPrefix(pathPattern, "/") {
		pathPattern = "/" + pathPattern
	}

	// Clean paths to remove double slashes, etc.
	urlPath = path.Clean(urlPath)
	pathPattern = path.Clean(pathPattern)

	// Check if pattern contains wildcards
	if !strings.Contains(pathPattern, "*") {
		// No wildcards - exact match
		return urlPath == pathPattern
	}

	// Handle ** (match any depth)
	if strings.Contains(pathPattern, "**") {
		return p.matchPathWithDoubleGlob(urlPath, pathPattern)
	}

	// Handle * (match single level)
	return p.matchPathWithSingleGlob(urlPath, pathPattern)
}

// matchPathWithDoubleGlob handles patterns with ** (match any depth).
// Examples:
//   - "/**/index.html" matches "/index.html", "/api/index.html", "/api/v1/index.html"
//   - "/api/**" matches "/api/foo", "/api/foo/bar", "/api/foo/bar/baz"
func (p *URLPattern) matchPathWithDoubleGlob(urlPath, pathPattern string) bool {
	// Split pattern into segments
	patternParts := strings.Split(pathPattern, "/")
	urlParts := strings.Split(urlPath, "/")

	// Remove empty first element (from leading slash)
	if len(patternParts) > 0 && patternParts[0] == "" {
		patternParts = patternParts[1:]
	}
	if len(urlParts) > 0 && urlParts[0] == "" {
		urlParts = urlParts[1:]
	}

	return p.matchSegments(urlParts, patternParts)
}

// matchSegments recursively matches URL segments against pattern segments.
func (p *URLPattern) matchSegments(urlParts, patternParts []string) bool {
	// Base cases
	if len(patternParts) == 0 {
		return len(urlParts) == 0
	}

	currentPattern := patternParts[0]

	// Handle ** (match zero or more path segments)
	if currentPattern == "**" {
		// ** at the end matches anything remaining
		if len(patternParts) == 1 {
			return true
		}

		// Try matching ** with 0, 1, 2, ... segments
		for i := 0; i <= len(urlParts); i++ {
			if p.matchSegments(urlParts[i:], patternParts[1:]) {
				return true
			}
		}
		return false
	}

	// Need at least one URL segment to match against
	if len(urlParts) == 0 {
		return false
	}

	currentURL := urlParts[0]

	// Handle * (match single segment)
	if currentPattern == "*" {
		return p.matchSegments(urlParts[1:], patternParts[1:])
	}

	// Exact match required
	if currentURL != currentPattern {
		return false
	}

	return p.matchSegments(urlParts[1:], patternParts[1:])
}

// matchPathWithSingleGlob handles patterns with * (match single level).
// Examples:
//   - "/api/*/users" matches "/api/v1/users", "/api/v2/users"
//   - "/api/*" matches "/api/foo" but not "/api/foo/bar"
func (p *URLPattern) matchPathWithSingleGlob(urlPath, pathPattern string) bool {
	// Split into segments
	patternParts := strings.Split(pathPattern, "/")
	urlParts := strings.Split(urlPath, "/")

	// Remove empty first element (from leading slash)
	if len(patternParts) > 0 && patternParts[0] == "" {
		patternParts = patternParts[1:]
	}
	if len(urlParts) > 0 && urlParts[0] == "" {
		urlParts = urlParts[1:]
	}

	// Must have same number of segments (since * matches exactly one)
	if len(patternParts) != len(urlParts) {
		return false
	}

	// Check each segment
	for i := range patternParts {
		if patternParts[i] == "*" {
			// * matches any single segment
			continue
		}
		if patternParts[i] != urlParts[i] {
			return false
		}
	}

	return true
}
