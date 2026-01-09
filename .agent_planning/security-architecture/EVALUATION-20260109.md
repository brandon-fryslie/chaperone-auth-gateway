# Evaluation: Security Architecture Documentation & Roadmap
Generated: 2026-01-09

## Topic
Formalize Chaperone's security architecture: document the security model, add roadmap items for hardening features, and create a `chaperone check` command for security posture assessment.

## Context from Conversation
The following security measures were discussed and agreed upon:

### Core Security Model
1. **Single-service instances** - Each Chaperone instance handles one credential (blast radius limitation)
2. **Placeholder token authentication** - Apps must send a placeholder that Chaperone recognizes and replaces
3. **Credentials never exposed to apps** - Apps never see real API keys

### Defense in Depth Layers
1. **Dedicated user isolation** - Chaperone runs as unprivileged `chaperone` user, credential files unreadable by normal users
2. **Unix socket with group permissions** - Only authorized users can connect
3. **Own CA bundle** - Don't trust system CA store for upstream connections
4. **Ignore system proxy** - Upstream connections bypass HTTP_PROXY env vars
5. **Upstream TLS validation** - Prevents DNS/hosts file redirection attacks

### Additional Measures Identified
- Memory protection (`mlock()` to prevent swap)
- Secure credential zeroing after use
- Audit logging (log injections without credentials)
- Rate limiting
- Certificate pinning for known services
- Connection timeout hardening

## What Exists
- Basic proxy implementation in `internal/proxy/`
- Credential storage in `internal/secrets/`
- No SECURITY.md documentation
- No ROADMAP.md file
- No `chaperone check` command

## What's Needed (This Sprint)

### P0: SECURITY.md Documentation
Create comprehensive security documentation explaining:
- Threat model
- Security layers (defense in depth)
- Current implementation status
- Future hardening roadmap

### P1: ROADMAP.md with Security Items
Create or update roadmap with security hardening items:
- Dedicated user mode
- Own CA bundle
- Ignore system proxy
- Placeholder token auth
- Unix socket mode
- `chaperone check` command
- Memory protection
- Audit logging

### P2: Roadmap Item for `chaperone check`
Document the planned command that will:
- Check if running as dedicated user
- Check socket permissions
- Check CA bundle in use
- Check for insecure configurations
- Provide actionable recommendations

## Dependencies
- None for documentation
- Security features themselves depend on this planning

## Risks
- Documentation could become stale if not maintained
- Roadmap items need prioritization

## Verdict
**CONTINUE** - Requirements are clear, this is documentation work.
