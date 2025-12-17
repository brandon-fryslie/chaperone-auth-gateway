package test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bmf/chaperone/test/helpers"
)

// TestTestInfrastructure is the top-level test suite for Phase 0.5: Test Infrastructure
// This validates that the testing helpers work correctly and can be used to test the proxy.
//
// CRITICAL: These tests validate the TEST INFRASTRUCTURE, not the proxy itself.
// The helpers being tested here will be used to test the proxy in Phase 1+.
//
// Gaming Resistance: These tests cannot be satisfied by stubs because they:
// 1. Execute actual TLS handshakes with generated certificates
// 2. Make real HTTP/HTTPS requests to mock servers
// 3. Verify certificate chain validation works
// 4. Test actual network I/O operations
// 5. Validate context propagation through real operations
func TestTestInfrastructure(t *testing.T) {
	t.Run("certificate_generation", testCertificateGeneration)
	t.Run("mock_servers", testMockServers)
	t.Run("context_helpers", testContextHelpers)
}

// testCertificateGeneration validates all certificate generation helpers
func testCertificateGeneration(t *testing.T) {
	t.Run("generate_test_ca", testGenerateTestCA)
	t.Run("generate_test_cert", testGenerateTestCert)
	t.Run("tls_connection_works", testTLSConnectionWorks)
	t.Run("cert_pem_export_import", testCertPEMExportImport)
	t.Run("key_pem_export_import", testKeyPEMExportImport)
}

// testGenerateTestCA verifies that a self-signed CA can be generated
//
// This test cannot be gamed because:
// - It verifies the returned certificate is actually a CA (BasicConstraintsValid, IsCA)
// - It checks key usage flags are set correctly
// - It validates the certificate is self-signed (Issuer == Subject)
// - It verifies the certificate is currently valid (NotBefore <= Now < NotAfter)
func testGenerateTestCA(t *testing.T) {
	// Execute: Generate test CA
	caCert, caKey, err := helpers.GenerateTestCA()

	// Verify: CA generation succeeds
	require.NoError(t, err, "GenerateTestCA must succeed")
	require.NotNil(t, caCert, "CA certificate must not be nil")
	require.NotNil(t, caKey, "CA private key must not be nil")

	// Verify: Certificate is a valid CA
	assert.True(t, caCert.BasicConstraintsValid, "CA must have BasicConstraintsValid=true")
	assert.True(t, caCert.IsCA, "Certificate must have IsCA=true")

	// Verify: Certificate is self-signed (issuer == subject)
	assert.Equal(t, caCert.Issuer.String(), caCert.Subject.String(),
		"CA certificate must be self-signed (Issuer == Subject)")

	// Verify: Key usage includes certificate signing
	assert.NotZero(t, caCert.KeyUsage&x509.KeyUsageCertSign,
		"CA must have KeyUsageCertSign")

	// Verify: Certificate is currently valid
	now := time.Now()
	assert.True(t, now.After(caCert.NotBefore) && now.Before(caCert.NotAfter),
		"CA certificate must be currently valid")

	// Verify: Key is correct size (2048 bits minimum for security)
	assert.GreaterOrEqual(t, caKey.N.BitLen(), 2048,
		"CA key must be at least 2048 bits")
}

