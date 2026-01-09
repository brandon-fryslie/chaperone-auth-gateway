# Evaluation: chaperone check Command
Generated: 2026-01-09

## Topic
Implement the `chaperone check` command - a security posture assessment tool.

## Context from SECURITY.md

The command should output something like:
```
Chaperone Security Check
========================

✅ Single-service mode: Only one credential per instance
✅ Upstream TLS: Certificates validated
⚠️  Running as current user (consider dedicated user)
⚠️  Using TCP port (consider Unix socket)
⚠️  Using system CA bundle (consider bundled CAs)
ℹ️  Placeholder auth: Not configured
ℹ️  Audit logging: Not enabled

Recommendations:
  • Run as dedicated 'chaperone' user for credential isolation
  • Use Unix socket instead of TCP port
  • Enable audit logging with --audit-log
  • See: chaperone docs security
```

## What Exists
- `cmd/chaperone/cmd/` - Other cobra commands (inject, examine, init)
- Security features implemented:
  - Placeholder authentication (config.ServiceConfig.Placeholder)
  - Audit logging (config.AuditConfig)
  - Ignore system proxy (hardcoded)
  - Upstream TLS validation (Go default)

## What's Needed

### New Command: `cmd/chaperone/cmd/check.go`

Create cobra command that:
1. Loads config (or uses defaults)
2. Runs security checks
3. Outputs results with status icons
4. Provides recommendations

### Security Checks to Implement

| Check | How to Verify | Status Icon |
|-------|---------------|-------------|
| Single-service mode | Always true (architecture) | ✅ |
| Upstream TLS | Always true (Go default) | ✅ |
| Ignore system proxy | Always true (hardcoded) | ✅ |
| Placeholder auth | Check each service has Placeholder set | ✅/⚠️/ℹ️ |
| Audit logging | Check config.Audit.Enabled | ✅/ℹ️ |
| Dedicated user | Check if running as 'chaperone' user | ✅/⚠️ |
| Unix socket | Check if using socket (not yet supported) | ⚠️ |
| Bundled CA | Check if using bundled CA (not yet supported) | ⚠️ |

### Status Icons
- ✅ = Secure / Enabled
- ⚠️ = Warning / Recommended improvement
- ℹ️ = Info / Optional feature not enabled

## Design Decisions

1. **Config optional**: Run without config shows what's missing
2. **No judgment**: Informative, not alarmist
3. **Actionable**: Each warning has a recommendation
4. **Exit code**: 0 always (it's informational, not a gate)

## Dependencies
- Config loading (already exists)
- No new packages needed

## Risks
- None significant

## Verdict
**CONTINUE** - Clear requirements from SECURITY.md, straightforward implementation.
