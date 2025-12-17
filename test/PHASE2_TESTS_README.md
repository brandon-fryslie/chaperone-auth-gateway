# Phase 2 Functional Tests - MITM & Service Routing

**Created:** 2025-11-30
**Phase:** Phase 2 - MITM Engine and Service Routing
**Test Status:** PENDING IMPLEMENTATION (tests written, implementation not started)

---

## Overview

This document describes the comprehensive functional test suite for Phase 2 of the Chaperone Authentication Gateway project. These tests validate the MITM (Man-in-the-Middle) engine, service routing, and policy enforcement capabilities.

**Critical:** These tests are written FIRST, before implementation. They define the contract that the implementation must fulfill.

---

## Test Philosophy

These tests follow the **un-gameable functional testing** philosophy:

1. **Real User Workflows** - Tests execute exactly as users would interact with the system
2. **Actual Operations** - Tests use real crypto operations, real file I/O, real network sockets
3. **Observable Outcomes** - Tests verify externally observable results (files on disk, HTTP responses, etc.)
4. **No Mocking Core Logic** - Tests do not mock the functionality being tested
5. **Clear Failures** - When functionality is broken, tests fail clearly and cannot be satisfied by stubs

---

## Test Files

### Unit Tests

#### `mitm_ca_test.go` - CA Certificate Management
Tests CA generation, storage, loading, and certificate signing.

**Key Tests:**
- `TestCAGenerationWithCorrectParameters` - Verifies 4096-bit RSA, 10-year validity, CA:TRUE
- `TestCAPersistenceToFilesystem` - Verifies file permissions (0600 key, 0644 cert)
- `TestCALoadingFromExistingFiles` - Verifies CA loads from existing files
- `TestCACanSignLeafCertificates` - Verifies CA can sign valid certificates
- `TestCAGenerationIsIdempotent` - Verifies LoadOrGenerateCA is idempotent
- `TestCAErrorHandling` - Verifies error conditions (missing files, invalid PEM, etc.)
- `TestCAThreadSafety` - Verifies concurrent certificate signing works

**Implementation Requirements:**
- `internal/mitm/ca.go` with `GenerateCA()`, `LoadCA()`, `StoreCA()`, `LoadOrGenerateCA()`
- CA struct with `SignCertificate()` method
- RSA 4096-bit key generation
- 10-year validity period
- Correct file permissions
- Thread-safe operations

**Anti-Gaming Measures:**
- Tests parse actual x509 certificates (cannot fake structure)
- Tests verify real file permissions with os.Stat()
- Tests verify real RSA key size
- Tests use real crypto signature verification
- Tests verify actual filesystem writes

---

#### `mitm_cert_test.go` - Dynamic Certificate Generation
Tests per-domain certificate generation, caching, and management.

**Key Tests:**
- `TestCertificateGenerationForHostname` - Verifies cert for specific hostname
- `TestCertificateSignedByCA` - Verifies cert is signed by CA
- `TestCertificateCaching` - Verifies same hostname returns cached cert
- `TestCertificateExpirationHandling` - Verifies expired certs regenerated
- `TestCertificateSANCorrectness` - Verifies SAN includes hostname
- `TestCertificatePortStripping` - Verifies api.example.com:443 → api.example.com
- `TestCertificateCaseInsensitiveHostname` - Verifies case-insensitive caching
- `TestCertificateConcurrentAccess` - Verifies thread-safe certificate generation

**Implementation Requirements:**
- `internal/mitm/cert.go` with `CertCache` struct
- `GetCertificate(hostname)` method
- sync.Map for thread-safe caching
- RSA 2048-bit keys for leaf certificates
- 90-day validity period
- Hostname normalization (lowercase, port stripping)
- SAN with correct DNS name

**Anti-Gaming Measures:**
- Tests parse actual x509 certificates
- Tests verify real certificate chains
- Tests verify actual cache behavior (same serial number)
- Tests verify real expiration dates
- Tests verify concurrent access with race detector

