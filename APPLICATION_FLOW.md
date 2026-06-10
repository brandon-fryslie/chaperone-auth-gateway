# Application Flow Documentation

This document describes the application flow for Chaperone's two primary operating modes: **run mode** and **examine mode**.

## Overview

| Mode | Purpose | Execution Model | Use Case |
|------|---------|-----------------|----------|
| **run** | Execute child process with credential injection | Starts proxy + spawns child with injected auth | Production use: run your app with injected credentials |
| **examine** | Passthrough logging for auth discovery | Starts proxy + optional command execution | Development: discover where auth headers are sent |

---

## Run Command Flow

The `run` command (`cmd/chaperone/cmd/run.go`) orchestrates a proxy server and child process lifecycle, injecting credentials into API requests made by the child.

### High-Level Sequence

```
User Command: chaperone run <service> [-- command args]
                    ↓
        Load & Merge Configuration
                    ↓
        Prepare Run Config (CLI override, expand vars)
                    ↓
        Setup Ephemeral CA (cleanup on exit)
                    ↓
        Start Proxy Server (Unix socket)
                    ↓
        Build Child Environment (proxy URL, CA cert, etc.)
                    ↓
        Spawn Child Process with Environment
                    ↓
        Forward Signals (SIGTERM → Child)
                    ↓
        Wait for Child Exit
                    ↓
        Cleanup & Exit with Child's Exit Code
```

### Detailed Steps

#### 1. Command Parsing (`runWithProxy` lines 55-69)

```go
// Input: "chaperone run openai -- python script.py"
serviceName := args[0]          // "openai"
cliCommand := args[2:]          // ["python", "script.py"]
```

**Validation:**
- Service name is required (enforced by `cobra.MinimumNArgs(1)`)
- If extra arguments provided, they must be preceded by `--` separator
- Returns error if invalid syntax detected

#### 2. Configuration Loading

```go
// Resolve config from a trusted source only (-c flag or ~/.config/chaperone/);
// config is never read from the current working directory:
configPath, err := getConfigPath()
cfg, err := config.Load(configPath)

// Merge CLI command over config service command:
svc, err := run.PrepareRunConfig(cfg, serviceName, cliCommand)
```

**Key Point:** CLI commands override config file settings. Variable expansion happens here (e.g., `${HOME}` → actual path).

**File trust gate:** `config.Load` refuses to parse a config that another local
user could have written — group/world-writable mode, ownership by a different
uid, or a non-regular file all fail loudly (with the `chmod`/`chown` fix in the
message) before any `credential_ref` is read. The check stats the open file
handle, so the verified file is the parsed file.

#### 3. Ephemeral CA Initialization (lines 124-128)

```go
ca, caKeyPath, caCertPath, err := orchestrate.InitializeEphemeralCA(ctx, os.Getpid(), shutdownMgr)
```

**Security Model:**
- Generates fresh CA in `/tmp/chaperone-ca-<pid>/`
- Only this invocation's child process trusts this CA
- Automatically deleted on proxy shutdown
- No system-wide CA modifications

**Cleanup Registration:**
- `shutdownMgr.Register()` registers callback to clean up on exit
- Ensures cleanup even if process crashes

#### 4. Shared Setup (lines 131-141)

```go
setupCfg := orchestrate.SetupConfig{
    Config:       cfg,
    ServiceNames: []string{serviceName},  // Single service filter
    CAKeyPath:    caKeyPath,
    CACertPath:   caCertPath,
}

result, err := orchestrate.Setup(ctx, setupCfg, ca, slog.Default())
```

**What `orchestrate.Setup()` does:**
- Loads service registry from config
- Pre-validates all secrets (env vars, files, keychain)
- Registers auth strategies (bearer, custom headers, etc.)
- Fails fast if config is invalid
- Returns `SetupResult` with initialized registries

#### 5. Proxy Server Creation & Start (lines 143-151)

```go
proxyServer := orchestrate.CreateProxy(ctx, cfg, slog.Default(), shutdownMgr, result)

if err := proxyServer.Start(); err != nil {
    return fmt.Errorf("failed to start proxy server: %w", err)
}
```

**Proxy Behavior:**
- Listens on Unix socket: `/tmp/chaperone-<service>-<pid>.sock`
- Intercepts requests to configured services (MITM via TLS)
- Fetches secrets and injects auth headers
- **Not spawned as separate process** — runs in main process

#### 6. Child Process Environment Setup (lines 153-170)

