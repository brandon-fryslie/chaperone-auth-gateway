# Audit Logging Enhancement - Evaluation

**Generated**: 2026-01-11T04:30:00Z
**Topic**: FedRAMP-compliant audit logging with industry-standard event taxonomy

---

## Current State Analysis

### What Exists

The codebase has a **minimal audit logging implementation** in `internal/audit/logger.go`:

**Single Event Type:**
- `EventCredentialInjected` - only event logged (after successful auth injection)

**Current Fields (8 total):**
```go
type Entry struct {
    Timestamp    time.Time `json:"timestamp"`
    Event        string    `json:"event"`
    Service      string    `json:"service"`
    Host         string    `json:"host"`
    Path         string    `json:"path"`
    Method       string    `json:"method"`
    AuthStrategy string    `json:"auth_strategy"`
    RequestID    string    `json:"request_id"`
}
```

**Current Usage:**
- Called only in `authHandler()` (handlers.go:381-389) after successful credential injection
- Config exposed via `AuditConfig{Enabled, Path}` in config.go
- Output format: JSON Lines (newline-delimited JSON)
- Storage: File (0600 permissions) or stdout
- Thread-safe with mutex protection
- Test coverage: 12 test cases in logger_test.go

### Critical Gaps

| FedRAMP Requirement | Current | Gap |
|---------------------|---------|-----|
| AU-2: Audit Events | 1 event type | Need 10+ event types |
| AU-3: Who (subject) | None | Need client_ip, user_agent |
| AU-3: Outcome | Implicit (success only) | Need explicit success/failure/blocked |
| AU-3: Where (source) | Partial (host) | Need proxy instance_id |
| AU-8: Time precision | UTC microsecond | Met |
| AU-9: Integrity | None | Need tamper-evidence (future) |

---

## Required Event Types

### Authentication Events (P0)
| Event | When | Fields |
|-------|------|--------|
| `credential_injected` | After successful injection | existing + new fields |
| `auth_failure` | Secret fetch/strategy error | error details, outcome=failure |
| `placeholder_mismatch` | Placeholder doesn't match | expected vs actual |

### Policy Events (P1)
| Event | When | Fields |
|-------|------|--------|
| `policy_denied` | Request blocked by policy | reason, rule, method/path |
| `request_dropped` | Matched drop pattern | drop_pattern, path |
| `body_size_exceeded` | Body too large | content_length, max_allowed |

### Security Events (P1)
| Event | When | Fields |
|-------|------|--------|
| `auth_header_stripped` | Existing auth removed | headers_removed, reason |

### Administrative Events (P2 - Future)
| Event | When | Fields |
|-------|------|--------|
| `server_start` | Process startup | config, services_count |
| `server_stop` | Shutdown | uptime, total_requests |

---

## Required Fields (AU-3 Compliance)

### Universal Fields (all events)
```go
type Entry struct {
    // WHEN - timestamp (existing)
    Timestamp    time.Time `json:"timestamp"`

    // WHAT - event type (existing, but need more types)
    Event        string    `json:"event"`

    // WHO - subject identification (NEW)
    ClientIP     string    `json:"client_ip"`
    UserAgent    string    `json:"user_agent,omitempty"`

    // WHERE - system identification (existing + NEW)
    Host         string    `json:"host"`
    Path         string    `json:"path"`
    Method       string    `json:"method"`
    InstanceID   string    `json:"instance_id,omitempty"`  // NEW

    // CONTEXT - correlation (existing)
    Service      string    `json:"service"`
    AuthStrategy string    `json:"auth_strategy,omitempty"`
    RequestID    string    `json:"request_id"`

    // OUTCOME - result (NEW)
    Outcome      string    `json:"outcome"`                // success|failure|blocked|pass_through
    StatusCode   int       `json:"status_code,omitempty"`  // HTTP status if applicable
    ErrorMessage string    `json:"error,omitempty"`        // Error details if failure

    // SECURITY CONTEXT (NEW - for security events)
    Detail       string    `json:"detail,omitempty"`       // Event-specific details
}
```

---

## OCSF Alignment Strategy

Keep JSON Lines format but use field names compatible with future OCSF migration:
- `client_ip` maps to OCSF `src_endpoint.ip`
- `outcome` maps to OCSF `status` / `activity_id`
- `error` maps to OCSF `status_detail`

This allows progressive enhancement without breaking changes.

---

## Dependencies

1. **Request Context** - Need to extract client IP from `http.Request.RemoteAddr`
2. **Policy Enforcer** - Need to add audit logging call in policy_enforcer.go
3. **Strip Handler** - Need to add audit logging in handlers.go strip logic
4. **Config** - May need `instance_id` config option

---

## Risks

1. **Log Volume** - Adding policy events could increase volume significantly
2. **Performance** - Synchronous logging may impact latency
3. **IP Privacy** - Client IP logging may have privacy implications

---

## Verdict: CONTINUE

Scope is clear. Implementation can proceed with phased approach.
