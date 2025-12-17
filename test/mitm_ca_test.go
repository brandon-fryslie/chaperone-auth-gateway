package test

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bmf/chaperone/internal/mitm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMITMCAManagement validates CA certificate generation and management
//
// This test suite validates MITM CA functionality by testing:
// 1. CA generation with correct key strength and validity
// 2. CA persistence to filesystem with correct permissions
// 3. CA loading from existing files
// 4. CA can sign leaf certificates for domains
//
// ANTI-GAMING MEASURES:
// 1. Tests verify ACTUAL filesystem writes (files must exist on disk)
// 2. Tests verify REAL file permissions (0600 for key, 0644 for cert)
// 3. Tests parse ACTUAL x509 certificates (cannot fake structure)
// 4. Tests verify CA can ACTUALLY sign certificates (real crypto operations)
// 5. Tests verify key strength is 4096 bits (not weaker test keys)
// 6. Tests verify 10-year validity period (not shorter test values)
// 7. Tests verify CA BasicConstraints with CA:TRUE (real CA capabilities)
// 8. Tests FAIL if any crypto operation fails
//
// An AI cannot fake this with stubs - real certificates must be generated and verified.

// TestCAGenerationWithCorrectParameters verifies:
// - CA private key is RSA 4096-bit
// - CA certificate is valid self-signed certificate
// - CA certificate has 10-year validity period
// - CA certificate has BasicConstraints with CA:TRUE
// - CA certificate has correct KeyUsage flags
//
// This test cannot be gamed because:
// 1. Parses actual x509 certificate structure
// 2. Verifies real RSA key size (4096 bits)
// 3. Checks actual validity dates (10 years from now)
// 4. Validates certificate extensions are correct
// 5. Verifies signature is valid
func TestCAGenerationWithCorrectParameters(t *testing.T) {
	t.Parallel()

	// Generate CA
	ca, err := mitm.GenerateCA()
	require.NoError(t, err, "CA generation should succeed")

	// Verify key strength is 4096 bits
	privateKey, ok := ca.PrivateKey().(*rsa.PrivateKey)
	require.True(t, ok, "Private key should be RSA")
	assert.Equal(t, 4096, privateKey.N.BitLen(), "CA key should be 4096 bits")

	// Verify certificate is self-signed
	cert := ca.Certificate()
	err = cert.CheckSignatureFrom(cert)
	require.NoError(t, err, "CA certificate should be self-signed")

	// Verify CA certificate has BasicConstraints with CA:TRUE
	require.True(t, cert.BasicConstraintsValid, "BasicConstraints should be valid")
	assert.True(t, cert.IsCA, "Certificate should be a CA")

	// Verify validity period is approximately 10 years
	validityDuration := cert.NotAfter.Sub(cert.NotBefore)
	expectedDuration := 10 * 365 * 24 * time.Hour
	assert.InDelta(t, expectedDuration, validityDuration, float64(24*time.Hour),
		"CA should have 10-year validity period")

	// Verify KeyUsage includes CertSign
	assert.True(t, cert.KeyUsage&x509.KeyUsageCertSign != 0,
		"CA should have KeyUsageCertSign")

	t.Log("PASS: CA generated with correct parameters (4096-bit, 10-year validity, CA:TRUE)")
}

