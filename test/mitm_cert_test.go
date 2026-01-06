package test

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/bmf/chaperone/internal/mitm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMITMDynamicCertificateGeneration validates dynamic certificate generation

func TestCertificateGenerationForHostname(t *testing.T) {
	t.Parallel()

	// Generate CA
	ca, err := mitm.GenerateCA()
	require.NoError(t, err, "CA generation should succeed")

	// Create certificate cache
	certCache := mitm.NewCertCache(ca, nil)

	// Generate certificate for hostname
	hostname := "api.example.com"
	tlsCert, err := certCache.GetCertificate(hostname)
	require.NoError(t, err, "Certificate generation should succeed")
	require.NotNil(t, tlsCert, "Certificate should not be nil")

	// Parse certificate
	parsed, err := x509.ParseCertificate(tlsCert.Certificate[0])
	require.NoError(t, err, "Should parse certificate")

	// Verify Subject CN matches hostname
	assert.Equal(t, hostname, parsed.Subject.CommonName,
		"Certificate CN should match hostname")

	// Verify SAN includes hostname
	assert.Contains(t, parsed.DNSNames, hostname,
		"Certificate SAN should include hostname")

	// Verify key type and size
	publicKey, ok := parsed.PublicKey.(*rsa.PublicKey)
	require.True(t, ok, "Public key should be RSA")
	assert.Equal(t, 2048, publicKey.N.BitLen(),
		"Certificate should use 2048-bit RSA key")

	// Verify certificate validity period (90 days)
	validityDuration := parsed.NotAfter.Sub(parsed.NotBefore)
	expectedDuration := 90 * 24 * time.Hour
	assert.InDelta(t, expectedDuration, validityDuration, float64(24*time.Hour),
		"Certificate should have 90-day validity")

	t.Log("PASS: Certificate generated with correct hostname and parameters")
}