```go
// Build proxy environment variables:
childEnv, err := run.BuildChildEnvironment(ctx, svc, serviceName, socketPath, caCertPath)
```

**Environment Variables Set:**
- `HTTP_PROXY=socks5://localhost:<fd>` (Unix socket proxy URL)
- `HTTPS_PROXY=socks5://localhost:<fd>`
- `SSL_CERT_FILE=<caCertPath>` (custom CA for TLS verification)
- `REQUESTS_CA_BUNDLE=<caCertPath>` (Python requests library)
- `NODE_EXTRA_CA_CERTS=<caCertPath>` (Node.js)
- `CHAPERONE_SERVICE=<serviceName>`
- `CHAPERONE_SOCKET=<socketPath>`
- All parent process environment variables (inherited)

**File Descriptor Configuration:**
```go
fdConfig, err := run.NewFDConfig(stdinMode, svc.Run.Stdout, svc.Run.Stderr)
```
- Default stdin: **"inherit"** (child gets terminal input)
- Stdout/stderr: configurable (file, inherit, or null)

#### 7. Child Process Spawning (lines 172-189)

```go
pm, err := run.NewProcessManager(ctx, svc.Run.Command, svc.Run.Args, childEnv, fdConfig)

if err := pm.Start(); err != nil {
    return err
}
```

**Process Creation:**
- Command and arguments come from config (or CLI override)
- Runs in **new process group** (via `Setpgid()`) for signal isolation
- Inherits file descriptors for terminal interaction

**Note:** Unlike examine mode, **`run` uses ProcessManager for signal forwarding**.

#### 8. Signal Forwarding & Execution (lines 195-200)

```go
// Run with signal forwarding (SIGTERM → child)
childExitCode := run.RunWithSignals(ctx, pm)

// Cleanup and exit with child's exit code
exitCode := run.CleanupProcess(ctx, pm, shutdownMgr, childExitCode)
os.Exit(exitCode)
```

**Signal Handling:**
- Parent intercepts SIGTERM/SIGINT
- Forwards to child process group
- Waits for graceful shutdown
- If child doesn't exit, kills forcefully

**Exit Code Propagation:**
- Child's exit code is parent's exit code
- Example: if child fails with code 42, chaperone exits with 42

### Authentication Flow During Execution

When the child makes an HTTPS request to `api.openai.com`:

```
Child Process
    ↓ (HTTPS request via proxy)
Proxy Server (Unix socket)
    ├─ CONNECT handler: Decide MITM vs transparent
    ├─ Policy handler: Check allowed methods/paths
    ├─ Auth handler:
    │   ├─ Lookup service by host
    │   ├─ Fetch secret (e.g., from env var)
    │   ├─ Apply strategy (Bearer token injection)
    │   └─ Add header: Authorization: Bearer sk-...
    └─ Forward to upstream (api.openai.com)
        ↓
    Upstream API receives: Authorization: Bearer sk-...
        ↓
    Response returned to child
```

---

## Examine Command Flow

The `examine` command (`cmd/chaperone/cmd/examine.go`) runs a passthrough MITM proxy to log requests and discover authentication patterns.

### High-Level Sequence

```
User Command: chaperone examine [-- command]
                    ↓
        Setup Examine Logger Configuration
                    ↓
        Load or Generate CA Certificate
                    ↓
        Start Proxy Server (TCP port 4010, or custom)
                    ↓
        If Command Provided:
            ├─ Print startup info + wait for user
            ├─ Spawn child process with proxy environment
            └─ Monitor for:
                ├─ Sentinel value detection → terminate + print config
                └─ Child exit → cleanup + exit
        Else:
            └─ Wait for Ctrl+C
                    ↓
        Write HAR file (if enabled)
        Write audit logs (if enabled)
        Print discovery summary
                    ↓
        Exit
```

### Detailed Steps

#### 1. Configuration Setup (lines 219-289)

```go
// Minimal config for examine mode - no services required
cfg := &config.Config{
    Server: config.ServerConfig{
        Address: "127.0.0.1",
        Port:    4010,  // Default TCP port
    },
    Logging: config.LoggingConfig{
        Level: "info",
    },
}

// Apply CLI transport flags:
orchestrate.ApplyTransportFlags(cfg, orchestrate.TransportFlags{
    SocketPath: examineSocketPath,
    HTTPMode:   examineHTTPMode,
    HTTPPort:   examineHTTPPort,
    HTTPAddr:   examineHTTPAddr,
})
```

