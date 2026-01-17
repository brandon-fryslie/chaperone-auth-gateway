# Sprint: Init Wizard Simplification

**Generated**: 2026-01-17
**Confidence**: HIGH
**Status**: READY FOR IMPLEMENTATION
**Bead**: CHAP-kjp

## Sprint Goal

Simplify the init wizard while preserving its essential value: discovering real-world API auth patterns that deviate from documentation.

## User Decisions (Resolved)

| Question | Answer |
|----------|--------|
| Scope | P0 + P1 + P2 (full) |
| Multi-service batch | Not needed (single service is fine) |
| --yes flag usage | Never use it (but good to have sensible defaults) |

## Deliverables

### P0: Extract UI from wizard.go (Required)

**Goal**: Separate prompts/display from coordination logic for testability.

**Current state**: wizard.go (512 LOC) mixes:
- Interactive prompts (reader.ReadString, fmt.Fprintf)
- Step coordination logic
- File I/O operations

**Target state**:
- `wizard.go` (~250 LOC): Step coordination, flow control
- `wizard_ui.go` (~250 LOC): All prompt functions, display helpers

**Methods to extract to wizard_ui.go**:
- `PrintIntroduction()` - Welcome message
- `promptAddress()`, `promptPort()` - Proxy config prompts
- `promptSentinel()` - Sentinel value prompt
- `printDetectionInstructions()` - Detection phase display
- `printServiceSummary()` - Finding summary
- `promptServiceSelection()` - Multi-finding choice
- `promptCredentialStorage()` - Storage type selection
- `promptCredentialValue()` - Credential input
- `promptConfigPath()` - Save location
- `printNextSteps()` - Post-save instructions

### P1: Add Non-Interactive Default (Required)

**Goal**: When `--yes` flag used, default to file storage.

**Current behavior**:
- `--yes` skips prompts but fails if storage type not specified
- No default credential storage

**Target behavior**:
- `--yes` uses `file:~/.config/chaperone/secrets/<service>` by default
- Clear message shows where credential stored
- Works without user interaction

**Implementation**:
```go
// In wizard.go Step4ConfigureService
if w.config.NonInteractive {
    // Default to file storage for non-interactive
    storageType = "file"
    storagePath = filepath.Join(configDir, "secrets", sanitizedHost)
}
```

### P2: Consolidate knownAuthHeaders (Required)

**Goal**: Single source of truth for auth header detection.

**Current state**:
- `internal/proxy/handlers.go:240-253`: `var knownAuthHeaders = []string{...}`
- `internal/init/heuristics.go:10-23`: `var knownAuthHeaders = map[string]bool{...}`
- Same headers, different data structures, potential drift

**Target state**:
- `internal/auth/known_headers.go`: Single canonical list
- Both handlers.go and heuristics.go import from auth package
- Consistent data structure (slice or map with helper functions)

**Implementation**:
```go
// internal/auth/known_headers.go
package auth

// KnownAuthHeaders is the canonical list of headers commonly used for authentication.
// These are automatically stripped from requests for security (handlers.go)
// and used for detection heuristics (init/heuristics.go).
var KnownAuthHeaders = []string{
    "authorization",
    "x-api-key",
    "x-auth-token",
    "api-key",
    "bearer",
    // ... rest of list
}

// IsKnownAuthHeader returns true if the header is a known auth header (case-insensitive).
func IsKnownAuthHeader(header string) bool {
    headerLower := strings.ToLower(header)
    for _, known := range KnownAuthHeaders {
        if headerLower == known {
            return true
        }
    }
    return false
}
```

## Work Items

### Item 1: Extract UI methods to wizard_ui.go

**Files**:
- `internal/init/wizard.go` (modify)
- `internal/init/wizard_ui.go` (create)

**Steps**:
1. Create wizard_ui.go with package declaration
2. Move prompt functions one at a time
3. Keep coordination logic in wizard.go
4. Run tests after each move

**Acceptance**:
- [ ] wizard.go reduced to ~250 LOC
- [ ] wizard_ui.go contains all prompt/display functions
- [ ] All existing tests pass
- [ ] No functional changes

### Item 2: Add non-interactive file storage default

**Files**:
- `internal/init/wizard.go` (modify)
- `internal/init/credential_writer.go` (verify)

**Steps**:
1. Add default path logic in Step4ConfigureService
2. Ensure file storage creates parent directories
3. Print message about where credential stored

**Acceptance**:
- [ ] `chaperone init --yes` uses file storage by default
- [ ] Credential saved to `~/.config/chaperone/secrets/<service>`
- [ ] Clear message shows storage location

### Item 3: Create shared auth headers

**Files**:
- `internal/auth/known_headers.go` (create)
- `internal/proxy/handlers.go` (modify - import)
- `internal/init/heuristics.go` (modify - import)

**Steps**:
1. Create known_headers.go with KnownAuthHeaders slice
2. Add IsKnownAuthHeader helper function
3. Update handlers.go to import and use auth.KnownAuthHeaders
4. Update heuristics.go to use auth.IsKnownAuthHeader
5. Remove duplicated definitions

**Acceptance**:
- [ ] Single definition of known auth headers
- [ ] Both packages import from auth
- [ ] Tests pass

## Dependencies

- None (self-contained)

## Out of Scope

- Multi-service batch configuration (not needed per user)
- Policy review step (future work)
- New credential storage backends (e.g., Vault, 1Password)
- Changes to detection heuristics
- UI/UX redesign beyond extraction

## Risks

| Risk | Mitigation |
|------|------------|
| Extraction breaks tests | Move one function at a time, test after each |
| Import cycles | auth package has no dependencies on proxy or init |
| Non-interactive edge cases | Test with various service types |

## Success Metrics

- wizard.go: 512 → ~250 LOC (50% reduction)
- knownAuthHeaders: 2 definitions → 1 definition
- All tests pass
- `chaperone init --yes` works end-to-end