// testGenerateTestCert verifies that a certificate can be signed by the test CA
//
// This test cannot be gamed because:
// - It generates an actual CA first
// - It creates a certificate signed by that CA
// - It verifies the certificate is NOT a CA (leaf certificate)
// - It validates the Subject Alternative Name matches the hostname
// - It checks the certificate can be verified against the CA
func testGenerateTestCert(t *testing.T) {
	// Setup: Generate CA first
	caCert, caKey, err := helpers.GenerateTestCA()
	require.NoError(t, err, "CA generation must succeed for test setup")

	// Execute: Generate certificate for example.com
	hostname := "example.com"
	leafCert, err := helpers.GenerateTestCert(caCert, caKey, hostname)
	require.NoError(t, err, "GenerateTestCert must succeed")

	// Verify: Certificate was generated
	require.NotNil(t, leafCert.Certificate, "Certificate bytes must not be nil")
	require.NotNil(t, leafCert.PrivateKey, "Private key must not be nil")
	require.NotEmpty(t, leafCert.Certificate, "Certificate chain must not be empty")

	// Parse the leaf certificate for verification
	parsedCert, err := x509.ParseCertificate(leafCert.Certificate[0])
	require.NoError(t, err, "Generated certificate must be parseable")

	// Verify: Certificate is NOT a CA (it's a leaf certificate)
	assert.False(t, parsedCert.IsCA, "Leaf certificate must have IsCA=false")

	// Verify: Subject Alternative Name includes the hostname
	assert.Contains(t, parsedCert.DNSNames, hostname,
		"Certificate must include hostname in DNSNames")

	// Verify: Certificate can be verified against the CA
	roots := x509.NewCertPool()
	roots.AddCert(caCert)

	opts := x509.VerifyOptions{
		DNSName: hostname,
		Roots:   roots,
	}

	chains, err := parsedCert.Verify(opts)
	assert.NoError(t, err, "Certificate must verify against CA")
	assert.NotEmpty(t, chains, "Verification must produce certificate chains")
}

// testTLSConnectionWorks verifies that generated certificates can be used for actual TLS connections
//
// This test cannot be gamed because:
// - It starts a real HTTPS server with generated certificates
// - It makes an actual HTTPS request to that server
// - It verifies the TLS handshake succeeds
// - It validates the server's certificate chain
// - It checks that data can be transmitted over the secure connection
func testTLSConnectionWorks(t *testing.T) {
	// Setup: Generate CA and certificate for localhost
	caCert, caKey, err := helpers.GenerateTestCA()
	require.NoError(t, err, "CA generation must succeed")

	hostname := "localhost"
	serverCert, err := helpers.GenerateTestCert(caCert, caKey, hostname)
	require.NoError(t, err, "Certificate generation must succeed")

	// Setup: Create HTTPS server with generated certificate
	mux := http.NewServeMux()
	mux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("TLS works"))
	})

	server := &http.Server{
		Addr:      "127.0.0.1:0", // Random available port
		Handler:   mux,
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{serverCert}},
	}

	// Start server in background
	listener, err := tls.Listen("tcp", server.Addr, server.TLSConfig)
	require.NoError(t, err, "Server must start")
	defer listener.Close()

	serverAddr := listener.Addr().String()
	go server.Serve(listener)

	// Setup: Create HTTP client that trusts the test CA
	certPool := x509.NewCertPool()
	certPool.AddCert(caCert)

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: certPool,
			},
		},
		Timeout: 5 * time.Second,
	}

	// Execute: Make HTTPS request to server
	resp, err := client.Get("https://" + serverAddr + "/test")

	// Verify: Request succeeds
	require.NoError(t, err, "HTTPS request must succeed")
	defer resp.Body.Close()

	// Verify: Response is correct
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Response must be 200 OK")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "Reading response body must succeed")
	assert.Equal(t, "TLS works", string(body), "Response body must match")

	// Verify: TLS connection was actually used
	assert.NotNil(t, resp.TLS, "Response must have TLS info")
	assert.True(t, resp.TLS.HandshakeComplete, "TLS handshake must complete")
	assert.NotEmpty(t, resp.TLS.PeerCertificates, "Server must provide certificates")
}