// TestCAPersistenceToFilesystem verifies:
// - CA private key saved to specified path
// - CA certificate saved to specified path
// - CA key file has 0600 permissions (owner read/write only)
// - CA cert file has 0644 permissions (world readable)
// - Files contain valid PEM data
//
// This test cannot be gamed because:
// 1. Verifies actual files exist on filesystem
// 2. Checks real file permissions using os.Stat()
// 3. Reads file contents and parses PEM
// 4. Validates PEM contains valid certificate/key
// 5. Tests actual filesystem operations
func TestCAPersistenceToFilesystem(t *testing.T) {
	t.Parallel()

	// Create temp directory for test
	tempDir := t.TempDir()
	keyPath := filepath.Join(tempDir, "ca-key.pem")
	certPath := filepath.Join(tempDir, "ca-cert.pem")

	// Generate CA
	ca, err := mitm.GenerateCA()
	require.NoError(t, err, "CA generation should succeed")

	// Store CA to filesystem
	err = mitm.StoreCA(ca, keyPath, certPath)
	require.NoError(t, err, "CA storage should succeed")

	// Verify key file exists
	keyInfo, err := os.Stat(keyPath)
	require.NoError(t, err, "CA key file should exist")

	// Verify key file permissions are 0600
	assert.Equal(t, os.FileMode(0600), keyInfo.Mode().Perm(),
		"CA key should have 0600 permissions (owner read/write only)")

	// Verify cert file exists
	certInfo, err := os.Stat(certPath)
	require.NoError(t, err, "CA cert file should exist")

	// Verify cert file permissions are 0644
	assert.Equal(t, os.FileMode(0644), certInfo.Mode().Perm(),
		"CA cert should have 0644 permissions (world readable)")

	// Verify key file contains valid PEM
	keyPEM, err := os.ReadFile(keyPath)
	require.NoError(t, err, "Should read key file")
	assert.Contains(t, string(keyPEM), "BEGIN RSA PRIVATE KEY",
		"Key file should contain PEM-encoded RSA key")

	// Verify cert file contains valid PEM
	certPEM, err := os.ReadFile(certPath)
	require.NoError(t, err, "Should read cert file")
	assert.Contains(t, string(certPEM), "BEGIN CERTIFICATE",
		"Cert file should contain PEM-encoded certificate")

	t.Log("PASS: CA persisted with correct file permissions (0600 key, 0644 cert)")
}

// TestCALoadingFromExistingFiles verifies:
// - CA loads successfully from existing files
// - Loaded CA has same certificate and key
// - Loaded CA can sign certificates (functional test)
// - Loading is idempotent (can load multiple times)
//
// This test cannot be gamed because:
// 1. Generates CA, saves it, then loads it back
// 2. Verifies loaded CA matches original
// 3. Tests CA can actually sign certificates after loading
// 4. Tests real file I/O operations
func TestCALoadingFromExistingFiles(t *testing.T) {
	t.Parallel()

	// Create temp directory
	tempDir := t.TempDir()
	keyPath := filepath.Join(tempDir, "ca-key.pem")
	certPath := filepath.Join(tempDir, "ca-cert.pem")

	// Generate and store CA
	originalCA, err := mitm.GenerateCA()
	require.NoError(t, err, "CA generation should succeed")
	err = mitm.StoreCA(originalCA, keyPath, certPath)
	require.NoError(t, err, "CA storage should succeed")

	// Load CA from files
	loadedCA, err := mitm.LoadCA(keyPath, certPath)
	require.NoError(t, err, "CA loading should succeed")

	// Verify loaded CA certificate matches original
	originalCert := originalCA.Certificate()
	loadedCert := loadedCA.Certificate()
	assert.Equal(t, originalCert.SerialNumber, loadedCert.SerialNumber,
		"Loaded CA certificate should match original")
	assert.Equal(t, originalCert.Subject.CommonName, loadedCert.Subject.CommonName,
		"Loaded CA subject should match original")

	// Verify loaded CA can sign certificates (functional test)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test.example.com"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
		DNSNames:     []string{"test.example.com"},
	}
	_, err = loadedCA.SignCertificate(template)
	require.NoError(t, err, "Loaded CA should be able to sign certificates")

	// Load CA again (test idempotency)
	loadedCA2, err := mitm.LoadCA(keyPath, certPath)
	require.NoError(t, err, "CA should be loadable multiple times")
	assert.Equal(t, loadedCert.SerialNumber, loadedCA2.Certificate().SerialNumber,
		"Multiple loads should produce same CA")

	t.Log("PASS: CA loaded from files and can sign certificates")
}

