# Evaluation: Init Wizard Simplification

**Timestamp**: 2026-01-17
**Bead**: CHAP-kjp

## Current Complexity

| File | LOC | Responsibility |
|------|-----|----------------|
| cmd/chaperone/cmd/init.go | 260 | CLI, templates, wizard orchestration |
| internal/init/wizard.go | 513 | UI prompts, step flow, file I/O |
| internal/init/detector.go | 63 | Auth detection orchestration |
| internal/init/evidence.go | 169 | Thread-safe finding collection |
| internal/init/heuristics.go | 152 | Confidence scoring (sentinel, known headers, patterns) |
| internal/init/generator.go | 139 | TOML config generation |
| internal/init/credential_writer.go | 151 | Storage backends (keychain, file, .env) |
| internal/init/policy_detector.go | 102 | Path generalization (UUID, numeric patterns) |
| internal/init/types.go | 77 | Data structures |
| Tests | ~750 | 3 test files |
| **Total** | **~2,550** | |

## What's Essential (Keep)

1. **Detection Heuristics** - Multi-tiered confidence scoring is core value
2. **Evidence Accumulation** - Thread-safe collection, deduplication
3. **Credential Storage** - Three backends serve security needs
4. **Policy Generalization** - Regex-based path patterns prevent overfitting
5. **Wizard Step Flow** - Clear interactive configuration

## Simplification Opportunities

### HIGH VALUE / MODERATE EFFORT
1. **Extract UI from wizard.go** - Reduce 513 → ~250 LOC, enables testing
2. **Move auth header list to shared location** - Eliminate duplication with handlers.go
3. **Implement CredentialStore interface** - Enable new backends, cleaner code

### MODERATE VALUE / LOW EFFORT
1. **Add Evidence.with() helper** - Reduce lock boilerplate (~50 lines saved)
2. **Add non-interactive default** - File storage for CI/automation
3. **Rename types for clarity** - Finding → AuthFinding, etc.

### NICE-TO-HAVE / HIGHER EFFORT
1. **Multi-service batch configuration** - Reduce user friction
2. **Policy review step** - Show generalized patterns before save

## UX Pain Points

1. Non-interactive mode fails on credential storage (should default to file)
2. Sentinel value is powerful but unclear when to use
3. Wizard configures ONE service at a time
4. Policy detection is opaque (no review step)
5. No rollback on partial failures

## Unknowns Needing User Input

1. Priority of single vs. multi-service configuration?
2. How often do generalized policies need adjustment?
3. Who uses --yes flag? CI/automation importance?
4. Interest in Vault/1Password integration?

## Verdict

**CONTINUE** - Architecture is sound, incrementally refine structure.

**Recommended approach**:
1. Quick win: Extract UI from wizard.go
2. Medium-term: Introduce CredentialStore interface
3. Long-term: Policy review + batch configuration
