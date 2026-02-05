# Handoff: Remove Legacy Code & Standardize on TCP Auth Model

**Created**: 2026-02-04
**For**: Implementation agent
**Status**: Ready to start

---

## Objective

Clean up chaperone codebase by removing legacy Unix socket mode, init command, and other dead code. Standardize all modes (inject, examine, run) on TCP with per-run Proxy-Authorization Basic auth.

## Current State

### What's Been Done
- ✅ Implemented per-run proxy auth for `run` mode with dual-stack TCP binding
- ✅ Created `internal/run/secret.go` for 256-bit secret generation
- ✅ Created `internal/run/transport.go` for dual-stack loopback binding
- ✅ Created `internal/proxy/auth_gate_connect_handler.go` for CONNECT-level auth
- ✅ Created `internal/proxy/auth_gate_handler.go` for request-level auth
- ✅ Added `ProxySecret` and `PermissiveMode` to `ServerConfig`
- ✅ Added Linux hardening (PR_SET_DUMPABLE) for run mode
- ✅ All tests passing with new auth model

### What's In Progress
- Nothing - ready to start cleanup

### What Remains
- Remove Unix socket support completely
- Enable proxy auth for `inject` mode
- Remove `init` command
- Clean up dead code (socket-related helpers, legacy flags)
- Update documentation
- Remove `--socket` flag, keep `--port` (default to 0 for ephemeral)

## Context & Background

### Why We're Doing This

The codebase currently has two transport modes (Unix socket + TCP) which adds complexity and maintenance burden. The new per-run auth model makes Unix sockets unnecessary - TCP with auth provides better security and simpler code.

**Benefits of cleanup**:
1. **Simpler mental model**: One transport mode (TCP + auth) for all commands
2. **Better security**: Auth enforcement prevents unauthorized local access
3. **Less code**: Remove ~500 lines of socket-specific code
4. **Easier testing**: One code path instead of two

### Key Decisions Made

| Decision | Rationale | Date |
|----------|-----------|------|
| Use TCP + auth instead of Unix sockets | Auth provides better isolation than file permissions, works across all platforms consistently | 2026-02-04 |
| Keep `--port` flag, default to 0 | Port 0 = ephemeral port (OS assigns), prevents conflicts | 2026-02-04 |
| Remove `init` command | Not used in practice, adds complexity | 2026-02-04 |
| Apply auth to inject mode | Consistent security model across all modes | 2026-02-04 |

### Important Constraints