func TestCertificateSignedByCA(t *testing.T) {
	t.Parallel()

	// Generate CA
	ca, err := mitm.GenerateCA()
	require.NoError(t, err, "CA generation should succeed")

	// Create certificate cache
	certCache := mitm.NewCertCache(ca, nil)

	// Generate certificate
	hostname := "test.example.com"
	tlsCert, err := certCache.GetCertificate(hostname)
	require.NoError(t, err, "Certificate generation should succeed")

	// Parse leaf certificate
	leafCert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	require.NoError(t, err, "Should parse leaf certificate")

	// Verify signature from CA
	caCert := ca.Certificate()
	err = leafCert.CheckSignatureFrom(caCert)
	require.NoError(t, err, "Leaf certificate should be signed by CA")

	// Verify certificate chain
	roots := x509.NewCertPool()
	roots.AddCert(caCert)
	opts := x509.VerifyOptions{
		Roots:     roots,
		DNSName:   hostname,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	chains, err := leafCert.Verify(opts)
	require.NoError(t, err, "Certificate chain should verify")
	assert.Len(t, chains, 1, "Should have one valid chain")

	t.Log("PASS: Certificate is validly signed by CA")
}

func TestCertificateCaching(t *testing.T) {
	t.Parallel()

	// Generate CA
	ca, err := mitm.GenerateCA()
	require.NoError(t, err, "CA generation should succeed")

	// Create certificate cache
	certCache := mitm.NewCertCache(ca, nil)

	// Generate certificate for hostname
	hostname := "cache-test.example.com"
	cert1, err := certCache.GetCertificate(hostname)
	require.NoError(t, err, "First certificate generation should succeed")

	// Parse first certificate
	parsed1, err := x509.ParseCertificate(cert1.Certificate[0])
	require.NoError(t, err, "Should parse first certificate")

	// Request same hostname again
	cert2, err := certCache.GetCertificate(hostname)
	require.NoError(t, err, "Second certificate request should succeed")

	// Parse second certificate
	parsed2, err := x509.ParseCertificate(cert2.Certificate[0])
	require.NoError(t, err, "Should parse second certificate")

	// Verify certificates are identical (same serial number = cache hit)
	assert.Equal(t, parsed1.SerialNumber, parsed2.SerialNumber,
		"Cached certificate should have same serial number")
	assert.Equal(t, parsed1.NotBefore, parsed2.NotBefore,
		"Cached certificate should have same NotBefore")

	// Request different hostname
	cert3, err := certCache.GetCertificate("different.example.com")
	require.NoError(t, err, "Different hostname should generate certificate")
	parsed3, err := x509.ParseCertificate(cert3.Certificate[0])
	require.NoError(t, err, "Should parse third certificate")

	// Verify different hostname gets different certificate
	assert.NotEqual(t, parsed1.SerialNumber, parsed3.SerialNumber,
		"Different hostname should get different certificate")

	t.Log("PASS: Certificate caching works correctly")
}

func TestCertificateExpirationHandling(t *testing.T) {
	t.Parallel()

	// Generate CA
	ca, err := mitm.GenerateCA()
	require.NoError(t, err, "CA generation should succeed")

	// Create certificate cache
	certCache := mitm.NewCertCache(ca, nil)

	// Generate certificate
	hostname := "expiry-test.example.com"
	cert1, err := certCache.GetCertificate(hostname)
	require.NoError(t, err, "Certificate generation should succeed")
	parsed1, err := x509.ParseCertificate(cert1.Certificate[0])
	require.NoError(t, err, "Should parse certificate")

	// Verify certificate is not expired
	now := time.Now()
	assert.True(t, parsed1.NotAfter.After(now),
		"Certificate should not be expired")
	assert.True(t, parsed1.NotBefore.Before(now),
		"Certificate should be valid now")

	// Request again immediately (should be cached)
	cert2, err := certCache.GetCertificate(hostname)
	require.NoError(t, err, "Cached certificate should be returned")
	parsed2, err := x509.ParseCertificate(cert2.Certificate[0])
	require.NoError(t, err, "Should parse cached certificate")
	assert.Equal(t, parsed1.SerialNumber, parsed2.SerialNumber,
		"Certificate should be cached (same serial)")

	t.Log("PASS: Certificate expiration handling works")
}

func TestCertificateSANCorrectness(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		hostname string
		wantDNS  []string
	}{
		{
			name:     "simple_hostname",
			hostname: "api.example.com",
			wantDNS:  []string{"api.example.com"},
		},
		{
			name:     "subdomain",
			hostname: "api.v1.example.com",
			wantDNS:  []string{"api.v1.example.com"},
		},
		{
			name:     "hyphenated_hostname",
			hostname: "api-staging.example.com",
			wantDNS:  []string{"api-staging.example.com"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Generate CA
			ca, err := mitm.GenerateCA()
			require.NoError(t, err, "CA generation should succeed")

			// Create certificate cache
			certCache := mitm.NewCertCache(ca, nil)

			// Generate certificate
			tlsCert, err := certCache.GetCertificate(tc.hostname)
			require.NoError(t, err, "Certificate generation should succeed")

			// Parse certificate
			parsed, err := x509.ParseCertificate(tlsCert.Certificate[0])
			require.NoError(t, err, "Should parse certificate")

			// Verify SAN
			assert.Equal(t, tc.wantDNS, parsed.DNSNames,
				"Certificate should have correct SAN")

			// Verify certificate is valid for hostname
			err = parsed.VerifyHostname(tc.hostname)
			require.NoError(t, err, "Certificate should be valid for hostname")
		})
	}
}

func TestCertificatePortStripping(t *testing.T) {
	t.Parallel()

	// Generate CA
	ca, err := mitm.GenerateCA()
	require.NoError(t, err, "CA generation should succeed")

	// Create certificate cache
	certCache := mitm.NewCertCache(ca, nil)

	// Generate certificate with port
	hostnameWithPort := "api.example.com:443"
	tlsCert, err := certCache.GetCertificate(hostnameWithPort)
	require.NoError(t, err, "Certificate generation should succeed")

	// Parse certificate
	parsed, err := x509.ParseCertificate(tlsCert.Certificate[0])
	require.NoError(t, err, "Should parse certificate")

	// Verify SAN does NOT include port
	assert.Contains(t, parsed.DNSNames, "api.example.com",
		"SAN should include hostname without port")
	assert.NotContains(t, parsed.DNSNames, "api.example.com:443",
		"SAN should not include port")

	// Verify same certificate is returned for hostname without port
	tlsCert2, err := certCache.GetCertificate("api.example.com")
	require.NoError(t, err, "Certificate request without port should succeed")
	parsed2, err := x509.ParseCertificate(tlsCert2.Certificate[0])
	require.NoError(t, err, "Should parse second certificate")

	assert.Equal(t, parsed.SerialNumber, parsed2.SerialNumber,
		"Same certificate should be returned regardless of port in request")

	t.Log("PASS: Port stripping works correctly")
}

