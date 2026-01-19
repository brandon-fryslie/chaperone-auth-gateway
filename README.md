# Chaperone

A transparent local HTTPS proxy that automatically injects API credentials into requests on behalf of applications.

## Overview

Chaperone securely manages API credentials by acting as a local proxy that:

- **Intercepts** HTTPS requests from applications using MITM (man-in-the-middle) TLS termination
- **Injects** API credentials from secure sources (environment variables, files, macOS Keychain)
- **Enforces** security policies (allowed methods, paths, body size limits)
- **Audits** all credential injections for compliance and security monitoring
- **Protects** against credential leakage through automatic auth header stripping

Applications never see or handle API keys - Chaperone injects them transparently.

## Key Features

### 🔐 Security Features

- **5-Layer Defense-in-Depth Security Model** - Progressive security hardening
- **Placeholder Token Authentication** - Optional process-level authentication to prevent accidental credential injection
- **Automatic Auth Header Stripping** - Removes 13 common auth headers to prevent credential leakage to wrong APIs
- **Comprehensive Audit Logging** - JSON audit trail for all credential injections and security events
- **Unix Socket Mode** - Eliminates network exposure entirely for maximum security
- **Security Posture Assessment** - `chaperone check` command shows security status and recommendations
- **Strict File Permissions** - Secret files enforced at 0600, audit logs at 0600, Unix sockets at 0660
- **No Secret Logging** - Credentials never written to logs (automatic redaction)
- **Upstream Proxy Bypass** - Ignores HTTP_PROXY/HTTPS_PROXY for upstream connections (security feature)

### 🚀 Core Proxy Features

- **Selective MITM** - Only terminates TLS for configured domains; other traffic passes through untouched
- **Transparent Operation** - Works at the OS proxy layer; most applications need zero code changes
- **Dynamic Certificate Generation** - Per-hostname certificates signed by local CA
- **Certificate Caching** - Reuses generated certificates for performance
- **Streaming Responses** - Byte-for-byte proxy with proper backpressure handling
- **Connection Pooling** - Efficient connection reuse per service

### 🔑 Authentication System

- **Bearer Token Strategy** - Injects `Authorization: Bearer <token>`
- **Custom Header Strategy** - Injects any custom header (e.g., `X-API-Key: <token>`)
- **Case-Insensitive Header Handling** - Finds and replaces headers regardless of capitalization
- **Capitalization Preservation** - Replaces values while preserving client's header capitalization
- **Pluggable Architecture** - Easy to add OAuth2, HMAC, AWS SigV4, etc.

### 💾 Secret Management

- **Environment Variable Provider** (`env:VAR_NAME`) - Simple environment variable lookup
- **File-Based Provider** (`file:/path/to/secret`) - Strict permission checking (0600 required)
- **macOS Keychain Provider** (`keychain:service/account`) - Native macOS Keychain integration
- **In-Memory Caching** - Secrets cached after first fetch for performance
- **Startup Preloading** - All configured secrets loaded during initialization
- **Thread-Safe Access** - Concurrent access protection

### 📋 Policy Enforcement

- **Method Whitelisting** - Allow only specific HTTP methods (GET, POST, etc.)
- **Path Whitelisting** - URL pattern matching with wildcards (`/api/*`, `/v1/**`)
- **Body Size Limits** - Request body size limits (default 10MB)
- **Drop Patterns** - Block requests matching specific URL patterns (returns 403)
- **Header Stripping** - Remove specified headers from requests
- **Client Groups** - Client identification and grouping (planned)

### 🧙 Interactive Init Wizard

- **Heuristic-Based Auth Detection** - 4 confidence levels (100%, 90%, 70%, 60%)
- **Pattern Recognition** - Detects OpenAI keys (`sk-*`), bearer tokens, base64 strings
- **Real-Time Finding Reports** - Shows detected auth patterns as they happen
- **Policy Detection** - Automatically captures methods and paths used
- **Multiple Storage Options** - Keychain (most secure), file, or .env
- **Template Configs** - Pre-built configs for OpenAI, Anthropic
- **Non-Interactive Mode** - `--yes` flag for automation
- **Dry-Run Mode** - Preview config without saving


### 🔍 Examine Mode (Auth Discovery)

- **Zero-Config Operation** - Runs with minimal configuration
- **Passthrough MITM** - Intercepts everything but doesn't modify requests
- **Auth-Relevant Header Filtering** - Filters noise headers with glob patterns
- **Configurable Display** - Show/hide params, cookies, bodies, responses
- **File Output** - Save findings to file for analysis
- **Real-Time Logging** - Immediate feedback on detected patterns
- **HAR Recording** - Capture traffic in HTTP Archive format for Chrome DevTools/Firefox