- **MUST maintain backward compatibility for config file format** (except removing socket-related fields)
- **MUST keep all three modes working**: inject, examine, run
- **MUST not break existing tests** (update them, don't delete core functionality tests)
- **MUST preserve --enable-permissive-mode flag** for debugging

## Acceptance Criteria

How we'll know this is complete:

- [ ] All Unix socket code removed (`internal/proxy/server.go`, `internal/run/env.go`, config fields)
- [ ] `--socket` flag removed from all commands
- [ ] `--port` flag defaults to 0 (ephemeral) for inject/examine/run
- [ ] `init` command and all related code removed
- [ ] Inject mode uses per-run proxy auth (like run mode)
- [ ] Examine mode uses per-run proxy auth (like run mode)
- [ ] All existing tests updated and passing
- [ ] No dead imports or unused helper functions
- [ ] CLAUDE.md updated to reflect new architecture
- [ ] README updated to remove socket references

## Scope

### Files to Modify (High Priority)

**Core proxy server**:
- `internal/proxy/server.go` - Remove socket mode, `listener` field, Unix socket Start() logic
- `internal/proxy/url.go` - Remove Unix socket URL generation, dialer logic
- `internal/config/config.go` - Remove `Socket` field from ServerConfig
- `internal/orchestrate/helpers.go` - Remove socket-related logic from transport flags

**Commands**:
- `cmd/chaperone/cmd/inject.go` - Enable per-run auth, use TCP only
- `cmd/chaperone/cmd/examine.go` - Enable per-run auth, use TCP only
- `cmd/chaperone/cmd/run.go` - Remove socket references (already uses TCP)
- `cmd/chaperone/cmd/init.go` - **DELETE THIS FILE**
- `cmd/chaperone/cmd/root.go` - Remove init command registration

**Environment building**:
- `internal/run/env.go` - Remove `SetProxyVars()` (Unix socket version), keep `SetProxyVarsWithAuth()`
- `internal/run/helpers.go` - Remove `BuildChildEnvironment()` (Unix socket version), keep `BuildChildEnvironmentWithAuth()`

**Init-related code**:
- `internal/init/*.go` - **DELETE ENTIRE DIRECTORY**

### Files to Modify (Medium Priority)

**Testing**:
- `test/integration/*_test.go` - Update tests to use TCP URLs instead of Unix socket URLs
- Remove any tests specific to socket mode

**Documentation**:
- `CLAUDE.md` - Update architecture section, remove socket references
- `README.md` - Update examples to use TCP

### Files to Check (Low Priority - may have dead code)

- `internal/run/spawner.go` - Check for socket-specific logic
- `internal/service/*.go` - Check for socket references
- `cmd/chaperone/cmd/check.go` - Check if it references sockets

### Out of Scope

- Changing the MITM/certificate architecture
- Modifying auth strategies (bearer, header)
- Changing secret providers (env, file, keychain)
- Modifying policy enforcement logic
- Changing audit logging format

## Implementation Approach

### Recommended Steps

**Phase 1: Enable auth for inject/examine modes** (Safest first - adds functionality)

1. **Update inject command**:
   - Generate per-run secret at startup
   - Bind dual-stack loopback with port 0
   - Set `cfg.Server.ProxySecret` and `cfg.Server.PermissiveMode`
   - Use `CreateProxyWithListeners()` instead of regular proxy creation
   - Update logging to show port instead of socket

2. **Update examine command**:
   - Same changes as inject
   - Examine mode already has its own proxy creation logic - update that

3. **Test**: Verify inject and examine work with new auth model

**Phase 2: Remove Unix socket code** (Clean up after migration)

4. **Remove from config**:
   - Delete `Socket` field from `ServerConfig`
   - Remove socket validation from `Validate()`
   - Remove socket defaults from `SetDefaults()`
   - Update `ApplyTransportFlags()` to only handle TCP

5. **Remove from proxy server**:
   - Delete Unix socket listener code in `Start()`
   - Delete `GetProxyURL()` Unix socket logic
   - Remove socket cleanup from shutdown
   - Delete `listener` field (only use `preListeners`)

6. **Remove from environment builders**:
   - Delete `SetProxyVars()` method (socket version)
   - Delete `BuildChildEnvironment()` function (socket version)
   - Rename `BuildChildEnvironmentWithAuth()` → `BuildChildEnvironment()`
   - Rename `SetProxyVarsWithAuth()` → `SetProxyVars()`

7. **Test**: Run full test suite, fix any socket-related test failures

**Phase 3: Remove init command** (Independent cleanup)

8. **Delete init command**:
   - Delete `cmd/chaperone/cmd/init.go`
   - Delete `internal/init/` directory
   - Remove init command registration from `cmd/chaperone/cmd/root.go`
   - Search codebase for any imports of `internal/init` and remove them

9. **Test**: Verify `chaperone --help` doesn't show init command

**Phase 4: Clean up dead code** (Final polish)

10. **Remove dead imports/helpers**:
    - Run `go mod tidy`
    - Search for TODO/FIXME comments related to sockets
    - Remove any helper functions only used by socket code
    - Check `internal/orchestrate/helpers.go` for dead socket logic

11. **Update --port flag**:
    - Verify default is 0 (ephemeral) in inject/examine/run
    - Remove hardcoded port 4010 defaults if any remain
    - Update help text to explain port 0 behavior

12. **Final test**: Run full test suite + manual smoke test

**Phase 5: Documentation** (Last step)

13. **Update CLAUDE.md**:
    - Remove Unix socket references from architecture section
    - Update "Recent Features" to remove socket-related items
    - Add note about TCP + auth being the only transport mode

14. **Update README.md**:
    - Remove socket examples
    - Show TCP examples with auth
    - Update security model description

### Patterns to Follow

**For auth enablement in inject/examine**:
```go
// Pattern from run.go - copy this structure
runSecret, err := run.GenerateProxySecret()
dualStackListener, err := run.BindDualStackLoopback(ctx)
cfg.Server.ProxySecret = runSecret
cfg.Server.PermissiveMode = permissiveMode
proxyServer := orchestrate.CreateProxyWithListeners(ctx, cfg, logger, shutdownMgr, result, dualStackListener.Listeners())
```

**For config cleanup**:
- Delete fields completely, don't leave them commented out
- Update SetDefaults() to only set TCP-related defaults
- Remove socket validation completely

**For test updates**:
- Replace `http+unix://...` URLs with `http://127.0.0.1:<port>`
- Add proxy auth to client setup: `http://u:<secret>@127.0.0.1:<port>`
- Use `InsecureSkipVerify: true` for test backends (they use self-signed certs)

### Known Gotchas

- **Test failures**: Existing tests use `proxy.GetProxyURL()` which returns Unix socket URLs - these will break
  - **Fix**: Update tests to manually construct TCP URLs with auth

- **Config migration**: Users with `socket = "..."` in config files will see validation errors
  - **Fix**: This is acceptable - socket mode is being removed. Users can delete the line or use `port = 0`

- **Shutdown cleanup**: Unix socket mode deleted socket files on shutdown - TCP doesn't need this
  - **Fix**: Remove socket cleanup from `shutdownMgr.Register()` calls

- **Environment variable format**: `HTTP_PROXY=http+unix://...` won't work anymore
  - **Fix**: Already fixed - we use `HTTP_PROXY=http://u:<secret>@127.0.0.1:<port>` now

- **Dual-stack binding may fail on IPv6-disabled systems**: ::1 bind fails gracefully
  - **Fix**: Already handled - we log at DEBUG and continue with IPv4 only

## Reference Materials

### Planning Documents
- Implementation is complete for run mode, this handoff covers extending to inject/examine

### Beads Issues
- None currently - this is cleanup work based on recent implementation

### Codebase References
- `cmd/chaperone/cmd/run.go:85-130` - Reference implementation for TCP + auth setup
- `internal/run/secret.go` - Secret generation (reuse for inject/examine)
- `internal/run/transport.go` - Dual-stack binding (reuse for inject/examine)
- `internal/proxy/auth_gate_connect_handler.go` - CONNECT-level auth (already registered globally)
- `internal/orchestrate/helpers.go:131-149` - `CreateProxyWithListeners()` pattern

### Current State Files

**Socket mode currently used in**:
- `cmd/chaperone/cmd/inject.go` - Default socket mode
- `cmd/chaperone/cmd/examine.go` - Default socket mode
- `internal/config/config.go:23` - `Socket` field
- `internal/proxy/server.go:333-390` - Socket listener creation
- `internal/proxy/url.go:25-44` - Socket URL/dialer

**Init command files**:
- `cmd/chaperone/cmd/init.go` - Main command
- `internal/init/detector.go` - Auth detection
- `internal/init/evidence.go` - Evidence gathering
- `internal/init/handlers.go` - Request/response handlers
- All of `internal/init/` directory

## Questions & Blockers

### Open Questions
- [ ] Should we add a deprecation warning for users with `socket =` in config, or just fail validation?
  - **Recommendation**: Fail validation with helpful error message
- [ ] Should --enable-permissive-mode be available in inject/examine, or only run?
  - **Recommendation**: Make it available in all modes for consistency

### Current Blockers
- None - ready to start

### Need User Input On
- Confirm that removing init command is acceptable (it's not documented in CLAUDE.md as a main feature)
- Confirm that breaking socket mode configs is acceptable (clean break vs migration path)

## Testing Strategy

### Existing Tests That Will Break
- `test/integration/auth_integration_test.go` - Uses `proxy.GetProxyURL()` which returns socket URLs
- `test/integration/mitm_integration_test.go` - May use socket mode
- Any test that checks `cfg.Server.Socket` field

### New Tests Needed
- [ ] Inject mode with auth enforcement (similar to `TestRunModeProxyAuth`)
- [ ] Examine mode with auth enforcement (similar to `TestRunModeProxyAuth`)
- [ ] Port 0 behavior test (verify ephemeral port assignment)

### Manual Testing Checklist
- [ ] `chaperone inject` - starts on random port, logs show port number
- [ ] `chaperone examine` - starts on random port, works with traffic
- [ ] `chaperone run openai -- curl https://api.openai.com` - still works
- [ ] `chaperone --help` - init command not shown
- [ ] Config with `socket = "/tmp/foo.sock"` - validation fails with clear error

### Test Update Pattern

**Before** (socket mode):
```go
proxyURL, dialer := proxy.GetProxyURL(cfg)
client := &http.Client{
    Transport: &http.Transport{
        Proxy: http.ProxyURL(proxyURL),
        DialContext: dialer,
        TLSClientConfig: &tls.Config{RootCAs: certPool},
    },
}
```

**After** (TCP + auth):
```go
proxyURL := fmt.Sprintf("http://u:%s@127.0.0.1:%d", runSecret, actualPort)
client := &http.Client{
    Transport: &http.Transport{
        Proxy: http.ProxyURL(mustParseURL(proxyURL)),
        TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
    },
}
```

## Success Metrics

How to validate implementation:

- **Code reduction**: Remove ~500+ lines of socket-specific code
- **All tests pass**: `make test` succeeds
- **Build succeeds**: `make build` succeeds
- **No dead imports**: `go mod tidy` doesn't remove anything
- **Grep checks**:
  - `grep -r "Socket" internal/ cmd/` - should only find comments/docs
  - `grep -r "socket" internal/ cmd/` - should only find comments/docs
  - `grep -r "unix://" internal/ cmd/` - should return nothing
  - `grep -r "internal/init" cmd/ internal/` - should return nothing
- **Help text**: `chaperone --help` shows inject/examine/run/check, NOT init
- **Modes work**:
  - `chaperone inject` - starts proxy on TCP
  - `chaperone examine` - starts proxy on TCP
  - `chaperone run <service> -- <cmd>` - starts proxy on TCP

---

## Next Steps for Agent

**Immediate actions**:
1. Read this handoff document completely
2. Review reference implementations in run.go
3. Start with Phase 1: Enable auth for inject mode (safest first)

**Before starting implementation**:
- [ ] Confirm with user that init command removal is OK
- [ ] Confirm with user that breaking socket configs is OK
- [ ] Check for any open PRs that might conflict

**Implementation order** (follow phases above):
1. ✅ Enable auth for inject/examine (adds functionality, safe)
2. ✅ Remove Unix socket code (cleanup, may break tests)
3. ✅ Remove init command (independent cleanup)
4. ✅ Clean up dead code (final polish)
5. ✅ Update documentation (last step)

**When complete**:
- [ ] Run full test suite: `make test`
- [ ] Run linter: `make lint`
- [ ] Build: `make build`
- [ ] Manual smoke test all three modes
- [ ] Update this handoff with "COMPLETED" status
- [ ] Report results to user

**Verification commands**:
```bash
# Check for socket references (should be minimal/none)
grep -r "Socket\|socket\|unix://" internal/ cmd/ --include="*.go" | grep -v "test" | grep -v "//"

# Check for init references (should be none)
grep -r "internal/init" cmd/ internal/ --include="*.go"

# Verify init command is gone
./chaperone --help | grep init

# Test all modes work
./chaperone inject &
./chaperone examine &
./chaperone run <service> -- echo "test"
```

---

## Code Deletion Checklist

Track what gets deleted:

### Config Fields
- [ ] `ServerConfig.Socket` field - DELETE
- [ ] Socket validation in `Validate()` - DELETE
- [ ] Socket defaults in `SetDefaults()` - DELETE
- [ ] Socket handling in `ApplyTransportFlags()` - DELETE

### Proxy Server
- [ ] Socket listener creation in `Start()` - DELETE
- [ ] Socket cleanup in shutdown - DELETE
- [ ] `GetProxyURL()` Unix socket branch - DELETE
- [ ] Unix socket dialer in url.go - DELETE
- [ ] `listener net.Listener` field (replace with preListeners only) - DELETE

### Environment Builders
- [ ] `SetProxyVars(socketPath, serviceName)` method - DELETE
- [ ] `BuildChildEnvironment(ctx, svc, serviceName, socketPath, caCertPath)` - DELETE
- [ ] `GenerateSocketPath(serviceName, pid)` - DELETE

### Commands
- [ ] `cmd/chaperone/cmd/init.go` - DELETE ENTIRE FILE
- [ ] Init command registration in root.go - DELETE
- [ ] `--socket` flag from inject.go - DELETE
- [ ] `--socket` flag from examine.go - DELETE

### Directories
- [ ] `internal/init/` - DELETE ENTIRE DIRECTORY
  - `detector.go`
  - `evidence.go`
  - `handlers.go`
  - `proxy.go`
  - Any other files in this directory

### Tests
- [ ] Socket-specific test cases - UPDATE OR DELETE
- [ ] Tests using `proxy.GetProxyURL()` - UPDATE
- [ ] Init command tests - DELETE

---

## Migration Notes for Users

Users upgrading to this version will need to:

**If they have `socket = "..."` in config**:
```toml
# BEFORE (won't work anymore)
[server]
socket = "/tmp/chaperone.sock"

# AFTER (use port 0 for ephemeral)
[server]
port = 0  # OS assigns random port
address = "127.0.0.1"
```

**If they use init command**:
- Init command removed - no direct replacement
- Users must manually configure services in chaperone.toml
- Examine mode can help discover auth patterns

**If they have scripts using socket mode**:
```bash
# BEFORE
export HTTP_PROXY=http+unix:///tmp/chaperone.sock

# AFTER (get port from chaperone logs)
export HTTP_PROXY=http://127.0.0.1:<port>
# Or use run mode which handles this automatically
```