**Transport Mode:**
- Default: TCP mode (HTTP/HTTPS passthrough)
- Can be overridden with `--socket` for Unix socket
- Examine mode **doesn't require service configuration**

#### 2. Examine Logger Configuration (lines 323-332)

```go
examineConfig := examine.Config{
    ShowBody:      showBody,      // -b flag
    ShowParams:    showParams,    // -p flag
    ShowCookies:   showCookies,   // --show-cookies
    ShowResponse:  showResponse,  // -r flag
    MaxBodyBytes:  4096,          // Truncate large bodies
    AllHeaders:    allHeaders,    // Disable filtering heuristics
    SentinelValue: "chaperone-sentinel",
}

examineLogger := examine.NewLogger(examineConfig)
```

**Filtering Behavior:**
- By default, filters out non-auth headers (Content-Type, Accept, etc.)
- `--all-headers` disables filtering
- Sentinel value detection finds injected test tokens

#### 3. CA Certificate Setup (lines 334-360)

```go
caDir, caKeyPath, caCertPath, err := getCAPath()

ca, err := mitm.LoadOrGenerateCA(caKeyPath, caCertPath)

if isNewCA {
    log.Info(ctx, "generated new CA certificate", ...)
    log.Info(ctx, "trust this CA in your browser/system to avoid certificate warnings")
}
```

