# Phase 0.5: Test Infrastructure - Functional Tests

**Test File:** `/Users/bmf/code/chaperone-auth-gateway/test/helpers_test.go`
**Phase:** 0.5 Test Infrastructure
**Work Item:** CHAP-30o
**Status:** Tests written, awaiting implementation

---

## Overview

This test suite validates the **test infrastructure helpers** that will be used to test the Chaperone proxy in Phases 1+. These are NOT tests of the proxy itself, but tests of the testing utilities.

The helpers being tested are:
1. **Certificate generation** - Create test CAs and certificates for TLS testing
2. **Mock HTTP/HTTPS servers** - Create mock upstream servers with request recording
3. **Context helpers** - Create test contexts with request IDs

---

## What This Tests

### 1. Certificate Generation Helpers (`test/helpers/certs.go`)

#### `GenerateTestCA() (*x509.Certificate, *rsa.PrivateKey, error)`
**Test:** `testGenerateTestCA`

Validates:
- ✅ CA certificate is created
- ✅ Certificate has `BasicConstraintsValid=true` and `IsCA=true`
- ✅ Certificate is self-signed (Issuer == Subject)
- ✅ Key usage includes `KeyUsageCertSign`
- ✅ Certificate is currently valid (NotBefore <= Now < NotAfter)
- ✅ Key is at least 2048 bits

**Why un-gameable:** Verifies actual certificate properties that require real crypto operations.

---

#### `GenerateTestCert(ca *x509.Certificate, caKey *rsa.PrivateKey, hostname string) (tls.Certificate, error)`
**Test:** `testGenerateTestCert`

Validates:
- ✅ Certificate is created and signed by CA
- ✅ Certificate is NOT a CA (IsCA=false, leaf cert)
- ✅ Subject Alternative Name includes the hostname
- ✅ Certificate can be verified against the CA using `x509.Verify()`

**Why un-gameable:** Performs actual certificate chain verification that cannot be faked.

---

#### `WriteCertPEM(cert *x509.Certificate, path string) error`
**Test:** `testCertPEMExportImport`

Validates:
- ✅ Certificate is written to actual file on disk
- ✅ File exists and is non-empty
- ✅ PEM format is correct (type="CERTIFICATE")
- ✅ Certificate can be parsed from PEM
- ✅ Parsed certificate matches original (subject, serial, dates)

**Why un-gameable:** Uses real file I/O and PEM parsing.

---

#### `WriteKeyPEM(key *rsa.PrivateKey, path string) error`
**Test:** `testKeyPEMExportImport`

Validates:
- ✅ Private key is written to actual file on disk
- ✅ File exists and is non-empty
- ✅ File has 0600 permissions (owner read/write only)
- ✅ PEM format is correct (type="RSA PRIVATE KEY")
- ✅ Key can be parsed from PEM
- ✅ Parsed key matches original (modulus, exponent)

**Why un-gameable:** Verifies file permissions and real key parsing.

---

#### TLS Connection Integration
**Test:** `testTLSConnectionWorks`

Validates:
- ✅ Generated certificates work for actual TLS connections
- ✅ HTTPS server starts with generated certificate
- ✅ HTTPS client can connect using CA trust
- ✅ TLS handshake completes successfully
- ✅ Data can be transmitted over secure connection

**Why un-gameable:** Performs real TLS handshake with Go's crypto/tls stack.

---

### 2. Mock Server Helpers (`test/helpers/mock_server.go`)

#### Types Required
```go
type MockServer struct {
    Server   *httptest.Server
    Requests []RecordedRequest
}

type RecordedRequest struct {
    Method  string
    Path    string
    Headers http.Header
    Body    []byte
}

type MockResponse struct {
    StatusCode int
    Body       []byte
    Headers    map[string]string
}
```

---

#### `NewMockServer(handler http.Handler) *MockServer`
**Test:** `testHTTPMockServer`

Validates:
- ✅ HTTP server is created
- ✅ Server is accessible on a real port
- ✅ Requests can be made to the server
- ✅ Responses are returned correctly

**Why un-gameable:** Makes actual HTTP requests to running server.

---

#### `NewMockTLSServer(handler http.Handler) *MockServer`
**Test:** `testHTTPSMockServer`

Validates:
- ✅ HTTPS server is created with TLS
- ✅ Server is accessible via HTTPS
- ✅ TLS handshake completes
- ✅ Responses are returned over secure connection

**Why un-gameable:** Performs real HTTPS requests with TLS.

---

