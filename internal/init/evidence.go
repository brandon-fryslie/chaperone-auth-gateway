package init

import (
	"strings"
	"sync"
)

// Evidence is a thread-safe store for accumulating detection findings.
// It collects auth findings and policy constraints per host during detection.
type Evidence struct {
	mu       sync.RWMutex
	findings map[string]*ServiceFindings // key: lowercase hostname
}

// NewEvidence creates a new evidence store.
func NewEvidence() *Evidence {
	return &Evidence{
		findings: make(map[string]*ServiceFindings),
	}
}

// RecordAuthFinding records an authentication finding for a host.
// If a finding for the same header already exists, it updates the confidence
// score to the maximum of the two (we keep the highest confidence detection).
func (e *Evidence) RecordAuthFinding(host string, finding *Finding) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Normalize hostname (lowercase)
	hostKey := strings.ToLower(host)

	// Ensure ServiceFindings exists for this host
	if e.findings[hostKey] == nil {
		e.findings[hostKey] = NewServiceFindings(host)
	}

	sf := e.findings[hostKey]

	// Deduplicate by header name (lowercase)
	headerKey := strings.ToLower(finding.HeaderName)

	// If we already have a finding for this header, keep the one with higher confidence
	if existing, exists := sf.AuthFindings[headerKey]; exists {
		if finding.Confidence > existing.Confidence {
			sf.AuthFindings[headerKey] = finding
		}
		// Otherwise keep the existing higher-confidence finding
	} else {
		// New finding
		sf.AuthFindings[headerKey] = finding
	}
}

// RecordMethod records an observed HTTP method for a host.
func (e *Evidence) RecordMethod(host, method string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	hostKey := strings.ToLower(host)
	if e.findings[hostKey] == nil {
		e.findings[hostKey] = NewServiceFindings(host)
	}

	e.findings[hostKey].Policy.Methods[method] = true
}

// RecordPath records an observed URL path for a host.
// The path is stored as-is; generalization happens during config generation.
func (e *Evidence) RecordPath(host, path string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	hostKey := strings.ToLower(host)
	if e.findings[hostKey] == nil {
		e.findings[hostKey] = NewServiceFindings(host)
	}

	e.findings[hostKey].Policy.Paths[path] = true
}

// RecordBodySize records an observed Content-Length for a host.
// Only updates if the new size is larger than the current maximum.
func (e *Evidence) RecordBodySize(host string, bodySize int64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	hostKey := strings.ToLower(host)
	if e.findings[hostKey] == nil {
		e.findings[hostKey] = NewServiceFindings(host)
	}

	if bodySize > e.findings[hostKey].Policy.MaxBodyBytes {
		e.findings[hostKey].Policy.MaxBodyBytes = bodySize
	}
}

// GetFindings returns a copy of all findings for a host.
// Returns nil if no findings exist for the host.
func (e *Evidence) GetFindings(host string) *ServiceFindings {
	e.mu.RLock()
	defer e.mu.RUnlock()

	hostKey := strings.ToLower(host)
	sf := e.findings[hostKey]
	if sf == nil {
		return nil
	}

	// Return a deep copy to prevent external modification
	copy := NewServiceFindings(sf.Host)

	// Copy auth findings
	for k, v := range sf.AuthFindings {
		findingCopy := *v
		copy.AuthFindings[k] = &findingCopy
	}

	// Copy policy findings
	for method := range sf.Policy.Methods {
		copy.Policy.Methods[method] = true
	}
	for path := range sf.Policy.Paths {
		copy.Policy.Paths[path] = true
	}
	copy.Policy.MaxBodyBytes = sf.Policy.MaxBodyBytes

	return copy
}

// GetAllHosts returns a list of all hosts that have findings.
func (e *Evidence) GetAllHosts() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	hosts := make([]string, 0, len(e.findings))
	for _, sf := range e.findings {
		hosts = append(hosts, sf.Host)
	}
	return hosts
}

// GetTopFinding returns the auth finding with the highest confidence for a host.
// Returns nil if no auth findings exist for the host.
func (e *Evidence) GetTopFinding(host string) *Finding {
	e.mu.RLock()
	defer e.mu.RUnlock()

	hostKey := strings.ToLower(host)
	sf := e.findings[hostKey]
	if sf == nil || len(sf.AuthFindings) == 0 {
		return nil
	}

	var topFinding *Finding
	for _, finding := range sf.AuthFindings {
		if topFinding == nil || finding.Confidence > topFinding.Confidence {
			topFinding = finding
		}
	}

	if topFinding == nil {
		return nil
	}

	// Return a copy
	copy := *topFinding
	return &copy
}
