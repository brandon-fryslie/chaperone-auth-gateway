# Sprint: split-handlers - Split handlers.go by Concern
Generated: 2026-01-18
Confidence: HIGH
Status: READY FOR IMPLEMENTATION

## Sprint Goal
Refactor `internal/proxy/handlers.go` (658 lines) into 4-5 focused files (~100-200 lines each) for improved maintainability and readability.

## Scope
**Deliverables:**
- Split handlers into focused single-responsibility files
- Preserve all existing behavior and tests
- No API changes (internal package, function signatures unchanged)

## Work Items

### P0: Extract Connect Handler
**Acceptance Criteria:**
- [ ] Create `internal/proxy/connect_handler.go`
- [ ] Move `connectHandler()` function
- [ ] Move MITM decision logic
- [ ] File ~100 lines
- [ ] Tests pass

**Functions to Move:**
- `connectHandler()` - lines ~71-85

### P0: Extract Policy & Drop Handlers
**Acceptance Criteria:**
- [ ] Create `internal/proxy/policy_handler.go`
- [ ] Move `policyHandler()` function
- [ ] Move `dropHandler()` function
- [ ] Move policy enforcement logic
- [ ] File ~150 lines
- [ ] Tests pass

**Functions to Move:**
- `policyHandler()` - lines ~88-176
- `dropHandler()` - lines ~178-230

### P0: Extract Auth Handlers
**Acceptance Criteria:**
- [ ] Create `internal/proxy/auth_handler.go`
- [ ] Move `securityStripAuthHandler()` function
- [ ] Move `authHandler()` function
- [ ] Move placeholder matching logic
- [ ] File ~200 lines
- [ ] Tests pass

**Functions to Move:**
- `securityStripAuthHandler()` - lines ~237-360
- `authHandler()` - lines ~363-540

### P0: Extract Recording Handlers
**Acceptance Criteria:**
- [ ] Create `internal/proxy/recording_handler.go`
- [ ] Move `recordRequestHandler()` function
- [ ] Move `recordResponseHandler()` function
- [ ] Move HAR recording logic
- [ ] File ~100 lines
- [ ] Tests pass

**Functions to Move:**
- `recordRequestHandler()` - lines ~544-575
- `recordResponseHandler()` - lines ~578-620

### P0: Extract Utility Functions
**Acceptance Criteria:**
- [ ] Create `internal/proxy/util.go`
- [ ] Move `requestIDMiddleware()` function
- [ ] Move `extractClientIP()` function
- [ ] Move helper functions
- [ ] File ~50-80 lines
- [ ] Tests pass

**Functions to Move:**
- `requestIDMiddleware()` - lines ~622-640
- `extractClientIP()` - lines ~643-658
- `headerContainsPlaceholder()` - if exists

### P1: Update Original handlers.go
**Acceptance Criteria:**
- [ ] Keep only imports and possibly re-exports if needed
- [ ] File < 50 lines (essentially just package doc)
- [ ] All tests pass

**Technical Notes:**
- Internal package, so no backward compatibility concerns
- All handler functions are called from `server.go`
- Keep all function signatures identical
- Add package comment explaining the split

## Dependencies
- Sprint: commit-wip (commit existing changes first)

## Risks
| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Test failures after split | LOW | MEDIUM | Run tests after each file move |
| Import cycles | LOW | HIGH | All handlers in same package, no cycle risk |
| Missing function references | LOW | MEDIUM | `go build ./...` will catch |

## Verification
After each file:
1. `go build ./...`
2. `go test ./internal/proxy/...`

After all files:
1. `go test ./...`
2. `go test -race ./...`
3. Manual smoke test: `chaperone run`