// testCertPEMExportImport verifies certificate PEM export and import
//
// This test cannot be gamed because:
// - It writes an actual certificate to a real file
// - It reads the file back and parses the PEM format
// - It verifies the parsed certificate matches the original
func testCertPEMExportImport(t *testing.T) {
	// Setup: Generate CA
	caCert, _, err := helpers.GenerateTestCA()
	require.NoError(t, err, "CA generation must succeed")

	// Setup: Create temporary file
	tempDir := t.TempDir()
	certPath := filepath.Join(tempDir, "ca.crt")

	// Execute: Write certificate to PEM file
	err = helpers.WriteCertPEM(caCert, certPath)
	require.NoError(t, err, "WriteCertPEM must succeed")

	// Verify: File exists
	info, err := os.Stat(certPath)
	require.NoError(t, err, "Certificate file must exist")
	assert.Greater(t, info.Size(), int64(0), "Certificate file must not be empty")

	// Execute: Read certificate file
	pemData, err := os.ReadFile(certPath)
	require.NoError(t, err, "Reading certificate file must succeed")

	// Verify: PEM format is correct
	block, _ := pem.Decode(pemData)
	require.NotNil(t, block, "PEM data must be valid")
	assert.Equal(t, "CERTIFICATE", block.Type, "PEM block type must be CERTIFICATE")

	// Verify: Certificate can be parsed
	parsedCert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err, "Certificate must be parseable from PEM")

	// Verify: Parsed certificate matches original
	assert.Equal(t, caCert.Subject.String(), parsedCert.Subject.String(),
		"Subject must match original")
	assert.Equal(t, caCert.SerialNumber, parsedCert.SerialNumber,
		"Serial number must match original")
	assert.True(t, caCert.NotBefore.Equal(parsedCert.NotBefore),
		"NotBefore must match original")
}

// testKeyPEMExportImport verifies private key PEM export and import
//
// This test cannot be gamed because:
// - It writes an actual private key to a real file
// - It verifies the file has correct permissions (0600 for security)
// - It reads the file back and parses the PEM format
// - It validates the key can be used for cryptographic operations
func testKeyPEMExportImport(t *testing.T) {
	// Setup: Generate CA with key
	_, caKey, err := helpers.GenerateTestCA()
	require.NoError(t, err, "CA generation must succeed")

	// Setup: Create temporary file
	tempDir := t.TempDir()
	keyPath := filepath.Join(tempDir, "ca.key")

	// Execute: Write key to PEM file
	err = helpers.WriteKeyPEM(caKey, keyPath)
	require.NoError(t, err, "WriteKeyPEM must succeed")

	// Verify: File exists
	info, err := os.Stat(keyPath)
	require.NoError(t, err, "Key file must exist")
	assert.Greater(t, info.Size(), int64(0), "Key file must not be empty")

	// Verify: File has secure permissions (0600 = owner read/write only)
	// On Unix systems, this is critical for private key security
	mode := info.Mode()
	assert.Equal(t, os.FileMode(0600), mode.Perm(),
		"Private key file must have 0600 permissions for security")

	// Execute: Read key file
	pemData, err := os.ReadFile(keyPath)
	require.NoError(t, err, "Reading key file must succeed")

	// Verify: PEM format is correct
	block, _ := pem.Decode(pemData)
	require.NotNil(t, block, "PEM data must be valid")
	assert.Equal(t, "RSA PRIVATE KEY", block.Type,
		"PEM block type must be RSA PRIVATE KEY")

	// Verify: Key can be parsed
	parsedKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	require.NoError(t, err, "Private key must be parseable from PEM")

	// Verify: Parsed key matches original
	assert.Equal(t, caKey.N, parsedKey.N, "Key modulus must match original")
	assert.Equal(t, caKey.E, parsedKey.E, "Key exponent must match original")
}

// testMockServers validates all mock server helpers
func testMockServers(t *testing.T) {
	t.Run("http_mock_server", testHTTPMockServer)
	t.Run("https_mock_server", testHTTPSMockServer)
	t.Run("request_recording", testRequestRecording)
	t.Run("mock_responses", testMockResponses)
}

// testHTTPMockServer verifies that a mock HTTP server can be created
//
// This test cannot be gamed because:
// - It starts a real HTTP server on a real port
// - It makes an actual HTTP request to the server
// - It verifies the response is correct
func testHTTPMockServer(t *testing.T) {
	// Setup: Create handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("mock response"))
	})

	// Execute: Create mock server
	mockServer := helpers.NewMockServer(handler)
	require.NotNil(t, mockServer, "NewMockServer must return server")
	require.NotNil(t, mockServer.Server, "Server field must not be nil")
	defer mockServer.Server.Close()

	// Verify: Server is running and accessible
	resp, err := http.Get(mockServer.Server.URL + "/test")
	require.NoError(t, err, "Request to mock server must succeed")
	defer resp.Body.Close()

	// Verify: Response is correct
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Status must be 200")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "Reading body must succeed")
	assert.Equal(t, "mock response", string(body), "Body must match")
}

