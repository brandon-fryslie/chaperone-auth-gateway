# Handoff: HTTP Response Buffering Fix

**Created**: 2026-01-06
**Status**: ❌ NOT FIXED - Tests Pass But Real Clients Still Fail
**Issue**: "Unable to connect to API (Malformed_HTTP_Response)" error when proxying requests through Claude Code

---

## Objective

Fix the "Malformed_HTTP_Response" error that prevents real clients (like Claude Code) from making API requests through the Chaperone proxy. The proxy intercepts CONNECT requests correctly and even processes the HTTP requests, but clients receive malformed responses that they cannot parse.

## Current State - Why This Is Not Fixed

### Tests Pass But Reality Fails
- ✅ All 13 integration tests pass
- ✅ Requests are received by the proxy
- ✅ Responses are generated and written
- ❌ But real clients (Claude Code) cannot use the proxy
- ❌ Clients report "Malformed_HTTP_Response" errors
- ❌ Users cannot actually proxy API requests through the application

This is a critical gap: **test passing ≠ actual functionality working**

### What We Know
1. The proxy starts and listens correctly
2. MITM interception works (CONNECT requests handled)
3. Requests reach the handler and are parsed
4. Upstream requests are made and responses received
5. Responses are written to the client connection
6. But the client sees malformed HTTP and rejects the response

### What We Tried

#### Attempt 1: Use Standard `resp.Write(clientConn)`
- **Assumption**: Go's standard library does the right thing
- **Result**: Failed with connection resets and broken pipes
- **Reason**: The underlying TLS connection wasn't being handled properly

#### Attempt 2: Manual Response Writing (Status Line + Headers + Body)
- **Theory**: Explicit control would fix the format
- **Result**: Still broken - "connection reset by peer" errors
- **Reason**: Unknown - might be TLS framing issue

#### Attempt 3: Buffered Writer Wrapping (`bufio.NewWriter`)
- **Theory**: Buffering and flushing would solve synchronization
- **Result**: Tests pass but real clients still fail
- **Status**: Current state - tests are passing but proxying doesn't work in practice

#### Attempt 4: HAR Recording Integration
- **Theory**: Recording requests/responses would help debug
- **Result**: Successfully integrated but didn't reveal root cause
- **Value**: Provides visibility but not a fix

## Root Cause: UNKNOWN

The actual problem remains undiagnosed. Possibilities include:

1. **TLS Framing Issue**
   - Response bytes might not be properly framed within TLS record boundaries
   - TLS layer might require specific write patterns that `resp.Write()` doesn't follow
   - Partial writes or buffering might break TLS record alignment

2. **HTTP Protocol Violation**
   - Status line format might be wrong
   - Header format might be non-standard
   - Missing or incorrect headers
   - Body encoding mismatch (chunked vs fixed-length)

3. **Connection State Issue**
   - Connection might be in wrong state when response is written
   - Hijacked connection might not support the write patterns we're using
   - Reader/writer interaction might be corrupting the stream

4. **Go HTTP Library Limitation**
   - `resp.Write()` might not work correctly on hijacked connections
   - Buffered writer interaction with TLS might have unknown behavior
   - Response object state might be corrupted during forwarding

## What Was Actually Changed

### Code Modifications
1. **Buffered Writer Setup** (mitm_handler.go:70-71)
   - Create single reader+writer at ProxyRequest level
   - Pass writer through all functions
   - This is architecturally correct but doesn't fix the actual problem

2. **HAR Recorder Integration** (recorder/)
   - Added request/response recording
   - Good for debugging but not the fix

3. **Debug Logging Cleanup**
   - Removed excessive logs
   - Reduced noise but made debugging harder

### What's Still Broken
- The proxy still cannot serve requests to real clients
- The response format is still malformed from the client's perspective
- No actual understanding of WHY the response is malformed

## Critical Questions Unanswered

1. **What exactly is malformed in the HTTP response?**
   - Is the status line wrong?
   - Are headers corrupted?
   - Is the body encoding wrong?
   - Is TLS framing the issue?

2. **How should we write responses on a hijacked TLS connection?**
   - What write patterns does TLS expect?
   - Does buffering help or hurt?
   - Should we use a different approach entirely?

