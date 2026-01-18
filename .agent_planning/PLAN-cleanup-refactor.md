# Release Cleanup & Refactoring Plan

## Overview

This plan addresses 26 identified issues across 7 categories to prepare Chaperone Auth Gateway for release. The work is organized into 4 sprints with clear dependencies and verification criteria.

---

## Sprint 1: Foundation Cleanup (Critical Path)

**Goal**: Remove dead code, consolidate constants, fix immediate issues

### 1.1 Remove Dead Code and Unused Functions
**Confidence**: HIGH | **Risk**: LOW

**Files to modify**:
- `internal/secrets/registry.go` - Remove unused `indexOf()` helper (lines 129-136), use `strings.Index()`
- `internal/secrets/registry.go` - Evaluate `Get()` method (lines 139-143) - appears unused, `Fetch()` is canonical
- `internal/auth/registry.go` - Evaluate `Has()` method (lines 44-51) - check usage
- `internal/examine/logger.go` - Remove unused `io.Writer` parameter from `NewLogger()` (line 92)

**Verification**:
- `go build ./...` passes
- `go test ./...` passes
- No grep matches for removed function names

### 1.2 Consolidate Magic Numbers into Constants
**Confidence**: HIGH | **Risk**: LOW

**Create** `internal/defaults/defaults.go`:
```go
package defaults

const (
    DefaultMaxBodyBytes     = 10 * 1024 * 1024  // 10MB
    DefaultExamineBodyBytes = 4096               // 4KB for examine mode
    DefaultVersion          = "0.1.0"            // Version string
)
```

**Files to update**:
- `internal/service/types.go:78` - Use `defaults.DefaultMaxBodyBytes`
- `internal/examine/logger.go:95` - Use `defaults.DefaultExamineBodyBytes`
- `cmd/chaperone/cmd/examine.go:299` - Use `defaults.DefaultExamineBodyBytes`
- `cmd/chaperone/cmd/root.go:22` - Use `defaults.DefaultVersion`

**Verification**: Grep for hardcoded values shows only constants package

### 1.3 Fix Error Message Quality
**Confidence**: HIGH | **Risk**: LOW

**Files to modify**:
- `internal/proxy/handlers.go:438` - Change `svc.HostPattern` to `svc.Name`
- `internal/proxy/handlers.go:119,142,165` - Remove redundant error info in messages

**Verification**: Manual review of log output format

---

## Sprint 2: Registry API Consolidation

**Goal**: Consistent lookup patterns across all registries

### 2.1 Define Canonical Registry Patterns
**Confidence**: HIGH | **Risk**: MEDIUM

**Decision**: All registries will use `Lookup(key) (value, error)` pattern:
- Returns error when not found (explicit, testable)
- Matches Go conventions

**Files to modify**:

**A. Secrets Registry** (`internal/secrets/registry.go`):
- Keep `Fetch()` as primary (it's the best API already)
- Remove `Get()` if confirmed unused
- Keep `HasProvider()` for validation use cases

**B. Auth Registry** (`internal/auth/registry.go`):
- Rename `Get()` → `Lookup()` for consistency (or keep, but document)
- Keep `Has()` if used for validation

**C. Service Registry** (`internal/service/registry.go`):
- Current `Lookup() (*Service, bool)` → change to `Lookup() (*Service, error)`
- Provides better error context

### 2.2 Update All Callers
**Confidence**: HIGH | **Risk**: MEDIUM

Update all handlers and tests that call registry methods to use new signatures.

**Verification**:
- All tests pass
- Consistent error handling patterns in handlers

---

## Sprint 3: Handler Refactoring (Major Effort)

**Goal**: Extract cross-cutting concerns, reduce handlers.go size

### 3.1 Extract Audit Logging Middleware
**Confidence**: MEDIUM | **Risk**: MEDIUM

**Current problem**: Every handler has this pattern:
```go
if auditLogger != nil {
    auditLogger.Log(audit.Entry{...})
}
```

**Solution**: Create audit middleware wrapper

**Create** `internal/proxy/audit_middleware.go`:
```go
package proxy

type AuditableHandler interface {
    Handle(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response)
    AuditEvent() audit.EventType
}

func WithAudit(logger *audit.Logger, handler AuditableHandler) HandlerFunc {
    return func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
        req, resp := handler.Handle(r, ctx)
        if resp != nil && logger != nil {
            // Handler returned error response, log it
            logger.Log(...)
        }
        return req, resp
    }
}
```

**Alternative (simpler)**: Create `audit.Logger` interface with no-op implementation
- This allows `auditLogger.Log()` calls without nil checks
- Less structural change

### 3.2 Create Audit Logger Interface
**Confidence**: HIGH | **Risk**: LOW

**Create** `internal/audit/interface.go`:
```go
package audit

type AuditLogger interface {
    Log(entry Entry) error
}

type noopLogger struct{}
func (n *noopLogger) Log(Entry) error { return nil }

func Noop() AuditLogger { return &noopLogger{} }
```

**Update** `internal/proxy/handlers.go`:
- Change all `*audit.Logger` parameters to `audit.AuditLogger`
- Remove nil checks

### 3.3 Extract Header Utilities
**Confidence**: HIGH | **Risk**: LOW

**Move to** `internal/auth/headers_util.go`:
- `extractAuthHeader()` from handlers.go
- `headerContainsPlaceholder()` from handlers.go
- `extractClientIP()` could move to `internal/util/` or `internal/proxy/util.go`

**Verification**: handlers.go size reduced, all tests pass

### 3.4 Split handlers.go by Concern
**Confidence**: MEDIUM | **Risk**: MEDIUM

Current 658 lines → target 4 focused files:

1. **`internal/proxy/connect_handler.go`** (~100 lines)
   - `connectHandler()`
   - MITM decision logic

2. **`internal/proxy/policy_handler.go`** (~150 lines)
   - `policyHandler()`
   - `dropHandler()`
   - Policy enforcement

3. **`internal/proxy/auth_handler.go`** (~200 lines)
   - `authHandler()`
   - `securityStripAuthHandler()`
   - Placeholder matching

4. **`internal/proxy/recording_handler.go`** (~100 lines)
   - `recordRequestHandler()`
   - `recordResponseHandler()`
   - HAR recording

5. **`internal/proxy/util.go`** (~50 lines)
   - `requestIDMiddleware()`
   - `extractClientIP()`
   - Request ID helpers

**Keep** `handlers.go` as re-export file:
```go
package proxy

// Re-export for backward compatibility
// All handlers are now in focused files:
// - connect_handler.go
// - policy_handler.go
// - auth_handler.go
// - recording_handler.go
```

---

## Sprint 4: Examine Mode & Validation

**Goal**: Clean up examine mode, improve configuration validation

### 4.1 Split Examine Logger
**Confidence**: MEDIUM | **Risk**: LOW

Current `internal/examine/logger.go` (504 lines) → 3 files:

1. **`internal/examine/logger.go`** (~150 lines)
   - Core logging functions
   - `LogRequest()`, `LogResponse()`

2. **`internal/examine/tracker.go`** (~100 lines)
   - Discovery tracking
   - `Track()`, `GetDiscoveries()`

3. **`internal/examine/report.go`** (~150 lines)
   - Summary generation
   - `PrintSummaryReport()`
   - Configuration suggestions

### 4.2 Improve Service Validation
**Confidence**: HIGH | **Risk**: LOW

**Enhance** `internal/service/types.go:Validate()`:

```go
func (s *Service) Validate() error {
    if s.HostPattern == "" {
        return errors.New("host_pattern required")
    }
    if s.AuthStrategyRef == "" {
        return errors.New("auth_strategy required")
    }
    if s.CredentialRef == "" {
        return errors.New("credential_ref required")
    }

    // NEW: Validate credential ref format
    if !strings.Contains(s.CredentialRef, ":") {
        return fmt.Errorf("credential_ref must be provider:path format, got %q", s.CredentialRef)
    }

    // NEW: Validate auth strategy format
    if strings.HasPrefix(s.AuthStrategyRef, "header:") {
        headerName := strings.TrimPrefix(s.AuthStrategyRef, "header:")
        if headerName == "" {
            return errors.New("header strategy requires header name (header:X-API-Key)")
        }
    } else if s.AuthStrategyRef != "bearer" {
        return fmt.Errorf("unknown auth strategy %q (expected 'bearer' or 'header:HeaderName')", s.AuthStrategyRef)
    }

    return nil
}
```

### 4.3 Document Placeholder Authentication
**Confidence**: HIGH | **Risk**: LOW

**Add** documentation in code and README:
- When placeholder is required vs optional
- Format conventions (recommend `chap_<service>_<random>`)
- Security implications of missing placeholder

---

## Deferred / Out of Scope

These items were identified but deferred:

1. **Logging approach consolidation** - Different packages use different patterns. Would require extensive changes. Document as tech debt.

2. **Test coverage for handlers.go** - Integration tests exist, unit tests limited. Add in separate testing sprint.

3. **CA path resolution duplication** - Low impact, leave as-is.

4. **Version from build system** - Requires build tooling changes, defer to CI/CD work.

---

## Execution Order & Dependencies

```
Sprint 1 (Foundation) - No dependencies
    ↓
Sprint 2 (Registry APIs) - Depends on Sprint 1 completing
    ↓
Sprint 3 (Handler Refactoring) - Depends on Sprint 2 for registry APIs
    ↓
Sprint 4 (Examine & Validation) - Can run parallel to Sprint 3
```

**Suggested Execution**:
1. Sprint 1 first (quick wins, no risk)
2. Sprint 2 next (API changes before structural changes)
3. Sprint 3 & 4 can overlap

---

## Verification Criteria

After each sprint:
- [ ] `go build ./...` passes
- [ ] `go test ./...` passes
- [ ] `go vet ./...` passes
- [ ] `golint ./...` has no new warnings
- [ ] Manual smoke test of `chaperone run` and `chaperone examine`

After all sprints:
- [ ] Code coverage unchanged or improved
- [ ] handlers.go < 200 lines (down from 658)
- [ ] examine/logger.go < 200 lines (down from 504)
- [ ] No unused exports in registries
- [ ] All magic numbers in constants package

---

## Risk Mitigation

1. **Make changes incrementally** - One file at a time, test after each
2. **Preserve backward compatibility** - Keep old function signatures if needed, mark deprecated
3. **Run integration tests after each change** - Catch regressions early
4. **Git commit after each successful step** - Easy rollback

---

## Estimated Scope

| Sprint | Files Modified | New Files | Lines Changed (est) |
|--------|---------------|-----------|---------------------|
| 1 | 5 | 1 | ~50 |
| 2 | 4 | 0 | ~100 |
| 3 | 1 → 6 | 5 | ~400 (refactor) |
| 4 | 1 → 3 | 2 | ~200 (refactor) |

**Total**: ~750 lines of changes, net reduction of ~200 lines
