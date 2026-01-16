# Chaperone Auth Gateway - Project Roadmap

Last updated: 2026-01-15

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

- **examine-mode-credential-warning** [COMPLETE]
  - Priority: QUICK WIN
  - UX: Warn when auth headers detected in examine mode
  - Implemented: Startup banner + per-request WARN log

---

## Phase 3: Hardening [QUEUED]

Goal: Defense-in-depth and production readiness.

- **test-coverage-improvement** [PROPOSED]
  - Target: 80% coverage (currently 65.1%)
  - Gaps: OS-specific code, error scenarios

- **streaming-response-validation** [PROPOSED]
  - Priority: P2

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

- **v1-release-packaging** [PROPOSED]
  - Priority: P0
  - Package for v1 release
  - Depends on: All Phase 2-3 work complete

---

## Phase 5: Cleanup [QUEUED]

Goal: Remove technical debt and temporary files.

- **debug-file-cleanup** [PROPOSED]
  - Remove .gitignore debug files, test backups, repomix output

- **test-file-cleanup** [PROPOSED]
  - Priority: P3
  - Technical debt: Remove/fix failing test files for unimplemented features
  - Files: interfaces_test.go, scaffolding_test.go, setup_test.go
  - Note: These test planned-but-unimplemented features from earlier development

- **har-recording-fix** [PROPOSED]
  - Priority: P3
  - Technical debt: HAR recording runs but output never consumed
  - Issues: WriteToFile() is broken (creates buffer, returns nil), GetRecorder() never called
  - Options: Fix and expose via --har-output flag, or remove entirely (~300 lines)
  - Source: Identified in PRUNE-2026-01-16 analysis

---

## Phase 6: Advanced Audit (FedRAMP Enhancement) [QUEUED]

Goal: Extended audit logging capabilities for compliance.

- **ocsf-schema-transformation** [PROPOSED]
  - Priority: P2
  - Transform audit logs to OCSF (Open Cybersecurity Schema Framework)
  - Depends on: Current audit logging implementation

- **tamper-evidence-au9** [PROPOSED]
  - Priority: P1
  - AU-9: Audit log tamper-evidence (cryptographic signing)
  - FedRAMP compliance requirement

- **log-rotation-configuration** [PROPOSED]
  - Priority: P2
  - Configurable log rotation policies (size, time-based)
  - Integration with system log management

- **remote-syslog-transport** [PROPOSED]
  - Priority: P2
  - Forward audit logs to remote syslog/SIEM systems
  - Support TLS-encrypted transport

- **administrative-events** [PROPOSED]
  - Priority: P2
  - Log administrative actions (config changes, service updates)
  - Separate event taxonomy from operational events

- **async-logging** [PROPOSED]
  - Priority: P3
  - Performance: Non-blocking async audit log writes
  - Buffering and batching strategies

---

## Legend

| Status | Meaning |
|--------|---------|
| COMPLETE | All acceptance criteria met |
| ACTIVE | Currently being worked on |
| BLOCKING | Blocks other work or real usage |
| PROPOSED | Ready to start, not yet begun |
| QUEUED | Future phase, not ready yet |