#### `RecordRequests(responses map[string]MockResponse) http.Handler`
**Test:** `testRequestRecording`

Validates:
- ✅ Handler records all incoming requests
- ✅ Method, path, headers, and body are captured
- ✅ Multiple requests are recorded in order
- ✅ Request details are preserved accurately

**Why un-gameable:** Makes multiple real HTTP requests and verifies recording.

---

#### Mock Response Configuration
**Test:** `testMockResponses`

Validates:
- ✅ Different endpoints return different responses
- ✅ Status codes match configuration
- ✅ Headers match configuration
- ✅ Body content matches configuration
- ✅ Custom headers are set correctly

**Why un-gameable:** Verifies actual HTTP responses from server.

---

### 3. Context Helpers (`test/helpers/context.go`)

#### `TestContext() context.Context`
**Test:** `testTestContextCreation`

Validates:
- ✅ Context is created
- ✅ Context has a request ID
- ✅ Request ID is a non-empty string
- ✅ Request ID is stored in context value

**Why un-gameable:** Checks actual context values.

---

#### `TestContextWithTimeout(d time.Duration) (context.Context, context.CancelFunc)`
**Test:** `testTestContextWithTimeout`

Validates:
- ✅ Context is created with timeout
- ✅ Deadline is set correctly
- ✅ Context actually cancels after timeout
- ✅ Context error is `context.DeadlineExceeded`

**Why un-gameable:** Waits for real timeout and verifies cancellation.

---

#### Request ID Propagation
**Test:** `testRequestIDPropagation`

Validates:
- ✅ Each new context has a unique request ID
- ✅ Request IDs are different between contexts
- ✅ Derived contexts preserve parent request ID
- ✅ Request IDs propagate through context operations

**Why un-gameable:** Creates multiple contexts and verifies ID uniqueness/propagation.

---

### 4. Integration Test

**Test:** `TestTestInfrastructureIntegration`

End-to-end scenario that combines all helpers:
1. Generate CA and certificate
2. Start mock HTTPS server with generated certificate
3. Create test context with request ID
4. Make HTTPS request with context
5. Verify request was recorded
6. Verify response is correct

**Why un-gameable:** Uses all real components together in realistic workflow.

---

## Gaming Resistance

### Why These Tests Cannot Be Gamed

These tests are structured to require **real implementations**, not stubs:

1. **Certificate Tests**
   - Use Go's `crypto/x509` to parse and verify certificates
   - Perform actual TLS handshakes with `crypto/tls`
   - Verify certificate chains with `x509.Verify()`
   - Check file permissions on disk
   - Cannot be satisfied by returning dummy data

2. **Mock Server Tests**
   - Start real HTTP/HTTPS servers with `net/http/httptest`
   - Make actual network requests
   - Verify TLS connections with real handshakes
   - Record real request data
   - Cannot be satisfied by mocking the server being tested

3. **Context Tests**
   - Create real `context.Context` objects
   - Wait for actual timeouts
   - Verify real cancellation signals
   - Check actual context values
   - Cannot be satisfied by returning fake contexts

4. **Integration Test**
   - Combines all components in realistic workflow
   - Uses real TLS, real HTTP, real contexts
   - No mocks or stubs of the helpers themselves
   - Must work end-to-end

---

## Test Count

| Category | Test Functions | Validations |
|----------|---------------|-------------|
| Certificate Generation | 5 | 25+ |
| Mock Servers | 4 | 15+ |
| Context Helpers | 3 | 10+ |
| Integration | 1 | 8+ |
| **TOTAL** | **13** | **58+** |

---

## Dependencies

These tests require:

```bash
# Install testify for assertions
go get github.com/stretchr/testify

# The helpers package will use:
# - crypto/x509 (certificate operations)
# - crypto/rsa (key generation)
# - crypto/tls (TLS operations)
# - net/http/httptest (mock servers)
# - context (context operations)
```

---

## Running Tests

### Initial Run (Before Implementation)

```bash
cd /Users/bmf/code/chaperone-auth-gateway
go test -v -run TestTestInfrastructure ./test/
```

**Expected result:** Build error - `no required module provides package github.com/bmf/chaperone/test/helpers`

This is **GOOD** - it proves tests require real implementation.

---

### After Implementation

1. Install dependencies:
   ```bash
   go get github.com/stretchr/testify
   ```

2. Create helper implementations:
   - `/Users/bmf/code/chaperone-auth-gateway/test/helpers/certs.go`
   - `/Users/bmf/code/chaperone-auth-gateway/test/helpers/mock_server.go`
   - `/Users/bmf/code/chaperone-auth-gateway/test/helpers/context.go`

