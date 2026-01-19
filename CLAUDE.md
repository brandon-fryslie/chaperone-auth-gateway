# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

**Chaperone** is a Man-in-the-Middle (MITM) proxy that injects API credentials into requests. Applications use the proxy without handling secrets directly, enabling centralized, audited credential management.

**Security Model:** 5-layer defense-in-depth (credential isolation, placeholder auth, user/permission isolation, network hardening, audit logging).

## Build & Development Commands

### Build
```bash
make build              # Build chaperone binary to ./chaperone
make all                # Alias for build
```

### Testing
```bash
make test               # Run all tests with verbose output
make test-race          # Run tests with race detector (slow but catches concurrency bugs)
go test ./...           # Same as make test
go test -short ./...    # Skip long integration tests
go test -race ./...     # Run with race detection
```

**Run specific tests:**
```bash
go test ./internal/auth -run TestBearerStrategy            # Single test
go test ./test/integration/... -run TestSelectiveMITM      # Integration tests only
```

**View test coverage:**
```bash
go test ./... -coverprofile=coverage.out -coverpkg=./...
go tool cover -html=coverage.out                           # Opens in browser
```

### Linting & Formatting
```bash
make lint               # Run golangci-lint (8 checkers enabled)
make fmt                # Format all Go files with gofmt
```

### Cleanup
```bash
make clean              # Remove chaperone binary and test artifacts
```

### Help
```bash
make help               # Show all targets with descriptions
```

## CLI Commands Architecture

- `cmd/chaperone/cmd/inject.go` - **Inject mode**: Start proxy with credential injection (main mode)
- `cmd/chaperone/cmd/examine.go` - **Examine mode**: Auth discovery (passthrough MITM logging)
- `cmd/chaperone/cmd/check.go` - **Check mode**: Security posture assessment
- `cmd/chaperone/cmd/init.go` - **Init mode**: Interactive config wizard with auth detection
- `cmd/chaperone/cmd/root.go` - Root command, config resolution, CA path helpers

## Core Proxy Architecture
- `internal/proxy/server.go` - Server setup, `NewWithMITM()`, `NewExamineProxy()`, start/stop
- `internal/proxy/handlers.go` - Request pipeline:
  - `connectHandler` - MITM vs transparent tunnel decision
  - `policyHandler` - Enforce allowed methods/paths/body size
  - `authHandler` - **Main auth injection** (fetches secret, applies strategy)
  - `recordRequestHandler/recordResponseHandler` - HAR recording
  - `examineConnectHandler/examineRequestHandler/examineResponseHandler` - Examine mode
- `internal/proxy/conditions.go` - `ChaperoneCondition()` - filter which requests get auth
- `internal/proxy/cert_adapter.go` - Adapter for goproxy cert store

## Authentication System
- `internal/auth/strategy.go` - `Strategy` interface
- `internal/auth/bearer.go` - `Authorization: Bearer` strategy
- `internal/auth/header.go` - Custom header strategy (e.g., `X-API-Key`)
- `internal/auth/headers_util.go` - **Case-insensitive header matching & replacement**
  - `findHeaderVariants()` - Find all case variations of header
  - `setHeaderPreservingCapitalization()` - Replace header, preserve client's capitalization
- `internal/auth/registry.go` - Strategy registry (register/lookup strategies)

## Secrets Management
- `internal/secrets/registry.go` - Secret provider registry, `Fetch()`, preloading
- `internal/secrets/env.go` - Environment variable provider (`env:VAR_NAME`)
- `internal/secrets/file.go` - File provider (`file:/path/to/secret`)
- `internal/secrets/keychain.go` - macOS Keychain provider (`keychain:service/account`)

## Service Configuration
- `internal/service/types.go` - `Service` struct (host pattern, auth strategy, policy)
- `internal/service/registry.go` - Interface for service lookup
- `internal/service/registry_impl.go` - Implementation (register/lookup services)
- `internal/service/matcher.go` - `ShouldMITM()` - check if host needs MITM
- `internal/service/policy.go` - Policy struct (allowed methods/paths, max body size)
- `internal/service/policy_enforcer.go` - Policy validation logic

