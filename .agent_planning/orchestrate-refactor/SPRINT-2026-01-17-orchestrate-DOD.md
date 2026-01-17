# Definition of Done: Orchestrate Refactoring

**Confidence**: HIGH (mechanical refactoring, well-defined boundaries)

## Acceptance Criteria

### 1. Create `internal/orchestrate/` Package
- [ ] Create `internal/orchestrate/setup.go` with shared setup logic
- [ ] Define `SetupConfig` struct with: Config, ServiceNames (filter), CAKeyPath, CACertPath
- [ ] Define `SetupResult` struct with: ServiceRegistry, SecretRegistry, AuthRegistry, CertCache
- [ ] Implement `Setup(ctx, cfg) (*SetupResult, error)` function

### 2. Shared Logic Extracted
- [ ] Secret provider registration (env, file, keychain)
- [ ] Auth strategy registration (bearer, header:*)
- [ ] Service registry population from config
- [ ] Header strategy detection from config (`header:x-api-key` parsing)
- [ ] Secret preloading
- [ ] Configuration validation (move `validateConfiguration` or equivalent)

### 3. Update inject.go
- [ ] Replace inline setup with `orchestrate.Setup()` call
- [ ] Keep only CLI-specific logic (flag parsing, server start, shutdown wait)
- [ ] Significant LOC reduction (~300 lines)

### 4. Update run.go
- [ ] Replace inline setup with `orchestrate.Setup()` call
- [ ] Keep only run-mode-specific logic (ephemeral CA, child process spawning, signal forwarding)
- [ ] Significant LOC reduction (~250 lines)

### 5. Verification
- [ ] `go build ./...` passes
- [ ] `go test ./...` passes
- [ ] Manual test: `chaperone inject` works
- [ ] Manual test: `chaperone run <service> -- <command>` works
- [ ] Manual test: `chaperone examine` works (unaffected)
- [ ] Manual test: `chaperone init` works (unaffected)
- [ ] Manual test: `chaperone check` works (unaffected)

## Out of Scope
- Changes to init, examine, check commands
- Changes to handler logic
- HAR recording changes
- Any feature changes

## Success Metric
Single source of truth for setup logic. Bugs in registry initialization need to be fixed in one place.