**CA Storage:**
- Persistent at `~/.config/chaperone/ca-cert.pem`
- Reused across invocations (unlike run mode's ephemeral CA)
- User must add to system/browser trust store to avoid certificate warnings

#### 4. Proxy Server Creation (lines 362-475)

```go
server := proxy.NewExamineProxy(cfg, slog.Default(), shutdownMgr, certCache, examineLogger, rec, sentinelChan)

if err := server.Start(); err != nil {
    return err
}
```

**Examine Mode Behavior:**
- **Always MITM** (no filtering by service)
- Logs request headers, optionally bodies/params/cookies
- Logs response status and headers
- No credential injection
- HAR recording (optional)

#### 5a. Manual Mode (lines 598-603)

If no command provided (`chaperone examine`):

```
Print proxy configuration:
    Configure your application to use: http://127.0.0.1:4010 as proxy
    export HTTP_PROXY=http://127.0.0.1:4010
    export HTTPS_PROXY=http://127.0.0.1:4010

Wait for Ctrl+C → Shutdown
```

**User Workflow:**
1. Start proxy: `chaperone examine`
2. In another terminal, configure app: `export HTTP_PROXY=http://127.0.0.1:4010`
3. Run app
4. Review logs in real-time or HAR file
5. Press Ctrl+C to stop proxy

#### 5b. Command Mode (lines 477-594)

If command provided (`chaperone examine -- curl https://api.example.com`):

```go
// Print startup info
fmt.Fprintf(os.Stderr, "=== Chaperone Examine Mode ===\n")
fmt.Fprintf(os.Stderr, "Press return to continue...\n")

// Wait for user input (from /dev/tty, not stdin)
tty, err := os.Open("/dev/tty")
reader := bufio.NewReader(tty)
_, _ = reader.ReadByte()  // Wait for Enter
```

**Setup Before Execution:**
1. Create temporary log file for proxy logs
2. Create symlink at `/tmp/chaperone-examine.latest.log` for convenience
3. Build proxy environment variables
4. Build child environment with proxy variables

#### 6. Child Process Execution (Command Mode Only)

```go
// Build environment with proxy vars:
envBuilder := run.NewEnvBuilder()
envBuilder.InheritParent()
envBuilder.Set("HTTP_PROXY", proxyURL)      // http://127.0.0.1:4010
envBuilder.Set("HTTPS_PROXY", proxyURL)
envBuilder.SetCAEnvVars(caCertPath, [
    "SSL_CERT_FILE",
    "REQUESTS_CA_BUNDLE",
    "NODE_EXTRA_CA_CERTS",
])

// Create child process (NO ProcessManager for examine mode)
cmd := exec.Command(cliCommand[0], cliCommand[1:]...)
cmd.Env = childEnv
cmd.Stdin = os.Stdin    // Inherit terminal
cmd.Stdout = os.Stdout
cmd.Stderr = os.Stderr
// No Setpgid - child stays in same process group for proper signal handling
```

**Key Difference from Run Mode:**
- **Does NOT use ProcessManager** (no process group isolation)
- Child stays in **same process group** as parent
- Allows child to receive signals directly from terminal
- When user presses Ctrl+C, **both child and parent get SIGINT**

#### 7. Sentinel Detection (lines 548-566)

```go
select {
case <-sentinelChan:
    // Sentinel found - terminate child and print config
    log.Info(ctx, "sentinel value detected, terminating child process")
    if err := cmd.Process.Signal(os.Interrupt); err != nil {
        _ = cmd.Process.Kill()
    }
    // Wait for graceful shutdown (up to 10 seconds), then kill if still running
    select {
    case <-time.After(10 * time.Second):
        _ = cmd.Process.Kill()
    case exitCode = <-childExitChan:
        // Child exited gracefully
    }

    // Print complete config to stdout
    printCompleteConfig(ctx, examineLogger)
    os.Exit(exitCode)

case exitCode = <-childExitChan:
    // Child exited normally
    exitCode = <-childExitChan
    // Cleanup and exit
    os.Exit(exitCode)
}
```

**Sentinel Value Workflow:**
1. User starts command with sentinel: `chaperone examine -- curl -H "X-API-Key: chaperone-sentinel" https://api.example.com`
2. Examine mode intercepts request, finds sentinel in X-API-Key header
3. Signals sentinel channel
4. Child process is terminated gracefully (SIGINT)
5. Complete TOML config is printed to stdout with discovered auth pattern
6. User can copy config and add to chaperone.toml

**Complete Config Output:**
```toml
=== Complete Configuration ===

[services.myservice]
host_pattern = "api.example.com"
auth_strategy = "header:X-API-Key"

# Recommended: Use keychain or file for secure credential storage

# macOS Keychain (Recommended):
# security add-generic-password -s "chaperone/myservice" -a "api_key" -w "YOUR_API_KEY"
# credential_ref = "keychain:chaperone/myservice/api_key"

# credential_ref = "env:YOUR_API_KEY"
```

#### 8. Cleanup & Shutdown (lines 567-603)

```go
shutdownMgr.Register(func(ctx context.Context) error {
    examineLogger.PrintSummaryReport(ctx)
    return nil
})

// Shutdown handler for HAR recording:
shutdownMgr.Register(func(ctx context.Context) error {
    if err := rec.WriteToFile(harPath); err != nil {
        return fmt.Errorf("failed to write HAR file: %w", err)
    }
    return nil
})

// On shutdown:
shutdownMgr.Shutdown(30 * time.Second)
```

**Shutdown Sequence:**
1. Proxy stops accepting new connections
2. HAR file written to disk (if enabled)
3. Discovery summary printed (if enabled)
4. Temp log file closed (if command mode)
5. Exit with child's exit code

### Discovery Tracking

The examine logger tracks authentication headers discovered in requests:

```go
type AuthHeaderDiscovery struct {
    HeaderName      string  // "Authorization", "X-API-Key", etc.
    HeaderValue     string  // Full header value (redacted)
    IsStandardAuthKey bool  // Is it Authorization or X-*-Key?
    FoundSentinel   bool    // Was sentinel value found?
    URL             string  // Request URL
    Method          string  // HTTP method
    Count           int     // Number of times seen
}
```

**Discovery Heuristics:**
1. Look for headers containing "auth", "key", "token", "secret", "credential"
2. Prefer standard auth headers (Authorization, X-API-Key)
3. Detect sentinel value injection (user provides test token)
4. Extract hostname from request URL

---

## Comparison: Run vs Examine

| Aspect | Run | Examine |
|--------|-----|---------|
| **Purpose** | Execute with credential injection | Discover auth headers |
| **Service Config Required** | Yes | No |
| **CA Mode** | Ephemeral (temp, cleaned up) | Persistent (~/.config) |
| **Network Mode** | Unix socket | TCP (default) |
| **Proxy Filter** | Services from config | All traffic (MITM) |
| **Credential Injection** | Yes | No |
| **Child Signal Handling** | ProcessManager (process group) | Direct (same group) |
| **Output Logs** | Temp file (path printed to stderr) | Temp file (command mode) or stdout (manual mode) |
| **HAR Recording** | Not available | Available with --har flag |
| **Sentinel Detection** | N/A | Yes, auto-terminates on discovery |
| **Exit Code** | Child's exit code | Child's exit code (command mode) or 0 (manual mode) |

---

## Environment Variable Resolution

Both modes support variable expansion in config:

```toml
[services]
name = "myservice"
credential_ref = "env:${API_KEY_VAR}"  # Expands to env value
command = "${HOME}/my-app.py"          # Expands to home dir
```

**Resolution happens in:** `run.PrepareRunConfig()` (run mode only)

For examine mode, variable expansion is **not applied** (minimal config).

---

## Error Handling

### Run Mode Failures

| Failure Point | Behavior |
|---------------|----------|
| Config loading | Error message, exit 1 |
| Service not found | Error message, exit 1 |
| CA initialization | Error message, exit 1 |
| Proxy startup | Error message, exit 1 |
| Child spawn | Shutdown proxy, error message, exit 1 |
| Child execution error | Forward exit code from child |

### Examine Mode Failures

| Failure Point | Behavior |
|---------------|----------|
| CA loading | Error message, exit 1 |
| Proxy startup | Error message, exit 1 |
| HAR write | Warning, continue exit |
| Child spawn (command mode) | Shutdown proxy, error message, exit 1 |

---

## Logging

### Run Mode Logging

```
[INFO] chaperone run mode starting version=0.1.0 service=openai socket=/tmp/chaperone-openai-12345.sock
[INFO] starting proxy server socket=/tmp/chaperone-openai-12345.sock
[INFO] proxy server started successfully
[INFO] starting child process command=python args=[script.py]
[INFO] child process started successfully
[INFO] injected credential credential_ref=env:OPENAI_API_KEY auth_strategy=bearer path=/v1/chat/completions host=api.openai.com
...
[INFO] child process exited exit_code=0
[INFO] stopping proxy server
```

### Examine Mode Logging

```
[INFO] chaperone examine mode starting address=127.0.0.1 port=4010
[INFO] generated new CA certificate cert_path=~/.config/chaperone/ca-cert.pem
[INFO] HAR recording enabled output_path=chaperone-2024-02-04-123456.har
...
[EXAMINE] GET api.example.com/v1/models
[EXAMINE] Headers: Authorization: Bearer ***redacted***
...
```

**Note:** Examine mode logs go to temp file when command mode is used, preventing output pollution of stdout/stderr.

---

## Security Implications

### Run Mode
- **Ephemeral CA:** Fresh certificate per invocation, auto-deleted
- **Single Service:** Only credentials for specified service are injected
- **Process Isolation:** Child runs in separate process group
- **Environment Leak:** Child inherits all parent env vars (potential risk if secrets elsewhere)

### Examine Mode
- **Persistent CA:** Same certificate across invocations (must trust in system)
- **Passthrough:** No credential injection, purely logging
- **Full Traffic Logging:** All headers logged by default (potential secret leakage if not careful)
- **Sentinel Detection:** Automatic termination on test token injection

---

## Advanced Usage Patterns

### Pattern 1: Run with Override Command
```bash
chaperone run openai -- python my_script.py
```
- Config specifies `openai` service
- CLI command `python my_script.py` overrides config's command
- Proxy connects to same service, credential injection still works

### Pattern 2: Discover, Configure, Run
```bash
# 1. Discover auth pattern
chaperone examine -- curl https://api.example.com

# 2. When sentinel value is sent, config is printed
# Copy config section to chaperone.toml

# 3. Run with credentials
chaperone run myservice -- curl https://api.example.com
```

### Pattern 3: Multiple Proxies
```bash
# Terminal 1: Examine on default port
chaperone examine --http --port 4010

# Terminal 2: Run on custom socket
chaperone run openai --socket /tmp/my-proxy.sock
```

### Pattern 4: HAR Export for Analysis
```bash
chaperone examine --har --har-output traffic.har -- python script.py
# HAR file can be imported into Chrome DevTools, Postman, etc.
```

---

## Troubleshooting

### Proxy Not Intercepting Requests (Run Mode)
1. Verify child's `HTTP_PROXY` and `HTTPS_PROXY` env vars
2. Check socket path in logs
3. Ensure service host pattern matches request destination

### Certificate Warnings in Examine Mode
1. Add CA cert to system trust store
2. Or add to app's trusted CAs via environment variable
3. Check `~/.config/chaperone/ca-cert.pem` exists

### Log File Not Found (Run Mode)
- Path is printed to stderr before child starts
- Check last line of command output
- Format: `/tmp/chaperone-<service>-<timestamp>.log`

### Sentinel Detection Not Working (Examine Mode)
1. Verify sentinel value matches `--sentinel` flag (default: `chaperone-sentinel`)
2. Ensure header containing sentinel is being logged
3. Check `--all-headers` flag if header is being filtered

