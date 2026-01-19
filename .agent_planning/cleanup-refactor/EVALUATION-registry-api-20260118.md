# Evaluation: Registry API Standardization
**Date:** 2026-01-18
**Topic:** registry-api
**Status:** RESEARCH COMPLETE → HIGH CONFIDENCE

## Research Findings

### Caller Analysis

**Total callers of `ServiceRegistry.Lookup()`:** 6 locations in production code

**Breakdown:**
1. `internal/proxy/auth_handler.go:36` - Auth stripping handler
2. `internal/proxy/auth_handler.go:107` - Auth injection handler
3. `internal/proxy/policy_handler.go:23` - Policy enforcement (method check)
4. `internal/proxy/policy_handler.go:111` - Policy enforcement (path check)
5. `internal/proxy/policy_handler.go:161` - Policy enforcement (body size check)
6. `internal/service/matcher.go:12` - MITM decision

### Usage Patterns

All 6 callers follow the same pattern:
```go
svc, found := registry.Lookup(r.Host)
if !found {
    return r, nil  // Pass through - not our service
}
// ... use svc
```

**Key Insight:** The `bool` return is ONLY used for "not found" flow control. It's never used to distinguish between:
- Service not configured (bool = false)
- Service lookup error (would need error return)

Currently there is NO way for `Lookup()` to return an error - it's always successful or not found.

### Current Registry Comparison

| Registry | Method | Returns | Error Handling |
|----------|--------|---------|----------------|
| Secrets | `Fetch(ctx, ref)` | `(string, error)` | Returns error on provider failure |
| Auth Strategy | `Get(key)` | `(Strategy, error)` | Returns error if not found |
| **Service** | `Lookup(hostname)` | `(*Service, bool)` | **INCONSISTENT** - uses bool |

### Error Message Decision

**Question:** What error should be returned when service not found?

**Answer:** Simple, descriptive error with hostname context:
```go
fmt.Errorf("service not found for hostname: %s", hostname)
```

This aligns with other registries and provides useful debugging info.

### Implementation Impact

**Low impact change:**
- Only 6 callers to update
- All follow identical pattern (check bool → check error)
- No semantic changes - still just "found" vs "not found"
- No complex error handling needed - all callers just pass through if not found

**Change pattern:**
```go
// Before
svc, found := registry.Lookup(r.Host)
if !found {
    return r, nil
}

// After
svc, err := registry.Lookup(r.Host)
if err != nil {
    return r, nil
}
```

## Resolved Unknowns

✅ **Caller impact** - 6 callers, all trivial updates
✅ **Error messages** - Use descriptive format with hostname
✅ **Semantic changes** - None, just bool → error pattern

## Architectural Assessment

**Consistency gain:**
- Aligns ServiceRegistry with Secrets and Auth registries
- Standard Go idiom: `(value, error)` instead of `(value, bool)`
- More extensible - future could distinguish "not found" from "lookup failed"

**Risk:** LOW
- Small, localized change
- All callers in same package (proxy) or service package
- No behavior changes, just API consistency

## Verdict

**CONTINUE** - All unknowns resolved. Ready for HIGH confidence sprint plan.

## Next Steps

1. Update SPRINT-20260118-registry-api.md to HIGH confidence
2. Create detailed DOD and CONTEXT files
3. Present to user for approval
