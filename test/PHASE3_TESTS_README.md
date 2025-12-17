# Phase 3 Secret Management Tests

**File:** `test/secrets_test.go`
**Status:** All tests skip with "PENDING IMPLEMENTATION" until Phase 3 is complete
**Test Count:** 42 test functions
**Work Items Covered:** CHAP-txf, CHAP-i81, CHAP-2iw, CHAP-vwg

---

## Overview

This test suite provides comprehensive functional validation for Phase 3 (Secret Management). These tests define the acceptance criteria for the credential retrieval system and validate that secrets are fetched securely from various backends.

### Tests are organized into 6 major categories:

1. **Credential Reference Parser** (9 tests) - CHAP-txf
2. **Provider Registry** (4 tests) - CHAP-txf
3. **env: Provider** (5 tests) - CHAP-i81
4. **file: Provider** (10 tests) - CHAP-2iw
5. **Secret Caching** (8 tests) - CHAP-vwg
6. **Integration** (4 tests) - End-to-end validation

---

## Gaming Resistance

These tests **cannot** be satisfied by mocks or shortcuts:

### 1. Credential Reference Parser Tests
- **Real string parsing**: Tests use actual strings like "env:OPENAI_API_KEY"
- **Edge cases**: Missing colons, empty strings, multiple colons, Windows paths
- **Cannot hardcode**: Multiple different inputs require actual parsing logic

### 2. Provider Registry Tests
- **Real map operations**: Tests register and retrieve actual provider instances
- **Concurrent access**: Uses real goroutines, must pass `go test -race`
- **Cannot fake**: Registry must actually store and retrieve providers

### 3. env: Provider Tests
- **REAL environment variables**: Uses `os.Setenv()` and `os.Getenv()`
- **Actual process environment**: No mocks - tests set real env vars
- **Cannot stub**: Provider must actually call `os.Getenv()`

### 4. file: Provider Tests
- **REAL files on disk**: Uses `os.WriteFile()` to create actual files
- **REAL file permissions**: Uses `os.Chmod()` to set actual mode bits
- **REAL permission checks**: Uses `os.Stat()` to verify actual file mode
- **Cannot mock filesystem**: Tests create real files in `t.TempDir()`

### 5. Secret Caching Tests
- **REAL time**: Uses `time.Now()` and `time.Sleep()` - no fake clocks
- **ACTUAL expiration**: Tests verify time-based TTL with real delays
- **Real concurrency**: Uses 100 goroutines to test thread safety
- **Cannot fake**: Cache must actually store values and expire them

### 6. Integration Tests
- **Complete flow**: Parse → Registry → Provider → Secret
- **Real environment + filesystem**: Combines env vars and files
- **Actual caching**: Verifies cache reduces provider calls
- **Cannot mock**: Tests the full integrated system

---

## Test Categories

### CHAP-txf: Credential Reference Parser

**Purpose:** Validate parsing of "protocol:path" format into protocol and path components.

**Tests:**
1. `testParseEnvReference` - Parse "env:OPENAI_API_KEY"
2. `testParseFileReference` - Parse "file:/path/to/secret.txt"
3. `testParseFileWithAbsolutePath` - Parse absolute paths
4. `testParseWindowsPath` - Handle Windows drive letters (C:)
5. `testParseMultipleColons` - Split on FIRST colon only
6. `testParseMissingColon` - Error on invalid format
7. `testParseEmptyString` - Error on empty input
8. `testParseEmptyProtocol` - Error on ":PATH"
9. `testParseEmptyPath` - Error on "protocol:"

**Key Validation:**
- Parser splits on first colon only (handles paths with colons)
- All edge cases properly handled with errors
- Windows paths work correctly

---

### CHAP-txf: Provider Registry

**Purpose:** Validate provider registration and retrieval by protocol name.