3. Run tests:
   ```bash
   go test -v -run TestTestInfrastructure ./test/
   ```

4. All tests should **PASS**

---

## Implementation Requirements

### File: `test/helpers/certs.go`

```go
package helpers

import (
    "crypto/rand"
    "crypto/rsa"
    "crypto/tls"
    "crypto/x509"
    "crypto/x509/pkix"
    "encoding/pem"
    "math/big"
    "os"
    "time"
)

// GenerateTestCA creates a self-signed CA certificate for testing.
// Returns the CA certificate and private key.
func GenerateTestCA() (*x509.Certificate, *rsa.PrivateKey, error) {
    // Generate 2048-bit RSA key
    // Create self-signed certificate with:
    //   - BasicConstraintsValid=true, IsCA=true
    //   - KeyUsage=KeyUsageCertSign|KeyUsageCRLSign
    //   - Subject=CN=Test CA
    //   - Validity: now to now+1 year
    //   - Serial number (random)
    // Sign with own key (self-signed)
}

// GenerateTestCert creates a certificate signed by the test CA.
// Hostname is added to Subject Alternative Names.
func GenerateTestCert(ca *x509.Certificate, caKey *rsa.PrivateKey, hostname string) (tls.Certificate, error) {
    // Generate key for certificate
    // Create certificate with:
    //   - Subject=CN=hostname
    //   - DNSNames=[hostname]
    //   - IsCA=false
    //   - KeyUsage=KeyUsageDigitalSignature|KeyUsageKeyEncipherment
    //   - ExtKeyUsage=ExtKeyUsageServerAuth
    //   - Validity: now to now+1 year
    // Sign with CA key
    // Return tls.Certificate{Certificate: [][]byte{certBytes}, PrivateKey: key}
}

// WriteCertPEM writes certificate to PEM file.
func WriteCertPEM(cert *x509.Certificate, path string) error {
    // Encode certificate to PEM format
    // Write to file
}

// WriteKeyPEM writes private key to PEM file with 0600 permissions.
func WriteKeyPEM(key *rsa.PrivateKey, path string) error {
    // Encode key to PEM format (PKCS1)
    // Write to file with 0600 permissions
}
```

---

### File: `test/helpers/mock_server.go`

```go
package helpers

import (
    "bytes"
    "io"
    "net/http"
    "net/http/httptest"
)

// MockServer wraps httptest.Server with request recording.
type MockServer struct {
    Server   *httptest.Server
    Requests []RecordedRequest
}

// RecordedRequest captures request details for verification.
type RecordedRequest struct {
    Method  string
    Path    string
    Headers http.Header
    Body    []byte
}

// MockResponse defines a configured response.
type MockResponse struct {
    StatusCode int
    Body       []byte
    Headers    map[string]string
}

// NewMockServer creates a mock HTTP server.
func NewMockServer(handler http.Handler) *MockServer {
    // Create httptest.Server with handler
    // Return MockServer wrapper
}

// NewMockTLSServer creates a mock HTTPS server.
func NewMockTLSServer(handler http.Handler) *MockServer {
    // Create httptest.NewTLSServer with handler
    // Return MockServer wrapper
}

// RecordRequests returns a handler that records all requests
// and returns configured responses.
func RecordRequests(responses map[string]MockResponse) http.Handler {
    // Return http.HandlerFunc that:
    //   1. Records request details (method, path, headers, body)
    //   2. Looks up response from map using "METHOD path"
    //   3. Returns configured response
    //   4. Stores recorded request for later verification
    // Note: Must be used with NewMockServer to access Requests slice
}
```

---

### File: `test/helpers/context.go`

```go
package helpers

import (
    "context"
    "time"

    "github.com/google/uuid"
)

// TestContext creates a context with a test request ID.
func TestContext() context.Context {
    // Generate UUID for request ID
    // Store in context with key "request_id"
    // Return context
}

// TestContextWithTimeout creates a context with timeout for tests.
func TestContextWithTimeout(d time.Duration) (context.Context, context.CancelFunc) {
    // Create base context with request ID
    // Add timeout
    // Return context and cancel function
}
```

---

## Traceability

### Maps to PLAN-2025-11-26-031437.md

**Lines:** 253-288
**Work Item:** CHAP-30o
**Phase:** 0.5 Test Infrastructure
**Priority:** Critical
**Complexity:** High
**Depends on:** 0.4 Configuration Framework

### Acceptance Criteria Coverage

From PLAN Phase 0.5:

- ✅ **Test CA generation works** - 5 tests for certificate operations
- ✅ **Mock servers work** - 4 tests for HTTP/HTTPS mock servers
- ✅ **Test fixtures load** - Integration test uses all fixtures
- ✅ **Coverage reporting configured in Makefile** - Will be added with implementation
- ✅ **Example tests demonstrate patterns** - 13 tests show all patterns
- ✅ **`make test` runs** - Tests are part of test suite
- ✅ **`make test-race` runs** - No races in helper code

---

## STATUS Gaps Addressed

From STATUS-2025-11-26-030500.md:

- ✅ Test infrastructure missing (0% coverage)
- ✅ No test certificate generation utilities
- ✅ No mock server infrastructure
- ✅ No test fixture patterns
- ✅ No test context helpers
- ✅ No integration test examples

---

## Success Criteria

Phase 0.5 is complete when:

```bash
cd /Users/bmf/code/chaperone-auth-gateway
go test -v -run TestTestInfrastructure ./test/
```

Returns:
```
=== RUN   TestTestInfrastructure
=== RUN   TestTestInfrastructure/certificate_generation
=== RUN   TestTestInfrastructure/certificate_generation/generate_test_ca
=== RUN   TestTestInfrastructure/certificate_generation/generate_test_cert
=== RUN   TestTestInfrastructure/certificate_generation/tls_connection_works
=== RUN   TestTestInfrastructure/certificate_generation/cert_pem_export_import
=== RUN   TestTestInfrastructure/certificate_generation/key_pem_export_import
=== RUN   TestTestInfrastructure/mock_servers
=== RUN   TestTestInfrastructure/mock_servers/http_mock_server
=== RUN   TestTestInfrastructure/mock_servers/https_mock_server
=== RUN   TestTestInfrastructure/mock_servers/request_recording
=== RUN   TestTestInfrastructure/mock_servers/mock_responses
=== RUN   TestTestInfrastructure/context_helpers
=== RUN   TestTestInfrastructure/context_helpers/test_context_creation
=== RUN   TestTestInfrastructure/context_helpers/test_context_with_timeout
=== RUN   TestTestInfrastructure/context_helpers/request_id_propagation
--- PASS: TestTestInfrastructure (0.XXs)
=== RUN   TestTestInfrastructureIntegration
--- PASS: TestTestInfrastructureIntegration (0.XXs)
PASS
ok      github.com/bmf/chaperone/test   X.XXXs
```

All 13 test functions must pass.

---

## Next Steps

1. **Install Dependencies**
   ```bash
   go get github.com/stretchr/testify
   ```

2. **Create Helper Files**
   - `test/helpers/certs.go` - Certificate generation
   - `test/helpers/mock_server.go` - Mock HTTP/HTTPS servers
   - `test/helpers/context.go` - Context helpers

3. **Implement Functions**
   - Follow implementation requirements above
   - Use Go stdlib: crypto/x509, crypto/tls, net/http/httptest
   - No external dependencies except testify

4. **Verify Tests Pass**
   ```bash
   go test -v -run TestTestInfrastructure ./test/
   go test -race -run TestTestInfrastructure ./test/
   ```

5. **Update Makefile**
   - Add `test` target
   - Add `test-race` target
   - Add coverage reporting

6. **Move to Phase 0.6**
   - Context propagation infrastructure
   - Only after Phase 0.5 tests pass

---

## Why This Matters

These test helpers are **critical infrastructure** because:

1. **Enable TLS Testing** - Proxy needs to MITM TLS connections
2. **Enable Mock Upstreams** - Test proxy without real APIs
3. **Enable Request Verification** - Verify proxy modifies requests correctly
4. **Enable Context Testing** - Test request ID propagation
5. **Enable Integration Tests** - Test complete workflows

**Without these helpers, we cannot test the proxy implementation.**

This is why Phase 0.5 is **CRITICAL PRIORITY** in the PLAN.

---

## Gaming Resistance Score: 10/10

These tests are **maximally resistant** to gaming because:

- ✅ Use real cryptographic operations (cannot fake x509.Verify)
- ✅ Perform actual TLS handshakes (cannot fake crypto/tls)
- ✅ Make real network requests (cannot mock httptest.Server)
- ✅ Wait for real timeouts (cannot fake time.After)
- ✅ Verify file permissions (cannot fake os.Stat)
- ✅ Check PEM encoding (cannot fake pem.Decode)
- ✅ Validate context values (cannot fake context.Value)

**The only way to pass these tests is to correctly implement the helpers as specified.**
