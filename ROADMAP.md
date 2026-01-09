# Chaperone Roadmap

## Security Hardening

### High Priority

- [ ] **Placeholder token authentication**
  Require apps to send a placeholder token for credential injection. Prevents accidental injection into random processes.

- [ ] **Dedicated user mode**
  Run Chaperone as unprivileged `chaperone` user. Credential files unreadable by normal users.

- [ ] **Unix socket mode**
  Use Unix socket instead of TCP port. Enables peer credential verification and avoids port conflicts.

- [ ] **Ignore system proxy**
  Upstream connections bypass HTTP_PROXY/HTTPS_PROXY environment variables. Prevents proxy chain attacks.

- [ ] **Audit logging**
  Log all credential injections with timestamp, service, and request path (credentials redacted). Provides forensic trail and usage visibility.

### Medium Priority

- [ ] **Bundled CA certificates**
  Ship trusted CA bundle. Don't trust system CA store for upstream connections. Prevents CA injection attacks.

- [ ] **`chaperone check` command**
  Security posture assessment tool. Shows current configuration and recommendations without nagging.

### Lower Priority

- [ ] **Memory protection (mlock) - Linux only**
  Use mlock() to prevent credential memory from being swapped to disk. Requires elevated privileges.

- [ ] **Secure credential zeroing - Linux only**
  Explicitly zero credential memory after use. Prevents memory dump attacks.

- [ ] **Credential rotation hooks**
  Webhook or callback when credential rotation is detected. Enables integration with secret management systems.

---

## Init Wizard

- [x] **Interactive wizard** - Detect services, configure credentials, generate config
- [x] **Credential storage** - Keychain, file, .env support
- [x] **Introduction and help text** - Explain what Chaperone does and what each option means
- [ ] **Single-service focus** - Each init configures exactly one service

---

## Lifecycle Management

- [ ] **Process wrapper mode**
  `chaperone run -- python app.py` - Chaperone spawns and supervises the app.

- [ ] **Session-scoped instances**
  Tie Chaperone lifecycle to shell session. Start with shell, exit with shell.

- [ ] **Editor integration**
  VSCode/Neovim extensions to manage Chaperone for projects.

---

## Architecture

- [ ] **Single-service instances**
  Each instance handles one service. Simplifies config, limits blast radius.

- [ ] **Project-local config**
  `.chaperone.toml` in project directory instead of global config.