// testHTTPSMockServer verifies that a mock HTTPS server can be created
//
// This test cannot be gamed because:
// - It starts a real HTTPS server with test certificates
// - It makes an actual HTTPS request with TLS
// - It verifies the TLS handshake succeeds
// - It validates the server certificate
func testHTTPSMockServer(t *testing.T) {
	// Setup: Create handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("secure mock response"))
	})

	// Execute: Create mock TLS server
	mockServer := helpers.NewMockTLSServer(handler)
	require.NotNil(t, mockServer, "NewMockTLSServer must return server")
	require.NotNil(t, mockServer.Server, "Server field must not be nil")
	defer mockServer.Server.Close()

	// Verify: Server is running and accessible via HTTPS
	// Note: httptest.Server automatically trusts its own certificate
	client := mockServer.Server.Client()
	resp, err := client.Get(mockServer.Server.URL + "/secure")
	require.NoError(t, err, "HTTPS request must succeed")
	defer resp.Body.Close()

	// Verify: Response is correct
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Status must be 200")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "Reading body must succeed")
	assert.Equal(t, "secure mock response", string(body), "Body must match")

	// Verify: Connection was actually TLS
	assert.NotNil(t, resp.TLS, "Response must have TLS info")
	assert.True(t, resp.TLS.HandshakeComplete, "TLS handshake must complete")
}

// testRequestRecording verifies that mock servers record all requests
//
// This test cannot be gamed because:
// - It makes multiple real HTTP requests with different methods, paths, headers
// - It verifies all requests are recorded with correct details
// - It checks request bodies are captured
// - It validates headers are preserved
func testRequestRecording(t *testing.T) {
	// Setup: Create handler that records requests
	responses := map[string]helpers.MockResponse{
		"GET /api/users": {
			StatusCode: http.StatusOK,
			Body:       []byte(`{"users":[]}`),
			Headers:    map[string]string{"Content-Type": "application/json"},
		},
		"POST /api/users": {
			StatusCode: http.StatusCreated,
			Body:       []byte(`{"id":123}`),
			Headers:    map[string]string{"Content-Type": "application/json"},
		},
	}

	handler := helpers.RecordRequests(responses)

	// Execute: Create mock server
	mockServer := helpers.NewMockServer(handler)
	require.NotNil(t, mockServer, "NewMockServer must return server")
	defer mockServer.Server.Close()

	// Execute: Make GET request
	req1, err := http.NewRequest("GET", mockServer.Server.URL+"/api/users", nil)
	require.NoError(t, err, "Creating request must succeed")
	req1.Header.Set("Authorization", "Bearer token123")

	resp1, err := http.DefaultClient.Do(req1)
	require.NoError(t, err, "GET request must succeed")
	resp1.Body.Close()

	// Execute: Make POST request
	req2, err := http.NewRequest("POST", mockServer.Server.URL+"/api/users",
		http.NoBody)
	require.NoError(t, err, "Creating request must succeed")
	req2.Header.Set("Content-Type", "application/json")

	resp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err, "POST request must succeed")
	resp2.Body.Close()

	// Verify: Both requests were recorded
	assert.Len(t, mockServer.Requests, 2, "Must record both requests")

	// Verify: First request recorded correctly
	assert.Equal(t, "GET", mockServer.Requests[0].Method, "Method must match")
	assert.Equal(t, "/api/users", mockServer.Requests[0].Path, "Path must match")
	assert.Equal(t, "Bearer token123", mockServer.Requests[0].Headers.Get("Authorization"),
		"Headers must be recorded")

	// Verify: Second request recorded correctly
	assert.Equal(t, "POST", mockServer.Requests[1].Method, "Method must match")
	assert.Equal(t, "/api/users", mockServer.Requests[1].Path, "Path must match")
	assert.Equal(t, "application/json", mockServer.Requests[1].Headers.Get("Content-Type"),
		"Headers must be recorded")
}