func TestCertificateCaseInsensitiveHostname(t *testing.T) {
	t.Parallel()

	// Generate CA
	ca, err := mitm.GenerateCA()
	require.NoError(t, err, "CA generation should succeed")

	// Create certificate cache
	certCache := mitm.NewCertCache(ca, nil)

	// Generate certificate with lowercase hostname
	tlsCert1, err := certCache.GetCertificate("api.example.com")
	require.NoError(t, err, "Certificate generation should succeed")
	parsed1, err := x509.ParseCertificate(tlsCert1.Certificate[0])
	require.NoError(t, err, "Should parse first certificate")

	// Request with uppercase hostname
	tlsCert2, err := certCache.GetCertificate("API.EXAMPLE.COM")
	require.NoError(t, err, "Certificate request with uppercase should succeed")
	parsed2, err := x509.ParseCertificate(tlsCert2.Certificate[0])
	require.NoError(t, err, "Should parse second certificate")

	// Verify same certificate is returned
	assert.Equal(t, parsed1.SerialNumber, parsed2.SerialNumber,
		"Same certificate should be returned for case variants")

	// Verify SAN is lowercase
	assert.Contains(t, parsed1.DNSNames, "api.example.com",
		"SAN should be lowercase")
	assert.NotContains(t, parsed1.DNSNames, "API.EXAMPLE.COM",
		"SAN should not contain uppercase")

	t.Log("PASS: Case-insensitive hostname handling works")
}

func TestCertificateConcurrentAccess(t *testing.T) {
	t.Parallel()

	// Generate CA
	ca, err := mitm.GenerateCA()
	require.NoError(t, err, "CA generation should succeed")

	// Create certificate cache
	certCache := mitm.NewCertCache(ca, nil)

	// Concurrently request certificates
	numRequests := 100
	var wg sync.WaitGroup
	errors := make([]error, numRequests)
	certs := make([]*tls.Certificate, numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			hostname := fmt.Sprintf("host-%d.example.com", index%10) // 10 unique hosts
			cert, err := certCache.GetCertificate(hostname)
			certs[index] = cert
			errors[index] = err
		}(i)
	}

	wg.Wait()

	// Verify all requests succeeded
	for i, err := range errors {
		require.NoError(t, err, "Request %d should succeed", i)
		require.NotNil(t, certs[i], "Certificate %d should not be nil", i)
	}

	// Verify certificates for same hostname have same serial (cached)
	serialsByHost := make(map[string]*big.Int)
	for i := 0; i < numRequests; i++ {
		parsed, err := x509.ParseCertificate(certs[i].Certificate[0])
		require.NoError(t, err, "Should parse certificate %d", i)

		hostname := fmt.Sprintf("host-%d.example.com", i%10)
		if existing, ok := serialsByHost[hostname]; ok {
			assert.Equal(t, existing, parsed.SerialNumber,
				"Certificates for same hostname should have same serial")
		} else {
			serialsByHost[hostname] = parsed.SerialNumber
		}
	}

	t.Logf("PASS: %d concurrent certificate requests succeeded", numRequests)
}

func TestCertificateValidityDates(t *testing.T) {
	t.Parallel()

	// Generate CA
	ca, err := mitm.GenerateCA()
	require.NoError(t, err, "CA generation should succeed")

	// Create certificate cache
	certCache := mitm.NewCertCache(ca, nil)

	// Generate certificate
	tlsCert, err := certCache.GetCertificate("validity-test.example.com")
	require.NoError(t, err, "Certificate generation should succeed")

	// Parse certificate
	parsed, err := x509.ParseCertificate(tlsCert.Certificate[0])
	require.NoError(t, err, "Should parse certificate")

	// Verify validity dates
	now := time.Now()
	assert.True(t, parsed.NotBefore.Before(now),
		"Certificate NotBefore should be in the past")
	assert.True(t, parsed.NotAfter.After(now),
		"Certificate NotAfter should be in the future")

	// Verify certificate is currently valid (using x509 validation)
	roots := x509.NewCertPool()
	roots.AddCert(ca.Certificate())
	opts := x509.VerifyOptions{
		Roots:       roots,
		CurrentTime: now,
		DNSName:     "validity-test.example.com",
	}
	_, err = parsed.Verify(opts)
	require.NoError(t, err, "Certificate should be valid now")

	t.Log("PASS: Certificate has valid NotBefore/NotAfter dates")
}