## Examine Mode (Auth Discovery)
- `internal/examine/logger.go` - Request/response logging
  - `Config` - Control what's logged (body, params, cookies, responses)
  - `LogRequest()` - Log request with auth-relevant headers
  - `LogResponse()` - Log response (status, headers, cookies)
- `internal/examine/headers.go` - **Header filtering**
  - `noAuthHeaderPatterns` - Exclusion list (Content-Type, x-stainless-*, etc.)
  - `IsAuthRelevant()` - Filter out noise headers
  - Supports glob patterns, case-insensitive

## Audit Logging
- `internal/audit/logger.go` - Audit trail for credential injections
  - `Logger` - Thread-safe JSON logger
  - `Entry` - Audit log entry (timestamp, event, service, host, path, method, auth_strategy, request_id)
  - `NewLogger()` - Create logger (stdout or file with 0600 permissions)
  - `Log()` - Write audit entry as JSON

## MITM / TLS
- `internal/mitm/ca.go` - CA generation/loading (`LoadOrGenerateCA`)
- `internal/mitm/certcache.go` - Certificate cache (generate certs per hostname)

## Configuration
- `internal/config/config.go` - Config structs, validation, defaults
  - `ServerConfig` - Listen address/port
  - `ServiceConfig` - Per-service config (host, auth strategy, credential ref, policy)
  - `LoggingConfig` - Log level

## Logging
- `internal/log/logger.go` - Structured logging helpers
  - Request ID injection (`WithRequestID()`)
  - Sensitive field redaction (auto-redacts secrets, tokens, passwords)
  - `Info()`, `Debug()`, `Error()`

## Utilities
- `internal/shutdown/shutdown.go` - Graceful shutdown manager (register callbacks)
- `internal/recorder/har.go` - HAR (HTTP Archive) recording
- `internal/errors/errors.go` - Common error types

## Tests
- `test/integration/auth_integration_test.go` - End-to-end auth tests
- `test/integration/mitm_integration_test.go` - MITM/TLS tests
- `internal/audit/logger_test.go` - Audit logger unit tests
- `cmd/chaperone/cmd/check_test.go` - Check command unit tests

## Key Workflows

### Request Flow (Inject Mode)
1. Client → CONNECT → `connectHandler` → decides MITM or passthrough
2. If MITM: `policyHandler` → check allowed methods/paths/body size
3. `authHandler` → fetch secret → apply strategy → **inject header**
4. `recordRequestHandler` → HAR recording
5. Request sent to upstream
6. `recordResponseHandler` → HAR recording
7. Response → Client

### Auth Injection Details (`authHandler` in handlers.go)
- Lookup service by host
- Fetch secret from registry (with provider like `env:`, `file:`, `keychain:`)
- Get auth strategy from registry (e.g., `bearer`, `header:x-api-key`)
- Call `strategy.Apply(ctx, request, secret)` → injects header
- **Logs:** `"injected credential"` with credential_ref, auth_strategy, path, host

### Header Capitalization Handling (headers_util.go)
- Client may send `authorization`, `Authorization`, `AUTHORIZATION`
- Go HTTP canonicalizes to `Authorization`
- `setHeaderPreservingCapitalization()` detects existing header, logs **WARNING**
- Replaces value while preserving client's capitalization

### Examine Mode Flow
1. Client → CONNECT → `examineConnectHandler` → **always MITM** (no filtering)
2. `examineRequestHandler` → log request (headers, optional: params, cookies, body)
3. Request sent to upstream (unmodified)
4. `examineResponseHandler` → log response (status, headers, optional: cookies, body)
5. Response → Client

