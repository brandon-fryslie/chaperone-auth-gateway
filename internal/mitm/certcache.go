package mitm

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"strings"
	"sync"
	"time"
)

// CertCache manages dynamic certificate generation and caching.
type CertCache struct {
	ca *CA
	// mu guards both cache and generating, so "is it cached / is someone
	// already generating / claim the generator slot" is one atomic decision.
	mu         sync.Mutex
	cache      map[string]*tls.Certificate
	generating map[string]chan struct{} // single-flight coordination per hostname
	serialMu   sync.Mutex
	serialNext int64
	logger     *slog.Logger
}

// NewCertCache creates a new certificate cache.
func NewCertCache(ca *CA, logger *slog.Logger) *CertCache {
	return &CertCache{
		ca:         ca,
		cache:      make(map[string]*tls.Certificate),
		generating: make(map[string]chan struct{}),
		serialNext: 1,
		logger:     logger,
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

	c.mu.Lock()
	if cert, ok := c.cache[host]; ok {
		c.mu.Unlock()
		if c.logger != nil {
			c.logger.Debug("using cached certificate", "hostname", host)
		}
		return cert, nil
	}

	// Another goroutine is already generating this hostname: wait for it,
	// then read the result it stored.
	if ch, generating := c.generating[host]; generating {
		c.mu.Unlock()
		<-ch
		c.mu.Lock()
		cert, ok := c.cache[host]
		c.mu.Unlock()
		if ok {
			return cert, nil
		}
		// The generator failed (errored before storing). Retry as a fresh attempt.
		return c.GetCertificate(hostname)
	}

	// We hold the lock and there is no cached cert and no generation in
	// flight, so we claim the generator slot atomically.
	ch := make(chan struct{})
	c.generating[host] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.generating, host)
		c.mu.Unlock()
		close(ch)
	}()

	cert, err := c.generateCertificate(host)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.cache[host] = cert
	c.mu.Unlock()
	if c.logger != nil {
		c.logger.Info("generated certificate with CA", "hostname", host)
	}

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

	// Create tls.Certificate with complete chain: [leaf, root CA]
	tlsCert := &tls.Certificate{
		Certificate: [][]byte{certDER, c.ca.cert.Raw},
		PrivateKey:  leafKey,
	}

	return tlsCert, nil
}
