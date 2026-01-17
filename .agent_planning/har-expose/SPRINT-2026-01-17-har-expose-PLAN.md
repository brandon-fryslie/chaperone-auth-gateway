# Sprint: HAR Recording Exposure

**Generated**: 2026-01-17
**Confidence**: HIGH
**Status**: READY FOR IMPLEMENTATION
**Bead**: CHAP-deh

## Sprint Goal

Expose HAR recording to users via the examine command, enabling traffic capture for debugging and analysis.

## Scope

### Deliverables

1. **Add `--har` flag to examine command**
   - Boolean flag: `chaperone examine --har`
   - Writes HAR alongside examine logs

2. **Integrate recorder with examine proxy**
   - Attach recorder to `NewExamineProxy()`
   - Currently only used by `NewWithMITM()`

3. **Complete WriteToFile() implementation**
   - Currently returns nil without writing
   - Use os.WriteFile with 0600 permissions

4. **Update documentation**
   - Help text explains HAR format and usage
   - Example showing typical workflow

## Work Items

### P0: Core Integration

**Add recorder to examine mode**
- Modify `NewExamineProxy()` to accept optional recorder
- Create recorder in examine.go when `--har` flag set
- Wire up recordRequestHandler/recordResponseHandler

**Acceptance Criteria:**
- [ ] `--har` flag added to examine command
- [ ] Recorder created when flag enabled
- [ ] Request/response handlers attached to examine proxy

### P1: File Output

**Implement HAR file writing**
- Complete `WriteToFile()` in har.go
- Add shutdown hook to write HAR on exit
- Default filename: `chaperone-<timestamp>.har`

**Acceptance Criteria:**
- [ ] HAR written on graceful shutdown
- [ ] File has 0600 permissions
- [ ] Valid JSON format (HAR 1.2 spec)

### P2: Documentation

**Update help and examples**
- Add to examine --help output
- Add example to README

**Acceptance Criteria:**
- [ ] Help text explains --har flag
- [ ] Example shows: `chaperone examine --har -o results.txt`

## Dependencies

- None (self-contained)

## Risks

| Risk | Mitigation |
|------|------------|
| Large HAR files | Existing 100KB body limit |
| Partial writes on crash | Document graceful shutdown requirement |
