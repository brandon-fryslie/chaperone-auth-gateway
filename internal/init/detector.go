package init

import (
	"net"
	"net/http"
	"strings"
)

// Detector analyzes HTTP requests to detect authentication patterns and policy constraints.
type Detector struct {
	config   DetectorConfig
	evidence *Evidence
}

// NewDetector creates a new request detector.
func NewDetector(config DetectorConfig, evidence *Evidence) *Detector {
	return &Detector{
		config:   config,
		evidence: evidence,
	}
}

// AnalyzeRequest examines an HTTP request and records findings in the evidence store.
// This is called by the init proxy for every request that passes through.
func (d *Detector) AnalyzeRequest(r *http.Request) {
	// Normalize hostname (strip port if present)
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	// Check if host should be excluded
	if d.shouldExclude(host) {
		return
	}

	// Detect auth patterns in headers
	findings := DetectAuth(r.Header, d.config)
	for _, finding := range findings {
		d.evidence.RecordAuthFinding(host, finding)
	}

	// Record policy constraints
	d.evidence.RecordMethod(host, r.Method)
	d.evidence.RecordPath(host, r.URL.Path)

	// Record body size if Content-Length is set
	if r.ContentLength > 0 {
		d.evidence.RecordBodySize(host, r.ContentLength)
	}
}

// shouldExclude checks if a host should be excluded from detection.
func (d *Detector) shouldExclude(host string) bool {
	hostLower := strings.ToLower(host)
	for _, excluded := range d.config.ExcludeHosts {
		if strings.ToLower(excluded) == hostLower {
			return true
		}
	}
	return false
}
