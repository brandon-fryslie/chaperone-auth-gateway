# Handoff: Remaining Security Improvements

**Created**: 2026-01-06
**For**: Agent implementing additional credential protection features
**Status**: ready-to-start

---

## Objective

Implement additional security layers to prevent credential leakage to unintended hosts, beyond the auto-strip feature already implemented. The goal is defense-in-depth: even if users misconfigure their applications, Chaperone should protect credentials.

## Current State

### What's Been Done (This Session)

1. **Auto-Strip Auth Headers** (IMPLEMENTED)
   - All known auth headers stripped from incoming requests automatically
   - Big warning logged when headers are stripped
   - `internal/proxy/handlers.go`: `securityStripAuthHandler()`
   - Headers stripped: `authorization`, `x-api-key`, `x-auth-token`, `api-key`, `apikey`, `x-access-token`, `x-token`, `token`, `bearer`, `x-session-token`, `x-csrf-token`, `x-xsrf-token`

2. **Drop Pattern Feature** (IMPLEMENTED)
   - Block requests matching URL patterns
   - Config: `drop = ["anthropic.com", "datadoghq.com"]`
   - URL-aware globbing: `*.example.com/**/sensitive`
   - `internal/service/urlpattern.go`

3. **Strip Headers Feature** (IMPLEMENTED)
   - User-configurable additional headers to strip
   - Config: `strip = ["X-Custom-Token"]`
   - Runs after auto-strip, before auth injection

4. **Log Format CLI Flag** (IMPLEMENTED)
   - `--log-format` flag: text (default), json, logfmt
   - Applied to inject and examine commands

### What Remains (Security Improvements)

1. **Strict Allowlist Mode** - HIGH PRIORITY
2. **Credential Scope Enforcement** - MEDIUM PRIORITY
3. **Examine Mode Credential Warning** - QUICK WIN

## Context & Background

### Why We're Doing This

**Real-world scenario discovered**: User set `ANTHROPIC_BASE_URL=z.ai` but forgot to override `ANTHROPIC_API_KEY`. Claude Code sent the user's Anthropic subscription OAuth token to z.ai - a third-party provider that now has harvested credentials.

The auto-strip feature prevents this specific case, but defense-in-depth requires:
1. Blocking requests to non-allowlisted hosts entirely (strict mode)
2. Scoping credentials to specific hosts (credential scope)
3. Warning users about credentials in examine mode

### Key Decisions Made

| Decision | Rationale | Date |
|----------|-----------|------|
| Auto-strip is mandatory, not configurable | Security shouldn't depend on user remembering to configure | 2026-01-06 |
| Strip known auth headers list | Common headers across major API providers | 2026-01-06 |
| Big visual warning for stripped headers | User needs to know their app is sending wrong creds | 2026-01-06 |

### Important Constraints

- Must maintain backward compatibility with existing configs
- Security features should be ON by default (safe defaults)
- Don't break legitimate multi-auth scenarios (rare but exist)
- Warning/logging should help users understand what's happening

## Remaining Features Specification

### 1. Strict Allowlist Mode

**Purpose**: Only allow requests to explicitly configured service hosts. Everything else blocked.

**Config**:
```toml
[security]
mode = "strict"  # "strict" or "permissive" (default)
```

**Behavior**:
- `strict`: ONLY forward requests to hosts with configured services. All other hosts return 403.
- `permissive` (default): Forward to any host, only MITM configured hosts.

**Implementation approach**:
- Add `SecurityConfig` struct to `internal/config/config.go`
- Add check in `connectHandler()` or early request handler
- If strict mode AND host not in service registry → block with clear error

**Edge cases**:
- Examine mode should probably not use strict mode (user is discovering)
- Consider `--strict` CLI flag override for quick testing

### 2. Credential Scope Enforcement

**Purpose**: Define which hosts a credential can be sent to. If credential X is configured for service Y but somehow ends up going to Z, block it.

**Config**:
```toml
[[services]]
name = "anthropic"
host_pattern = "api.anthropic.com"
credential_ref = "keychain:anthropic"
credential_scope = ["*.anthropic.com"]  # ONLY allow this credential here
```

**Behavior**:
- If `credential_scope` is set, verify request host matches scope
- If no match, block request with clear error
- Optional: default scope to `host_pattern` if not specified

**Implementation approach**:
- Add `CredentialScope []string` to `ServiceConfig` and `Service`
- Add `URLPattern` matching check in auth handler before injecting
- Log warning if credential would go to out-of-scope host

**Complexity note**: This is more complex because:
- Need to track which credential is being injected
- Need to verify destination matches scope
- Edge case: what if user configures overlapping services?

### 3. Examine Mode Credential Warning

**Purpose**: In examine mode, loudly warn when auth headers are detected in requests.

**Current state**: Examine mode logs requests but doesn't highlight auth headers specially.

**Desired behavior**:
```
⚠️  WARNING: Request contains authentication header!
   Header: Authorization
   Value: Bearer sk-ant-... (first 10 chars shown)
   Destination: z.ai
   This may be credential leakage from your application!
```