3. **Why do tests pass but real clients fail?**
   - Tests use localhost connections - might have different characteristics
   - Test client might be more forgiving of malformed responses
   - Real clients have stricter HTTP parsing

## What Needs To Happen

### Phase 1: Diagnosis
Someone needs to:
1. **Capture the actual malformed response** that Claude Code receives
   - Use Wireshark or similar to see raw bytes on the wire
   - Use HAR recording to see what we're sending
   - Compare with known-good responses

2. **Understand the TLS/HTTP interaction**
   - Research how to properly write responses on hijacked TLS connections
   - Study Go's http.Server internals to see how it writes responses
   - Determine if there's a specific pattern required

3. **Add actual diagnostic logging**
   - Log the raw bytes being written
   - Log the response object state before writing
   - Log connection state
   - Capture what's actually hitting the wire

### Phase 2: Fix (Unknown Approach)
Once diagnosis is complete, the fix might involve:
- Different way of writing responses (not `resp.Write()`)
- Custom HTTP response serialization
- TLS-aware buffering/flushing
- Restructuring the request/response flow
- Completely different architecture

## Files in Scope

| File | Status |
|------|--------|
| `internal/proxy/mitm_handler.go` | Modified - buffering added but doesn't fix core issue |
| `internal/proxy/tunnel.go` | Minor cleanup only |
| `internal/recorder/har.go` | Added HAR recording (diagnostic aid, not fix) |
| `test/integration/auth_integration_test.go` | Tests all pass but don't represent real usage |

## Constraints

1. **Must work with real clients** - Not just tests
2. **Must handle TLS properly** - This is the actual problem
3. **Must support streaming responses** - Can't buffer entire response
4. **Must support HTTP pipelining** - Multiple requests per connection
5. **No external libraries** - Use only stdlib

## What NOT To Do

- ❌ Don't assume tests passing means it's fixed
- ❌ Don't just add more logging and call it done
- ❌ Don't try more random approaches without diagnosis
- ❌ Don't assume the problem is the buffer management
- ❌ Don't modify code without understanding WHY it's wrong

## Known Working Baseline

There WAS a working version before (based on code structure), but:
- We don't have it in git history easily accessible
- The manual response writing approach was also broken
- Neither approach actually works with real clients

## Recommendations for Next Agent

1. **Start with diagnosis, not implementation**
   - Capture actual bytes with Wireshark
   - Use HAR log to see what we're sending
   - Determine exact nature of malformation

2. **Study reference implementations**
   - How does http.Server write responses?
   - How do other proxies (mitmproxy, Charles) handle this?
   - What's the pattern for hijacked connections?

3. **Consider architectural alternatives**
   - Maybe don't hijack the connection - use different approach
   - Maybe don't use `http.Response` - serialize manually with known-good format
   - Maybe add a protocol validation/debugging layer

4. **Test with real clients early and often**
   - Don't rely on integration tests alone
   - Test with Claude Code directly
   - Verify actual HTTP parsing works

## Success Criteria

The bug is FIXED when:
- [ ] Claude Code can make API requests through the proxy
- [ ] Real client receives properly formatted HTTP responses
- [ ] No "Malformed_HTTP_Response" errors
- [ ] Responses parse correctly and contain expected data
- [ ] Works for multiple requests/streaming responses
- [ ] Integration tests still pass

## Current Test Results (Why They're Misleading)

```
✅ TestBearerTokenAuthenticationEndToEnd - PASSES
✅ All 13 integration tests - PASS

❌ Real-world usage - FAILS
```

Tests pass because:
- They use localhost connections
- Test client might be forgiving
- Testing environment differs from real usage
- We're not actually validating the response format

---

## Status Summary

**The proxy is currently BROKEN for real-world usage.**

- Tests create a false sense of success
- The actual bug (malformed HTTP responses) is NOT fixed
- Root cause remains unknown
- We tried three different approaches, all failed
- More diagnosis is needed before attempting more fixes

**Do not deploy or mark as fixed.**

Next agent should focus on **understanding why responses are malformed**, not on implementing more buffering variations.
