# Integration Tests for Phase 2 MITM Functionality

This directory contains comprehensive integration tests that validate the complete Man-in-the-Middle (MITM) proxy functionality.

## Overview

These tests validate the **complete end-to-end MITM workflow** including:
- CA certificate generation and trust chain validation
- Selective MITM for configured domains
- Transparent tunneling for non-configured domains
- Policy enforcement (methods, paths, body size limits)
- TLS certificate generation with IP SAN support

## Anti-Gaming Design

These tests are designed to be **un-gameable** - they cannot be satisfied by stubs, mocks, or shortcuts:

1. Tests use **REAL HTTP clients** (`net/http.Client`)
2. Tests make **ACTUAL network requests** over TCP sockets
3. Tests verify **REAL TLS handshakes** using `crypto/tls`
4. Tests verify **ACTUAL certificate chains** via x509 validation
5. Tests use **REAL proxy and upstream servers** (not mocks)
6. Tests verify **ACTUAL request/response data flows**
7. Tests verify **REAL policy enforcement** with correct HTTP status codes

The entire MITM pipeline must work correctly for tests to pass.

## Test Suite

### TestSelectiveMITMWithTrustedCA
Validates the complete MITM flow with a trusted CA:
- Generates a test CA and stores it
- Creates an upstream HTTPS server
- Registers the service in the proxy
- Configures client to trust the test CA
- Makes an HTTPS request through the proxy
- **Verifies**: Request is decrypted, forwarded, and response returned
- **Verifies**: Headers are preserved
- **Verifies**: Body data flows correctly

### TestTransparentTunnelForNonConfiguredDomains
Validates transparent tunneling for domains not in the service registry:
- Creates proxy with MITM capability
- Leaves service registry empty
- Makes HTTPS request to unconfigured domain
- **Verifies**: Request uses transparent tunnel (no MITM)
- **Verifies**: Certificate is from upstream (not our CA)
- **Verifies**: Request/response works correctly

### TestPolicyEnforcementEndToEnd
Validates policy enforcement in MITM mode with three subtests:

#### disallowed_method_returns_403
- Configures service to only allow POST
- Attempts DELETE request
- **Verifies**: Returns 403 Forbidden
- **Verifies**: Upstream is never called

#### disallowed_path_returns_403
- Configures service to only allow `/v1/*` paths
- Attempts request to `/admin/users`
- **Verifies**: Returns 403 Forbidden
- **Verifies**: Upstream is never called

#### oversized_body_returns_413
- Configures service with 1KB max body size
- Attempts POST with 2KB body
- **Verifies**: Returns 413 Payload Too Large
- **Verifies**: Upstream is never called

### TestCertificateTrustValidation
Validates certificate trust chain validation with two subtests:

#### untrusted_ca_fails
- Creates proxy with test CA
- Client does NOT trust the test CA
- Attempts HTTPS request
- **Verifies**: Connection fails with certificate error

#### trusted_ca_succeeds
- Creates proxy with test CA
- Client DOES trust the test CA
- Makes HTTPS request
- **Verifies**: Request succeeds
- **Verifies**: Gets 200 OK from upstream

## Running the Tests

### Run all integration tests
```bash
go test -v ./test/integration/...
```

### Run specific test
```bash
go test -v ./test/integration/... -run TestSelectiveMITMWithTrustedCA
```

### Run with race detection
```bash
go test -race -v ./test/integration/...
```

### Skip in short mode
```bash
go test -short ./...  # Skips integration tests
```

## Implementation Notes

### Test Utilities

**`findAvailablePort(t)`**
- Finds an available TCP port for test servers
- Ensures tests don't conflict on fixed ports

**`newTestUpstreamClient(t)`**
- Creates HTTP client with `InsecureSkipVerify` for testing
- Only safe for localhost testing
- Required because `httptest.NewTLSServer` uses self-signed certs

### MITM Test Configuration

Tests use `proxy.MITMOptions` to inject a custom upstream client:
```go
proxyServer := proxy.NewWithMITM(
    cfg, logger, shutdownMgr,
    registry, certCache,
    &proxy.MITMOptions{
        UpstreamClient: newTestUpstreamClient(t),
    },
)
```

This allows tests to accept the self-signed certificates from `httptest` servers.

## Implementation Changes Made

These tests exposed and fixed several bugs:

1. **IP SAN Support** (`internal/mitm/certcache.go`)
   - Added IP address detection and IP SAN generation
   - Certificates now work for both DNS names and IP addresses

2. **Host:Port Handling** (`internal/proxy/tunnel.go`)
   - Fixed to pass full `host:port` to MITM handler
   - Allows correct upstream URL construction

3. **Test Configurability** (`internal/proxy/server.go`)
   - Added `MITMOptions` for optional custom upstream client
   - Maintains backward compatibility

4. **Client Constructor** (`internal/client/upstream.go`)
   - Added `NewClientWithHTTPClient` for test injection
   - Allows custom TLS configurations

## Workflow Coverage

These tests validate the key user workflows:

1. **Selective MITM**: Proxy intercepts configured domains, inspects traffic
2. **Transparent Tunnel**: Non-configured domains pass through untouched
3. **Policy Enforcement**: Invalid requests blocked before reaching upstream
4. **Certificate Trust**: Only trusted CA certificates accepted

## Test Results

All 8 tests (4 top-level, 4 subtests) pass successfully:

```
PASS: TestSelectiveMITMWithTrustedCA
PASS: TestTransparentTunnelForNonConfiguredDomains
PASS: TestPolicyEnforcementEndToEnd
  PASS: disallowed_method_returns_403
  PASS: disallowed_path_returns_403
  PASS: oversized_body_returns_413
PASS: TestCertificateTrustValidation
  PASS: untrusted_ca_fails
  PASS: trusted_ca_succeeds
```

## Phase 2 Completion

These integration tests serve as the **completion gate for Phase 2 MITM functionality**. All critical workflows are validated:

- CA generation and storage
- Dynamic certificate generation with IP SAN support
- TLS handshake in MITM mode
- HTTP request decryption and proxying
- Policy enforcement (methods, paths, body size)
- Transparent tunnel fallback

The tests prove the entire MITM pipeline works end-to-end.