---

#### `service_test.go` - Service Registry
Tests service registration, lookup, and domain matching.

**Key Tests:**
- `TestServiceRegistrationAndLookup` - Verifies services register and can be found
- `TestServiceCaseInsensitiveMatching` - Verifies api.openai.com == API.OPENAI.COM
- `TestServicePortStripping` - Verifies api.example.com:443 → api.example.com
- `TestServiceMissingReturnsNotFound` - Verifies (nil, false) for missing services
- `TestServiceDuplicateRegistration` - Verifies duplicate detection
- `TestServiceConcurrentAccess` - Verifies thread-safe lookup
- `TestServiceValidation` - Verifies required fields checked
- `TestDomainMatching` - Verifies ShouldMITM() logic
- `TestServiceListAll` - Verifies listing all services

**Implementation Requirements:**
- `internal/service/registry.go` with `Registry` struct
- `Register()`, `Lookup()`, `LoadFromConfig()`, `ListAll()` methods
- sync.RWMutex for thread-safe operations
- Hostname normalization (lowercase, port stripping)
- Service validation

**Anti-Gaming Measures:**
- Tests verify actual map storage/retrieval
- Tests verify real string normalization
- Tests verify actual concurrent access with race detector
- Tests verify real validation errors

---

#### `policy_test.go` - Policy Enforcement
Tests method allowlist, path matching, and body size limits.

