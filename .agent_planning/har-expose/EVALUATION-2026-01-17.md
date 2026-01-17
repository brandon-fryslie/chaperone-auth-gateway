# Evaluation: HAR Recording Exposure

**Timestamp**: 2026-01-17
**Bead**: CHAP-deh

## Current Implementation

**Location**: `internal/recorder/har.go` (300 LOC)

**What's Captured**:
- Full HAR 1.2 format with request/response pairs
- Request: method, URL, headers, cookies, query strings, POST data
- Response: status, headers, content with MIME type
- Timing: overall duration, send/wait/receive breakdown
- Thread-safe via mutex
- Body size limits (100KB) to prevent massive files

**Current Usage**:
- `NewWithMITM()` creates recorder (server.go:103)
- `recordRequestHandler()` captures request start (handlers.go:180)
- `recordResponseHandler()` records response (handlers.go:183)
- `GetRecorder()` exposes for testing only

**Problem**: HAR is captured but never exposed to users.

## Exposure Options

| Option | Approach | Effort | Viability |
|--------|----------|--------|-----------|
| A | Add `--har-output` flag to `inject` | 2hr | ✅ VIABLE |
| B | New `record` command | 4hr | ⚠️ MEDIUM |
| C | Integrate with `examine` mode | 2hr | ✅ HIGHEST |
| D | Auto-save per run session | 6hr | ⚠️ COMPLEX |
| E | REST API endpoint | - | ❌ NOT ALIGNED |

## Recommended Approach: C (examine mode integration)

**Rationale**:
- Examine mode is for discovery/debugging (natural fit)
- Already has file output infrastructure (`-o` flag)
- Most users discovering auth will use examine mode
- Low implementation complexity

**Implementation**:
1. Add `--har` flag to examine command
2. Attach recorder to `NewExamineProxy()`
3. On shutdown, write HAR if flag set
4. Update help text and examples

## Simplification Opportunities

1. **Complete WriteToFile()** - Currently returns nil without writing
2. **Add SaveToPath() helper** - Reduce boilerplate
3. **Add filtering capability** - Filter by host/path before export (future)

## Unknowns Needing User Input

1. Primary use case: debugging, analysis, compliance?
2. File output location: current dir, temp, configurable?
3. Filtering before export needed?
4. Should HAR be in inject/run modes too?
5. Size/rotation management needed?

## Verification Criteria

- [ ] Flag added to examine command
- [ ] Valid HAR JSON generated
- [ ] Correct permissions (0600)
- [ ] All request/response pairs captured
- [ ] Help text explains usage
- [ ] README mentions HAR availability

## Verdict

**CONTINUE** - Low complexity, high value, aligns with existing patterns.
