# Per-run proxy Basic auth (fixed port)

## Goal
Prevent local processes from using the proxy unless they have a per-run credential delivered via standard proxy env vars.

## Security invariants
- Proxy MUST reject any request (including CONNECT) without valid Proxy-Authorization.
- Secret MUST be per-run, high entropy, and never written to disk.
- Proxy MUST bind to loopback only (127.0.0.1, optionally ::1).

## Wrapper behavior
1) Generate secret
- secret := random_bytes(32)  // 256-bit
- Encode as hex or URL-safe base64.
- Username can be fixed (e.g., "u"); password carries entropy.

2) Start proxy
- Bind to 127.0.0.1 (optionally ::1) on a fixed configured port.
- Provide secret to proxy via argv/env/inherited memory (not disk).
- Proxy prints nothing sensitive by default.

3) Configure child env
- HTTP_PROXY=http://u:<secret>@127.0.0.1:<port>
- HTTPS_PROXY=http://u:<secret>@127.0.0.1:<port>
- NO_PROXY=localhost,127.0.0.1,::1 (merge with existing if needed)
- Best-effort scrub conflicting parent proxy env vars.

4) Lifecycle
- Child exits -> proxy exits.
- Proxy exits unexpectedly -> terminate child or warn (choose UX).
- Each wrapper invocation generates a new secret.

## Proxy behavior (auth gate)
1) Requirements
- HTTP requests: require Proxy-Authorization: Basic ...
- CONNECT requests: require Proxy-Authorization on CONNECT.
- Missing/invalid:
    - Respond 407 Proxy Authentication Required
    - Include Proxy-Authenticate: Basic realm="cred-proxy"
    - Do not MITM and do not forward.

2) Validation
- Accept only Basic auth.
- Password MUST exactly match the per-run secret.
- Optional: check username; optional: constant-time compare.

3) After auth
- Proceed normally (MITM/injection per existing rules).
- Optional: cache auth per TCP connection.

## Failure behavior / UX
- Clients that do not support proxy auth in proxy URLs fail with 407.
- Debug logs may include a one-line hint, but must not include the secret.
- Optional "no-auth" mode only for debugging; off by default.

---

# Linux hardening with PR_SET_DUMPABLE (best effort)

## Goal
Reduce risk that other same-user processes can read the wrapped process environment (including the proxy secret) via /proc/<pid>/environ or ptrace-style attachment.

## Invariants
- Hardening MUST NOT prevent the target app from running if the call fails.
- Apply as early as possible.

## Wrapper behavior (Linux only)
1) When
- In the child process path, immediately after fork and before execve().

2) Syscall
- prctl(PR_SET_DUMPABLE, 0)

3) Failure handling
- On error: continue normally.
- Optional debug log: "non-dumpable hardening unavailable (errno=...)"

4) Scope
- Apply to the wrapped app process.
- Optionally also apply to the proxy process.

## Expected side effects
- Debuggers/profilers/strace may not attach without sufficient privileges.