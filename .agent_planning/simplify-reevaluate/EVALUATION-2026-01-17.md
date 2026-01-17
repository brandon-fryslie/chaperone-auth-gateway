# Evaluation: Re-evaluate Simplification Opportunities

**Timestamp**: 2026-01-17
**Bead**: CHAP-uep
**Dependencies**: CHAP-kjp (init), CHAP-deh (HAR)

## Context

After completing:
1. ✅ Orchestrate refactoring (extracted common setup)
2. ⏳ Init wizard simplification (CHAP-kjp)
3. ⏳ HAR recording exposure (CHAP-deh)

What else can be simplified?

## What Distillation Identified (Remaining)

| Category | Pattern | Status |
|----------|---------|--------|
| **Scattered** | Config loading (Load vs LoadWithMerge) | NOT ADDRESSED |
| **Scattered** | Header stripping (knownAuthHeaders duplicated) | NOT ADDRESSED |
| **Scattered** | Proxy mode setup (4 cmd files) | PARTIALLY ADDRESSED (orchestrate) |
| **Overloaded** | handlers.go (803 LOC) | NOT ADDRESSED |
| **Obscured** | Strip-then-inject invariant | NOT ADDRESSED |
| **Obscured** | Placeholder authentication | NOT ADDRESSED |

## handlers.go Analysis (803 LOC)

**Breakdown by concern**:

| Concern | Handlers | LOC | Status |
|---------|----------|-----|--------|
| Core pipeline | connect, policy, drop, strip, auth, requestID | ~540 | Essential |
| HAR recording | recordRequest, recordResponse | ~64 | Being addressed (CHAP-deh) |
| Examine mode | examineConnect, examineRequest, examineResponse | ~53 | Isolated |
| Init mode | initConnect, initRequest, initResponse | ~64 | Isolated |
| Helpers | extractClientIP, redactCredential, etc. | ~82 | Essential |

**Key finding**: `authHandler` is 190 LOC with 5+ responsibilities:
1. Service lookup
2. Placeholder detection + warning
3. Secret fetching
4. Auth strategy application
5. Audit logging

This is the remaining monolith within handlers.go.

## Areas NOT Covered by Pending Work

### 1. Config Loading Duplication
- `config.Load()` vs `config.LoadWithMerge()` called from 5 commands
- Similar boilerplate: resolve path → load → apply CLI overrides
- Could centralize in orchestrate package

### 2. Transport Mode Resolution
- Unix socket vs HTTP logic duplicated in inject, run, examine, check
- Same algorithm in 4 places
- Should extract to single source

### 3. knownAuthHeaders Duplication
- Hardcoded list in handlers.go:240-253
- Should be shared with examine mode filtering
- Single source of truth needed

### 4. Security vs Policy Stripping
- `securityStripAuthHandler()` - strips KNOWN auth headers (security)
- `stripHandler()` - strips CONFIGURED headers (policy)
- Same concept, different reasons - ordering matters but isn't enforced

## What Can Be Planned Now (HIGH Confidence)

1. Split `authHandler` into focused functions
2. Extract transport mode resolution to internal/config/
3. Move knownAuthHeaders to shared location
4. Split handlers.go into core/examine/init files

## What Needs Architectural Decision (MEDIUM Confidence)

1. Config loading consolidation - per-command differences may be essential
2. Pipeline abstraction - is explicit type worth indirection?
3. Security invariant enforcement - document vs structural enforcement?

## Recommended Sprint Scope

**Focus**: handlers.go modularization + config consolidation

1. Split authHandler (5 functions from 1)
2. Split handlers.go (core.go, examine.go, init.go)
3. Extract transport resolution
4. Centralize knownAuthHeaders

**Success Metric**: handlers.go < 600 LOC, single source of truth for shared concerns

## Verdict

**CONTINUE** - Groundwork complete, remaining dilution well-characterized.

**Note**: This sprint should wait for CHAP-kjp and CHAP-deh to complete per the dependency graph.
