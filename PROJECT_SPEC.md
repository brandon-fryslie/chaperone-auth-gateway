Chaperone: A Local Credential-Injecting HTTPS Proxy

Architecture, Specification, and Operational Model

⸻

Overview

Chaperone is a lightweight, Go-based, local HTTPS proxy that securely manages API credentials on behalf of applications and developer tools. It isolates API secrets from untrusted code by placing a transparent, policy-enforced, outbound request broker between applications and the APIs they consume.

Chaperone handles:
 • Secret storage (via pluggable backends)
 • Authentication injection (Bearer tokens, API keys, HMAC, OAuth2 client creds, etc.)
 • Selective TLS termination (only for configured domains)
 • Per-service routing and policy
 • Streaming-safe request/response proxying
 • Audit logging and access controls

Critically, Chaperone requires no code changes for most applications. It operates at the OS proxy layer, transparently brokering outbound API calls.

⸻

Goals and Guarantees

Chaperone is designed to:
 1. Eliminate API keys from application runtime environments
 • Keys are never loaded into app memory, environment variables, or config files.
 2. Provide strong process isolation
 • Only Chaperone holds secrets; apps interact solely via normal HTTPS proxying.
 3. Support all HTTP APIs, including streaming
 • Chaperone proxies streaming responses byte-for-byte with backpressure.
 4. Be secure, ergonomic, and operationally simple
 • A single Go binary, declarative config, pluggable secret backends, and minimal moving parts.
 5. Be compatible with enterprise proxies (e.g., Zscaler)
 • Outbound TLS is untouched; Chaperone acts as a standard HTTPS client.

⸻

Core Architecture

Chaperone consists of the following fundamental components:

1. Proxy Frontend

A Go-based HTTP(S) proxy that supports:
 • Standard HTTP proxying for HTTP traffic
 • CONNECT tunneling for HTTPS traffic
 • Automatic upgrade to selective MITM for configured domains

Applications are pointed to Chaperone via either:
 • OS proxy settings,
 • Environment variables (HTTP_PROXY, HTTPS_PROXY), or
 • Wrapper scripts.

2. Selective MITM Engine

Chaperone terminates TLS only for configured API hosts.

When an application issues:

CONNECT api.service.com:443

Chaperone:
 1. Identifies whether api.service.com is a managed domain.
 2. If yes, performs TLS MITM:
 • Presents a dynamically generated certificate signed by Chaperone’s local CA.
 • Decrypts the HTTP layer.
 • Applies service policy and authentication.
 3. If not, tunnels bytes without inspection.

3. Service Engine

Defines all behavior for each managed API host.

A service entry includes:
 • host_pattern (e.g., api.openai.com)
 • auth_strategy (bearer, header template, HMAC, AWS SigV4, OAuth2 client creds, etc.)
 • credential_ref (reference to a secret in a backend)
 • Optional:
 • request transforms
 • rate limits
 • path/method allowances
 • payload size caps
 • client-group access control

Services map inbound requests to outbound requests with policy enforcement.

4. Secret Providers (Pluggable)

Chaperone retrieves secrets from any number of pluggable backends:
 • env: — environment variable
 • file: — local file with strict permissions
 • keychain: — macOS Keychain, gnome-keyring, kwallet
 • command: — shell command returning secret (for: pass, 1Password CLI, Bitwarden CLI, Vault CLI)
 • vault: — direct Vault KV2 integration

No secrets ever enter app memory.

5. Outbound HTTPS Client

Chaperone behaves like a standard HTTPS client, using:
 • system trust store
 • standard TLS handshakes
 • normal certificate validation
 • pooled connections per-service

Outbound traffic is untouched by Chaperone’s local CA and is fully compatible with corporate TLS MITM tools.

6. Audit & Policy Layer

Every proxied request generates structured logs:

{
  "timestamp": "...",
  "request_id": "...",
  "client_id": "neovim",
  "service": "openai",
  "host": "api.openai.com",
  "method": "POST",
  "path": "/v1/chat/completions",
  "status": 200,
  "request_bytes": 512,
  "response_bytes": 10240,
  "duration_ms": 312,
  "policy": { "rate_limited": false, "denied": false }
}

Logs can be written to:
 • stdout
 • file with rotation
 • syslog
 • OpenTelemetry export targets

⸻

Data Flow

Normal HTTPS Request (Managed Domain)
 1. App issues CONNECT api.openai.com:443 via OS proxy.
 2. Chaperone accepts and establishes a TLS session with the app.
 3. App sends:

