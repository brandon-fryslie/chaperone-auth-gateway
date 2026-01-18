# Sprint: split-examine - Split examine/logger.go by Concern
Generated: 2026-01-18
Confidence: HIGH
Status: READY FOR IMPLEMENTATION

## Sprint Goal
Refactor `internal/examine/logger.go` (503 lines) into 3 focused files (~150 lines each) for improved maintainability.

## Scope
**Deliverables:**
- Split examine logger into focused single-responsibility files
- Preserve all existing behavior and tests
- No API changes (exported functions remain unchanged)

## Work Items

### P0: Extract Core Logger
**Acceptance Criteria:**
- [ ] Keep `internal/examine/logger.go` as core logging file
- [ ] Contains `Logger` struct, `NewLogger()`, `LogRequest()`, `LogResponse()`
- [ ] File ~150 lines
- [ ] Tests pass

**Functions to Keep:**
- `Logger` struct
- `Config` struct
- `NewLogger()`
- `LogRequest()`
- `LogResponse()`

### P0: Extract Discovery Tracker
**Acceptance Criteria:**
- [ ] Create `internal/examine/tracker.go`
- [ ] Move discovery tracking logic
- [ ] Move `Track()` and `GetDiscoveries()` functions
- [ ] File ~100 lines
- [ ] Tests pass

**Functions to Move:**
- Discovery tracking structs
- `Track()` method
- `GetDiscoveries()` method
- Discovery counting logic

### P0: Extract Report Generator
**Acceptance Criteria:**
- [ ] Create `internal/examine/report.go`
- [ ] Move `PrintSummaryReport()` function
- [ ] Move configuration suggestion generation
- [ ] Move formatting helpers
- [ ] File ~150 lines
- [ ] Tests pass

**Functions to Move:**
- `PrintSummaryReport()`
- Summary formatting helpers
- Config suggestion generation

### P1: Consolidate Header Logic
**Acceptance Criteria:**
- [ ] Remove `isStandardAuthHeader()` duplication if present
- [ ] Use `internal/examine/headers.go` as single source
- [ ] All header filtering in one place

**Technical Notes:**
- The exploration identified duplication between `logger.go:337-363` and `headers.go`
- Consolidate into `headers.go`, use from `logger.go`

## Dependencies
- Sprint: commit-wip (commit existing changes first)
- Can run in parallel with Sprint: split-handlers

## Risks
| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Test failures after split | LOW | MEDIUM | Run tests after each file move |
| Export changes | LOW | MEDIUM | Verify all exported symbols preserved |

## Verification
After each file:
1. `go build ./...`
2. `go test ./internal/examine/...`

After all files:
1. `go test ./...`
2. Manual smoke test: `chaperone examine`
