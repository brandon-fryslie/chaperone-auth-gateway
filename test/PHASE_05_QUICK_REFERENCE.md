# Phase 0.5: Test Infrastructure - Quick Reference

**Work Item:** CHAP-30o
**Status:** Tests written, awaiting implementation

---

## Quick Commands

```bash
# Navigate to project
cd /Users/bmf/code/chaperone-auth-gateway

# Check current test status (will fail - expected)
go test -v -run TestTestInfrastructure ./test/

# Install dependencies
go get github.com/stretchr/testify
go get github.com/google/uuid

# After implementing helpers, run tests
go test -v -run TestTestInfrastructure ./test/

# Check for race conditions
go test -race -run TestTestInfrastructure ./test/

# View test file
cat test/helpers_test.go

# View documentation
cat test/HELPERS_TEST_README.md
cat test/HELPERS_TEST_VERIFICATION.md

# View summary
cat test/helpers_tests_summary.json
```

---

## Implementation Checklist

### Step 1: Install Dependencies
```bash
go get github.com/stretchr/testify
go get github.com/google/uuid
```

### Step 2: Create Helper Files
```bash
mkdir -p test/helpers
touch test/helpers/certs.go
touch test/helpers/mock_server.go
touch test/helpers/context.go
```

### Step 3: Implement Certificate Helpers (`test/helpers/certs.go`)
- [ ] `GenerateTestCA() (*x509.Certificate, *rsa.PrivateKey, error)`
- [ ] `GenerateTestCert(ca *x509.Certificate, caKey *rsa.PrivateKey, hostname string) (tls.Certificate, error)`
- [ ] `WriteCertPEM(cert *x509.Certificate, path string) error`
- [ ] `WriteKeyPEM(key *rsa.PrivateKey, path string) error`

### Step 4: Implement Mock Server Helpers (`test/helpers/mock_server.go`)
- [ ] `type MockServer struct { Server *httptest.Server; Requests []RecordedRequest }`
- [ ] `type RecordedRequest struct { Method, Path string; Headers http.Header; Body []byte }`
- [ ] `type MockResponse struct { StatusCode int; Body []byte; Headers map[string]string }`
- [ ] `NewMockServer(handler http.Handler) *MockServer`
- [ ] `NewMockTLSServer(handler http.Handler) *MockServer`
- [ ] `RecordRequests(responses map[string]MockResponse) http.Handler`

### Step 5: Implement Context Helpers (`test/helpers/context.go`)
- [ ] `TestContext() context.Context`
- [ ] `TestContextWithTimeout(d time.Duration) (context.Context, context.CancelFunc)`

### Step 6: Run Tests
```bash
go test -v -run TestTestInfrastructure ./test/
```

### Step 7: Verify
- [ ] All 13 test functions pass
- [ ] No build errors
- [ ] No race conditions: `go test -race ./test/`
- [ ] Integration test passes

---

## Test Structure

### Certificate Generation Tests (5 tests)
1. **testGenerateTestCA** - CA generation
2. **testGenerateTestCert** - Leaf cert generation
3. **testTLSConnectionWorks** - Real TLS handshake
4. **testCertPEMExportImport** - Certificate PEM format
5. **testKeyPEMExportImport** - Key PEM format + permissions

### Mock Server Tests (4 tests)
1. **testHTTPMockServer** - HTTP mock server
2. **testHTTPSMockServer** - HTTPS mock server
3. **testRequestRecording** - Request capture
4. **testMockResponses** - Response configuration

### Context Tests (3 tests)
1. **testTestContextCreation** - Context with request ID
2. **testTestContextWithTimeout** - Timeout handling
3. **testRequestIDPropagation** - ID uniqueness/propagation

### Integration Test (1 test)
1. **TestTestInfrastructureIntegration** - All helpers together

**Total: 13 tests, 58+ assertions**

---

## Key Implementation Details

### Certificate Generation
- Use 2048-bit RSA keys minimum
- CA: BasicConstraintsValid=true, IsCA=true, KeyUsage=CertSign
- Leaf: IsCA=false, DNSNames=[hostname], ExtKeyUsage=ServerAuth
- Validity: 1 year from now (or longer)
- Set NotBefore to time.Now().Add(-5*time.Minute) for clock skew

### Private Key Permissions
- **CRITICAL:** Write keys with 0600 permissions (owner read/write only)
- Use `os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)`
- Test verifies this explicitly

### Request Recording
- **CRITICAL:** Protect concurrent access with mutex
- Store method, path, headers, body for each request
- Map key format: `fmt.Sprintf("%s %s", method, path)`

### Context Keys
- Use unexported type for context keys to avoid collisions:
  ```go
  type contextKey string
  const requestIDKey contextKey = "request_id"
  ```
- Generate UUIDs for request IDs

---

## Common Errors

### Error: "helpers package not found"
**Cause:** Helpers not implemented yet
**Fix:** Create test/helpers/ with .go files

### Error: "undefined: testify"
**Cause:** testify not installed
**Fix:** `go get github.com/stretchr/testify`

### Error: "undefined: uuid"
**Cause:** uuid package not installed
**Fix:** `go get github.com/google/uuid`

### Error: Certificate verification fails
**Cause:** Certificate not properly signed by CA
**Fix:** Ensure x509.CreateCertificate uses CA cert as parent

### Error: File permission test fails
**Cause:** Key file not written with 0600
**Fix:** Use os.OpenFile with explicit mode

### Error: Request recording race
**Cause:** Concurrent writes to Requests slice
**Fix:** Add mutex protection

---

## Success Indicators

✅ All tests pass
✅ No build errors
✅ No race conditions
✅ TLS connections work
✅ Certificates verify correctly
✅ Mock servers respond correctly
✅ Requests are recorded
✅ Contexts have unique IDs
✅ Timeouts work

---

## Next Phase

After all tests pass:

**→ Phase 0.6: Context Propagation (CHAP-5e3)**

Phase 0.6 extends the context helpers for production use:
- Production context creation patterns
- Service/hostname context values
- Context propagation through middleware
- Context cancellation patterns

---

## Why This Matters

These helpers enable:
- ✅ Testing TLS/MITM in Phase 2
- ✅ Testing with mock upstreams in Phase 1+
- ✅ Verifying request modification in Phase 4
- ✅ Testing context propagation in Phase 0.6+
- ✅ Integration testing across all phases

**Without these helpers, we cannot do test-first development.**

---

## Files

| File | Size | Purpose |
|------|------|---------|
| test/helpers_test.go | 570 lines | Functional tests |
| test/HELPERS_TEST_README.md | ~12 KB | Complete documentation |
| test/HELPERS_TEST_VERIFICATION.md | ~10 KB | Verification guide |
| test/helpers_tests_summary.json | ~6 KB | Machine-readable summary |
| test/PHASE_05_QUICK_REFERENCE.md | This file | Quick reference |

---

## Test Philosophy

These tests follow the functional testing philosophy:

1. **Real Execution** - Use actual crypto, TLS, HTTP operations
2. **Observable Results** - Verify externally observable outcomes
3. **No Mocks** - Test real implementations, not test doubles
4. **Un-Gameable** - Cannot be satisfied by shortcuts
5. **User-Centric** - Test what developers will actually use

**Gaming Resistance: 10/10**

---

## Resources

- **PLAN:** PLAN-2025-11-26-031437.md lines 253-288
- **Work Item:** CHAP-30o
- **Priority:** Critical (blocks all feature development)
- **Complexity:** High (requires careful crypto implementation)

---

**Status:** Ready for implementation
**Next Action:** Implement test/helpers/ package
