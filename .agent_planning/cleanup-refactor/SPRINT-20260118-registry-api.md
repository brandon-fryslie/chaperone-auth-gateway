# Sprint: registry-api - Standardize Registry API Pattern
**Generated:** 2026-01-18 (Updated after research)
**Confidence:** HIGH
**Status:** READY FOR IMPLEMENTATION

## Sprint Goal
Standardize `ServiceRegistry.Lookup()` to use `(value, error)` pattern, aligning with other registries.

## Scope

**Deliverables:**
- Update `ServiceRegistry` interface: `Lookup(hostname) (*Service, error)`
- Update all 6 callers to handle error instead of bool
- Consistent error handling across all registries

## Work Items

### P0: Update Service Registry Interface

**Files:**
- `internal/service/registry.go` - Interface definition
- `internal/service/registry_impl.go` - Implementation

**Acceptance Criteria:**
- [ ] Change `Lookup(hostname string) (*Service, bool)` to `Lookup(hostname string) (*Service, error)`
- [ ] Return `fmt.Errorf("service not found for hostname: %s", hostname)` when not found
- [ ] Update implementation to return error instead of bool
- [ ] Update interface tests in `test/service_registry_impl_test.go`
- [ ] Tests pass

**Technical Notes:**
- Keep the same lookup logic (hostname matching)
- Only change return signature: `(svc, false)` → `(nil, error)`
- Error message should include hostname for debugging

### P1: Update All Callers (6 locations)

**Files:**
- `internal/proxy/auth_handler.go` (2 locations: lines 36, 107)
- `internal/proxy/policy_handler.go` (3 locations: lines 23, 111, 161)
- `internal/service/matcher.go` (1 location: line 12)

**Acceptance Criteria:**
- [ ] All callers updated from `svc, found := Lookup()` to `svc, err := Lookup()`
- [ ] All bool checks changed to error checks: `if !found` → `if err != nil`
- [ ] Behavior unchanged: not found → pass through request
- [ ] No new error logging needed (pass-through is expected behavior)
- [ ] Tests pass

**Change Pattern:**
```go
// Before
svc, found := registry.Lookup(r.Host)
if !found {
    return r, nil  // Pass through
}

// After
svc, err := registry.Lookup(r.Host)
if err != nil {
    return r, nil  // Pass through
}
```

**Special Case (matcher.go):**
```go
// Before
_, found := registry.Lookup(hostname)
return found

// After
_, err := registry.Lookup(hostname)
return err == nil  // true if found, false if error
```

### P2: Verify Consistency

**Acceptance Criteria:**
- [ ] All three registries now use `(value, error)` pattern
- [ ] No bool returns remain in registry interfaces
- [ ] Error messages are descriptive and consistent
- [ ] All tests pass (unit + integration)

## Dependencies
- Sprint: split-handlers (COMPLETE) - handlers are main callers
- Sprint: split-examine (COMPLETE) - no dependencies, but good to have clean baseline

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Callers missed | LOW | HIGH | Research found all 6, grep confirmed |
| Tests break | LOW | LOW | Tests use same pattern, easy to fix |
| Semantic mismatch | VERY LOW | LOW | All callers just need "found vs not found" |

## Implementation Notes

**Order of changes:**
1. Update interface definition (`registry.go`)
2. Update implementation (`registry_impl.go`)
3. Update tests (`test/service_registry_impl_test.go`)
4. Update all 6 callers (one file at a time, commit each)
5. Run full test suite

**Commit strategy:**
- Commit 1: Update interface + implementation + tests
- Commit 2: Update auth_handler.go callers
- Commit 3: Update policy_handler.go callers
- Commit 4: Update matcher.go caller

**Why this matters:**
- Consistency with other registries (Secrets, Auth Strategy)
- Standard Go idiom for "value or error"
- More extensible (future could distinguish error types)
- Cleaner API surface

## Verification Commands

```bash
# Ensure all callers updated
grep -r "registry.Lookup" internal/ | grep -v "// "

# Build + test
go build ./...
go test ./...
go test -race ./...
go vet ./...

# Verify no bool pattern remains
grep "found :=" internal/proxy/*.go internal/service/*.go
```

## Success Criteria

All three registry types now use consistent `(value, error)` API:
- ✅ SecretsRegistry: `Fetch(ctx, ref) (string, error)`
- ✅ AuthRegistry: `Get(key) (Strategy, error)`
- ✅ ServiceRegistry: `Lookup(hostname) (*Service, error)`