## Config File Format (chaperone.toml)
```toml
[server]
address = "127.0.0.1"
port = 4010

[logging]
level = "info"  # debug, info, warn, error

# Audit logging (optional)
[audit]
enabled = true
path = "/var/log/chaperone/audit.log"  # or "stdout"

[[services]]
name = "openai"
host_pattern = "api.openai.com"
auth_strategy = "bearer"  # or "header:X-API-Key"
credential_ref = "env:OPENAI_API_KEY"  # or "file:/path" or "keychain:service/account"

# Optional: Placeholder token for process authentication
placeholder = "chap_openai_xxxxxxxx"  # App uses this, Chaperone swaps for real key

[services.policy]
allowed_methods = ["GET", "POST"]
allowed_paths = ["/v1/*"]
max_body_bytes = 1048576
```

## CLI Usage
```bash
# Run mode: Start proxy and spawn child process with injected auth
chaperone run openai                                    # Uses config [services.openai.run]
chaperone run openai -- python script.py                # Override config with CLI command
chaperone run zai -- claude --dangerously-skip-permissions  # Complex command
chaperone run -c custom.toml myservice -- bash          # Custom config + CLI command

# Inject mode (default config: ~/.config/chaperone/chaperone.toml)
chaperone inject                    # All services
chaperone inject openai             # Specific service
chaperone inject -c custom.toml     # Custom config

# Examine mode (auth discovery)
chaperone examine                   # Basic (just headers)
chaperone examine -p --show-cookies # Show params & cookies
chaperone examine -b -r             # Show bodies & responses
chaperone examine -o results.txt    # Save to file (enables all flags)

# Security check
chaperone check                     # Show security posture
chaperone check -c custom.toml      # Check with specific config
```

## Recent Features
- **CLI command override in run mode**: Commands can be defined in config file OR overridden via CLI with `-- command args` syntax
- **Run mode**: `chaperone run` starts proxy + spawns child process with injected auth
- **Security check**: `chaperone check` command to assess security posture (cmd/chaperone/cmd/check.go)
- **Audit logging**: JSON audit trail for credential injections (internal/audit/)
- **Placeholder authentication**: Optional placeholder token verification (Layer 2 security)
- **Examine mode flags**: `-b` (body), `-p` (params), `--show-cookies`, `-r` (response), `-o` (output file)
- **Header capitalization detection**: Warns when client sends header we're trying to inject
- **Glob pattern exclusions**: Examine mode filters `x-stainless-*` headers via glob
- **Improved logging**: DEBUG-level CONNECT/TLS messages, structured "injected credential" message

## Development Notes

### Dependencies
- `elazarl/goproxy` - MITM proxy foundation
- `spf13/cobra` - CLI framework
- `BurntSushi/toml` - Configuration parsing
- `google/uuid` - Request ID generation
- `stretchr/testify` - Testing assertions

### Key Architectural Patterns

**Error Handling:** Uses custom error types from `internal/errors/` to maintain error context across package boundaries. All public functions should return errors explicitly; never panic.

**Observability:**
- Structured logging via `internal/log/logger.go` with automatic request ID injection
- Secret redaction in logs (checks: authorization, secret, password, token, api_key)
- Request correlation via X-Request-ID headers
- Audit logging for security events via `internal/audit/logger.go` (JSON format)

**Configuration Management:**
- Default paths: `-c` flag → `~/.config/chaperone/chaperone.toml` → `./chaperone.toml`
- TOML format with validation
- Service registry preloaded on startup
- All secrets validated at startup (preloading catches config errors early)

**Certificate Management:**
- CA cert stored at `~/.config/chaperone/ca-cert.pem` (10-year validity)
- Per-hostname certificates cached in memory
- Certificates support both DNS names and IP addresses (SAN)

### Testing Philosophy

**Integration Tests:** Located in `test/integration/`, use **real HTTP clients and actual TLS handshakes**. They cannot be satisfied by mocks - the entire MITM pipeline must work end-to-end. These tests serve as completion gates for features.

**Unit Tests:** Test individual functions and modules. Use `net/http/httptest` for server mocking only where appropriate.

