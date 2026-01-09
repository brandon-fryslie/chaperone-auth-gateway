# HTTP Response Handling Evaluation - MITM Proxy Codebase

**Evaluation Date**: 2026-01-09
**Evaluator**: Claude Code Read-Only Analysis
**Focus Area**: HTTP Response Handling in MITM Proxy

---

## Executive Summary

The HTTP response handling in this MITM proxy codebase is in a **critical broken state** despite passing all integration tests. The proxy correctly intercepts requests and receives responses from upstream servers, but it produces malformed HTTP responses that cause real clients (like Claude Code) to reject them with "Malformed_HTTP_Response" errors. This represents a severe gap between test passing and real-world functionality.

---

## 1. Current Architecture Overview

### 1.1 Proxy Server Setup (`internal/proxy/server.go`)

**Lines 88-181**: `NewWithMITM()` function creates the proxy server with:
- goproxy.ProxyHttpServer as the core engine
- Chain of request handlers for MITM services
- Response handler for HAR recording (Line 162)
- Standard HTTP server timeouts (30s read/write, 1s idle)

**Critical Architecture**: The proxy uses goproxy's `OnResponse().DoFunc()` mechanism (Line 162) to intercept responses after they're received from upstream but before they're sent to the client.

### 1.2 Response Handler Implementation (`internal/proxy/handlers.go`)

**Lines 398-438**: `recordResponseHandler()` function:
- Accepts response from goproxy
- Records HAR entry with `rec.RecordRequest().RecordResponse()`
- Logs request completion with status code
- **Returns the response unchanged** (Line 436: `return resp`)

**Key Observation**: The response handler does NOT modify the response - it only records and logs it. This suggests the issue is not in response modification but in how goproxy handles response forwarding.

### 1.3 Request Flow Architecture

1. **CONNECT Handling** (Lines 31-76): MITM vs transparent tunnel decision
2. **Request Chain** (Lines 140-159):
   - Request ID setup
   - Policy enforcement
   - Auth header stripping
   - Authentication injection
   - HAR recording (request start)
3. **Response Handling** (Lines 162, 400-438): HAR recording (response end)
4. **Response Forwarding**: Handled internally by goproxy

---

## 2. Test vs Real-World Gap Analysis

### 2.1 Integration Test Validation (`test/integration/auth_integration_test.go`)

**Test Approach**:
- Uses `httptest.NewTLSServer` for upstream servers
- Creates real HTTP clients with proper TLS config
- Makes actual requests through proxy
- Validates upstream receives correct headers
- **Tests pass completely** (13/13 tests)

**Critical Gap**: Tests only verify that:
- Upstream server receives the request
- Response status is 200 OK
- Response body contains expected text

**What Tests DON'T Validate**:
- HTTP response format correctness
- TLS record layer integrity
- Client-side parsing success
- Connection state management
- Actual HTTP protocol compliance

### 2.2 Why Tests Pass But Real Clients Fail

1. **Test Environment Differences**:
   - Tests use localhost (127.0.0.1) connections
   - Real clients use various network paths
   - Test servers might be more forgiving

2. **Client Differences**:
   - httptest.Client vs real HTTP clients (Claude Code)
   - Different HTTP parsing strictness
   - Different connection handling expectations

3. **Missing Validation**:
   - Tests don't capture/validate the raw bytes sent to client
   - No response format validation
   - No TLS framing checks

---

## 3. goproxy Behavior Analysis

### 3.1 How goproxy Handles Responses

Based on code analysis:

1. **OnResponse().DoFunc()** (Line 162 in server.go):
   - Called after response received from upstream
   - Given response object and proxy context
   - Handler can modify response or return as-is
   - goproxy continues with response forwarding

2. **Response Forwarding**:
   - Handled internally by goproxy
   - Uses standard Go HTTP libraries
   - Should theoretically be correct

**Contradiction**: If goproxy is reliable, why are responses malformed?

---

## 4. Potential Root Causes

### 4.1 TLS Response Framing Issues

**Hypothesis**: The issue might be in how responses are framed within TLS records.

**Evidence**:
- Handoff document mentions TLS framing as a possibility
- Connection reset errors suggest TLS layer issues
- MALFORMED_HTTP_RESPONSE could be TLS corruption

**Potential Issues**:
- Partial writes breaking TLS record alignment
- Buffering interfering with TLS record boundaries
- Connection state during response writing

### 4.2 HTTP Protocol Violations

**Possible Issues**:
1. **Status Line**: Wrong format, wrong CRLF endings
2. **Headers**: Malformed headers, missing CRLF between headers and body
3. **Body Encoding**: Incorrect chunked encoding, wrong content-length
4. **Connection Management**: Keep-alive vs close handling

### 4.3 Connection State Problems

**Architecture Considerations**:
- Proxy hijacks connections during CONNECT
- Connection might be in unexpected state during response writing
- Reader/writer concurrency issues
- Buffering state corruption

### 4.4 goproxy Internal Issues

