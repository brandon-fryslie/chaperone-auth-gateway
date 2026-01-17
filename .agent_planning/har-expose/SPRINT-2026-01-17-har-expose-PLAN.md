# Sprint: HAR Recording Exposure

**Generated**: 2026-01-17
**Confidence**: HIGH
**Status**: ✅ COMPLETE
**Bead**: CHAP-deh

## Sprint Goal

Expose HAR recording to users via the examine command, enabling traffic capture for debugging and analysis.

## Scope

### Deliverables

1. **Add `--har` flag to examine command** ✅
   - Boolean flag: `chaperone examine --har`
   - Writes HAR alongside examine logs

2. **Integrate recorder with examine proxy** ✅
   - Attach recorder to `NewExamineProxy()`
   - Currently only used by `NewWithMITM()`

3. **Complete WriteToFile() implementation** ✅
   - Currently returns nil without writing
   - Use os.WriteFile with 0600 permissions

4. **Update documentation** ✅
   - Help text explains HAR format and usage
   - Example showing typical workflow

## Work Items

### P0: Core Integration ✅

**Add recorder to examine mode**
- Modify `NewExamineProxy()` to accept optional recorder
- Create recorder in examine.go when `--har` flag set
- Wire up recordRequestHandler/recordResponseHandler

**Acceptance Criteria:**
- [x] `--har` flag added to examine command
- [x] Recorder created when flag enabled
- [x] Request/response handlers attached to examine proxy

### P1: File Output ✅

**Implement HAR file writing**
- Complete `WriteToFile()` in har.go
- Add shutdown hook to write HAR on exit
- Default filename: `chaperone-<timestamp>.har`

**Acceptance Criteria:**
- [x] HAR written on graceful shutdown
- [x] File has 0600 permissions
- [x] Valid JSON format (HAR 1.2 spec)

### P2: Documentation ✅

**Update help and examples**
- Add to examine --help output
- Add example to README

**Acceptance Criteria:**
- [x] Help text explains --har flag
- [x] Example shows: `chaperone examine --har --har-output traffic.har`

## Dependencies

- None (self-contained)

## Risks

| Risk | Mitigation |
|------|------------|
| Large HAR files | Existing 100KB body limit |
| Partial writes on crash | Document graceful shutdown requirement |

## Implementation Summary

**Completed**: 2026-01-17

**Commits:**
- `e12a1a6` - feat(recorder): implement WriteToFile for HAR archives
- `25d59e5` - feat(proxy): add optional HAR recorder to NewExamineProxy
- `47a6d44` - feat(examine): add HAR recording flags
- `5022496` - docs: document HAR recording capability in examine mode
- `fa3ea1c` - style: align variable declarations in examine.go
- `07058a9` - docs: mark HAR recording implementation complete

**Files Modified:**
- `internal/recorder/har.go` - Implemented WriteToFile
- `internal/proxy/server.go` - Added recorder param to NewExamineProxy
- `cmd/chaperone/cmd/examine.go` - Added --har and --har-output flags
- `README.md` - Documented HAR capability
