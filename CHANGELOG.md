# Changelog

All notable changes to the Chaperone project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-01-18

### Initial Release

This is the first release of Chaperone, a transparent local HTTPS proxy that automatically injects API credentials into requests.

### Features

#### Core Proxy Functionality
- **Selective MITM** - Only terminates TLS for configured domains; other traffic passes through untouched
- **Transparent Operation** - Works at the OS proxy layer with zero code changes required
- **Dynamic Certificate Generation** - Per-hostname certificates signed by local CA with IP SAN support
- **Certificate Caching** - Reuses generated certificates for performance
- **Streaming Responses** - Byte-for-byte proxy with proper backpressure handling
- **Connection Pooling** - Efficient connection reuse per service

#### Security Features (5-Layer Defense-in-Depth)
- **Layer 1: Credential Isolation** - Applications never see or handle API keys
- **Layer 2: Placeholder Token Authentication** - Optional process-level authentication
- **Layer 3: User/Permission Isolation** - Dedicated user accounts for production deployments
- **Layer 4: Network Hardening** - Unix socket mode eliminates network exposure
- **Layer 5: Audit Logging** - Comprehensive JSON audit trail for compliance

#### Authentication System
- **Bearer Token Strategy** - Injects `Authorization: Bearer <token>`
- **Custom Header Strategy** - Injects any custom header (e.g., `X-API-Key`)
- **Case-Insensitive Header Handling** - Finds and replaces headers regardless of capitalization
- **Automatic Auth Header Stripping** - Removes 13 common auth headers to prevent credential leakage
- **Pluggable Architecture** - Easy to extend with OAuth2, HMAC, AWS SigV4, etc.

#### Secret Management
- **Environment Variable Provider** (`env:VAR_NAME`) - Simple environment variable lookup
- **File Provider** (`file:/path/to/secret`) - Read secrets from files with permission validation
- **macOS Keychain Provider** (`keychain:service/account`) - Secure system keychain integration
- **Startup Validation** - All secrets preloaded at startup to catch configuration errors immediately

#### Policy Enforcement
- **Method Whitelisting** - Restrict allowed HTTP methods per service
- **Path Whitelisting** - Enforce allowed URL path patterns
- **Body Size Limits** - Prevent oversized request bodies
- **Fail-Fast Security** - Policy violations blocked before credential injection

#### CLI Modes
- **Inject Mode** (`chaperone inject`) - Standard proxy mode with credential injection
- **Run Mode** (`chaperone run`) - Start proxy + spawn child process with auto-configured environment
- **Examine Mode** (`chaperone examine`) - Auth discovery mode with passthrough logging
- **Check Mode** (`chaperone check`) - Security posture assessment
- **Init Mode** (`chaperone init`) - Interactive configuration wizard with auto-detection
- **CA Cert Mode** (`chaperone ca-cert`) - Display CA certificate for trust installation

#### Examine Mode Features
- **Intelligent Header Filtering** - Excludes irrelevant headers, shows auth-relevant ones
- **Glob Pattern Support** - Filter headers like `x-stainless-*` automatically
- **Optional Body/Params/Cookies** - Configurable verbosity with `-b`, `-p`, `--show-cookies`
- **Response Logging** - Optional response capture with `-r` flag
- **File Output** - Save discovery results with `-o` flag
- **HAR Recording** - Optional HTTP Archive recording with `--har` flag

#### Audit Logging
- **JSON Format** - Structured logs for machine parsing
- **FedRAMP AU-3 Compliance** - Meets federal audit requirements
- **Security Events** - Logs credential injections, policy denials, auth failures, dropped requests
- **File Permissions** - Audit logs created with 0600 permissions
- **Stdout Support** - Can log to stdout for container environments

#### Configuration
- **TOML Format** - Human-readable configuration files
- **Default Paths** - Checks `-c` flag → `~/.config/chaperone/chaperone.toml` → `./chaperone.toml`
- **Validation** - All configuration validated at startup
- **Hot Reload** - Can reload configuration without restart (future feature)

### Code Quality
- **Comprehensive Test Suite** - Unit tests, integration tests, and security tests
- **"Un-gameable" Integration Tests** - Tests use real HTTP clients and actual TLS handshakes
- **Race Detection** - All tests pass with `-race` flag
- **Linting** - golangci-lint with 8 checkers (errcheck, govet, ineffassign, staticcheck, unused, misspell, gosec)
- **Secret Redaction** - Credentials never written to logs
- **Structured Logging** - Request correlation with X-Request-ID headers

### Documentation
- **Comprehensive README** - 1250+ lines covering all features, configuration, troubleshooting
- **CLAUDE.md** - AI-optimized codebase guide for future development (381 lines)
- **SECURITY.md** - Security model, threat assessment, compliance information
- **Integration Test README** - Detailed test design and anti-gaming philosophy
- **PROJECT_SPEC.md** - Original project specification

### Dependencies
- `elazarl/goproxy` v1.7.2 - MITM proxy foundation
- `spf13/cobra` v1.8.0 - CLI framework
- `BurntSushi/toml` v1.5.0 - Configuration parsing
- `google/uuid` v1.6.0 - Request ID generation
- `stretchr/testify` v1.11.1 - Testing assertions

### Known Limitations
- Certificate trust must be manually configured (per OS)
- No OAuth2 flow support yet (planned for future release)
- No Windows support for Unix socket mode (TCP socket works)
- No hot configuration reload (requires restart)

### Upgrade Notes
This is the initial release. No upgrade path exists yet.

[0.1.0]: https://github.com/bmf/chaperone/releases/tag/v0.1.0