**Less Likely But Possible**:
- Bug in goproxy's response forwarding
- Interaction between HAR recording and response forwarding
- Memory corruption in response object

---

## 5. What's Missing: Critical Diagnostic Gaps

### 5.1 Response Inspection Capabilities

**Missing Elements**:
1. **Raw Response Capture**: No mechanism to capture bytes sent to client
2. **TLS Record Inspection**: No visibility into TLS layer
3. **HTTP Format Validation**: No validation of response format
4. **Connection State Logging**: Limited visibility into connection state

### 5.2 Current Diagnostic Limitations

From `recordResponseHandler` (Lines 413-434):
- Only logs status code and duration
- HAR recording captures response but doesn't validate format
- No byte-level inspection
- No connection state logging

### 5.3 Test Validation Gaps

**Integration Tests Only Check**:
- Status code (Line 425 in handlers.go)
- Response body content (Lines 200-203 in auth_integration_test.go)
- Upstream header receipt

**NOT Validated**:
- Actual HTTP response format
- TLS integrity
- Client-side parsing success

---

## 6. Critical Ambiguities That Must Be Resolved

### 6.1 Response Format Ambiguity

**Question**: What exactly is malformed in the HTTP response?
- Is it the status line format?
- Are headers corrupted?
- Is the body encoding wrong?
- Is there TLS framing corruption?

**Current State**: Unknown - no mechanism to capture actual bytes sent to client

### 6.2 goproxy Behavior Ambiguity

**Question**: How does goproxy actually forward responses?
- What internal mechanism does it use?
- Does it use `resp.Write()` directly?
- Are there buffering considerations?

**Current State**: Assumed to work correctly but unverified

### 6.3 TLS Connection Ambiguity

**Question**: What's the actual state of the TLS connection during response writing?
- Is the connection properly established?
- Is it in the right state for response writing?
- Are there any TLS-level issues?

**Current State**: No TLS state logging or validation

### 6.4 Client Behavior Ambiguity

**Question**: Why do test clients accept responses but real clients reject them?
- What parsing differences exist?
- What strictness differences exist?
- What timing differences exist?

**Current State**: Uninvestigated - need real client testing

---

## 7. Specific File Locations and Issues

### 7.1 Critical Code Locations

**File**: `internal/proxy/handlers.go`
- **Lines 400-438**: `recordResponseHandler()` - Returns response unchanged
- **Line 436**: `return resp` - No modification, just logging

**File**: `internal/proxy/server.go`
- **Line 162**: `proxy.OnResponse(ChaperoneRespCondition(registry)).DoFunc(recordResponseHandler(s.recorder))`
- This is where response handling is configured

### 7.2 Test Validation Locations

**File**: `test/integration/auth_integration_test.go`
- **Lines 194-203**: Response validation (only checks status and body content)
- **Lines 780-796**: Concurrent request validation (no response format check)

---

## 8. Recommended Diagnostic Approach

### 8.1 Immediate Diagnostics Needed

1. **Capture Raw Response Bytes**:
   - Implement byte capture before sending to client
   - Compare with known-good responses
   - Use HAR recording as baseline

2. **Add Response Validation**:
   - Validate HTTP format (status line, headers, body)
   - Check for proper CRLF endings
   - Validate header format and values

3. **TLS Layer Inspection**:
   - Add TLS state logging
   - Check for record boundary alignment
   - Monitor connection state

4. **Real Client Testing**:
   - Test with Claude Code directly
   - Capture what it actually receives
   - Compare with test client behavior

### 8.2 Architecture Questions to Answer

1. **Is the issue in response generation or response forwarding?**
   - Create handler that generates test responses
   - See if they work
   - Isolate the problem area

2. **Is it TLS-specific or HTTP-agnostic?**
   - Test with HTTP (non-TLS) connections
   - See if problem persists
   - Narrow down the layer

3. **Is it buffering-related?**
   - Test with different buffering strategies
   - Try no buffering
   - Try different flush patterns

---

## 9. Conclusion

The HTTP response handling in this MITM proxy is **fundamentally broken** despite passing all tests. The architecture appears sound, but there's a critical gap between the proxy's output and what real clients expect.

**Key Findings**:
1. Response handler returns responses unchanged (no modification)
2. Tests only validate basic response properties, not format
3. No diagnostic capabilities for response inspection
4. TLS layer and connection state are unmonitored
5. Root cause remains unknown and uninvestigated

**Critical Next Steps**:
1. Implement response byte capture and inspection
2. Add real client testing framework
3. Investigate TLS response framing
4. Add HTTP format validation
5. Create diagnostic logging for all response handling

**Bottom Line**: The proxy cannot be considered functional until real clients can successfully parse and use the responses it generates. The current test passing creates a dangerous false sense of security.

---

**Status**: CRITICAL BROKEN - Needs Immediate Diagnosis
**Priority**: P0 - Real functionality is impaired
**Estimate**: Significant effort required - root cause unknown
**Risk**: High - Proxy cannot be used with real clients
