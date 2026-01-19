# Chaperone v0.1.0 - Initial Release 🎉

Chaperone is a transparent local HTTPS proxy that automatically injects API credentials into requests on behalf of applications. Applications never see or handle API keys - Chaperone injects them transparently with comprehensive security controls and audit logging.

## 🚀 What is Chaperone?

A Man-in-the-Middle (MITM) proxy that:
- **Intercepts** HTTPS requests using selective TLS termination
- **Injects** API credentials from secure sources (environment, files, macOS Keychain)
- **Enforces** security policies (allowed methods, paths, body size limits)
- **Audits** all credential injections for compliance monitoring
- **Protects** against credential leakage through automatic header stripping

## ✨ Key Features

### Security (5-Layer Defense-in-Depth)
- **Layer 1: Credential Isolation** - Apps never see API keys
- **Layer 2: Placeholder Token Authentication** - Process-level authentication
- **Layer 3: User/Permission Isolation** - Dedicated user accounts for production
- **Layer 4: Network Hardening** - Unix socket mode (no TCP exposure)
- **Layer 5: Audit Logging** - Comprehensive JSON audit trail (FedRAMP AU-3 compliant)

### Core Features
- ✅ Selective MITM - Only configured domains intercepted; others pass through untouched
- ✅ Pluggable Authentication - Bearer tokens, custom headers, easily extensible
- ✅ Flexible Secret Management - Environment vars, files, macOS Keychain
- ✅ Policy Enforcement - Method/path whitelisting, body size limits
- ✅ Interactive Init Wizard - Heuristic auth detection with 4 confidence levels
- ✅ Examine Mode - Auth discovery with intelligent header filtering
- ✅ Security Assessment - `chaperone check` shows security posture
- ✅ HAR Recording - Capture traffic for debugging

## 📦 Installation

### Pre-Built Binaries (Recommended)

**macOS (Apple Silicon):**
```bash
curl -LO https://github.com/bmf/chaperone/releases/download/v0.1.0/chaperone-darwin-arm64
chmod +x chaperone-darwin-arm64
sudo mv chaperone-darwin-arm64 /usr/local/bin/chaperone
chaperone version  # Verify: chaperone version 0.1.0
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

### Verify Installation
```bash
chaperone version
# Output: chaperone version 0.1.0
```

## 🎯 Quick Start

### 1. Interactive Wizard (Recommended)
```bash
chaperone init
# Follow prompts to auto-detect auth and create config
```

### 2. Template Configuration
```bash
chaperone init openai     # Pre-configured for OpenAI
chaperone init anthropic  # Pre-configured for Anthropic
```

### 3. Start the Proxy
```bash
chaperone inject
# Proxy runs on http://127.0.0.1:4010
```

### 4. Trust the CA Certificate
```bash
chaperone ca-cert
# Follow OS-specific instructions to trust the certificate
```

### 5. Configure Your Application
```bash
export HTTPS_PROXY=http://127.0.0.1:4010
# Now your app's HTTPS requests will be proxied with credential injection
```

## 📚 Documentation

- **README.md** - Comprehensive feature documentation and examples
- **CHANGELOG.md** - Full feature list for v0.1.0
- **SECURITY.md** - Security model, threat assessment, compliance
- **CLAUDE.md** - Developer guide for contributing

## 🔐 Security Notes

### Certificate Trust Required
The local CA certificate must be trusted by your OS/browser. Use `chaperone ca-cert` to display the certificate and follow OS-specific trust instructions in the README.

### Recommended Security Practices
1. **Development**: Use placeholder tokens (`chap_service_xxxxxxxx`)
2. **Production**: Use dedicated user account + Unix socket mode
3. **Audit**: Enable audit logging to file with `[audit]` config
4. **Secrets**: Prefer macOS Keychain > file > environment variables

### Security Assessment
```bash
chaperone check
# Shows security posture and recommendations
```

## 🧪 Testing

All binaries built and tested on macOS. Linux binaries cross-compiled.

**Test Coverage:**
- ✅ Unit tests for all packages
- ✅ Integration tests with real HTTP clients and TLS handshakes
- ✅ Race condition testing (`-race` flag)
- ✅ Security tests for policy enforcement and audit logging

## 📊 Platform Support

| Platform | Architecture | Status |
|----------|-------------|---------|
| macOS    | Intel (amd64) | ✅ Tested |
| macOS    | Apple Silicon (arm64) | ✅ Tested |
| Linux    | x86_64 (amd64) | ✅ Cross-compiled |
| Linux    | ARM64 | ✅ Cross-compiled |

## 🔍 Checksums

SHA256 checksums for all binaries are provided in `checksums.txt`.

Verify downloads:
```bash
# macOS
shasum -a 256 -c checksums.txt

# Linux
sha256sum -c checksums.txt
```

## 🐛 Known Issues

- Certificate trust must be manually configured (OS-specific)
- No hot configuration reload (requires restart)
- No OAuth2 flow support yet (planned for future release)

## 🙏 Acknowledgments

Built with:
- [elazarl/goproxy](https://github.com/elazarl/goproxy) - MITM proxy foundation
- [spf13/cobra](https://github.com/spf13/cobra) - CLI framework
- [BurntSushi/toml](https://github.com/BurntSushi/toml) - Configuration parsing

## 📝 License

Licensed under the MIT License. See LICENSE file for details.

---

**First time using Chaperone?** Start with `chaperone init` for an interactive setup wizard!

**Questions or issues?** Open an issue on GitHub.

**Security concerns?** See SECURITY.md for our security model and responsible disclosure policy.
