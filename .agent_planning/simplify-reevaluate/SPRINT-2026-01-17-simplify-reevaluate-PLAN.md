# Sprint: Codebase Simplification Phase 2

**Generated**: 2026-01-17
**Confidence**: HIGH
**Status**: READY FOR IMPLEMENTATION
**Bead**: CHAP-uep

## Sprint Goal

Split handlers.go into focused modules and move mode-specific handlers to their respective packages.

## User Decisions (Resolved)

| Question | Answer |
|----------|--------|
| Scope | Medium (handlers split + cmd cleanup) |
| Organization | Move examine/init handlers to own packages |

## Prerequisites (Completed)

- ✅ CHAP-deh: HAR exposure complete
- ✅ CHAP-kjp: Init wizard simplification complete
- ✅ Orchestrate package created with shared setup
- ✅ knownAuthHeaders consolidated to auth package

## Deliverables

### P0: Split handlers.go (Required)

**Current state**: `internal/proxy/handlers.go` (778 LOC) contains:
- Core pipeline handlers (connect, requestID, policy, drop, strip, auth, record)
- Examine mode handlers (examineConnect, examineRequest, examineResponse)
- Init mode handlers (initConnect, initRequest, initResponse)
- Helper functions (extractClientIP, redactCredential, headerContainsPlaceholder)

**Target state**:
- `internal/proxy/handlers.go` (~500 LOC): Core pipeline handlers only
- `internal/examine/handlers.go` (~80 LOC): Examine mode handlers
- `internal/init/handlers.go` (~100 LOC): Init mode handlers

**Functions to move to internal/examine/handlers.go**:
- `examineConnectHandler()` (lines 665-693)
- `examineRequestHandler()` (lines 695-704)
- `examineResponseHandler()` (lines 706-716)

**Functions to move to internal/init/handlers.go**:
- `initConnectHandler()` (lines 718-744)
- `initRequestHandler()` (lines 746-772)
- `initResponseHandler()` (lines 774-778)

### P1: Update server.go imports (Required)

**Files to modify**:
- `internal/proxy/server.go` - Update NewExamineProxy() and NewInitProxy() to use handlers from respective packages

**Changes**:
```go
// In NewExamineProxy()
proxy.OnRequest().HandleConnectFunc(examine.ConnectHandler(certStore, logger))
proxy.OnRequest().DoFunc(examine.RequestHandler(examineLogger))
proxy.OnResponse().DoFunc(examine.ResponseHandler(examineLogger))

// In NewInitProxy()
proxy.OnRequest().HandleConnectFunc(chaperoneInit.ConnectHandler(certStore, logger))
proxy.OnRequest().DoFunc(chaperoneInit.RequestHandler(detector, evidence, onFinding))
proxy.OnResponse().DoFunc(chaperoneInit.ResponseHandler())
```

### P2: Reduce cmd file sizes (Optional but valuable)

**Goal**: Move remaining orchestration logic to internal packages.

**Target**:
- `cmd/chaperone/cmd/inject.go`: 240 → ~150 LOC
- `cmd/chaperone/cmd/run.go`: 337 → ~200 LOC

**Approach**:
- Move validation logic to orchestrate package
- Move environment building to orchestrate package
- Keep only CLI flag handling and top-level flow in cmd

## Work Items

### Item 1: Extract examine handlers

**Files**:
- `internal/proxy/handlers.go` (modify - remove examine handlers)
- `internal/examine/handlers.go` (create)

**Steps**:
1. Create handlers.go in examine package
2. Move examineConnectHandler, examineRequestHandler, examineResponseHandler
3. Export as ConnectHandler, RequestHandler, ResponseHandler
4. Update imports in proxy/server.go
5. Run tests

**Dependencies needed in examine/handlers.go**:
- `crypto/tls`
- `log/slog`
- `net/http`
- `github.com/bmf/chaperone/internal/log`
- `github.com/elazarl/goproxy`

### Item 2: Extract init handlers

**Files**:
- `internal/proxy/handlers.go` (modify - remove init handlers)
- `internal/init/handlers.go` (create)

**Steps**:
1. Create handlers.go in init package (already has other files)
2. Move initConnectHandler, initRequestHandler, initResponseHandler
3. Export as ConnectHandler, RequestHandler, ResponseHandler
4. Update imports in proxy/server.go
5. Run tests

**Note**: init package already exists with wizard, detector, etc. Handlers fit naturally here.

### Item 3: Clean up proxy/handlers.go

**After extraction**:
- Remove mode-specific handlers
- Keep only core pipeline handlers
- Verify clean compilation

**Expected result**: handlers.go ~500 LOC with clear single responsibility

### Item 4: Update proxy/server.go (if needed)

**Changes**:
- Import examine and init packages for handlers
- Update NewExamineProxy() to use examine.ConnectHandler, etc.
- Update NewInitProxy() to use init.ConnectHandler, etc.

## Dependencies

- None (self-contained refactoring)

## Out of Scope

- Pipeline type abstraction (deferred - not essential)
- Security invariant structural enforcement (deferred - complex)
- Further cmd file reduction beyond P2

## Risks

| Risk | Mitigation |
|------|------------|
| Import cycles | examine/init have no deps on proxy; handlers only need log, goproxy |
| Breaking changes | All handlers keep same signature, just move location |
| Test breakage | Run full test suite after each extraction |

## Success Metrics

| Metric | Before | After |
|--------|--------|-------|
| handlers.go LOC | 778 | ~500 |
| examine/handlers.go | 0 | ~80 |
| init/handlers.go | 0 | ~100 |
| inject.go LOC | 240 | ~150 (if P2 done) |
| run.go LOC | 337 | ~200 (if P2 done) |

## Verification

- `go build ./...` passes
- `go test ./...` passes
- `chaperone inject` works
- `chaperone run` works
- `chaperone examine` works
- `chaperone init` works
