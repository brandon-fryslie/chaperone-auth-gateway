package mitm

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/bmf/chaperone/internal/errors"
)

// CA represents a certificate authority for MITM operations.
type CA struct {
	cert       *x509.Certificate
	privateKey *rsa.PrivateKey
}

// GenerateCA creates a new self-signed CA certificate and private key.
// The CA uses RSA 4096-bit key and has a 10-year validity period.
func GenerateCA() (*CA, error) {
	// Generate RSA 4096-bit private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, fmt.Errorf("failed to generate RSA private key: %w", err)
	}

	// Generate serial number
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	// Create CA certificate template
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   "Chaperone Local CA",
			Organization: []string{"Chaperone"},
		},
		NotBefore:             now,
		NotAfter:              now.Add(10 * 365 * 24 * time.Hour), // 10 years
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	// Create self-signed certificate
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create CA certificate: %w", err)
	}

	// Parse the certificate
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	return &CA{
		cert:       cert,
		privateKey: privateKey,
	}, nil
}

// LoadCA loads a CA from PEM-encoded key and certificate files.
func LoadCA(keyPath, certPath string) (*CA, error) {
	// Read private key
	keyPEM, err := os.ReadFile(keyPath) //nolint:gosec // Path is user-provided CA key path
	if err != nil {
		return nil, fmt.Errorf("failed to read CA private key: %w", err)
	}

	// Decode PEM block
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, &errors.ConfigError{Field: "ca-key", Value: keyPath, Cause: fmt.Errorf("failed to parse PEM")}
	}

	// Parse private key
	privateKey, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA private key: %w", err)
	}

	// Read certificate
	certPEM, err := os.ReadFile(certPath) //nolint:gosec // Path is user-provided CA cert path
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}

	// Decode PEM block
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, &errors.ConfigError{Field: "ca-cert", Value: certPath, Cause: fmt.Errorf("failed to parse PEM")}
	}

	// Parse certificate
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	// Verify that the private key and certificate match
	publicKey, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, &errors.ConfigError{Field: "ca-cert", Value: certPath, Cause: fmt.Errorf("public key is not RSA")}
	}
	if publicKey.N.Cmp(privateKey.N) != 0 || publicKey.E != privateKey.E {
		return nil, &errors.ConfigError{Field: "ca", Value: "key/cert pair", Cause: fmt.Errorf("private key and certificate mismatch")}
	}

	return &CA{
		cert:       cert,
		privateKey: privateKey,
	}, nil
}

// StoreCA saves the CA private key and certificate to PEM files.
// The private key is saved with 0600 permissions (owner read/write only).
// The certificate is saved with 0644 permissions (world readable).
func StoreCA(ca *CA, keyPath, certPath string) error {
	// Create directory if it doesn't exist (0755 is appropriate for directories)
	keyDir := filepath.Dir(keyPath)
	if err := os.MkdirAll(keyDir, 0755); err != nil { //nolint:gosec // Directory permissions, not file
		return fmt.Errorf("failed to create CA directory: %w", err)
	}

	// Encode private key to PEM
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(ca.privateKey),
	})

	// Write private key with 0600 permissions
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return fmt.Errorf("failed to write CA private key: %w", err)
	}

	// Encode certificate to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: ca.cert.Raw,
	})

	// Write certificate with 0644 permissions (certs are public, unlike keys)
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil { //nolint:gosec // Cert is public, 0644 is appropriate
		return fmt.Errorf("failed to write CA certificate: %w", err)
	}

	return nil
}

// LoadOrGenerateCA loads a CA from files if they exist, or generates a new CA.
// This function is idempotent - it will not regenerate if files already exist.
func LoadOrGenerateCA(keyPath, certPath string) (*CA, error) {
	// Check if both files exist
	_, keyErr := os.Stat(keyPath)
	_, certErr := os.Stat(certPath)

	if keyErr == nil && certErr == nil {
		// Both files exist, load them
		return LoadCA(keyPath, certPath)
	}

	// Generate new CA
	ca, err := GenerateCA()
	if err != nil {
		return nil, err
	}

	// Store the CA
	if err := StoreCA(ca, keyPath, certPath); err != nil {
		return nil, err
	}

	return ca, nil
}

// Certificate returns the CA's certificate.
func (ca *CA) Certificate() *x509.Certificate {
	return ca.cert
}

// PrivateKey returns the CA's private key.
func (ca *CA) PrivateKey() crypto.PrivateKey {
	return ca.privateKey
}

// SignCertificate signs a certificate template with the CA's private key.
// It returns a tls.Certificate containing the signed certificate and its private key.
func (ca *CA) SignCertificate(template *x509.Certificate) (*tls.Certificate, error) {
	if template == nil {
		return nil, &errors.ConfigError{Field: "template", Value: nil, Cause: fmt.Errorf("certificate template cannot be nil")}
	}

	// Generate a private key for the leaf certificate
	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate leaf certificate private key: %w", err)
	}

	// Sign the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &leafKey.PublicKey, ca.privateKey)
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