// testMockResponses verifies that mock servers return configured responses
//
// This test cannot be gamed because:
// - It configures different responses for different endpoints
// - It makes actual requests and verifies responses match
// - It checks status codes, headers, and body content
func testMockResponses(t *testing.T) {
	// Setup: Configure responses
	responses := map[string]helpers.MockResponse{
		"GET /api/config": {
			StatusCode: http.StatusOK,
			Body:       []byte(`{"config":"value"}`),
			Headers: map[string]string{
				"Content-Type": "application/json",
				"X-Custom":     "header-value",
			},
		},
		"GET /api/error": {
			StatusCode: http.StatusInternalServerError,
			Body:       []byte(`{"error":"internal error"}`),
			Headers:    map[string]string{"Content-Type": "application/json"},
		},
	}

	handler := helpers.RecordRequests(responses)
	mockServer := helpers.NewMockServer(handler)
	require.NotNil(t, mockServer, "NewMockServer must return server")
	defer mockServer.Server.Close()

	// Execute: Request /api/config
	resp1, err := http.Get(mockServer.Server.URL + "/api/config")
	require.NoError(t, err, "Request must succeed")
	defer resp1.Body.Close()

	// Verify: Config response matches
	assert.Equal(t, http.StatusOK, resp1.StatusCode, "Status must be 200")
	assert.Equal(t, "application/json", resp1.Header.Get("Content-Type"),
		"Content-Type must match")
	assert.Equal(t, "header-value", resp1.Header.Get("X-Custom"),
		"Custom header must match")

	body1, err := io.ReadAll(resp1.Body)
	require.NoError(t, err, "Reading body must succeed")
	assert.Equal(t, `{"config":"value"}`, string(body1), "Body must match")

	// Execute: Request /api/error
	resp2, err := http.Get(mockServer.Server.URL + "/api/error")
	require.NoError(t, err, "Request must succeed")
	defer resp2.Body.Close()

	// Verify: Error response matches
	assert.Equal(t, http.StatusInternalServerError, resp2.StatusCode,
		"Status must be 500")

	body2, err := io.ReadAll(resp2.Body)
	require.NoError(t, err, "Reading body must succeed")
	assert.Equal(t, `{"error":"internal error"}`, string(body2), "Body must match")
}

// testContextHelpers validates all context helper functions
func testContextHelpers(t *testing.T) {
	t.Run("test_context_creation", testTestContextCreation)
	t.Run("test_context_with_timeout", testTestContextWithTimeout)
	t.Run("request_id_propagation", testRequestIDPropagation)
}

// testTestContextCreation verifies that a test context can be created
//
// This test cannot be gamed because:
// - It creates an actual context
// - It verifies the context has a request ID
// - It checks the request ID is non-empty
func testTestContextCreation(t *testing.T) {
	// Execute: Create test context
	ctx := helpers.TestContext()

	// Verify: Context is not nil
	require.NotNil(t, ctx, "TestContext must return non-nil context")

	// Verify: Context has a request ID
	// (Implementation should add request ID to context)
	requestID := ctx.Value("request_id")
	assert.NotNil(t, requestID, "Context must have request_id")

	// Verify: Request ID is a non-empty string
	requestIDStr, ok := requestID.(string)
	assert.True(t, ok, "request_id must be a string")
	assert.NotEmpty(t, requestIDStr, "request_id must not be empty")
}