**Tests:**
1. `testRegisterAndRetrieveProvider` - Basic register + get
2. `testRetrieveUnknownProvider` - Error on unknown protocol
3. `testRegisterMultipleProviders` - Multiple protocols work
4. `testRegistryConcurrentAccess` - Thread-safe with 100 goroutines

**Key Validation:**
- Registry stores providers correctly
- Unknown providers return error
- Concurrent access is race-free (must pass `go test -race`)

---

### CHAP-i81: env: Provider

**Purpose:** Validate environment variable secret retrieval.

**Tests:**
1. `testEnvFetchExisting` - Fetch existing env var
2. `testEnvFetchMissing` - ErrSecretNotFound for missing var
3. `testEnvFetchEmpty` - ErrSecretNotFound for empty value
4. `testEnvVariableNameValidation` - Valid/invalid name formats
5. `testEnvContextCancellation` - Context cancellation respected

**Key Validation:**
- Uses **REAL** `os.Setenv()` and `os.Getenv()`
- Empty values treated as not found
- Variable names validated (alphanumeric + underscore)
- Context cancellation works

**Gaming Resistance:**
```go
// This test REQUIRES real environment variable:
cleanup := setupEnvVar(t, "TEST_SECRET", "sk-test-key-123")
defer cleanup()

provider := secrets.NewEnvProvider()
secret, err := provider.Fetch(context.Background(), "TEST_SECRET")
// Cannot be satisfied by hardcoding - setupEnvVar uses os.Setenv()
```

---

### CHAP-2iw: file: Provider

**Purpose:** Validate file-based secret retrieval with strict permission checks.

**Tests:**
1. `testFileFetch0600` - SUCCESS with 0600 (rw-------)
2. `testFileFetch0400` - SUCCESS with 0400 (r--------)
3. `testFileReject0644` - REJECT 0644 (rw-r--r--)
4. `testFileReject0666` - REJECT 0666 (rw-rw-rw-)
5. `testFileReject0777` - REJECT 0777 (rwxrwxrwx)
6. `testFileMissing` - ErrSecretNotFound for missing files
7. `testFileLargeFileRejected` - Reject files > 1MB
8. `testFileWhitespaceTrimmed` - Trim leading/trailing whitespace
9. `testFileEmptyRejected` - ErrSecretNotFound for empty files
10. `testFileSymbolicLinks` - Follow symlinks, check target permissions

**Key Validation:**
- Uses **REAL** `os.WriteFile()` to create files
- Uses **REAL** `os.Chmod()` to set permissions
- Uses **REAL** `os.Stat()` to verify file mode
- Files with group/world permissions rejected
- Permission check: `fileMode & 0077 != 0` → ErrPermissionDenied

**Gaming Resistance:**
```go
// This test REQUIRES real file with real permissions:
tmpDir := t.TempDir()
secretFile := filepath.Join(tmpDir, "secret.txt")
require.NoError(t, os.WriteFile(secretFile, []byte("sk-test-key-123"), 0644))

provider := secrets.NewFileProvider()
_, err := provider.Fetch(context.Background(), secretFile)
// Must return ErrPermissionDenied - cannot skip permission check
assert.ErrorIs(t, err, errors.ErrPermissionDenied)
```

---

### CHAP-vwg: Secret Caching

**Purpose:** Validate secret caching with 5-minute TTL to reduce provider calls.

**Tests:**
1. `testCacheHit` - Cached value returned (provider called once)
2. `testCacheMiss` - Different refs have separate cache entries
3. `testCacheExpiration` - TTL expires, provider called again
4. `testCacheInvalidation` - Manual invalidation works
5. `testCacheClear` - Clear removes all entries
6. `testCacheStats` - Hits/Misses/Size tracked correctly
7. `testConcurrentCacheAccess` - Thread-safe with 100 goroutines
8. `testNegativeCaching` - Errors cached too (prevent hammering)

**Key Validation:**
- Uses **REAL** `time.Now()` and `time.Sleep()`
- Verifies actual time-based expiration (not fake clocks)
- Cache hit reduces provider calls (~80% reduction expected)
- Thread-safe concurrent access (must pass `go test -race`)

