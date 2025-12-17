package mitm

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"strings"
	"sync"
	"time"
)

// CertCache manages dynamic certificate generation and caching.
type CertCache struct {
	ca         *CA
	cache      sync.Map // map[string]*tls.Certificate
	generating sync.Map // map[string]chan struct{} - for coordinating concurrent generation
	serialMu   sync.Mutex
	serialNext int64
}

// NewCertCache creates a new certificate cache.
func NewCertCache(ca *CA) *CertCache {
	return &CertCache{
		ca:         ca,
		serialNext: 1,
	}
}

// GetCertificate retrieves or generates a certificate for the given hostname.
// Hostnames are normalized to lowercase and ports are stripped.
func (c *CertCache) GetCertificate(hostname string) (*tls.Certificate, error) {
	// Normalize hostname: strip port and convert to lowercase
	host, _, err := net.SplitHostPort(hostname)
	if err != nil {
		// No port present, use hostname as-is
		host = hostname
	}
	host = strings.ToLower(host)

	// Check cache
	if cached, ok := c.cache.Load(host); ok {
		return cached.(*tls.Certificate), nil
	}

	// Try to atomically mark this hostname as being generated
	// If another goroutine is already generating, wait for it
	ch := make(chan struct{})
	actual, loaded := c.generating.LoadOrStore(host, ch)
	if loaded {
		// Another goroutine is generating, wait for it
		<-actual.(chan struct{})
		// Now it should be in cache
		if cached, ok := c.cache.Load(host); ok {
			return cached.(*tls.Certificate), nil
		}
		// Should not happen, but handle it anyway
		return c.GetCertificate(hostname)
	}

	// We're the generator
	defer func() {
		c.generating.Delete(host)
		close(ch)
	}()

	// Double-check cache (another goroutine might have completed between our first check and LoadOrStore)
	if cached, ok := c.cache.Load(host); ok {
		return cached.(*tls.Certificate), nil
	}

	// Generate new certificate
	cert, err := c.generateCertificate(host)
	if err != nil {
		return nil, err
	}

	// Store in cache
	c.cache.Store(host, cert)

	return cert, nil
}

// generateCertificate creates a new certificate for the given hostname.
func (c *CertCache) generateCertificate(hostname string) (*tls.Certificate, error) {
	// Generate serial number (protected by mutex)
	c.serialMu.Lock()
	serialNumber := big.NewInt(c.serialNext)
	c.serialNext++
	c.serialMu.Unlock()

	// Create certificate template
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: hostname,
		},
		NotBefore:   now,
		NotAfter:    now.Add(90 * 24 * time.Hour), // 90 days
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	// Check if hostname is an IP address or a DNS name
	if ip := net.ParseIP(hostname); ip != nil {
		// It's an IP address - add to IPAddresses
		template.IPAddresses = []net.IP{ip}
	} else {
		// It's a DNS name - add to DNSNames
		template.DNSNames = []string{hostname}
	}

	// Generate private key for leaf certificate
	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate leaf certificate private key: %w", err)
	}

	// Sign certificate with CA
	certDER, err := x509.CreateCertificate(rand.Reader, template, c.ca.cert, &leafKey.PublicKey, c.ca.privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign certificate: %w", err)
	}

	// Create tls.Certificate
	tlsCert := &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  leafKey,
	}

	return tlsCert, nil
}
