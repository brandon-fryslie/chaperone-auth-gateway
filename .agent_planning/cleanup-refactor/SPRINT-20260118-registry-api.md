# Sprint: registry-api - Consolidate Registry API Patterns
Generated: 2026-01-18
Confidence: MEDIUM
Status: RESEARCH REQUIRED

## Sprint Goal
Standardize all registry APIs to use consistent `(value, error)` pattern for lookup failures.

## Known Elements
- Secrets Registry already uses `Fetch(ctx, ref) (string, error)` - GOOD
- Auth Registry uses `Get(key) (Strategy, error)` - GOOD
- Service Registry uses `Lookup(key) (*Service, bool)` - INCONSISTENT

## Unknowns to Resolve
1. **Caller impact** - How many callers use `Lookup()` with the bool pattern?
2. **Error messages** - What error context is useful when service not found?

## Tentative Deliverables
- Change `ServiceRegistry.Lookup()` to return `(*Service, error)`
- Update all callers to handle error instead of bool
- Consistent error handling patterns across registries

## Research Tasks
- [ ] Count callers of `ServiceRegistry.Lookup()`
- [ ] Identify all places checking the bool return
- [ ] Determine if any callers rely on bool for non-error flow

## Work Items (Pending Research)

### P0: Update Service Registry Interface
**Acceptance Criteria (tentative):**
- [ ] Change `Lookup(host) (*Service, bool)` to `Lookup(host) (*Service, error)`
- [ ] Return descriptive error when service not found
- [ ] Tests updated to check error instead of bool
- [ ] Tests pass

### P0: Update All Callers
**Acceptance Criteria (tentative):**
- [ ] All callers updated to handle error
- [ ] No bool checks remain for service lookup
- [ ] Error handling is consistent with other registries

## Exit Criteria (to reach HIGH confidence)
- [ ] Caller count documented
- [ ] Error message format decided
- [ ] All callers identified and change approach confirmed

## Dependencies
- Sprint: commit-wip (clean baseline)
- Should run AFTER split-handlers (handlers are main callers)

## Risks
| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Many callers affected | MEDIUM | MEDIUM | Research first |
| Semantic change breaks assumptions | LOW | HIGH | Review each caller |

## Notes
This sprint is LOWER PRIORITY than the file splits. The current API works; this is a consistency improvement. Consider deferring to v0.2 if time-constrained.
