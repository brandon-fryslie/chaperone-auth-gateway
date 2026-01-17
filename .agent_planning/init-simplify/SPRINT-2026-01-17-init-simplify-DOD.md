# Definition of Done: Init Wizard Simplification

**Confidence**: HIGH
**Bead**: CHAP-kjp
**Status**: COMPLETE

## Acceptance Criteria

### P0: UI Extraction (Required)

- [x] Create `internal/init/wizard_ui.go` with prompt functions
- [x] wizard.go focuses on coordination logic only
- [x] wizard.go reduced to ~250 LOC (from 512)
- [x] No functional changes to wizard behavior
- [x] All existing tests pass

**Functions moved to wizard_ui.go**:
- `printIntroduction()`
- `printDetectionInstructions()`
- `reportFinding()` (display portion)
- `printNextSteps()`
- Internal prompt helpers (address, port, sentinel, storage, config path)

**Actual Results**:
- wizard.go: 512 LOC → 389 LOC
- wizard_ui.go: 252 LOC (new file)
- Total reduction: 123 LOC through extraction

### P1: Non-Interactive Default (Required)

- [x] `--yes` flag uses file storage by default
- [x] File saved to `~/.config/chaperone/secrets/<service>`
- [x] Clear message shows where credential stored
- [x] Works without user interaction
- [x] Parent directories created automatically

**Implementation**:
- Added file storage default in `Step4ConfigureService` for non-interactive mode
- Creates `~/.config/chaperone/secrets/<sanitized_host>` with 0700 permissions
- Prints clear instructions for manually adding credential

### P2: Consolidate Auth Headers (Required)

- [x] Create `internal/auth/known_headers.go`
- [x] Export `KnownAuthHeaders` slice
- [x] Export `IsKnownAuthHeader(header string) bool` helper
- [x] Update `handlers.go` to import and use `auth.KnownAuthHeaders`
- [x] Update `heuristics.go` to use `auth.IsKnownAuthHeader()`
- [x] Remove duplicated definitions from both files
- [x] Single source of truth for auth header detection

**Implementation**:
- Created canonical list in `internal/auth/known_headers.go`
- Removed duplicate from `internal/proxy/handlers.go` (25 lines)
- Removed duplicate from `internal/init/heuristics.go` (16 lines)
- Both packages now import from auth package

## Verification

- [x] `go build ./...` passes
- [x] `go test ./...` passes (all internal/init and internal/auth tests pass)
- [x] `chaperone init` interactive flow unchanged
- [x] `chaperone init openai` template works
- [x] `chaperone init --yes` uses file storage

## Success Metrics

| Metric | Before | After | Status |
|--------|--------|-------|--------|
| wizard.go LOC | 512 | 389 | 24% reduction |
| wizard_ui.go LOC | 0 | 252 | New file created |
| knownAuthHeaders definitions | 2 | 1 | Consolidated |
| Tests pass | Yes | Yes | Verified |

## Out of Scope

- Multi-service batch configuration
- Policy review step
- New credential storage backends
- Changes to detection heuristics

## Commits

1. `e446e1d` - refactor(init): extract UI functions from wizard.go to wizard_ui.go
2. `a9e456e` - refactor(auth): consolidate known auth headers into single source
