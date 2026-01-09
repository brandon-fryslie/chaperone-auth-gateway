# Chaperone Auth Gateway - Project Roadmap

Last updated: 2025-01-08

---

## Phase 0: Foundation [COMPLETE]

Goal: Establish core architecture and testing infrastructure.

- **MITM Core** [COMPLETE]
- **Testing Infrastructure** [COMPLETE]

---

## Phase 1: MVP [COMPLETE]

Goal: Deliver functional proxy with credential injection.

- **Secret Management** [COMPLETE] - env, file, keychain providers
- **Authentication Strategies** [COMPLETE] - Bearer, custom headers
- **Service Registry** [COMPLETE] - Host matching, policy enforcement
- **Examine Mode** [COMPLETE] - Auth discovery/logging

---

## Phase 2: Polish [ACTIVE]

Goal: Production-ready feature set and UX.

- **init-wizard** [COMPLETE]
  - Epic: INIT-WIZARD
  - P0-P3: Detection, storage, config generation, proxy integration all done

- **http-response-buffering-fix** [BLOCKING]
  - Epic: HTTP-BUFFERING
  - Priority: P0 (CRITICAL)
  - Status: Real clients fail with "Malformed_HTTP_Response"
  - Details: 13 integration tests pass, but actual usage broken

- **strict-allowlist-mode** [PROPOSED]
  - Priority: P1 (HIGH)
  - Security: Only forward to configured hosts in strict mode

- **credential-scope-enforcement** [PROPOSED]
  - Priority: P2 (MEDIUM)
  - Security: Block requests outside credential scope

- **examine-mode-credential-warning** [PROPOSED]
  - Priority: QUICK WIN
  - UX: Warn when auth headers detected in examine mode

---

## Phase 3: Hardening [QUEUED]

Goal: Defense-in-depth and production readiness.

- **test-coverage-improvement** [PROPOSED]
  - Target: 80% coverage (currently 65.1%)
  - Gaps: OS-specific code, error scenarios

- **streaming-response-validation** [PROPOSED]
  - Priority: P2
  - Depends on: http-response-buffering-fix

- **error-scenario-tests** [PROPOSED]
  - Secret not found, upstream 500s, timeouts

- **concurrent-request-testing** [PROPOSED]
  - 100 simultaneous requests, race condition verification

---

## Phase 4: Production [QUEUED]

Goal: Performance, monitoring, and documentation.

- **performance-benchmarking** [PROPOSED]
  - Baseline metrics, optimization targets

- **real-api-integration-tests** [PROPOSED]
  - End-to-end with real OpenAI/Anthropic APIs

- **documentation-improvement** [PROPOSED]
  - README updates, examples, troubleshooting guides

---

## Phase 5: Cleanup [QUEUED]

Goal: Remove technical debt and temporary files.

- **debug-file-cleanup** [PROPOSED]
  - Remove .gitignore debug files, test backups, repomix output

---

## Legend

| Status | Meaning |
|--------|---------|
| COMPLETE | All acceptance criteria met |
| ACTIVE | Currently being worked on |
| BLOCKING | Blocks other work or real usage |
| PROPOSED | Ready to start, not yet begun |
| QUEUED | Future phase, not ready yet |
