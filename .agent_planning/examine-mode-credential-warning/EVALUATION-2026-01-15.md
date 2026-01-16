# Evaluation: Examine Mode Credential Warning

**Date**: 2026-01-15
**Topic**: examine-mode-credential-warning
**Status**: READY TO IMPLEMENT

## 1. Current Examine Mode Implementation

**How it works:**
- Entry point: `cmd/chaperone/cmd/examine.go`
- Runs a MITM proxy with **no filtering** - MITMs ALL connections
- Creates `examine.Logger` that logs requests/responses without modifying them
- Handlers in `internal/proxy/handlers.go`:
  - `examineConnectHandler()`: Always MITMs all connections
  - `examineRequestHandler()`: Logs request via `examineLogger.LogRequest(r)`
  - `examineResponseHandler()`: Logs response via `examineLogger.LogResponse(resp)`

**Key insight:** Examine mode ALREADY logs auth headers! In `logger.go` lines 54-65:
```go
// Collect auth-relevant headers
var authHeaders []string
for name, values := range r.Header {
    if IsAuthRelevant(name) {
        for _, v := range values {
            authHeaders = append(authHeaders, name+": "+v)
        }
    }
}
if len(authHeaders) > 0 {
    args = append(args, "auth_headers", authHeaders)
}
```

## 2. Auth Header Detection System

**Existing filtering logic** in `internal/examine/headers.go`:
- `IsAuthRelevant(name)`: Returns `true` if header could contain auth
- Uses exclusion list `noAuthHeaderPatterns`: ~40 headers that are non-auth

**Known auth headers** in `internal/proxy/handlers.go`:
- authorization, x-api-key, x-auth-token, api-key, apikey
- x-access-token, x-token, token, bearer
- x-session-token, x-csrf-token, x-xsrf-token

## 3. Where Warnings Should Appear

**Current situation:**
- Examine mode logs auth headers but NO explicit WARNING
- Users may not realize real credentials are flowing through

**Recommendation:**
1. **Per-request warning**: When auth headers detected, emit WARN log
2. **Startup message**: Add security disclaimer at CLI startup

## 4. Implementation Complexity: VERY SIMPLE

**What needs to change:** 2 files, ~15 lines of code

1. `internal/examine/logger.go` - Add warning when auth headers detected
2. `cmd/chaperone/cmd/examine.go` - Add startup disclaimer

**Why this is simple:**
- Auth header detection already exists via `IsAuthRelevant()`
- Logging infrastructure in place
- No new dependencies
- No architectural changes
- No state tracking needed

## 5. Ambiguities Resolved

| Question | Answer |
|----------|--------|
| What warning message? | "auth credentials detected in request" + host |
| Per-request or aggregated? | Per-request (simple, clear) |
| Flag to suppress warnings? | No - security advisory should not be suppressible |
| Output to stdout or stderr? | Both - startup to stdout, per-request to log (stderr) |

## 6. Quick Win Confirmed

- Estimated effort: **<30 minutes**
- Lines of code: **~15 lines**
- Testing: **Manual verification**
- Risk: **Zero** - purely additive
- Dependencies: **None**
