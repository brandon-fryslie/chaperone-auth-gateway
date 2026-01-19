# Registry API Standardization - Implementation Context
**Generated:** 2026-01-18
**Sprint:** registry-api

## Current State

### Registry Interfaces

**SecretsRegistry (already consistent):**
```go
// internal/secrets/registry.go
type Registry interface {
    Register(provider Provider) error
    Fetch(ctx context.Context, ref string) (string, error)  // ✓ Uses error
}
```

**AuthRegistry (already consistent):**
```go
// internal/auth/registry.go
type Registry interface {
    Register(name string, strategy Strategy) error
    Get(name string) (Strategy, error)  // ✓ Uses error
}
```

**ServiceRegistry (needs update):**
```go
// internal/service/registry.go
type ServiceRegistry interface {
    Register(service *Service) error
    Lookup(hostname string) (*Service, bool)  // ✗ Uses bool - INCONSISTENT
    ListAll() []*Service
}
```

### Current Implementation

```go
// internal/service/registry_impl.go
func (r *serviceRegistryImpl) Lookup(hostname string) (*Service, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    for _, svc := range r.services {
        if svc.HostPattern == hostname {
            return svc, true
        }
    }
    return nil, false  // Not found
}
```

### All Callers (6 locations)

**auth_handler.go - 2 calls:**
```go
// Line 36 - Security strip handler
svc, found := registry.Lookup(r.Host)
if !found {
    return r, nil
}

// Line 107 - Auth injection handler
svc, found := registry.Lookup(r.Host)
if !found {
    return r, nil
}
```

**policy_handler.go - 3 calls:**
```go
// Line 23 - Method check
svc, found := registry.Lookup(r.Host)
if !found || svc.Policy == nil {
    return r, nil
}

// Line 111 - Path check
svc, found := registry.Lookup(r.Host)
if !found || svc.Policy == nil {
    return r, nil
}

// Line 161 - Body size check
svc, found := registry.Lookup(r.Host)
if !found || svc.Policy == nil {
    return r, nil
}
```

**matcher.go - 1 call:**
```go
// Line 12 - MITM decision
_, found := registry.Lookup(hostname)
return found
```

## Target State

### Updated Interface

```go
// internal/service/registry.go
type ServiceRegistry interface {
    Register(service *Service) error
    Lookup(hostname string) (*Service, error)  // ✓ Now consistent
    ListAll() []*Service
}
```

### Updated Implementation

```go
// internal/service/registry_impl.go
func (r *serviceRegistryImpl) Lookup(hostname string) (*Service, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    for _, svc := range r.services {
        if svc.HostPattern == hostname {
            return svc, nil  // Found - no error
        }
    }
    return nil, fmt.Errorf("service not found for hostname: %s", hostname)
}
```

### Updated Callers

**auth_handler.go:**
```go
// Line 36
svc, err := registry.Lookup(r.Host)
if err != nil {
    return r, nil
}

// Line 107
svc, err := registry.Lookup(r.Host)
if err != nil {
    return r, nil
}
```

**policy_handler.go:**
```go
// Line 23
svc, err := registry.Lookup(r.Host)
if err != nil || svc.Policy == nil {
    return r, nil
}

// Line 111
svc, err := registry.Lookup(r.Host)
if err != nil || svc.Policy == nil {
    return r, nil
}

// Line 161
svc, err := registry.Lookup(r.Host)
if err != nil || svc.Policy == nil {
    return r, nil
}
```

**matcher.go:**
```go
// Line 12
_, err := registry.Lookup(hostname)
return err == nil  // true if found, false if error
```

## Implementation Steps

### Step 1: Update Interface + Implementation

**Files to modify:**
1. `internal/service/registry.go` - Change interface signature
2. `internal/service/registry_impl.go` - Update implementation
3. `test/service_registry_impl_test.go` - Update tests

**Changes:**
```go
// registry.go - Update interface
-    Lookup(hostname string) (*Service, bool)
+    Lookup(hostname string) (*Service, error)

// registry_impl.go - Update implementation
 func (r *serviceRegistryImpl) Lookup(hostname string) (*Service, error) {
     r.mu.RLock()
     defer r.mu.RUnlock()

     for _, svc := range r.services {
         if svc.HostPattern == hostname {
-            return svc, true
+            return svc, nil
         }
     }
-    return nil, false
+    return nil, fmt.Errorf("service not found for hostname: %s", hostname)
 }
```

