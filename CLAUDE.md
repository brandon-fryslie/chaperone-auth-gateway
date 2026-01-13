# Chaperone Auth Gateway - Codebase Index

## Overview
MITM proxy that injects API credentials into requests. Apps use proxy without handling secrets directly.

## CLI Commands
- `cmd/chaperone/cmd/run.go` - **Run mode**: Start proxy + spawn child process with injected auth (supports config file and CLI-defined commands)
- `cmd/chaperone/cmd/inject.go` - Inject mode: inject credentials (legacy)
- `cmd/chaperone/cmd/examine.go` - Examine mode: Auth discovery mode (passthrough logging)
- `cmd/chaperone/cmd/check.go` - Check mode: Security posture check (assess configuration)
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
- Uses `elazarl/goproxy` for MITM proxy
- Uses `spf13/cobra` for CLI
- Secret redaction in logs (checks key names: authorization, secret, password, token, api_key)
- Default config paths: `-c` flag → `~/.config/chaperone/chaperone.toml` → `./chaperone.toml`
- CA cert stored: `~/.config/chaperone/ca-cert.pem` (must be trusted by system/browser)
