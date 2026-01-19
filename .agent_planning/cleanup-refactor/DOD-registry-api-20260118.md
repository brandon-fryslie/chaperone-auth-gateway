# Registry API Standardization - Definition of Done
**Generated:** 2026-01-18
**Sprint:** registry-api
**Confidence:** HIGH

## Overall Acceptance Criteria

### Interface Changes
- [ ] `ServiceRegistry.Lookup()` signature changed to `(*Service, error)`
- [ ] Implementation returns descriptive error when not found
- [ ] Interface documentation updated

### Code Updates
- [ ] All 6 callers updated to use error pattern
- [ ] `internal/proxy/auth_handler.go` - 2 locations updated
- [ ] `internal/proxy/policy_handler.go` - 3 locations updated
- [ ] `internal/service/matcher.go` - 1 location updated
- [ ] No `found :=` pattern remains for service lookups

### Testing
- [ ] `go build ./...` passes
- [ ] `go test ./...` passes
- [ ] `go test -race ./...` passes
- [ ] `go vet ./...` passes
- [ ] Interface tests updated and passing

### Verification
- [ ] Grep confirms all callers updated: `grep -r "registry.Lookup" internal/`
- [ ] Grep confirms no bool pattern: `grep "found :=" internal/proxy/*.go internal/service/*.go` returns nothing
- [ ] All three registries now use `(value, error)` pattern

### Behavior
- [ ] No behavior changes - requests still pass through when service not found
- [ ] No new logging added (pass-through is expected, not an error)
- [ ] Integration tests unchanged or improved

## Per-Work-Item Acceptance

### P0: Update Service Registry Interface
- [ ] `registry.go` - Interface signature updated
- [ ] `registry_impl.go` - Implementation returns error
- [ ] Error message format: `"service not found for hostname: %s"`
- [ ] Tests in `test/service_registry_impl_test.go` updated
- [ ] Tests pass

### P1: Update All Callers
- [ ] `auth_handler.go:36` - `svc, err := registry.Lookup(r.Host)`
- [ ] `auth_handler.go:107` - `svc, err := registry.Lookup(r.Host)`
- [ ] `policy_handler.go:23` - `svc, err := registry.Lookup(r.Host)`
- [ ] `policy_handler.go:111` - `svc, err := registry.Lookup(r.Host)`
- [ ] `policy_handler.go:161` - `svc, err := registry.Lookup(r.Host)`
- [ ] `matcher.go:12` - `_, err := registry.Lookup(hostname); return err == nil`
- [ ] All bool checks changed to error checks

### P2: Verify Consistency
- [ ] SecretsRegistry uses `(value, error)` ✓ (already done)
- [ ] AuthRegistry uses `(value, error)` ✓ (already done)
- [ ] ServiceRegistry uses `(value, error)` ← (this sprint)
- [ ] No registry interfaces use bool returns
- [ ] Error messages are descriptive

## Verification Commands

```bash
# Build verification
go build ./...

# Test verification
go test ./... -v
go test -race ./...

# Lint verification
go vet ./...

# Caller verification
echo "=== All Lookup calls (should show error pattern) ==="
grep -r "registry.Lookup" internal/ | grep -v "// "

echo "=== Bool pattern check (should be empty) ==="
grep "found :=" internal/proxy/*.go internal/service/*.go || echo "✓ No bool pattern found"

# Integration test
go test ./test/integration/... -v
```

## Success Indicators

**Code quality:**
- API consistency across all registries
- Standard Go error handling idiom
- Clear, descriptive error messages

**Test coverage:**
- All existing tests pass
- Interface tests verify new signature
- No behavior changes in integration tests

**Documentation:**
- Interface godoc updated
- Error cases documented
- Change pattern clear in commits

## Commit Strategy

Each commit should:
1. Be atomic (one logical change)
2. Pass all tests
3. Have clear commit message

Expected commits:
1. `refactor(service): change Lookup to return error instead of bool`
2. `refactor(proxy): update auth_handler to use error pattern`
3. `refactor(proxy): update policy_handler to use error pattern`
4. `refactor(service): update matcher to use error pattern`

## Rollback Plan

If issues arise:
1. Each commit is atomic - can revert individual commits
2. Interface change is the foundation - reverting commit 1 reverts all
3. Tests guard against semantic changes