**Key Tests:**
- `TestMethodAllowlistEnforcement` - Verifies disallowed methods fail
- `TestPathGlobMatching` - Verifies /v1/* matches /v1/chat
- `TestBodySizeLimits` - Verifies oversized bodies rejected
- `TestPolicyCombinedEnforcement` - Verifies all checks apply together
- `TestPolicyValidation` - Verifies policy validation
- `TestPolicyDefaultValues` - Verifies default MaxBodyBytes

**Implementation Requirements:**
- `internal/service/policy.go` with `PolicyEnforcer` struct
- `EnforcePolicy(req, policy)` method
- Method checking (exact match)
- Path glob matching (/v1/* matches /v1/*)
- Body size checking (Content-Length header)
- Appropriate HTTP status codes (403, 413)

**Anti-Gaming Measures:**
- Tests create real http.Request objects
- Tests verify actual error returns
- Tests verify real glob pattern matching
- Tests verify actual integer comparisons (body size)

---

### Integration Tests

#### `test/integration/mitm_integration_test.go` - End-to-End MITM
Tests complete MITM workflow with real HTTP clients and servers.

**Key Tests:**
- `TestSelectiveMITMWithTrustedCA` - Complete MITM flow with trusted CA
- `TestTransparentTunnelForNonConfiguredDomains` - Fallback to transparent tunnel
- `TestPolicyEnforcementEndToEnd` - Policy returns 403/413
- `TestCertificateTrustValidation` - Certificate trust chain
- `TestMITMConcurrentRequests` - Concurrent MITM requests
- `TestMITMStreamingResponse` - Streaming through MITM
- `TestMITMLargeRequestResponse` - Large data transfers
- `TestPhase2CompletionChecklist` - Phase 2 acceptance criteria

**Implementation Requirements:**
- Complete integration of all Phase 2 components
- `proxy.NewWithMITM()` constructor
- MITM-aware tunnel handler
- Policy enforcement in HTTP handler
- Certificate generation on demand
- Service-based routing

**Anti-Gaming Measures:**
- Tests use real HTTP clients (net/http)
- Tests make actual network requests
- Tests verify real TLS handshakes
- Tests verify actual certificate chains
- Tests verify real HTTP status codes
- Tests verify actual data flows

---

## Running Tests

### Run All Phase 2 Tests
```bash
go test ./test/mitm_ca_test.go -v
go test ./test/mitm_cert_test.go -v
go test ./test/service_test.go -v
go test ./test/policy_test.go -v
go test ./test/integration/mitm_integration_test.go -v
```

### Run with Race Detector
```bash
go test ./test -race -v
```

### Run Integration Tests Only
```bash
go test ./test/integration -v
```

### Skip Integration Tests (fast)
```bash
go test ./test -short -v
```

---

## Test Coverage Goals

**Phase 2 Test Coverage Targets:**
- CA Management: 90%+ coverage
- Certificate Generation: 90%+ coverage
- Service Registry: 85%+ coverage
- Policy Enforcement: 85%+ coverage
- Integration: All critical paths covered

**Quality Metrics:**
- 0 race conditions (verified with -race)
- All tests pass
- Clear failure messages
- Fast test execution (< 5s for unit tests, < 30s for integration)

---

## Implementation Checklist

Phase 2 is COMPLETE when:

### Functionality
- [ ] CA certificate generated and stored on first run
- [ ] CA certificate loads successfully on subsequent runs
- [ ] Dynamic certificates generated and cached per domain
- [ ] TLS termination works for configured domains (api.openai.com)
- [ ] HTTP requests successfully decrypted and proxied
- [ ] Transparent tunneling works for non-configured domains (google.com)
- [ ] Service registry loads services from config
- [ ] Host pattern matching works (exact match)
- [ ] Policy enforcement blocks disallowed methods (403)
- [ ] Policy enforcement blocks disallowed paths (403)

### Testing
- [ ] All unit tests pass (make test)
- [ ] All integration tests pass (MITM test works)
- [ ] Race detector passes (make test-race)
- [ ] Manual test with trusted CA works (curl through proxy)
- [ ] Phase 0, 0.9, 1 tests continue to pass (139 tests)

### Quality
- [ ] All logs use Phase 0.3 structured JSON logging
- [ ] All errors use Phase 0.2 error types
- [ ] All code uses Phase 0.6 context propagation
- [ ] CA operations logged clearly
- [ ] Certificate generation logged (without private keys)
- [ ] MITM vs tunnel decisions logged
- [ ] Policy violations logged with details
- [ ] Code passes golangci-lint
- [ ] Code passes go vet

### Security
- [ ] CA private key has 0600 permissions
- [ ] CA certificate has 0644 permissions
- [ ] Upstream TLS uses system trust (no InsecureSkipVerify)
- [ ] Policy enforcement prevents unauthorized access
- [ ] No sensitive data logged (credentials, private keys)

---

## Test-Driven Implementation Order

Implement in this order, making tests pass incrementally:

### Sprint 1: CA and Service Foundations
1. Implement `internal/service/types.go` (Service, Policy structs)
   - Make `TestServiceValidation` pass
   - Make `TestPolicyValidation` pass

2. Implement `internal/mitm/ca.go` (CA generation and management)
   - Make `TestCAGenerationWithCorrectParameters` pass
   - Make `TestCAPersistenceToFilesystem` pass
   - Make `TestCALoadingFromExistingFiles` pass
   - Make `TestCACanSignLeafCertificates` pass

### Sprint 2: Registry and Certificate Generation
3. Implement `internal/service/registry.go` (Service registry)
   - Make `TestServiceRegistrationAndLookup` pass
   - Make `TestServiceCaseInsensitiveMatching` pass
   - Make `TestServicePortStripping` pass
   - Make `TestServiceConcurrentAccess` pass

4. Implement `internal/mitm/cert.go` (Certificate cache)
   - Make `TestCertificateGenerationForHostname` pass
   - Make `TestCertificateCaching` pass
   - Make `TestCertificateSANCorrectness` pass
   - Make `TestCertificateConcurrentAccess` pass

### Sprint 3: Domain Matching and Policy
5. Implement `internal/service/matcher.go` (ShouldMITM logic)
   - Make `TestDomainMatching` pass

6. Implement `internal/service/policy.go` (Policy enforcement)
   - Make `TestMethodAllowlistEnforcement` pass
   - Make `TestPathGlobMatching` pass
   - Make `TestBodySizeLimits` pass
   - Make `TestPolicyCombinedEnforcement` pass

### Sprint 4: Integration
7. Wire MITM into proxy server
   - Modify `internal/proxy/tunnel.go` to check ShouldMITM
   - Implement TLS handshake with client
   - Implement HTTP request proxying
   - Make `TestSelectiveMITMWithTrustedCA` pass

8. Complete integration
   - Wire into `cmd/chaperone/cmd/run.go`
   - Make all integration tests pass
   - Make `TestPhase2CompletionChecklist` pass

---

## Manual Testing

After all automated tests pass, perform manual verification:

```bash
# 1. Initialize and start server
./chaperone init openai
./chaperone run --config chaperone.toml

# Expected: CA generated at ~/.config/chaperone/ca-cert.pem
# Expected: Services loaded from config
# Expected: Server starts on port 4010

# 2. Trust CA certificate (macOS)
sudo security add-trusted-cert -d -r trustRoot \
  -k /Library/Keychains/System.keychain \
  ~/.config/chaperone/ca-cert.pem

# Or Linux:
sudo cp ~/.config/chaperone/ca-cert.pem \
  /usr/local/share/ca-certificates/chaperone-ca.crt
sudo update-ca-certificates

# 3. Test MITM for configured domain
export HTTPS_PROXY=http://127.0.0.1:4010
curl -v https://api.openai.com/v1/models

# Expected: MITM occurs (see logs: "MITM handshake" or similar)
# Expected: 401 Unauthorized (no auth yet, but request was decrypted)

# 4. Test transparent tunnel for non-configured domain
curl -v https://www.google.com

# Expected: Transparent tunnel (see logs: "transparent tunnel")
# Expected: 200 OK from Google

# 5. Test policy enforcement
curl -X DELETE https://api.openai.com/v1/models

# Expected: 403 Forbidden (DELETE not allowed per policy)

curl https://api.openai.com/admin/secret

# Expected: 403 Forbidden (path not allowed per policy)
```

---

## Traceability to PLAN

These tests validate the following work items from `PLAN-2025-11-30-150900.md`:

- **CHAP-5v5**: CA certificate generation and storage → `mitm_ca_test.go`
- **CHAP-w9q**: Dynamic leaf certificate generation → `mitm_cert_test.go`
- **CHAP-qkq**: Service and Policy configuration structures → `service_test.go`, `policy_test.go`
- **CHAP-a1z**: Service registry and lookup → `service_test.go`
- **CHAP-3ot**: Domain matching logic → `service_test.go::TestDomainMatching`
- **CHAP-c2s**: Path and method policy enforcement → `policy_test.go`
- **CHAP-rtn**: Integration test for MITM with trusted CA → `mitm_integration_test.go`

---

## Success Criteria

Phase 2 tests validate these success criteria:

1. **Functional**: Can selectively MITM configured domains and decrypt HTTP requests
2. **Tested**: All tests pass (estimated 80+ new tests for Phase 2)
3. **Integrated**: Works via `chaperone run` command with service config
4. **Secure**: CA trust workflow documented, policy enforcement working
5. **Quality**: Maintains Phase 0 quality standards (clean code, good tests, 0 debt)

**Key Indicator**: User can:
1. Trust CA certificate
2. Configure service for api.openai.com
3. Make HTTPS request through proxy
4. See request MITM'd in logs (decrypted but NOT authenticated yet - returns 401)
5. Make request to google.com and see transparent tunnel in logs

---

## Notes

- All tests are currently skipped with `t.Skip("PENDING IMPLEMENTATION: ...")`
- Remove `t.Skip()` as implementation progresses
- Tests should be unskipped in the order listed in "Test-Driven Implementation Order"
- Integration tests require all Phase 2 components to be complete
- Run `go test ./test -race` frequently to catch race conditions early

---

**Next Steps:**
1. Review test suite with team
2. Begin implementation starting with CHAP-qkq (Service structs)
3. Unskip tests as implementation progresses
4. Ensure all tests pass before moving to Phase 3