**Implementation approach**:
- Modify `internal/examine/logger.go` `LogRequest()` method
- Check for known auth headers
- Print prominent warning with redacted value
- Different from normal header logging (uses color/box if terminal)

**Quick win**: This is the easiest to implement and provides immediate value.

## Acceptance Criteria

### Strict Allowlist Mode
- [ ] Config option `security.mode = "strict"` implemented
- [ ] In strict mode, requests to non-configured hosts return 403
- [ ] Clear error message explaining why request was blocked
- [ ] Default is permissive (backward compatible)
- [ ] Test: verify strict mode blocks unconfigured hosts
- [ ] Test: verify permissive mode allows unconfigured hosts

### Credential Scope Enforcement
- [ ] Config option `credential_scope` on services
- [ ] If scope set and request host doesn't match, block request
- [ ] Clear error explaining scope violation
- [ ] Test: credential_scope blocks out-of-scope injection
- [ ] Test: credential_scope allows in-scope injection

### Examine Mode Warning
- [ ] Auth headers highlighted differently in examine output
- [ ] Warning includes: header name, redacted value, destination
- [ ] Warning is visually prominent (box/color if terminal)
- [ ] Test: examine mode warns on auth headers

## Scope

### Files to Modify

**Strict Allowlist Mode**:
- `internal/config/config.go` - Add `SecurityConfig` struct
- `internal/proxy/handlers.go` - Add strict mode check handler
- `internal/proxy/server.go` - Wire up handler
- `cmd/chaperone/cmd/inject.go` - Pass security config
- `cmd/chaperone/cmd/root.go` - Optional `--strict` flag

**Credential Scope Enforcement**:
- `internal/config/config.go` - Add `CredentialScope` to `ServiceConfig`
- `internal/service/policy.go` - Add `CredentialScope` to `Policy` or `Service`
- `internal/proxy/handlers.go` - Add scope check in auth handler
- `internal/service/urlpattern.go` - Reuse for scope matching
- `cmd/chaperone/cmd/inject.go` - Pass scope from config

**Examine Mode Warning**:
- `internal/examine/logger.go` - Modify `LogRequest()`
- `internal/examine/headers.go` - May need auth header detection

### Related Components
- `internal/proxy/handlers.go` - All security handlers live here
- `internal/service/urlpattern.go` - URL matching utility (reuse)
- `test/integration/drop_strip_integration_test.go` - Pattern for security tests

### Out of Scope
- Credential rotation/refresh
- Multi-factor authentication
- Rate limiting
- Request signing

## Implementation Approach

### Recommended Order
1. **Examine Mode Warning** (quick win, immediate user value)
2. **Strict Allowlist Mode** (defense-in-depth, simple concept)
3. **Credential Scope Enforcement** (most complex, most protection)

### Patterns to Follow
- Follow handler pattern in `internal/proxy/handlers.go`
- Reuse `URLPattern` for scope matching
- Use `slog.Warn()` for security warnings
- Add integration tests like `drop_strip_integration_test.go`

### Known Gotchas
- Host in `r.Host` may include port - use `net.SplitHostPort()` to normalize
- Service registry `Lookup()` already normalizes hostname
- Examine mode uses different proxy setup (`NewExamineProxy`)
- Config changes need updates in both `inject.go` and `run.go`

## Reference Materials

### Key Files (Read These)
- `internal/proxy/handlers.go` - All security handlers, see `securityStripAuthHandler()` and `dropHandler()`
- `internal/service/urlpattern.go` - URL pattern matching implementation
- `internal/config/config.go` - Config structures
- `test/integration/drop_strip_integration_test.go` - Test patterns

### Codebase References
- `internal/proxy/server.go:137-155` - Handler pipeline order
- `internal/proxy/handlers.go:132-148` - Known auth headers list

## Testing Strategy

### Existing Tests
- `test/integration/drop_strip_integration_test.go` - Security feature tests
- `test/integration/auth_integration_test.go` - Auth flow tests
- `internal/service/urlpattern_test.go` - URL pattern tests

### New Tests Needed
- [ ] Test strict mode blocks unconfigured hosts
- [ ] Test strict mode allows configured hosts
- [ ] Test credential scope blocks out-of-scope
- [ ] Test credential scope allows in-scope
- [ ] Test examine mode warning output

## Success Metrics

- All existing tests continue to pass
- New tests cover new functionality
- `go build` succeeds
- Manual test: strict mode blocks requests to random hosts
- Manual test: examine mode shows warning for auth headers

---

## Next Steps for Agent

**Immediate actions**:
1. Read `internal/proxy/handlers.go` to understand security handler pattern
2. Read `internal/examine/logger.go` to understand examine mode
3. Start with Examine Mode Warning (quickest win)

**Before starting implementation**:
- [ ] Review handler pipeline in `server.go:137-155`
- [ ] Understand URL pattern matching in `urlpattern.go`
- [ ] Review test patterns in `drop_strip_integration_test.go`

**When complete**:
- [ ] Run `go test ./...` to verify all tests pass
- [ ] Run `go build ./cmd/chaperone` to verify build
- [ ] Manual test the new features
- [ ] Update this handoff marking items complete