**Test updates:**
```go
// Test "found" case
svc, err := registry.Lookup("api.openai.com")
assert.NoError(t, err)
assert.NotNil(t, svc)

// Test "not found" case
svc, err := registry.Lookup("unknown.example.com")
assert.Error(t, err)
assert.Nil(t, svc)
assert.Contains(t, err.Error(), "service not found for hostname")
```

### Step 2: Update auth_handler.go (2 locations)

```bash
# Find exact lines
grep -n "found := registry.Lookup" internal/proxy/auth_handler.go
```

**Location 1 (line ~36):**
```go
-    svc, found := registry.Lookup(r.Host)
-    if !found {
+    svc, err := registry.Lookup(r.Host)
+    if err != nil {
         return r, nil
     }
```

**Location 2 (line ~107):**
```go
-    svc, found := registry.Lookup(r.Host)
-    if !found {
+    svc, err := registry.Lookup(r.Host)
+    if err != nil {
         return r, nil
     }
```

### Step 3: Update policy_handler.go (3 locations)

```bash
# Find exact lines
grep -n "found := registry.Lookup" internal/proxy/policy_handler.go
```

All three follow same pattern:
```go
-    svc, found := registry.Lookup(r.Host)
-    if !found || svc.Policy == nil {
+    svc, err := registry.Lookup(r.Host)
+    if err != nil || svc.Policy == nil {
         return r, nil
     }
```

### Step 4: Update matcher.go (1 location)

```bash
# Find exact line
grep -n "found := registry.Lookup" internal/service/matcher.go
```

**Special case - returns bool:**
```go
-    _, found := registry.Lookup(hostname)
-    return found
+    _, err := registry.Lookup(hostname)
+    return err == nil  // true if found (no error), false if not found (error)
```

## Testing Strategy

### Unit Tests

Update `test/service_registry_impl_test.go`:
- Change assertions from `assert.True/False(found)` to `assert.Error/NoError(err)`
- Verify error message format
- Ensure nil service when error returned

### Integration Tests

No changes needed to integration tests:
- Behavior is unchanged (requests still pass through)
- Tests should still pass without modification
- If any fail, investigate why bool vs error matters

### Verification Greps

```bash
# Should find all Lookup calls with error pattern
grep -r "registry.Lookup" internal/ | grep -v "// "

# Should return empty (no more bool pattern)
grep "found :=" internal/proxy/*.go internal/service/*.go

# Should show consistent error patterns
grep -A 1 "registry.Lookup" internal/proxy/*.go internal/service/*.go
```

## Error Handling Philosophy

**Not an error condition:**
- Service not configured for a hostname is EXPECTED behavior
- The proxy is designed to pass through unconfigured requests
- Therefore: No logging needed, just pass through

**Why return error then?**
- API consistency with other registries
- Standard Go idiom (`value, error` not `value, bool`)
- More extensible (could add error types later)
- Clearer semantics in caller code

## Edge Cases

**None identified** - This is a straightforward signature change:
- No concurrency issues (already thread-safe with RWMutex)
- No nil pointer issues (callers already check before using svc)
- No semantic changes (still just found/not-found)
- No performance impact (same lookup logic)

## Rollback Strategy

Each commit is atomic and can be reverted independently:

1. **Commit 1** (interface + implementation): Reverts all changes if needed
2. **Commit 2-4** (callers): Can revert individual files if issues arise

Tests guard against semantic changes at each step.

## Why This Matters

**Architectural consistency:**
- All registries follow same pattern
- Predictable, idiomatic Go code
- Easier to reason about and maintain

**Future extensibility:**
- Could distinguish error types (NotFound vs LookupFailed)
- Could add metadata to errors
- Standard pattern for error wrapping

**Developer experience:**
- Familiar pattern for Go developers
- Clear semantics: `err != nil` means "didn't find it"
- Consistent with standard library practices