POST /v1/... HTTP/1.1
Host: api.openai.com
...

 4. Chaperone decrypts request.
 5. Chaperone loads service config → fetches secret → injects auth.
 6. Chaperone initiates outbound TLS to real api.openai.com.
 7. Chaperone forwards request.
 8. Chaperone streams response back through inbound TLS to the app.

Applications are completely unaware of Chaperone’s role.

⸻

Service Configuration Model

Below is the conceptual (not literal) TOML structure of a service config:

[service.openai]
host_pattern = "api.openai.com"
auth_strategy = "bearer"
credential_ref = "keychain:chaperone/openai"

[service.openai.policy]
allowed_methods = ["POST"]
allowed_paths = ["/v1/*", "/responses/*"]
max_body_bytes = 2097152
client_groups = ["devtools"]

More complex services can support:
 • path rewriting
 • OAuth2 token fetching/refresh
 • custom headers based on request content
 • HMAC signatures or AWS SigV4
 • rate limiting
 • quota enforcement

Everything is declarative.

⸻

Secret Provider Model

A credential_ref is an opaque string that maps directly to a backend provider. Examples:
 • env:OPENAI_API_KEY
 • file:/Users/me/.config/chaperone/internal.key
 • keychain:chaperone/openai
 • command:pass show apis/openai
 • vault:kv/data/apis/internal#token

Chaperone retrieves secrets at request time or caches briefly depending on backend.

⸻

Authentication Strategies

Chaperone supports pluggable auth strategies, including but not limited to:
 • Bearer tokens
 • Injects: Authorization: Bearer <value>
 • Header template

X-API-Key: <value>

 • Query parameter injection

<https://service.com?key=><value>

 • HMAC signing
 • Uses secret as HMAC key to sign specific headers or payloads.
 • AWS SigV4
 • Required for S3, Dynamo, etc.
 • OAuth2 Client Credentials flow
 • Chaperone obtains and caches short-lived tokens.

New strategies can be added with minimal Go code.

⸻

Client Identification & Access Control

Chaperone can identify clients via:
 • Proxy URL:

HTTPS_PROXY=http://client_id@127.0.0.1:4010

 • Unix domain socket filesystem permissions
 • Optional token-based client auth

Clients are mapped into client groups, and services define access based on group membership.

⸻

Operational Model

Installation & Quickstart
 1. Install Chaperone binary.
 2. Run:

chaperone init openai
chaperone run

 3. Configure OS or app to use:

HTTPS_PROXY=<http://127.0.0.1:4010>

That’s it.

Rotation

To rotate keys:
 • Rotate in backend (e.g. Keychain, Vault, etc.).
 • No code or app changes.
 • Chaperone immediately uses the new secret.

Deployment Models
 • Single user, local machine — preferred.
 • Shared developer workstation — socket permissions isolate users.
 • Team-dev environment — managed config in Git.
 • CI — ephemeral instance with restricted config.

Security Hardening
 • Go binary with minimal dependencies
 • Strict inbound TLS termination only for allowlisted hosts
 • No outbound certificate rewriting
 • No dynamic code execution
 • No plugin runtime loading
 • Config-based control only

This ensures low attack surface and predictable behavior.

⸻

Why Chaperone Is Secure
 • Secrets never leave secure storage.
 • Apps never receive or hold API tokens.
 • Process isolation ensures that even compromised plugins/tools cannot exfiltrate secrets.
 • TLS is handled cleanly:
 • inbound MITM → controlled by Chaperone
 • outbound TLS → standard, Zscaler-compatible
 • Policy enforcement prevents misuse:
 • path restrictions
 • method restrictions
 • payload size caps
 • Audit logs give visibility into all API usage.
 • Easy rotation without touching app environments.

⸻

Why Chaperone Is Useful

For developers:
 • Zero code changes for most tools
 • Unified secret handling
 • Drop-in use for multiple APIs
 • Works across languages and frameworks
 • Prevents accidental secret leakage in logs, env, configs

For enterprises:
 • Centralized control over which tools can call which APIs
 • Full audit trail
 • Minimal operational burden
 • Works behind corporate MITM proxies
 • Encourages best practices without complexity

⸻

Conclusion

Chaperone is a compact, Go-based local HTTPS MITM proxy designed specifically to eliminate API key leakage and enforce outbound API security policy. It abstracts secrets away from applications, adds centralized auditing, supports all HTTP APIs transparently, and integrates seamlessly with OS proxy settings and enterprise environments.

It is secure, minimal, flexible, streaming-safe, and deploys with almost no friction: a single binary, a simple config, and a powerful security model built around the correct separation of privilege.

Affirmative factual statement:
Chaperone defines a complete, secure, and flexible architecture for isolating API credentials behind a transparent, policy-controlled HTTPS proxy.
