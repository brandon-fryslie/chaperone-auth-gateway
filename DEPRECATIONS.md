# Chaperone Deprecation Schedule

This document tracks deprecated code, legacy patterns, and planned removal dates.

**Last Updated**: 2026-01-11
**Next Review**: 2026-04-11 (quarterly)

---

## Priority Levels

| Priority | Meaning | Target Action |
|----------|---------|---------------|
| P0 | Critical - removing ASAP | Remove in next minor release |
| P1 | High - scheduled removal | Remove within 2 releases |
| P2 | Medium - planned removal | Remove within 3-6 months |
| P3 | Low - legacy, keep for now | Review quarterly |

---

## Active Deprecations

### 1. `chaperone run` Command (P1)

**Location**: `cmd/chaperone/cmd/run.go`

**Status**: DEPRECATED - Marked with Cobra `Deprecated` flag

**Replacement**: `chaperone inject`

**Reason**: The command was renamed from `run` to `inject` to better describe what it does (credential injection). The `run` command remains as an alias for backward compatibility.

**Code Duplication**: `run.go` duplicates ~95% of `inject.go`. The only meaningful differences:
- `inject` accepts an optional service name argument to filter services
- `inject` has better help text and examples
- `run` shows deprecation warning when invoked

**Impact of Removal**:
- Users running `chaperone run` will get "unknown command" error
- Scripts using `run` will break

**Removal Plan**:
1. Document in CHANGELOG that `run` is deprecated (done - via Cobra `Deprecated` field)
2. Monitor usage (if metrics exist)
3. Remove `run.go` entirely

**Target Removal**: v1.0.0 or 2026-04-01, whichever is later

**Migration**: Change `chaperone run` to `chaperone inject` in all scripts and documentation.

---

### 2. Legacy Audit Types (P2)

**Location**: `internal/audit/logger.go:89-118`

**Status**: UNUSED - Defined but never used

**Affected Types**:
- `RequestLog` struct (lines 92-105)
- `PolicyResult` struct (lines 108-111)
- `AuditLogger` interface (lines 113-118)

**Reason**: These types were defined for a "future use" audit system that differs from the current implementation. The current audit system uses `Entry` and `Logger` types for credential injection logging. These legacy types were likely intended for broader request logging with rate limiting and policy enforcement metrics.

**Impact of Removal**:
- None - types are completely unused
- No external consumers

**Removal Plan**:
1. Verify no tests depend on these types
2. Remove the types
3. Remove the "Legacy types for compatibility (future use)" comment

**Target Removal**: v0.9.0 or 2026-02-15, whichever is later

---

### 3. Unused `io.Writer` Parameter in Examine Logger (P2)

**Location**: `internal/examine/logger.go:33`

**Status**: VESTIGIAL - Parameter accepted but ignored

**Code**:
```go
// NewLogger creates a new examine-mode logger.
// The io.Writer parameter is kept for backward compatibility but is no longer used.
func NewLogger(_ io.Writer, config Config) *Logger {
```

**Reason**: The examine logger was refactored to use structured logging instead of direct io.Writer output. The parameter was kept for API compatibility but is now a dead parameter.

**Impact of Removal**:
- Breaking change for any callers passing a writer
- Search shows no external packages depend on this

**Internal Callers**:
- `internal/proxy/server.go` - creates examine logger

**Removal Plan**:
1. Update `NewLogger` signature to remove unused parameter
2. Update all callers in `internal/proxy/`
3. Update any tests

**Target Removal**: v0.9.0 or 2026-02-15, whichever is later

---

### 4. Template-Based Init (Legacy) (P3)

**Location**: `cmd/chaperone/cmd/init.go:27-70`

**Status**: LEGACY - Works but superseded by wizard

**Affected Code**:
- `supportedServices` map (lines 29-70)
- `runTemplateInit()` function (lines 114-134)

**Reason**: The `chaperone init openai` and `chaperone init anthropic` template modes were the original init implementation. The interactive wizard (`chaperone init` without arguments) is now the recommended approach as it:
- Detects actual service configuration from traffic
- Stores credentials securely (keychain/file/env)
- Generates more complete configuration

**Current Behavior**: Both modes coexist:
- `chaperone init` → wizard mode (recommended)
- `chaperone init <service>` → template mode (legacy)

**Impact of Removal**:
- Users expecting quick template generation would need to use wizard
- Documented in README

**Removal Plan**:
1. Keep for now - provides value for quick setup
2. Consider converting to a separate `chaperone template` command if needed
3. Review at v1.0.0 for consolidation

**Target Removal**: Review at v1.0.0 (no fixed date - low priority)

---

### 5. Backward Compatibility Nil Checks in MITMOptions (P3)

**Location**: `internal/proxy/server.go:82-87`

**Status**: DEFENSIVE - Allows nil registries for testing

**Code**:
```go
type MITMOptions struct {
    // SecretRegistry provides secret fetching capabilities.
    // If nil, authentication will be skipped (backward compatibility).
    SecretRegistry *secrets.Registry

    // AuthRegistry provides authentication strategy implementations.
    // If nil, authentication will be skipped (backward compatibility).
    AuthRegistry *auth.Registry
}
```

**Reason**: These nil checks allow tests to create MITM proxies without fully setting up auth. This is valid for testing MITM/TLS functionality in isolation.

**Impact of Removal**:
- Would require all MITM proxy tests to set up full auth stack
- Minor - mostly affects test ergonomics

**Removal Plan**: Keep - this is intentional design for testability, not technical debt.

**Target Removal**: None planned - intentional design

---

### 6. Placeholder Warning for Backward Compatibility (P3)

**Location**: `internal/proxy/handlers.go:338-347`

**Status**: INTENTIONAL - Security warning

**Code**:
```go
} else {
    // No placeholder configured - warn once per service (backward compat)
    warnMutex.Lock()
    if !warnedServices[svc.Name] {
        log.Warn(reqCtx, "no placeholder configured...")
        warnedServices[svc.Name] = true
    }
    warnMutex.Unlock()
}
```

**Reason**: When placeholder authentication was added, existing configs without placeholders needed to keep working. The warning educates users about the security improvement while maintaining backward compatibility.

**Impact of Making Mandatory**:
- Breaking change for all existing configs without placeholders
- Would need migration period

**Removal Plan**: Keep warning behavior. Consider making placeholder required in v2.0.0 major version.

**Target Removal**: v2.0.0 (make placeholder required, remove fallback)

---

## Code Duplication

### `run.go` vs `inject.go`

**Severity**: P1 - Should be removed with run command

**Lines**: ~250 duplicated lines

**Resolution**: Remove `run.go` entirely when deprecating `run` command. The `inject.go` version is canonical.

---

## Quarterly Review Checklist

- [ ] Check if `run` command is still being used
- [ ] Verify no new code uses legacy audit types
- [ ] Review if template init is still valuable
- [ ] Check if any external packages depend on deprecated APIs

---

## Completed Removals

*None yet - this is the initial deprecation audit*

---

## Version Policy

- **Minor releases** (0.x.0): May remove P0/P1 deprecated code with changelog notice
- **Patch releases** (0.x.y): No removals
- **Major releases** (x.0.0): May remove any deprecated code with migration guide

---

## Notes for Future Maintainers

1. **Before removing deprecated code**: Search for any internal/external usage
2. **After removal**: Update this document's "Completed Removals" section
3. **Quarterly**: Review P2/P3 items for promotion to higher priority

