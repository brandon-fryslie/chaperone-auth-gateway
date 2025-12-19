# Chaperone

A transparent local HTTPS proxy that automatically injects API credentials into requests on behalf of applications.

## Overview

Chaperone securely manages API credentials by acting as a local proxy that:

- **Intercepts** HTTPS requests from applications
- **Injects** API credentials from secure sources (environment variables, files, keychains)
- **Forwards** authenticated requests to upstream APIs
- **Logs** all requests for audit and debugging

Applications never see or handle API keys - Chaperone injects them transparently.

## Features

- **Transparent Operation** - Works at the OS proxy layer; most applications need zero code changes
- **Selective MITM** - Only terminates TLS for configured domains; other traffic passes through untouched
- **Pluggable Authentication** - Bearer tokens, custom headers, extensible for OAuth2/HMAC/AWS SigV4
- **Pluggable Secret Providers** - Environment variables, files, macOS Keychain, HashiCorp Vault
- **Policy Enforcement** - Path restrictions, method restrictions, body size limits
- **Structured Logging** - JSON audit logs with request/response details

## Installation

### From Source

```bash
git clone https://github.com/bmf/chaperone.git
cd chaperone
make build
```

### From Releases

Download the latest release from the [Releases page](https://github.com/bmf/chaperone/releases).

## Quickstart

### 1. Initialize Configuration

```bash
./chaperone init openai
```

This creates `chaperone.toml` with a default configuration for OpenAI.

### 2. Set Your API Key

```bash
export OPENAI_API_KEY=sk-your-key-here
```

### 3. Start Chaperone

```bash
./chaperone run --config chaperone.toml
```

### 4. Configure Your Application

Set the proxy environment variable:

```bash
export HTTPS_PROXY=http://127.0.0.1:4010
```

### 5. Trust the CA Certificate (First Time Only)

Chaperone generates a CA certificate on first run. To avoid certificate warnings:

**macOS:**
```bash
sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain ~/.config/chaperone/ca.crt
```

**Linux:**
```bash
sudo cp ~/.config/chaperone/ca.crt /usr/local/share/ca-certificates/chaperone.crt
sudo update-ca-certificates
```

### 6. Make API Calls

```bash
# The API key is injected automatically!
curl -X POST https://api.openai.com/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "gpt-4", "messages": [{"role": "user", "content": "Hello!"}]}'
```

## Configuration

Chaperone uses TOML configuration. Here's a complete example:

```toml
[server]
address = "127.0.0.1"
port = 4010

[logging]
level = "info"

[[services]]
name = "openai"
host_pattern = "api.openai.com"
auth_strategy = "bearer"
credential_ref = "env:OPENAI_API_KEY"

[[services]]
name = "anthropic"
host_pattern = "api.anthropic.com"
auth_strategy = "header"
header_name = "x-api-key"
credential_ref = "env:ANTHROPIC_API_KEY"
allowed_methods = ["POST"]
allowed_paths = ["/v1/*"]
```

### Secret Providers

| Provider | Format | Example |
|----------|--------|---------|
| Environment | `env:VAR_NAME` | `env:OPENAI_API_KEY` |
| File | `file:/path/to/secret` | `file:~/.secrets/api.key` |
| Keychain (macOS) | `keychain:service/account` | `keychain:chaperone/openai` |
| Command | `command:cmd args` | `command:pass show api/openai` |

#### Environment Variables

Store secrets in environment variables:

```bash
export OPENAI_API_KEY=sk-your-key-here
```

Configuration:
```toml
credential_ref = "env:OPENAI_API_KEY"
```

#### File-based Secrets

Store secrets in files with strict permissions (0600 or stricter):

```bash
echo "sk-your-key-here" > ~/.secrets/openai.key
chmod 600 ~/.secrets/openai.key
```

Configuration:
```toml
credential_ref = "file:~/.secrets/openai.key"
```

Security notes:
- Files must have permissions 0600 (rw-------) or stricter (0400)
- Files with 0644, 0666, 0777, etc. are rejected
- Maximum file size: 1MB
- Whitespace is automatically trimmed

#### macOS Keychain

Store secrets in macOS Keychain for maximum security (macOS only):

```bash
# Add secret to keychain
security add-generic-password -s chaperone -a openai -w "sk-your-key-here"
```

Configuration:
```toml
credential_ref = "keychain:chaperone/openai"
```

The format is `keychain:service/account` where:
- `service` is the keychain service name (e.g., "chaperone")
- `account` is the account name (e.g., "openai")

Security notes:
- Only works on macOS
- Respects keychain access controls
- May prompt for keychain access on first use
- Secrets never written to disk in plain text
- Integration with macOS security features (Touch ID, etc.)

To view/edit keychain items:
```bash
# View in Keychain Access app
open -a "Keychain Access"

# Or use command line
security find-generic-password -s chaperone -a openai -w
```

To remove a keychain item:
```bash
security delete-generic-password -s chaperone -a openai
```

### Authentication Strategies

| Strategy | Header Injected |
|----------|-----------------|
| `bearer` | `Authorization: Bearer <secret>` |
| `header` | `<header_name>: <secret>` |

## Security

Chaperone is designed with security as a primary concern:

- **Secrets never enter application memory** - Only Chaperone handles credentials
- **Process isolation** - Compromised applications cannot access secrets
- **Strict file permissions** - Secret files must be 0600 or stricter
- **No secret logging** - Credentials are never written to logs
- **Selective TLS termination** - Only configured domains are intercepted
- **Standard outbound TLS** - Works with enterprise proxies (Zscaler, etc.)

## CLI Reference

```bash
# Initialize a new configuration
chaperone init [service-name]

# Start the proxy
chaperone run --config chaperone.toml

# Configure system proxy (macOS)
chaperone setup proxy

# Show version
chaperone --version
```

## How It Works

1. Application sends HTTPS request via proxy
2. Chaperone receives `CONNECT` request for target host
3. For configured domains, Chaperone:
   - Terminates TLS with application (using generated certificate)
   - Reads HTTP request
   - Looks up service configuration
   - Fetches secret from configured provider
   - Injects authentication headers
   - Forwards to real upstream over TLS
   - Streams response back to application
4. For non-configured domains, traffic passes through unchanged

## Development

### Running Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test ./... -coverprofile=coverage.out -coverpkg=./...
go tool cover -html=coverage.out

# Run with race detector
go test ./... -race
```

### Building

```bash
# Build binary
go build ./cmd/chaperone

# Build with version info
go build -ldflags "-X main.version=1.0.0" ./cmd/chaperone
```

## License

MIT License - see LICENSE file for details