### 📊 Logging & Monitoring

- **3 Log Formats** - Text (colored), JSON, logfmt
- **Request Correlation** - Unique request IDs for tracing
- **Color Correlation** - 20-color palette for visual request/response correlation
- **Sensitive Field Redaction** - Auto-redacts secrets, passwords, tokens, api_keys
- **HAR Recording** - HTTP Archive format for debugging (1.2 spec)
- **Structured Logging** - Context-aware with request ID propagation

### 🛠️ CLI Commands

- **`chaperone inject [service]`** - Start proxy with credential injection (main mode)
- **`chaperone examine`** - Auth discovery mode (passthrough MITM logging)
- **`chaperone check`** - Security posture assessment with recommendations
- **`chaperone init [service]`** - Interactive wizard or template-based config generation
- **`chaperone run`** - Deprecated alias for `inject` (backward compatibility)

## Installation

### From Releases (Recommended)

Download pre-built binaries from the [Releases page](https://github.com/bmf/chaperone/releases).

**macOS (Apple Silicon):**
```bash
curl -LO https://github.com/bmf/chaperone/releases/download/v0.1.0/chaperone-darwin-arm64
chmod +x chaperone-darwin-arm64
sudo mv chaperone-darwin-arm64 /usr/local/bin/chaperone
```

**macOS (Intel):**
```bash
curl -LO https://github.com/bmf/chaperone/releases/download/v0.1.0/chaperone-darwin-amd64
chmod +x chaperone-darwin-amd64
sudo mv chaperone-darwin-amd64 /usr/local/bin/chaperone
```

**Linux (x86_64):**
```bash
curl -LO https://github.com/bmf/chaperone/releases/download/v0.1.0/chaperone-linux-amd64
chmod +x chaperone-linux-amd64
sudo mv chaperone-linux-amd64 /usr/local/bin/chaperone
```

**Linux (ARM64):**
```bash
curl -LO https://github.com/bmf/chaperone/releases/download/v0.1.0/chaperone-linux-arm64
chmod +x chaperone-linux-arm64
sudo mv chaperone-linux-arm64 /usr/local/bin/chaperone
```

**Verify installation:**
```bash
chaperone version
# Output: chaperone version 0.1.0
```

### From Source

```bash
git clone https://github.com/bmf/chaperone.git
cd chaperone
make build
sudo mv ./chaperone /usr/local/bin/chaperone
```

**Requirements:**
- Go 1.25.4 or later
- Make (optional, can use `go build ./cmd/chaperone` directly)

## Quickstart

### Option 1: Interactive Wizard (Recommended)

The init wizard automatically detects authentication patterns:

```bash
# Run the wizard
./chaperone init

# Follow the prompts:
# 1. Configure proxy settings
# 2. Run detection proxy and make API calls
# 3. Review findings
# 4. Choose storage location (Keychain/file/.env)
# 5. Save configuration
```

### Option 2: Template Configuration

Generate a pre-configured template:

```bash
# For OpenAI
./chaperone init openai

# For Anthropic
./chaperone init anthropic
```

### Option 3: Manual Configuration

Create `chaperone.toml`:

```toml
[server]
address = "127.0.0.1"
port = 4010

[[services]]
name = "openai"
host_pattern = "api.openai.com"
auth_strategy = "bearer"
credential_ref = "env:OPENAI_API_KEY"
```

Set your API key:

```bash
export OPENAI_API_KEY=sk-your-key-here
```

### Start Chaperone

```bash
# Start with default config
./chaperone inject

# Start with specific config
./chaperone inject -c chaperone.toml

# Start specific service only
./chaperone inject openai

# Start in Unix socket mode (more secure)
./chaperone inject --socket /tmp/chaperone.sock
```

### Configure Your Application

Set the proxy environment variable:

```bash
export HTTPS_PROXY=http://127.0.0.1:4010

# Or for Unix socket mode
export HTTPS_PROXY=http://unix:/tmp/chaperone.sock
```

### Trust the CA Certificate (First Time Only)

Chaperone generates a CA certificate on first run. To avoid certificate warnings:

**macOS:**
```bash
sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain ~/.config/chaperone/ca-cert.pem
```

**Linux:**
```bash
sudo cp ~/.config/chaperone/ca-cert.pem /usr/local/share/ca-certificates/chaperone.crt
sudo update-ca-certificates
```

**Windows:**
```powershell
certutil -addstore -f "ROOT" %USERPROFILE%\.config\chaperone\ca-cert.pem
```

### Make API Calls

```bash
# The API key is injected automatically!
curl -X POST https://api.openai.com/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "gpt-4", "messages": [{"role": "user", "content": "Hello!"}]}'
```

## Configuration

### Complete Example

```toml
[server]
address = "127.0.0.1"
port = 4010
# Optional: Unix socket mode (more secure than TCP)
socket = "/tmp/chaperone.sock"

[logging]
level = "info"  # debug, info, warn, error
format = "text"  # text, json, logfmt

# Optional: Audit logging for security-relevant events
[audit]
enabled = true
path = "/var/log/chaperone/audit.log"  # or "stdout"

[[services]]
name = "openai"
host_pattern = "api.openai.com"
auth_strategy = "bearer"
credential_ref = "env:OPENAI_API_KEY"

# Optional: Placeholder token for process authentication (Layer 2 security)
placeholder = "chap_openai_dev_12345678"

# Optional: Policy enforcement
allowed_methods = ["GET", "POST"]
allowed_paths = ["/v1/*"]
max_body_bytes = 10485760  # 10MB

# Optional: Block specific patterns
drop = ["*/admin/*", "*/internal/*"]

# Optional: Strip specific headers
strip = ["X-Debug-Token", "X-Internal-Auth"]

[[services]]
name = "anthropic"
host_pattern = "api.anthropic.com"
auth_strategy = "header:x-api-key"
credential_ref = "keychain:chaperone/anthropic"
allowed_methods = ["POST"]
allowed_paths = ["/v1/messages", "/v1/complete"]
max_body_bytes = 5242880  # 5MB
```

### Server Configuration

| Field | Description | Default |
|-------|-------------|---------|
| `address` | Listen address | `127.0.0.1` |
| `port` | Listen port | `4010` |
| `socket` | Unix socket path (overrides address/port) | - |

**Unix Socket Mode:**
```toml
[server]
socket = "/tmp/chaperone.sock"
```

Benefits:
- No network exposure
- Filesystem-based permissions (0660)
- Better security through OS-level access control

### Logging Configuration

| Field | Description | Default |
|-------|-------------|---------|
| `level` | Log level (debug, info, warn, error) | `info` |
| `format` | Log format (text, json, logfmt) | `text` |

### Audit Logging

Comprehensive audit logging for all security-relevant events:

```toml
[audit]
enabled = true
path = "/var/log/chaperone/audit.log"  # or "stdout"
```

**Events logged:**
- `credential_injected` - Successful credential injection
- `placeholder_mismatch` - Placeholder token mismatch (Layer 2)
- `auth_header_stripped` - Known auth headers removed for security
- `policy_denied` - Request blocked by policy
- `request_dropped` - Request blocked by drop pattern

**Example audit log entries (FedRAMP AU-3 compliant):**

```json
// Successful credential injection
{
  "timestamp": "2026-01-11T04:30:45.123456Z",
  "event": "credential_injected",
  "service": "openai",
  "host": "api.openai.com",
  "path": "/v1/chat/completions",
  "method": "POST",
  "auth_strategy": "bearer",
  "request_id": "req_abc123",
  "client_ip": "127.0.0.1",
  "outcome": "success"
}

// Policy violation
{
  "timestamp": "2026-01-11T04:30:46.789012Z",
  "event": "policy_denied",
  "service": "openai",
  "host": "api.openai.com",
  "path": "/v1/admin/delete",
  "method": "DELETE",
  "request_id": "req_def456",
  "client_ip": "127.0.0.1",
  "outcome": "blocked",
  "detail": "method DELETE not in allowed_methods [GET POST]"
}

// Authentication failure
{
  "timestamp": "2026-01-11T04:30:47.345678Z",
  "event": "auth_failure",
  "service": "openai",
  "host": "api.openai.com",
  "path": "/v1/chat/completions",
  "method": "POST",
  "auth_strategy": "bearer",
  "request_id": "req_ghi789",
  "client_ip": "127.0.0.1",
  "outcome": "failure",
  "error": "secret not found: env:MISSING_KEY"
}
```

Audit log files are created with `0600` permissions (owner read-write only).

**AU-3 Field Compliance:**
- **WHO**: `client_ip` identifies the request source
- **WHAT**: `event` categorizes the security event type
- **WHEN**: `timestamp` in ISO 8601 UTC format with microsecond precision
- **WHERE**: `service`, `host`, `path` identify the target
- **OUTCOME**: `outcome` (success/failure/blocked), `error` for failure details

See [SECURITY.md](SECURITY.md) for complete audit event taxonomy and compliance mapping.

### Service Configuration

| Field | Description | Required |
|-------|-------------|----------|
| `name` | Service name (for logging/audit) | Yes |
| `host_pattern` | Domain pattern to match | Yes |
| `auth_strategy` | Authentication strategy | Yes |
| `credential_ref` | Secret provider reference | Yes |
| `placeholder` | Process authentication token | No |
| `allowed_methods` | Whitelisted HTTP methods | No |
| `allowed_paths` | Whitelisted URL patterns | No |
| `max_body_bytes` | Max request body size | No (default: 10MB) |
| `drop` | URL patterns to block | No |
| `strip` | Headers to remove | No |

### Secret Providers

| Provider | Format | Example | Security |
|----------|--------|---------|----------|
| Environment | `env:VAR_NAME` | `env:OPENAI_API_KEY` | Medium |
| File | `file:/path/to/secret` | `file:~/.secrets/api.key` | High |
| Keychain (macOS) | `keychain:service/account` | `keychain:chaperone/openai` | Highest |

#### Environment Variables

```bash
export OPENAI_API_KEY=sk-your-key-here
```

```toml
credential_ref = "env:OPENAI_API_KEY"
```

#### File-based Secrets

```bash
echo "sk-your-key-here" > ~/.secrets/openai.key
chmod 600 ~/.secrets/openai.key
```

```toml
credential_ref = "file:~/.secrets/openai.key"
```

**Security requirements:**
- Files must have permissions `0600` (rw-------) or stricter (`0400`)
- Files with `0644`, `0666`, `0777`, etc. are rejected
- Maximum file size: 1MB
- Whitespace is automatically trimmed

#### macOS Keychain (Most Secure)

```bash
# Add secret to keychain
security add-generic-password -s chaperone -a openai -w "sk-your-key-here"
```

```toml
credential_ref = "keychain:chaperone/openai"
```

**Benefits:**
- Only works on macOS
- Respects keychain access controls
- May prompt for keychain access on first use
- Secrets never written to disk in plain text
- Integration with macOS security (Touch ID, etc.)

**Managing keychain secrets:**

```bash
# Verify a secret
security find-generic-password -s chaperone -a openai -w

# Update a secret
security add-generic-password -s chaperone -a openai -w "new-key-value"

# Delete a secret
security delete-generic-password -s chaperone -a openai

# View in Keychain Access app
open -a "Keychain Access"
```

### Authentication Strategies

| Strategy | Format | Header Injected |
|----------|--------|-----------------|
| Bearer | `bearer` | `Authorization: Bearer <secret>` |
| Custom Header | `header:X-API-Key` | `X-API-Key: <secret>` |

**Examples:**

```toml
# Bearer token
auth_strategy = "bearer"
# Injects: Authorization: Bearer sk-...

# Custom header
auth_strategy = "header:x-api-key"
# Injects: x-api-key: sk-...

# Alternative syntax
auth_strategy = "header"
header_name = "x-api-key"
```

### Policy Enforcement

#### Allowed Methods

```toml
allowed_methods = ["GET", "POST"]
```

Requests with other methods return `403 Forbidden`.

#### Allowed Paths

```toml
allowed_paths = ["/v1/*", "/api/**"]
```

**Pattern syntax:**
- `*` - Single path segment (e.g., `/api/*` matches `/api/users` but not `/api/users/123`)
- `**` - Any depth (e.g., `/api/**` matches `/api/users/123/posts/456`)
- Patterns are tested in order; first match wins

Requests not matching any pattern return `403 Forbidden`.

#### Body Size Limits

```toml
max_body_bytes = 10485760  # 10MB
```

Requests exceeding limit return `413 Request Entity Too Large`.

#### Drop Patterns

```toml
drop = ["*/admin/*", "*/internal/*", "*/debug"]
```

Requests matching any drop pattern return `403 Forbidden` immediately (before credential injection).

#### Header Stripping

```toml
strip = ["X-Debug-Token", "X-Internal-Auth"]
```

Remove specified headers from requests before forwarding.

### Placeholder Authentication (Layer 2 Security)

Optional process-level authentication to prevent accidental credential injection:

```toml
[[services]]
name = "openai"
host_pattern = "api.openai.com"
auth_strategy = "bearer"
credential_ref = "env:OPENAI_API_KEY"
placeholder = "chap_openai_dev_12345678"  # Min 8 characters
```

**How it works:**
1. Application sends placeholder token instead of real API key
2. Chaperone checks if placeholder matches configuration
3. If match: Chaperone swaps placeholder for real credential
4. If mismatch: Request passes through unchanged (no injection)

**Example application code:**

```python
import openai

# Use placeholder instead of real API key
openai.api_key = "chap_openai_dev_12345678"

# Chaperone swaps this for the real key
response = openai.chat.completions.create(
    model="gpt-4",
    messages=[{"role": "user", "content": "Hello!"}]
)
```

**Benefits:**
- Prevents accidental credential injection into wrong processes
- Adds process-level authentication
- No real credentials in application memory
- Placeholders can be committed to version control safely

**Requirements:**
- Placeholder must be at least 8 characters
- Placeholder is not secret (it's a process identifier)
- Use descriptive names: `chap_<service>_<environment>_<random>`

## Security Model

Chaperone implements a **5-layer defense-in-depth security model**:

### Layer 1: Credential Isolation (Single Service Mode)
**Status:** ✅ Implemented

Run separate Chaperone instances per service:

```bash
# Terminal 1: OpenAI proxy
chaperone inject openai --socket /tmp/chaperone-openai.sock

# Terminal 2: Anthropic proxy
chaperone inject anthropic --socket /tmp/chaperone-anthropic.sock
```

**Benefits:**
- Each service gets dedicated proxy instance
- Compromise of one service doesn't expose others
- Clear security boundaries

### Layer 2: Process Authentication (Placeholder Tokens)
**Status:** ✅ Implemented

Configure placeholder tokens:

```toml
[[services]]
name = "openai"
placeholder = "chap_openai_dev_12345678"
```

**Benefits:**
- Only processes with correct placeholder get credentials
- Prevents accidental injection into wrong processes
- Adds authentication layer

### Layer 3: User/Permission Isolation (Unix Sockets + Dedicated User)
**Status:** ✅ Unix Socket Implemented | ⚠️ Dedicated User Documented

Use Unix sockets with filesystem permissions:

```bash
# Run as dedicated user
sudo -u chaperone chaperone inject --socket /var/run/chaperone/proxy.sock

# Socket created with 0660 (owner + group RW)
ls -l /var/run/chaperone/proxy.sock
# srw-rw---- 1 chaperone chaperone ... proxy.sock
```

**Benefits:**
- No network exposure
- OS-level access control
- Per-user/group permissions

### Layer 4: Network Hardening (TLS Validation + Proxy Bypass)
**Status:** ✅ Implemented

Chaperone ignores `HTTP_PROXY`/`HTTPS_PROXY` environment variables for upstream connections:

**Benefits:**
- Prevents routing through potentially malicious proxies
- Direct connection to upstream APIs
- TLS certificate validation enforced

### Layer 5: Runtime Protection (Audit Logging)
**Status:** ✅ Implemented

Enable audit logging:

```toml
[audit]
enabled = true
path = "/var/log/chaperone/audit.log"
```

**Benefits:**
- Complete audit trail of credential injections
- Security event monitoring
- Compliance support (FedRAMP AU-2/AU-3)
- Incident investigation

### Security Check Command

Assess your security posture:

```bash
chaperone check
```

Example output:

```
Chaperone Security Posture Assessment

Layer 1: Credential Isolation
  ⚠️  Multi-service mode (3 services configured)
  Recommendation: Run separate instances per service for maximum isolation

Layer 2: Process Authentication
  ⚠️  1 of 3 services use placeholder tokens
  Recommendation: Add placeholder tokens to all services

Layer 3: User/Permission Isolation
  ℹ️  Running as user: brandon (not dedicated user)
  ℹ️  TCP mode (127.0.0.1:4010)
  Recommendation: Use --socket flag for Unix socket mode

Layer 4: Network Hardening
  ✅ TLS validation enabled
  ✅ Upstream proxy bypass enabled

Layer 5: Runtime Protection
  ✅ Audit logging enabled (/var/log/chaperone/audit.log)
```

See [SECURITY.md](SECURITY.md) for complete security documentation.

## CLI Commands

### `chaperone inject [service]`

Start proxy with credential injection (main mode):

```bash
# Inject all configured services
chaperone inject

# Inject specific service only
chaperone inject openai

# Use custom config
chaperone inject -c custom.toml

# Unix socket mode
chaperone inject --socket /tmp/chaperone.sock

# With specific log level
chaperone inject --log-level debug
```

**Flags:**
- `-c, --config <path>` - Config file path (default: `~/.config/chaperone/chaperone.toml`)
- `--socket <path>` - Unix socket path (overrides config)
- `--log-level <level>` - Log level (debug, info, warn, error)
- `--log-format <format>` - Log format (text, json, logfmt)

### `chaperone examine`
### `chaperone examine`

Auth discovery mode (passthrough MITM logging):

```bash
# Basic examination (just headers)
chaperone examine

# Show query parameters
chaperone examine -p

# Show cookies
chaperone examine --show-cookies

# Show request/response bodies (truncated at 4KB)
chaperone examine -b

# Show response data (status, headers, cookies)
chaperone examine -r

# Save to file (enables all flags)
chaperone examine -o findings.txt

# Combine flags
chaperone examine -p -b -r --show-cookies

# Capture HAR (HTTP Archive) for debugging
chaperone examine --har

# Capture HAR with custom output path
chaperone examine --har --har-output traffic.har

# Combine HAR with other flags
chaperone examine -b -r --har --har-output session.har
```

**Flags:**
- `-p, --show-params` - Show query parameters
- `--show-cookies` - Show cookies
- `-b, --show-body` - Show request/response bodies
- `-r, --show-response` - Show response data
- `-o, --output <path>` - Save to file (enables all flags)
- `--har` - Enable HAR recording (HTTP Archive format)
- `--har-output <path>` - Custom HAR output file path (implies --har)
- `-c, --config <path>` - Config file path

**HAR Recording:**

The `--har` flag enables HTTP Archive (HAR) format recording:

```bash
# Default: chaperone-<timestamp>.har
chaperone examine --har

# Custom output path
chaperone examine --har-output debug-session.har
```

HAR files can be imported into:
- Chrome DevTools (Network tab → right-click → "Import HAR file")
- Firefox Developer Tools (Network tab → gear icon → "Import HAR")
- Charles Proxy, Fiddler, and other debugging tools

**How it works:**
1. Runs proxy in passthrough mode (doesn't modify requests)
2. Intercepts all traffic via MITM
3. Logs auth-relevant headers and data
4. Filters out noise (content-type, user-agent, x-stainless-*, etc.)
5. Shows findings in real-time
6. Optionally captures complete HAR archive on shutdown

**Use cases:**
- Discover authentication patterns for new APIs
- Debug authentication issues
- Understand what headers an application sends
- Capture traffic for offline analysis in browser DevTools

### `chaperone check`

Security posture assessment:

```bash
# Check security status
chaperone check

# Check with specific config
chaperone check -c chaperone.toml
```

Shows status for all 5 security layers with actionable recommendations.

**Exit code:** Always `0` (informational only, not a gate)

### `chaperone init [service]`

Interactive wizard or template-based config generation:

```bash
# Interactive wizard (auto-detection)
chaperone init

# Template for OpenAI
chaperone init openai

# Template for Anthropic
chaperone init anthropic
```

**Interactive wizard flags:**
- `--sentinel <value>` - Expected auth value for 100% confidence detection
- `--yes` - Non-interactive mode (use defaults)
- `--dry-run` - Preview config without saving

**Wizard steps:**
1. Configure proxy settings (address, port, sentinel)
2. Run detection proxy (make API calls in another terminal)
3. Review findings (confidence levels, methods, paths)
4. Configure service (name, credential storage)
5. Save configuration (choose location)

**Detection confidence levels:**
- **100% (Sentinel Match)** - Exact match with `--sentinel` value
- **90% (Known Auth Header)** - Matches common auth headers (Authorization, X-API-Key, etc.)
- **70% (Auth Keyword)** - Header name contains auth keywords
- **60% (Value Pattern)** - Matches credential formats (sk-, Bearer, base64-like)

**Credential storage options:**
1. **macOS Keychain** (most secure) - Native keychain integration
2. **File** - `~/.config/chaperone/secrets/<service>` with 0600 permissions
3. **.env File** - Appends to `.env` in current directory

### `chaperone run`

Deprecated alias for `inject` (backward compatibility):

```bash
chaperone run  # Same as: chaperone inject
```

## CLI Reference

**Global flags:**
- `-c, --config <path>` - Config file path
- `--log-level <level>` - Log level (debug, info, warn, error)
- `--log-format <format>` - Log format (text, json, logfmt)
- `--version` - Show version
- `--help` - Show help

**Config file resolution:**
1. `-c` flag path
2. `~/.config/chaperone/chaperone.toml`
3. `./chaperone.toml` (current directory)

## How It Works

### Request Flow (Inject Mode)

1. **Application sends HTTPS request** via proxy (e.g., `HTTPS_PROXY=http://127.0.0.1:4010`)
2. **Chaperone receives CONNECT request** for target host (e.g., `api.openai.com`)
3. **MITM decision:**
   - If host matches configured service → Terminate TLS (MITM)
   - If host not configured → Transparent tunnel (passthrough)
4. **For MITM requests:**
   - Terminate TLS with application (using generated certificate)
   - Read HTTP request from application
   - **Security stripping:** Remove 13 common auth headers automatically
   - **Policy enforcement:** Check allowed methods, paths, body size
   - **Placeholder verification:** Check if placeholder matches (if configured)
   - **Service lookup:** Find service configuration by host
   - **Secret fetch:** Retrieve credential from provider (env, file, keychain)
   - **Auth injection:** Apply strategy (bearer, custom header)
   - **Audit logging:** Log credential injection event
   - Forward authenticated request to upstream over TLS
   - Stream response back to application
5. **For passthrough requests:**
   - Create TCP tunnel between client and upstream
   - No credential injection, no logging

### MITM Certificate Handling

1. **CA Generation (first run):**
   - Generate root CA certificate
   - Save to `~/.config/chaperone/ca-cert.pem` and `ca-key.pem`
   - CA valid for 10 years

2. **Per-hostname Certificates:**
   - Generate certificate for each unique hostname
   - Sign with root CA
   - Cache in memory for reuse
   - Certificate valid for 1 year

3. **Trust Setup:**
   - User must trust CA certificate once
   - All generated certificates automatically trusted
   - Works with browsers, command-line tools, applications

### Security Stripping

Chaperone automatically removes these headers from ALL configured service requests:

- `authorization`
- `x-api-key`
- `x-auth-token`
- `api-key`
- `apikey`
- `x-access-token`
- `x-token`
- `token`
- `bearer`
- `x-session-token`
- `x-csrf-token`
- `x-xsrf-token`

**Why?** Prevents applications from accidentally sending credentials to wrong APIs.

**Exception:** Valid placeholder tokens are NOT stripped (they need to reach authHandler for verification).

## Use Cases

### 1. Local Development

Keep secrets out of source code:

```python
# No API key in code!
import openai

# Chaperone injects the key
response = openai.chat.completions.create(
    model="gpt-4",
    messages=[{"role": "user", "content": "Hello!"}]
)
```

### 2. CI/CD Pipelines

Centralized credential management:

```yaml
# .github/workflows/test.yml
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - name: Start Chaperone
        run: |
          chaperone inject --socket /tmp/chaperone.sock &
          export HTTPS_PROXY=http://unix:/tmp/chaperone.sock

      - name: Run tests
        run: npm test  # Tests use proxy automatically
```

### 3. Multi-Tenant Applications

Isolate credentials per tenant:

```bash
# Tenant A proxy
chaperone inject --config tenant-a.toml --socket /tmp/chaperone-a.sock

# Tenant B proxy
chaperone inject --config tenant-b.toml --socket /tmp/chaperone-b.sock
```

### 4. Security Auditing

Complete audit trail of API usage:

```toml
[audit]
enabled = true
path = "/var/log/chaperone/audit.log"
```

Parse with jq:

```bash
# Show all credential injections today
cat /var/log/chaperone/audit.log \
  | jq 'select(.event == "credential_injected")' \
  | jq 'select(.timestamp | startswith("2026-01-11"))'

# Count injections per service
cat /var/log/chaperone/audit.log \
  | jq -r '.service' \
  | sort | uniq -c
```

### 5. API Discovery

Discover authentication for new APIs:

```bash
# Start examine mode
chaperone examine -b -r -o findings.txt

# In another terminal, run your application
python my_app.py

# Review findings.txt to see authentication patterns
```

## Development

### Building

```bash
# Build binary
go build ./cmd/chaperone

# Build with version info
go build -ldflags "-X main.version=1.0.0" ./cmd/chaperone

# Build for multiple platforms
GOOS=linux GOARCH=amd64 go build -o chaperone-linux-amd64 ./cmd/chaperone
GOOS=darwin GOARCH=arm64 go build -o chaperone-darwin-arm64 ./cmd/chaperone
GOOS=windows GOARCH=amd64 go build -o chaperone-windows-amd64.exe ./cmd/chaperone
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test ./... -coverprofile=coverage.out -coverpkg=./...
go tool cover -html=coverage.out

# Run with race detector
go test ./... -race

# Run specific test
go test ./internal/proxy -run TestAuthHandler

# Run integration tests only
go test ./test/integration/...
```

### Project Structure

```
chaperone-auth-gateway/
├── cmd/chaperone/          # CLI entry point
│   └── cmd/                # Cobra commands (inject, examine, check, init)
├── internal/
│   ├── audit/              # Audit logging
│   ├── auth/               # Authentication strategies
│   ├── client/             # Upstream HTTP client
│   ├── config/             # Configuration parsing
│   ├── examine/            # Examine mode (auth discovery)
│   ├── init/               # Init wizard (auto-detection)
│   ├── log/                # Structured logging
│   ├── mitm/               # MITM certificate management
│   ├── proxy/              # Core proxy server
│   ├── recorder/           # HAR recording
│   ├── secrets/            # Secret providers
│   ├── service/            # Service registry and policy
│   └── shutdown/           # Graceful shutdown
├── test/                   # Integration tests
├── SECURITY.md             # Security documentation
├── ROADMAP.md              # Feature roadmap
└── chaperone.toml          # Example configuration
```

### Adding New Features

**New Authentication Strategy:**

1. Implement `auth.Strategy` interface in `internal/auth/`
2. Register strategy in `internal/auth/registry.go`
3. Add tests
4. Update documentation

**New Secret Provider:**

1. Implement `secrets.Provider` interface in `internal/secrets/`
2. Register provider in `internal/secrets/registry.go`
3. Add tests
4. Update documentation

**New Policy:**

1. Add policy field to `service.Policy` in `internal/service/policy.go`
2. Implement enforcement in `internal/service/policy_enforcer.go`
3. Add tests
4. Update configuration schema

## Troubleshooting

### Certificate Errors

**Problem:** Browser/application doesn't trust certificates

**Solution:** Trust the CA certificate:

```bash
# macOS
sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain ~/.config/chaperone/ca-cert.pem

# Linux
sudo cp ~/.config/chaperone/ca-cert.pem /usr/local/share/ca-certificates/chaperone.crt
sudo update-ca-certificates

# Windows
certutil -addstore -f "ROOT" %USERPROFILE%\.config\chaperone\ca-cert.pem
```

### No Credentials Injected

**Problem:** Requests reach upstream without authentication

**Check:**
1. Is the service configured? `chaperone check`
2. Does the host pattern match? Check proxy logs
3. Is the secret provider working? Test with `env` provider first
4. Is placeholder configured but missing from request? Check `placeholder` field

### Permission Denied Errors

**Problem:** Cannot read secret file

**Solution:** Fix file permissions:

```bash
chmod 600 ~/.secrets/api.key
```

**Problem:** Cannot create Unix socket

**Solution:** Check directory permissions:

```bash
mkdir -p /tmp/chaperone
chmod 755 /tmp/chaperone
chaperone inject --socket /tmp/chaperone/proxy.sock
```

### Proxy Not Working

**Problem:** Application doesn't use proxy

**Check:**
1. Is `HTTPS_PROXY` set? `echo $HTTPS_PROXY`
2. Does application respect proxy settings? (Some don't)
3. Is proxy running? `lsof -i :4010` or `ls -l /tmp/chaperone.sock`

**Alternative:** Use application-specific proxy settings:

```bash
# curl
curl --proxy http://127.0.0.1:4010 https://api.openai.com/v1/models

# Python requests
import requests
proxies = {"https": "http://127.0.0.1:4010"}
requests.get("https://api.openai.com/v1/models", proxies=proxies)

# Node.js
process.env.HTTPS_PROXY = "http://127.0.0.1:4010"
```

## Performance

Chaperone adds minimal latency:

- **CA generation:** One-time ~100ms (first run only)
- **Certificate generation:** ~5-10ms per unique hostname (cached)
- **Request overhead:** <1ms (secret lookup, header injection)
- **Streaming:** Zero buffering, byte-for-byte proxy

**Benchmarks (M1 MacBook Pro):**

- 1000 requests/sec sustained
- <2ms p95 latency
- <5MB memory per service
- 10,000+ concurrent connections

## Contributing

Contributions welcome! Please:

1. Read [SECURITY.md](SECURITY.md) for security guidelines
2. Write tests for new features
3. Follow existing code style
4. Update documentation
5. Add entry to CHANGELOG.md

## Roadmap

See [ROADMAP.md](ROADMAP.md) for planned features:

- ✅ Placeholder authentication
- ✅ Audit logging
- ✅ Unix socket mode
- ✅ Security check command
- ⚠️ Dedicated user mode (documented, not automated)
- 🔄 Memory protection (mlock)
- 🔄 Secure credential zeroing
- 🔄 Credential rotation hooks

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Security

For security issues, please email security@example.com (do not open public issues).

See [SECURITY.md](SECURITY.md) for:
- Security model and threat analysis
- Defense-in-depth architecture
- Secure setup guide
- Compliance mapping (FedRAMP AU-2/AU-3)
- Security best practices

## Acknowledgments

Built with:
- [elazarl/goproxy](https://github.com/elazarl/goproxy) - Go HTTP proxy library
- [spf13/cobra](https://github.com/spf13/cobra) - CLI framework
- [BurntSushi/toml](https://github.com/BurntSushi/toml) - TOML parser

Inspired by:
- AWS IAM Roles for Service Accounts
- Kubernetes service accounts
- HashiCorp Vault Agent
