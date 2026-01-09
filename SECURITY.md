# Chaperone Security Model

## Overview

Chaperone is a local proxy that injects API credentials into outgoing requests. Your applications connect through Chaperone and never have direct access to your API keys—they stay safely stored and managed by Chaperone.

This document explains our security model, what we protect against, and how to configure Chaperone for maximum security.

## Philosophy

Security is layered, not all-or-nothing. Each layer provides independent protection:

- **Layer 1 alone** already prevents apps from seeing your credentials
- **Adding Layer 2** prevents unauthorized processes from using the proxy
- **Adding Layers 3-5** hardens against increasingly sophisticated attacks

We don't pester you about security. Run `chaperone check` to see what you can improve—no judgment, just information.

## Threat Model

### What Chaperone Protects Against

| Threat | Protection |
|--------|------------|
| App exfiltrates API keys | Keys never reach the app |
| Malicious dependency steals credentials | Dependency never sees credentials |
| Compromised app abuses API | Blast radius limited to one service |
| Accidental credential exposure in logs | App logs placeholder, not real key |
| Credential in version control | Only placeholder committed |

### What Chaperone Does NOT Protect Against

| Threat | Why |
|--------|-----|
| Root/admin compromise | Game over for any security model |
| Malicious app abuses API through proxy | App can still make requests (but can't steal key) |
| Network-level attacks (with CA compromise) | Requires trusting a malicious CA |

## Defense in Depth

### Layer 1: Credential Isolation (Core)

**What:** Each Chaperone instance handles exactly one service and one credential.

**Why:** Even if something goes wrong, only one API key is exposed.

**Status:** ✅ Implemented (single-service mode)

---

### Layer 2: Process Authentication

**What:** Apps must send a placeholder token that Chaperone recognizes. Requests without the correct placeholder don't get credentials injected.

**Why:** Prevents random processes from accidentally (or maliciously) getting credentials injected.

**How it works:**
1. You configure your app with a placeholder: `OPENAI_API_KEY=chap_xxxxx`
2. App sends requests with the placeholder
3. Chaperone recognizes the placeholder, swaps in real credential
4. Requests without placeholder pass through unchanged (no injection)

**Status:** ✅ Implemented (placeholder authentication)

---

### Layer 3: User/Permission Isolation

**What:** Chaperone runs as a dedicated unprivileged user. Credential files are only readable by that user. Communication via Unix socket with group permissions.

**Why:** Even if your user account is compromised, attacker can't read credential files.

**How it works:**
```
User: chaperone (unprivileged)
  Owns: ~/.chaperone/secrets/
  Runs: chaperone process

User: you
  Can: Connect to Unix socket (group member)
  Cannot: Read credential files
```

**Status:** 🔲 Planned

---

### Layer 4: Network Hardening

**What:**
- Ship our own CA bundle (don't trust system CAs)
- Ignore system proxy settings for upstream connections
- Validate upstream TLS certificates

**Why:** Prevents MITM attacks via CA compromise, proxy chaining, or DNS hijacking.

| Measure | Protection | Status |
|---------|------------|--------|
| Own CA bundle | Attacker's CA in system store doesn't affect us | 🔲 Planned |
| Ignore system proxy | HTTP_PROXY can't redirect our upstream traffic | ✅ Implemented |
| Upstream TLS validation | DNS/hosts redirect fails (bad cert) | ✅ Implemented |

---

### Layer 5: Runtime Protection

**What:**
- Audit log all credential injections
- Lock credential memory (prevent swap to disk) - Linux only
- Zero credentials in memory after use - Linux only

**Why:** Provides forensic trail and protects against memory dump attacks.

| Measure | Protection | Status |
|---------|------------|--------|
| Audit logging | Forensic trail of credential injections | ✅ Implemented |
| Memory locking | Prevent swap to disk (Linux only) | 🔲 Planned |
| Memory zeroing | Clear credentials after use (Linux only) | 🔲 Planned |

---

## Security Posture Check

Run `chaperone check` to assess your security configuration:

```bash
$ chaperone check

Chaperone Security Check
========================

✅ Single-service mode: Only one credential per instance
✅ Upstream TLS: Certificates validated
⚠️  Running as current user (consider dedicated user)
⚠️  Using TCP port (consider Unix socket)
⚠️  Using system CA bundle (consider bundled CAs)
ℹ️  Placeholder auth: Not configured
ℹ️  Audit logging: Not enabled

Recommendations:
  • Run as dedicated 'chaperone' user for credential isolation
  • Use Unix socket instead of TCP port
  • Enable audit logging with --audit-log
  • See: chaperone docs security
```

**Status:** ✅ Implemented

---

## Secure Setup Guide

### Quick Start (Good Security)

```bash
chaperone init
chaperone inject
```

This gives you:
- ✅ Credential isolation (apps never see keys)
- ✅ Upstream TLS validation

### Enhanced Security

```bash
# Install with dedicated user
sudo chaperone install --create-user

# Run as dedicated user with Unix socket
chaperone inject --socket /run/chaperone/proxy.sock
```

This adds:
- ✅ File-level credential isolation
- ✅ Unix socket (no network exposure)

### Maximum Security

```bash
# All hardening options
chaperone inject \
  --socket /run/chaperone/proxy.sock \
  --bundled-ca \
  --audit-log /var/log/chaperone/audit.log
```

This adds:
- ✅ Own CA bundle
- ✅ Audit logging

---

## Reporting Vulnerabilities

If you discover a security vulnerability in Chaperone, please report it responsibly:

1. **Do not** open a public GitHub issue
2. Email: security@[project-domain] (TODO: set up)
3. Include: description, reproduction steps, impact assessment

We aim to respond within 48 hours and provide a fix within 7 days for critical issues.
