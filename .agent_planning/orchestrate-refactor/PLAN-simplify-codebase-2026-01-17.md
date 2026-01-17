# Plan: Simplify Chaperone Codebase (Revised)

Based on findings from: `.agent_planning/DISTILLATION-2026-01-16-141341.md`

## Overview

The distillation identified areas of dilution and duplication. This plan:
1. Executes known simplifications now
2. Queues exploratory work as future tasks

## Immediate Work: Factor Common Setup

### Phase 1: Create `internal/orchestrate/` Package

**What**: Extract shared setup code from inject.go and run.go into a reusable package.

**Why**:
- Distillation notes: "inject.go and run.go share 80% of their setup code"
- Both create: secret registry, auth registry, service registry, cert cache
- Duplication means bugs need to be fixed twice
- This is well-understood, mechanical refactoring

**New package**: `internal/orchestrate/`

```go
// setup.go
type SetupConfig struct {
    Config       *config.Config
    ServiceNames []string  // Empty = all services
    CAKeyPath    string
    CACertPath   string
}

type SetupResult struct {
    ServiceRegistry *service.Registry
    SecretRegistry  *secrets.Registry
    AuthRegistry    *auth.Registry
    CertCache       *mitm.CertCache
    PolicyEnforcer  *service.Enforcer
}

func Setup(ctx context.Context, cfg SetupConfig) (*SetupResult, error)
```

**Changes to cmd files**:
- `inject.go`: Use orchestrate.Setup() instead of inline setup (~300 LOC reduction)
- `run.go`: Use orchestrate.Setup() instead of inline setup (~250 LOC reduction)
- Both become thin CLI layers focused on argument parsing and mode-specific logic

**Shared code to extract**:
1. Secret provider registration (env, file, keychain)
2. Auth strategy registration (bearer, header:*)
3. Service registry population from config
4. Header strategy detection from config
5. Secret preloading
6. Configuration validation (`validateConfiguration`)

**Confidence**: High - mechanical refactoring, well-defined boundaries

## Execution

1. Create `internal/orchestrate/setup.go` with shared logic
2. Update `cmd/inject.go` to use orchestrate
3. Update `cmd/run.go` to use orchestrate
4. Run tests, verify build
5. Manual verification of all commands

## Verification

1. `go build ./...` - ensure compilation
2. `go test ./...` - ensure tests pass
3. Manual test: `chaperone run openai -- echo hello`
4. Manual test: `chaperone inject`
5. Manual test: `chaperone examine`
6. Manual test: `chaperone init`
7. Manual test: `chaperone check`

## Future Work (to be queued as separate tasks)

### Future Task 1: Simplify Init Flow
- Investigate what can be simplified about the init wizard
- Consider: storage type defaults, prompt flow, UX improvements
- Requires exploration and user feedback

### Future Task 2: Expose and Improve HAR Recording
- Currently HAR is captured but not exposed to users
- Add command/flag to export HAR data
- Consider simplification of the recording infrastructure
- Make it more useful for debugging

### Future Task 3: Re-evaluate After Simplifications
- After orchestrate refactoring is complete
- After init and HAR improvements
- Fresh look at what else can be simplified
- May reveal new opportunities

## Estimated Impact

| Component | Before | After | Change |
|-----------|--------|-------|--------|
| cmd/inject.go | ~450 LOC | ~150 LOC | -300 |
| cmd/run.go | ~400 LOC | ~150 LOC | -250 |
| internal/orchestrate/ (new) | 0 | ~200 LOC | +200 |
| **Net** | - | - | **-350 LOC** |

More importantly: single source of truth for setup logic, bugs fixed once.
