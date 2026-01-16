# Strict Allowlist Mode Requirements

**Created**: 2026-01-15
**Priority**: P1 (HIGH)
**Status**: Ready for Implementation

## Overview

Chaperone will ONLY forward requests to explicitly configured services. No transparent tunnel fallback.

## Requirements Clarification

### 1. Behavior for Non-Configured Domains
**Decision**: Reject CONNECT immediately (connection refused)
- No HTTP response needed
- TCP connection refused at proxy level
- Clean, immediate rejection

### 2. Default Mode
**Decision**: Strict mode is the ONLY mode
- Remove transparent tunnel fallback entirely
- However, services can be configured with explicit passthrough
- No global "strict mode" flag needed (it's always strict)

### 3. Audit Logging
**Decision**: Yes, log all blocked requests
- Audit event type: EventDomainBlocked
- Log: hostname, timestamp, source IP
- Required for security compliance

### 4. Configuration Scope
**Decision**: No global config option, allowlist is per-service
- Each service explicitly configured
- No catch-all transparent tunnel
- Services can have passthrough policy if needed

### 5. Error Message
**Decision**: No error message to user, just audit log
- Connection refused at TCP level
- Audit log captures the block event
- User sees standard connection refused error from their client

## Implementation Changes

### Remove Transparent Tunnel
- Delete transparent tunnel fallback in `handlers.go:86-87`
- Replace with connection rejection

### Per-Service Allowlist
- Service registry already handles this
- `ShouldMITM()` returns false for non-configured → reject
- `ShouldMITM()` returns true for configured → MITM as usual

### Audit Logging
- Add `EventDomainBlocked` to audit events
- Log hostname, timestamp, source when rejecting

## Files to Modify

1. `internal/proxy/handlers.go` - Replace transparent tunnel with reject
2. `internal/audit/events.go` - Add EventDomainBlocked
3. `internal/audit/logger.go` - Log blocked domain events
4. `test/integration/strict_mode_test.go` - Test rejection behavior
5. `SECURITY.md` - Document strict allowlist behavior

## Acceptance Criteria

- [ ] Non-configured domains return connection refused
- [ ] Configured domains continue to work normally
- [ ] Blocked requests are audit logged
- [ ] Integration tests verify rejection
- [ ] Documentation updated