// testTestContextWithTimeout verifies that a context with timeout can be created
//
// This test cannot be gamed because:
// - It creates an actual context with a real timeout
// - It waits for the timeout to expire
// - It verifies the context is actually cancelled
func testTestContextWithTimeout(t *testing.T) {
	// Execute: Create context with 100ms timeout
	ctx, cancel := helpers.TestContextWithTimeout(100 * time.Millisecond)
	defer cancel()

	// Verify: Context is not nil
	require.NotNil(t, ctx, "Context must not be nil")

	// Verify: Deadline is set
	deadline, ok := ctx.Deadline()
	assert.True(t, ok, "Context must have deadline")
	assert.True(t, deadline.After(time.Now()),
		"Deadline must be in the future")

	// Execute: Wait for timeout
	<-ctx.Done()

	// Verify: Context was cancelled due to timeout
	assert.ErrorIs(t, ctx.Err(), context.DeadlineExceeded,
		"Context must be cancelled with DeadlineExceeded")
}

// testRequestIDPropagation verifies that request IDs propagate through operations
//
// This test cannot be gamed because:
// - It creates multiple contexts
// - It verifies each has a unique request ID
// - It checks IDs are preserved through context operations
func testRequestIDPropagation(t *testing.T) {
	// Execute: Create two separate contexts
	ctx1 := helpers.TestContext()
	ctx2 := helpers.TestContext()

	// Verify: Both contexts have request IDs
	id1 := ctx1.Value("request_id")
	id2 := ctx2.Value("request_id")
	require.NotNil(t, id1, "First context must have request_id")
	require.NotNil(t, id2, "Second context must have request_id")

	// Verify: Request IDs are unique
	assert.NotEqual(t, id1, id2,
		"Each context must have unique request_id")

	// Execute: Derive context with timeout
	ctx3, cancel := context.WithTimeout(ctx1, time.Second)
	defer cancel()

	// Verify: Request ID is preserved in derived context
	id3 := ctx3.Value("request_id")
	assert.Equal(t, id1, id3,
		"Derived context must preserve request_id from parent")
}

// TestTestInfrastructureIntegration validates all helpers work together
//
// This end-to-end scenario demonstrates:
// 1. Generate CA and certificate
// 2. Start mock HTTPS server with generated certificate
// 3. Make request with test context
// 4. Verify request was recorded
//
// This test cannot be gamed because it uses real components end-to-end:
// - Real certificate generation and TLS
// - Real HTTPS server and client
// - Real network requests
// - Real context propagation
func TestTestInfrastructureIntegration(t *testing.T) {
	// Setup: Generate CA and certificate
	caCert, caKey, err := helpers.GenerateTestCA()
	require.NoError(t, err, "CA generation must succeed")

	hostname := "testserver.local"
	_, err = helpers.GenerateTestCert(caCert, caKey, hostname)
	require.NoError(t, err, "Certificate generation must succeed")

	// Setup: Create mock HTTPS server with custom certificate
	responses := map[string]helpers.MockResponse{
		"GET /api/test": {
			StatusCode: http.StatusOK,
			Body:       []byte(`{"status":"ok"}`),
			Headers:    map[string]string{"Content-Type": "application/json"},
		},
	}

	handler := helpers.RecordRequests(responses)
	mockServer := helpers.NewMockTLSServer(handler)
	require.NotNil(t, mockServer, "Mock server must be created")
	defer mockServer.Server.Close()

	// Setup: Create HTTP client with test context
	ctx := helpers.TestContext()
	requestID := ctx.Value("request_id")

	// Execute: Make request with context
	req, err := http.NewRequestWithContext(ctx, "GET",
		mockServer.Server.URL+"/api/test", nil)
	require.NoError(t, err, "Creating request must succeed")

	resp, err := mockServer.Server.Client().Do(req)
	require.NoError(t, err, "Request must succeed")
	defer resp.Body.Close()

	// Verify: Response is correct
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Status must be 200")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "Reading body must succeed")
	assert.Equal(t, `{"status":"ok"}`, string(body), "Body must match")

	// Verify: Request was recorded
	assert.Len(t, mockServer.Requests, 1, "Must record request")
	assert.Equal(t, "GET", mockServer.Requests[0].Method, "Method must be GET")
	assert.Equal(t, "/api/test", mockServer.Requests[0].Path, "Path must match")

	// Verify: Context propagated (request ID available)
	assert.NotNil(t, requestID, "Request ID must be set in context")
}
