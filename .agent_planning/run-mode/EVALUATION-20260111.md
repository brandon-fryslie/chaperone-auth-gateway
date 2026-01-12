# EVALUATION-20260111.md

## Executive Summary

The "run mode" feature requires transforming the `chaperone run` command from a simple proxy server launcher into a full-featured application spawner that:
1. Launches a child application with controlled environment variables
2. Runs the proxy on a Unix socket (preferred) or TCP port
3. Configures the child app to use the proxy for HTTP/HTTPS traffic
4. Handles file descriptors and process lifecycle management

**Current State**: The `chaperone run` command exists as a deprecated alias to `chaperone inject` (per DEPRECATIONS.md). It starts the proxy but does NOT spawn child applications. This is a significant new feature requiring substantial new code.

**Readiness**: **PAUSE** - Multiple critical design questions must be resolved before implementation.

---

## 1. What Exists

### Foundational Components (Ready to Use)

1. **Unix Socket Support** (COMPLETE - Just merged)
   - `internal/proxy/server.go:312-381` - Full Unix socket listener implementation
   - Socket creation, cleanup, file permissions (0660)
   - Stale socket removal before listen
   - Registered shutdown handler
   - Configuration support: `ServerConfig.Socket` field in config
   - CLI flags: `--socket`, `--http`, `--port`, `--addr` on both `run` and `inject` commands

2. **Configuration System** (Extensible)
   - `internal/config/config.go` - Config struct with `ServerConfig`, `ServiceConfig`
   - Supports TOML parsing, defaults, validation
   - `SetDefaults()` applies Unix socket mode by default
   - Config merging would need to be added for `.chaperone.toml` + `~/.config/chaperone/chaperone.toml`

3. **Service Registry & Auth** (Production-Ready)
   - Service lookup by host pattern
   - Auth strategies (Bearer, custom headers)
   - Secret providers (env, file, keychain)
   - Policy enforcement (allowed methods/paths, body size limits)

4. **Proxy Server Architecture** (MITM-Enabled)
   - `proxy.NewWithMITM()` creates full-featured MITM proxy
   - `server.Start()` handles both TCP and Unix socket listening
   - Request/response handlers for auth injection, policy enforcement, audit logging
   - Certificate caching for HTTPS interception

5. **Shutdown Manager** (`internal/shutdown/shutdown.go`)
   - Graceful shutdown with callback registration
   - Timeout support
   - Perfect for cleaning up child process on parent exit

---

## 2. What's Missing (Required for Run Mode)

### Core Functionality Gaps

1. **Process Spawning & Lifecycle**
   - Must spawn child application with `os.Exec` or similar
   - Must wait for process completion and handle exit codes
   - Must forward signals (SIGTERM, SIGINT) to child
   - Must handle process state (running, terminated, error)

2. **Environment Variable Setup**
   - Must compute socket path from service name and PID: `/tmp/chaperone-<service>-<pid>.sock`
   - Must inject `HTTP_PROXY`, `HTTPS_PROXY`, `CHAPERONE_SOCKET` env vars
   - Must support `env_file` option to load additional vars from file
   - Must decide: append to parent env or replace?

3. **File Descriptor Configuration**
   - `stdout` handling: "inherit" (pass through), "file:/path/to/log", "discard"
   - `stderr` handling: same options
   - Must decide: separate file handles vs. combined?

4. **Config Structure Extensions**
   - New `[services.run]` section in ServiceConfig
   - Must extend `ServiceConfig` struct to hold this data
   - Must handle variable expansion ($HOME, etc.)

5. **Configuration Merging Logic**
   - Load `~/.config/chaperone/chaperone.toml` first, then overlay `.chaperone.toml` from cwd
   - Must define merge semantics (deep merge? shallow? service-level?)

---

## 3. CRITICAL AMBIGUITIES & OPEN QUESTIONS

### **A. Configuration Merging Strategy (BLOCKING)**

**Question**: How should `.chaperone.toml` and `~/.config/chaperone/chaperone.toml` merge when both exist?