**Running Tests:**
- `make test` - Full suite with `-v` (verbose)
- `make test-race` - Detects race conditions
- `go test -short ./...` - Skips integration tests for fast iteration

### Code Quality Standards

- **Linting:** golangci-lint with 8 checkers (errcheck, govet, ineffassign, staticcheck, unused, misspell, gosec)
- **Security:** gosec enabled with some G-rules excluded (file reading, permissions checks allowed in config validation)
- **Formatting:** gofmt (no custom style)

## Package Organization

The codebase is organized by responsibility, not by layer:

| Package | Responsibility |
|---------|-----------------|
| `cmd/chaperone/cmd/` | CLI commands (inject, examine, check, init) |
| `internal/proxy/` | Core proxy server and request handlers |
| `internal/auth/` | Authentication strategies (bearer, custom header) |
| `internal/secrets/` | Secret providers (env, file, keychain) |
| `internal/service/` | Service registry, policy, host matching |
| `internal/config/` | Configuration parsing and validation |
| `internal/mitm/` | MITM certificate generation and caching |
| `internal/audit/` | Security event logging (JSON format) |
| `internal/examine/` | Auth discovery mode (passthrough logging) |
| `internal/init/` | Config wizard with auto-detection |
| `internal/log/` | Structured logging with redaction |
| `internal/recorder/` | HAR (HTTP Archive) recording |
| `internal/shutdown/` | Graceful shutdown management |
| `internal/errors/` | Custom error types |

## Key Implementation Details

### Header Capitalization Handling (`internal/auth/headers_util.go`)

HTTP headers are case-insensitive but Go canonicalizes them. When injecting auth headers:
- `setHeaderPreservingCapitalization()` finds existing header (any case)
- Replaces value while preserving client's capitalization
- Logs WARNING if header already exists

### Policy Enforcement Pipeline (`internal/proxy/handlers.go`)

Requests flow through handlers in order:
1. `connectHandler` - Decide MITM vs transparent tunnel
2. `policyHandler` - Check methods/paths/body size (fail fast)
3. `authHandler` - Fetch secret, inject credential
4. `recordRequestHandler` - HAR recording (if enabled)
5. Forward to upstream
6. `recordResponseHandler` - HAR response capture
7. Return to client

**Why fail-fast?** Policy rejection happens before credential injection for security.

### Service Lookup Flow

- Host from CONNECT request matched against service patterns
- If match → lookup service config → fetch secret → apply strategy
- If no match → transparent tunnel (no MITM)
- Pattern matching is first-match-wins

### Config Validation Strategy

All configuration is validated on startup via `config.Validate()`:
- All service host patterns are required
- All credential refs are validated
- Secret preloading catches missing env vars immediately
- Invalid patterns/strategies fail fast

## Common Refactoring Risks

**High Risk - Breaks Multiple Components:**
- Changes to `internal/service/registry.go` interface affect proxy handlers, config parsing, CLI commands
- Changes to `internal/auth/strategy.go` interface affect all auth implementations and proxy handlers
- Changes to error types in `internal/errors/` affect error handling across all packages

**Medium Risk - Localized Impact:**
- Adding fields to `Service` struct requires config parsing updates
- Changing logging format breaks test assertions on log output

**Low Risk - Isolated:**
- Adding new secret provider (implement interface, register in registry)
- Adding new auth strategy (implement interface, register in registry)
- Adding new CLI command (follows existing cobra patterns)

## Code Review Focus Areas

When reviewing changes:
1. **Error handling:** Are errors returned/logged with context?
2. **Secrets:** Are credentials ever logged/printed?
3. **Concurrency:** Use race detector - `make test-race`
4. **Config validation:** Does startup catch misconfigurations?
5. **Test coverage:** Are critical paths tested? (Especially auth injection)

## Verification Before Committing

```bash
make fmt                           # Auto-format
make lint                          # Static analysis
make test                          # All tests
make build                         # Compilation check
```

**For security-sensitive changes:**
```bash
make test-race                     # Check for races
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out   # Check coverage in auth/secrets/audit paths
```
