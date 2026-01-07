package proxy

import (
	"crypto/tls"

	"github.com/bmf/chaperone/internal/mitm"
)

// GoproxyCertStore adapts mitm.CertCache to goproxy's CertStorage interface.
// It wraps the existing certificate cache to provide the interface goproxy expects.
type GoproxyCertStore struct {
	cache *mitm.CertCache
}

// NewGoproxyCertStore creates a new certificate store adapter.
func NewGoproxyCertStore(cache *mitm.CertCache) *GoproxyCertStore {
	return &GoproxyCertStore{cache: cache}
}

// Fetch implements goproxy's CertStorage interface.
// The gen parameter is ignored since our CertCache handles generation internally.
func (s *GoproxyCertStore) Fetch(hostname string, gen func() (*tls.Certificate, error)) (*tls.Certificate, error) {
	return s.cache.GetCertificate(hostname)
}