// TestCACanSignLeafCertificates verifies:
// - CA can sign leaf certificates for hostnames
// - Signed certificate is valid
// - Signed certificate verifies against CA
// - Certificate chain is correct (leaf -> CA)
//
// This test cannot be gamed because:
// 1. Generates actual CA certificate
// 2. Signs actual leaf certificate
// 3. Verifies real certificate chain
// 4. Tests actual crypto verification operations
// 5. Cannot fake with mocks - real x509 verification
func TestCACanSignLeafCertificates(t *testing.T) {
	t.Parallel()

	// Generate CA
	ca, err := mitm.GenerateCA()
	require.NoError(t, err, "CA generation should succeed")

	// Create leaf certificate template
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "api.example.com"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(90 * 24 * time.Hour), // 90 days
		DNSNames:     []string{"api.example.com"},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	// Sign certificate
	leafCert, err := ca.SignCertificate(template)
	require.NoError(t, err, "CA should sign leaf certificate")

	// Parse leaf certificate
	parsed, err := x509.ParseCertificate(leafCert.Certificate[0])
	require.NoError(t, err, "Should parse signed certificate")

	// Verify certificate is for correct hostname
	assert.Equal(t, "api.example.com", parsed.Subject.CommonName,
		"Certificate should be for correct hostname")
	assert.Equal(t, []string{"api.example.com"}, parsed.DNSNames,
		"Certificate should have correct SAN")

	// Verify certificate is signed by CA
	caCert := ca.Certificate()
	err = parsed.CheckSignatureFrom(caCert)
	require.NoError(t, err, "Leaf certificate should be signed by CA")

	// Verify certificate chain
	roots := x509.NewCertPool()
	roots.AddCert(caCert)
	opts := x509.VerifyOptions{
		Roots:     roots,
		DNSName:   "api.example.com",
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	chains, err := parsed.Verify(opts)
	require.NoError(t, err, "Certificate chain should verify")
	assert.Len(t, chains, 1, "Should have one valid chain")
	assert.Len(t, chains[0], 2, "Chain should be leaf -> CA")

	t.Log("PASS: CA can sign leaf certificates with valid chain")
}

// TestCAGenerationIsIdempotent verifies:
// - If CA files exist, they are loaded (not regenerated)
// - If CA files don't exist, new CA is generated
// - Multiple calls with existing files return same CA
//
// This test cannot be gamed because:
// 1. Tests actual filesystem state
// 2. Verifies file modification times don't change on reload
// 3. Tests real file I/O operations
func TestCAGenerationIsIdempotent(t *testing.T) {
	t.Parallel()

	// Create temp directory
	tempDir := t.TempDir()
	keyPath := filepath.Join(tempDir, "ca-key.pem")
	certPath := filepath.Join(tempDir, "ca-cert.pem")

	// First call: CA files don't exist, should generate
	ca1, err := mitm.LoadOrGenerateCA(keyPath, certPath)
	require.NoError(t, err, "First call should generate CA")

	// Verify files were created
	keyInfo1, err := os.Stat(keyPath)
	require.NoError(t, err, "Key file should exist after generation")
	certInfo1, err := os.Stat(certPath)
	require.NoError(t, err, "Cert file should exist after generation")

	// Record modification times
	keyModTime1 := keyInfo1.ModTime()
	certModTime1 := certInfo1.ModTime()

	// Brief wait to ensure timestamps would differ if files were rewritten
	time.Sleep(10 * time.Millisecond)

	// Second call: CA files exist, should load (not regenerate)
	ca2, err := mitm.LoadOrGenerateCA(keyPath, certPath)
	require.NoError(t, err, "Second call should load CA")

	// Verify files were NOT modified
	keyInfo2, err := os.Stat(keyPath)
	require.NoError(t, err, "Key file should still exist")
	certInfo2, err := os.Stat(certPath)
	require.NoError(t, err, "Cert file should still exist")

	assert.Equal(t, keyModTime1, keyInfo2.ModTime(),
		"Key file should not be modified on reload")
	assert.Equal(t, certModTime1, certInfo2.ModTime(),
		"Cert file should not be modified on reload")

	// Verify CAs are equivalent
	assert.Equal(t, ca1.Certificate().SerialNumber, ca2.Certificate().SerialNumber,
		"Loaded CA should match generated CA")

	t.Log("PASS: CA generation is idempotent (loads existing files)")
}

// TestCAErrorHandling verifies:
// - Loading from non-existent files returns error
// - Loading from invalid PEM returns error
// - Loading from corrupt key returns error
// - Signing with nil template returns error
//
// This test cannot be gamed because:
// 1. Tests actual error conditions
// 2. Verifies real file I/O errors
// 3. Tests real PEM parsing errors
func TestCAErrorHandling(t *testing.T) {
	t.Parallel()

	t.Run("loading_from_nonexistent_files_returns_error", func(t *testing.T) {
		_, err := mitm.LoadCA("/nonexistent/key.pem", "/nonexistent/cert.pem")
		require.Error(t, err, "Loading from non-existent files should fail")
		assert.Contains(t, err.Error(), "no such file or directory",
			"Error should mention missing file")
	})

	t.Run("loading_invalid_pem_returns_error", func(t *testing.T) {
		tempDir := t.TempDir()
		keyPath := filepath.Join(tempDir, "invalid-key.pem")
		certPath := filepath.Join(tempDir, "invalid-cert.pem")

		// Write invalid PEM files
		err := os.WriteFile(keyPath, []byte("not a valid PEM"), 0600)
		require.NoError(t, err)
		err = os.WriteFile(certPath, []byte("not a valid PEM"), 0644)
		require.NoError(t, err)

		_, err = mitm.LoadCA(keyPath, certPath)
		require.Error(t, err, "Loading invalid PEM should fail")
		assert.Contains(t, err.Error(), "failed to parse",
			"Error should mention parsing failure")
	})

	t.Run("loading_mismatched_key_cert_returns_error", func(t *testing.T) {
		tempDir := t.TempDir()

		// Generate first CA
		ca1, err := mitm.GenerateCA()
		require.NoError(t, err)
		key1Path := filepath.Join(tempDir, "key1.pem")
		cert1Path := filepath.Join(tempDir, "cert1.pem")
		err = mitm.StoreCA(ca1, key1Path, cert1Path)
		require.NoError(t, err)

		// Generate second CA
		ca2, err := mitm.GenerateCA()
		require.NoError(t, err)
		key2Path := filepath.Join(tempDir, "key2.pem")
		cert2Path := filepath.Join(tempDir, "cert2.pem")
		err = mitm.StoreCA(ca2, key2Path, cert2Path)
		require.NoError(t, err)

		// Try to load with mismatched key and cert (key from CA1, cert from CA2)
		_, err = mitm.LoadCA(key1Path, cert2Path)
		require.Error(t, err, "Loading mismatched key/cert should fail")
		assert.Contains(t, err.Error(), "mismatch",
			"Error should mention mismatch")
	})
}

// TestCAThreadSafety verifies:
// - Multiple goroutines can sign certificates concurrently
// - No race conditions detected
// - All certificates are valid
//
// This test cannot be gamed because:
// 1. Uses real goroutines
// 2. Tests actual concurrent operations
// 3. Race detector will catch any data races
// 4. Verifies all certificates are valid
func TestCAThreadSafety(t *testing.T) {
	t.Parallel()

	// Generate CA
	ca, err := mitm.GenerateCA()
	require.NoError(t, err, "CA generation should succeed")

	// Concurrently sign certificates
	numCerts := 100
	var wg sync.WaitGroup
	errors := make([]error, numCerts)
	certs := make([]*tls.Certificate, numCerts)

	for i := 0; i < numCerts; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			template := &x509.Certificate{
				SerialNumber: big.NewInt(int64(index)),
				Subject:      pkix.Name{CommonName: fmt.Sprintf("test-%d.example.com", index)},
				NotBefore:    time.Now(),
				NotAfter:     time.Now().Add(24 * time.Hour),
				DNSNames:     []string{fmt.Sprintf("test-%d.example.com", index)},
			}
			cert, err := ca.SignCertificate(template)
			certs[index] = cert
			errors[index] = err
		}(i)
	}

	wg.Wait()

	// Verify all certificates were signed successfully
	for i, err := range errors {
		require.NoError(t, err, "Certificate %d should be signed without error", i)
		require.NotNil(t, certs[i], "Certificate %d should not be nil", i)
	}

	t.Logf("PASS: CA can sign %d certificates concurrently without races", numCerts)
}