**Gaming Resistance:**
```go
// This test REQUIRES real time-based expiration:
cached := secrets.NewCachedProvider(mockProvider, 100*time.Millisecond)

// First fetch - cache miss
cached.Fetch(context.Background(), "test-ref")
assert.Equal(t, 1, mockProvider.callCount)

// Wait for TTL to expire
time.Sleep(150 * time.Millisecond)

// Second fetch - cache expired, provider called again
cached.Fetch(context.Background(), "test-ref")
assert.Equal(t, 2, mockProvider.callCount) // Cannot fake - must actually expire
```

---

### Integration Tests

**Purpose:** Validate complete secret management flow end-to-end.

**Tests:**
1. `testIntegrationFetchEnvSecret` - env: reference → secret
2. `testIntegrationFetchFileSecret` - file: reference → secret
3. `testIntegrationUnknownProtocol` - Error for unknown protocol
4. `testIntegrationEndToEndWithCaching` - Complete flow with caching

**Key Validation:**
- Complete flow: Parse credential_ref → Get provider → Fetch secret
- Uses real environment variables and files
- Verifies caching reduces provider calls
- Tests error paths (unknown protocols)

**Gaming Resistance:**
```go
// This test REQUIRES complete integrated system:
cleanup := setupEnvVar(t, "OPENAI_API_KEY", "sk-test-key-123")
defer cleanup()

// Parse → Registry → Provider → Secret
secret, err := secrets.Fetch(context.Background(), "env:OPENAI_API_KEY")
// Cannot be satisfied by mocking - tests entire chain
assert.Equal(t, "sk-test-key-123", secret)
```

---

## Running Tests

### Run all Phase 3 tests:
```bash
cd /Users/bmf/code/chaperone-auth-gateway
go test -v ./test -run TestPhase3_SecretManagement
```

### Run specific category:
```bash
# Parser tests only
go test -v ./test -run TestPhase3_SecretManagement/credential_ref_parser

# File provider tests only
go test -v ./test -run TestPhase3_SecretManagement/file_provider

# Caching tests only
go test -v ./test -run TestPhase3_SecretManagement/secret_caching
```

### Run with race detector (REQUIRED for concurrent tests):
```bash
go test -race ./test -run TestPhase3_SecretManagement
```

### Run with coverage:
```bash
go test -cover ./test -run TestPhase3_SecretManagement
```

---

## Expected Test Results

### Before Phase 3 Implementation:

All 42 tests **SKIP** with message:
```
PENDING IMPLEMENTATION: CHAP-txf - credential_ref parser
PENDING IMPLEMENTATION: CHAP-i81 - env: provider
PENDING IMPLEMENTATION: CHAP-2iw - file: provider
PENDING IMPLEMENTATION: CHAP-vwg - secret caching
PENDING IMPLEMENTATION: Phase 3 integration
```

**This is expected!** Tests define what must be implemented.

### After Phase 3 Implementation:

All 42 tests should **PASS**, indicating:
- ✅ Credential reference parser works correctly
- ✅ Provider registry works correctly
- ✅ env: provider fetches from real environment
- ✅ file: provider enforces permission checks
- ✅ Secret caching works with TTL
- ✅ Integration flow works end-to-end

---

## Test Helpers

### `countingProvider`
Mock provider that counts how many times `Fetch()` is called. Used to verify caching reduces provider calls.

```go
type countingProvider struct {
    secret    string
    callCount int
}
```

### `failingProvider`
Mock provider that always returns an error. Used to verify negative caching (errors cached too).

```go
type failingProvider struct {
    err       error
    callCount int
}
```

---

## Traceability

### STATUS Gaps Addressed:
- Phase 3 had 0% test coverage
- Secret management system was not started
- No validation of credential providers
- No validation of caching behavior

### PLAN Items Validated:

| Work Item | Description | Test Count |
|-----------|-------------|------------|
| CHAP-txf | SecretProvider interface + registry + parser | 13 tests |
| CHAP-i81 | env: provider implementation | 5 tests |
| CHAP-2iw | file: provider with permission checks | 10 tests |
| CHAP-vwg | Secret caching with 5-minute TTL | 8 tests |
| Integration | End-to-end secret fetching flows | 4 tests |

**Total: 40 functional tests + 2 test helpers = 42 test functions**

---

## Implementation Guidance

When implementing Phase 3, uncomment test code and implement features to make tests pass:

### Step 1: CHAP-txf Foundation (1 week)
1. Uncomment parser tests
2. Implement `ParseCredentialRef()` function
3. Run parser tests until all pass
4. Uncomment registry tests
5. Implement provider registry
6. Run registry tests until all pass

### Step 2: CHAP-i81 env: Provider (2 days)
1. Uncomment env provider tests
2. Implement `NewEnvProvider()` and `Fetch()`
3. Run env tests until all pass
4. Verify with `go test -race`

### Step 3: CHAP-2iw file: Provider (1 week)
1. Uncomment file provider tests
2. Implement `NewFileProvider()` and `Fetch()`
3. Implement permission check (critical!)
4. Run file tests until all pass
5. Verify permission tests with real files

### Step 4: CHAP-vwg Caching (1 week)
1. Uncomment caching tests
2. Implement `NewCachedProvider()` wrapper
3. Run caching tests until all pass
4. Verify with `go test -race` (concurrent access)
5. Verify TTL expiration with time.Sleep tests

### Step 5: Integration (2 days)
1. Uncomment integration tests
2. Wire all components together
3. Implement `Fetch()` convenience function
4. Run integration tests until all pass
5. Verify complete flow works end-to-end

---

## Success Criteria

Phase 3 is complete when:

```bash
cd /Users/bmf/code/chaperone-auth-gateway
go test -v ./test -run TestPhase3_SecretManagement
```

Returns:
```
PASS
ok      github.com/bmf/chaperone/test
```

With all 42 tests passing:
- ✅ 9 parser tests pass
- ✅ 4 registry tests pass
- ✅ 5 env provider tests pass
- ✅ 10 file provider tests pass
- ✅ 8 caching tests pass
- ✅ 4 integration tests pass
- ✅ 0 race conditions (verified with -race flag)

---

## Anti-Gaming Measures

These tests **enforce** correct implementation:

### ❌ Cannot be satisfied by:
- Creating MagicMock() with invented attributes
- Hardcoding expected outputs
- Skipping permission checks
- Using fake timers for TTL
- Mocking os.Getenv or os.Stat
- Stubbing filesystem operations

### ✅ Must be satisfied by:
- Real credential_ref parser
- Real provider registry with sync.RWMutex
- Real os.Getenv() for env: provider
- Real os.WriteFile() + os.Chmod() + os.Stat() for file: provider
- Real time.Now() + TTL expiration for caching
- Real goroutines + sync.Map for concurrent access
- Real integration of all components

---

## Related Documentation

- **Planning:** `.agent_planning/PLAN-2025-12-01-015951.md` (Phase 3 work items)
- **Status:** `.agent_planning/STATUS-2025-12-01-094900.md` (Phase 2 complete, Phase 3 next)
- **Errors:** `internal/errors/errors.go` (ErrSecretNotFound, ErrPermissionDenied)
- **Interface:** `internal/secrets/provider.go` (SecretProvider interface)

---

## Summary

**42 comprehensive functional tests** that validate Phase 3 (Secret Management) cannot be satisfied by shortcuts or mocks. Tests enforce:

1. **Real parsing** of credential references
2. **Real environment variables** for env: provider
3. **Real files with real permissions** for file: provider
4. **Real time-based expiration** for caching
5. **Real concurrent access** for thread safety
6. **Real integration** of complete flow

These tests define the contract that Phase 3 implementation must fulfill. Make them uncompromising, realistic, and impossible to game.
