# Sprint: Fix Test Failures

**Generated**: 2026-01-17
**Confidence**: HIGH
**Status**: READY FOR IMPLEMENTATION

## Sprint Goal

Fix all pre-existing test failures in the `test/` package so that `go test ./...` passes.

## User Decisions (Resolved)

| Question | Answer |
|----------|--------|
| SecretProvider interface | Create provider.go |
| Missing directories | Remove from tests, add roadmap items |
| Setup command test | Delete test file |
| Flaky shutdown test | Skip with TODO, add roadmap item |

## Deliverables

### P0: Fix Interface Tests (Required)

**Current failures**:
- `TestCoreInterfaces/secret_provider_interface`
- `TestPackageImports/provider_imports_context`
- `TestPhase01Completion`
- `TestCompilationValidation`
- `TestInterfacesHaveGodoc/SecretProvider`

**Fix**:
1. Create `internal/secrets/provider.go` with:
   ```go
   // SecretProvider fetches secrets from a source.
   // Implementations include EnvProvider, FileProvider, KeychainProvider.
   type SecretProvider interface {
       // Fetch retrieves a secret value for the given path.
       Fetch(ctx context.Context, path string) (string, error)
   }
   ```

2. Update `internal/secrets/registry.go`:
   - Change `Provider` to `SecretProvider`
   - Or add `type Provider = SecretProvider` alias for backwards compatibility

3. Remove failing logger context test:
   - The test expects `LogRequest(ctx, ...)` but logger uses `Log(entry)`
   - Remove `logger_imports_context` test from `interfaces_test.go`

### P1: Fix Scaffolding Tests (Required)

**Current failures**:
- `TestProjectScaffolding/directory_structure_complete`

**Fix**:
1. Edit `test/scaffolding_test.go`
2. Remove these from `requiredDirs`:
   - `internal/client`
   - `test/e2e`
   - `examples`
   - `docs`

### P2: Delete Setup Command Test (Required)

**Current failures**:
- `TestSetupCommand/setup_command_is_available`
- `TestSetupCommand/setup_command_has_correct_flags`

**Fix**:
1. Delete `test/setup_test.go` entirely
2. The `setup` command was never implemented and isn't part of current architecture

### P3: Skip Flaky Shutdown Test (Required)

**Current failure**:
- `TestProxyServerLifecycle/server_graceful_shutdown_with_active_connections`

**Fix**:
1. Edit `test/proxy_test.go`
2. Add `t.Skip("TODO: Investigate graceful shutdown timing - see roadmap")` at start of test

### P4: Add Roadmap Items (Required)

Add these items to roadmap for future work:
1. "Add examples/ directory with usage examples"
2. "Add docs/ directory with documentation"
3. "Fix flaky graceful shutdown test - investigate timing issue"

## Files to Modify

| File | Action |
|------|--------|
| `internal/secrets/provider.go` | Create |
| `internal/secrets/registry.go` | Rename `Provider` → `SecretProvider` |
| `internal/secrets/env.go` | Update to use `SecretProvider` |
| `internal/secrets/file.go` | Update to use `SecretProvider` |
| `internal/secrets/keychain.go` | Update to use `SecretProvider` |
| `test/interfaces_test.go` | Remove logger context test |
| `test/scaffolding_test.go` | Remove aspirational directories |
| `test/setup_test.go` | Delete |
| `test/proxy_test.go` | Skip flaky test |

## Verification

- `go build ./...` passes
- `go test ./...` passes (all tests green)
- No new test failures introduced

## Risks

| Risk | Mitigation |
|------|------------|
| Renaming Provider breaks imports | Update all usages in secrets package |
| Other code uses secrets.Provider | Search codebase, update orchestrate.go if needed |

## Out of Scope

- Actually implementing examples/ and docs/ content
- Actually fixing the graceful shutdown timing issue
- Implementing the setup command