**Options**:
- **Option A**: Project file completely overrides user file (simple, clear)
- **Option B**: Deep merge - user file provides defaults, project file overrides service-by-service
- **Option C**: User file ignored if project file exists

**User specified**: "More specific config overwrites less specific (project-level config overwrites config keys in ~)"

**Interpretation needed**: Does "overwrites config keys" mean:
- Service-level replacement (if project defines service X, ignore user's service X)?
- Key-level replacement (merge services, but project's fields override user's fields)?

### **B. Socket Path Configuration (DESIGN)**

**Question**: Is default `/tmp/chaperone-<service>-<pid>.sock` acceptable?

**Concerns**:
- `/tmp/` may not be writable in containers/restricted environments
- No per-user isolation (other users can see sockets)

**User specified**: "Configurable with sensible default"

**Need clarity on**: Should default include user directory for isolation?

### **C. Environment Variable Injection (CRITICAL)**

**Question**: What format for proxy environment variables with Unix sockets?

**Problem**: Standard `HTTP_PROXY=http://127.0.0.1:4010` doesn't work for Unix sockets

**Options**:
- Use `HTTP_PROXY=unix:///tmp/chaperone-service.sock` (non-standard but some clients support)
- Custom var: `CHAPERONE_SOCKET=/tmp/...` only (app must know about it)
- Both standard format + custom var

**User Input Needed**: What URL format do target apps (like Claude CLI) expect?

### **D. Signal Handling & Graceful Shutdown (CRITICAL)**

**Question**: When parent proxy receives SIGTERM, what happens?

**User Input Needed**:
- Send SIGTERM to child first, then stop proxy?
- Wait for child to exit before proxy shuts down?
- Or proxy exits independently?

**Question**: When child process exits naturally, does proxy continue or exit?

**Expected**: Proxy should continue (may be shared), but need confirmation.

### **E. Environment File Loading**

**Question**: What format for `env_file`?

**Options**:
- Shell format (`.env` style): `KEY=value`
- TOML format (consistent with chaperone.toml)
- Auto-detect

**User Input Needed**: Format preference? Shell .env format is common for this use case.

### **F. Command & Args Handling**

**Question**: Should `command` and `args` support variable expansion?

**Examples**:
- `command = "$HOME/.local/bin/claude"`
- `args = ["--config", "${CLAUDE_HOME}/settings.json"]`

**User Input Needed**: Support variable expansion or require literal paths?

### **G. Service Selection Semantics**

**Question**: Does `chaperone run <service-name>` spawn ONLY that service's app, or start proxy for that service?

**Current spec**: Spawn the app defined in `[services.run]` for that service

**Confirm**: Service name is REQUIRED (not optional)?

### **H. Error Scenarios**

**Need decisions on**:
1. Service not found → Error before spawning proxy?
2. Socket creation fails → Cleanup proxy, don't spawn child?
3. Child spawn fails → Stop proxy? Return error?
4. env_file doesn't exist → Error or skip?
5. Command not found → Return error before spawning proxy?

---

## 4. Implementation Sequence (After Ambiguities Resolved)

1. Add `RunConfig` struct to `ServiceConfig`
2. Implement config file merging
3. Implement environment variable expansion
4. Create `internal/run/env.go` - env variable builder
5. Create `internal/run/spawner.go` - child process lifecycle
6. Extend `cmd/chaperone/cmd/run.go` to orchestrate
7. Write comprehensive tests

---

## 5. Risk Assessment

| Risk | Severity | Mitigation |
|------|----------|-----------|
| Socket path not writable | MEDIUM | Make configurable; test in containers |
| Child signals not forwarded | MEDIUM | Use syscall for signal handling |
| Config merging breaks existing setups | MEDIUM | Clear precedence rules; document |
| Socket permissions security | HIGH | Use 0660; document; consider user directories |

---

## Summary

**Status**: **PAUSE** - Resolve critical ambiguities A, C, D before planning implementation.

**Estimated scope**: 400-600 new lines across 2-3 new modules, 2-3 modified files.

**Critical path**: Ambiguity resolution → Implementation (3-5 days) → Testing (2-3 days)
