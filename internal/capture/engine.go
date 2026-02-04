package capture

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Engine provides memory-safe HTTP traffic capture for recording.
type Engine struct {
	maxBodySize int
}

// NewEngine creates a new capture engine.
func NewEngine(maxBodySize int) *Engine {
	if maxBodySize <= 0 {
		maxBodySize = 1024 * 1024 // Default: 1MB
	}
	return &Engine{
		maxBodySize: maxBodySize,
	}
}

// CaptureRequest captures request data without consuming the body stream.
// For streaming requests, only metadata is captured.
// For regular requests, the full body is captured using TeeReader.
// The request body is always restored for downstream proxy use.
func (e *Engine) CaptureRequest(req *http.Request, startTime time.Time) (*CapturedRequest, error) {
	captured := &CapturedRequest{
		Method:    req.Method,
		URL:       req.URL.String(),
		Host:      req.Host,
		Path:      req.URL.Path,
		Headers:   req.Header.Clone(),
		StartTime: startTime,
	}

	// No body to capture
	if req.Body == nil {
		captured.BodyType = BodyTypeEmpty
		return captured, nil
	}

	// Detect streaming requests
	if isStreamingRequest(req) {
		captured.BodyType = BodyTypeStreaming
		captured.BodyInfo = fmt.Sprintf("streaming request (%s, Content-Length: %d)",
			req.Header.Get("Content-Type"), req.ContentLength)
		// Leave body untouched for proxy
		return captured, nil
	}

	// Capture full body using TeeReader (non-streaming)
	var buf bytes.Buffer
	teeReader := io.TeeReader(req.Body, &buf)

	// Read ENTIRE body (no limit) to ensure proper restoration
	body, err := io.ReadAll(teeReader)
	req.Body.Close()

	if err != nil {
		captured.BodyError = err.Error()
		// Restore empty body on error
		req.Body = io.NopCloser(bytes.NewReader(nil))
		return captured, err
	}

	// Restore body with FULL buffer copy for downstream handlers
	req.Body = io.NopCloser(&buf)

	// Store captured body
	captured.Body = body
	captured.BodyType = detectBodyType(body, req.Header.Get("Content-Type"))

	return captured, nil
}

// CaptureResponse captures response data without consuming the body stream.
// For streaming responses, only metadata is captured.
// For regular responses, the full body is captured using TeeReader.
// The response body is always restored for downstream proxy use.
func (e *Engine) CaptureResponse(resp *http.Response, endTime time.Time) (*CapturedResponse, error) {
	captured := &CapturedResponse{
		Status:      resp.StatusCode,
		StatusText:  http.StatusText(resp.StatusCode),
		Headers:     resp.Header.Clone(),
		EndTime:     endTime,
		ContentType: resp.Header.Get("Content-Type"),
	}

	// No body to capture
	if resp.Body == nil {
		captured.BodyType = BodyTypeEmpty
		return captured, nil
	}

	// Detect streaming responses
	if isStreamingResponse(resp) {
		captured.BodyType = BodyTypeStreaming
		ct := resp.Header.Get("Content-Type")
		te := resp.Header.Get("Transfer-Encoding")
		cl := resp.ContentLength

		if ct == "text/event-stream" {
			captured.BodyInfo = "text/event-stream (SSE), not captured"
		} else if te == "chunked" {
			captured.BodyInfo = "chunked transfer encoding, not captured"
		} else if cl < 0 {
			captured.BodyInfo = "unknown content length, not captured"
		} else if cl > MaxNonStreamingBodySize {
			captured.BodyInfo = fmt.Sprintf("large response (%d bytes), not captured", cl)
		} else {
			captured.BodyInfo = "streaming response, not captured"
		}

		// Leave body untouched for proxy
		return captured, nil
	}

	// Capture full body using TeeReader (non-streaming)
	var buf bytes.Buffer
	teeReader := io.TeeReader(resp.Body, &buf)

	// Read ENTIRE body (no limit) to ensure proper restoration
	body, err := io.ReadAll(teeReader)
	resp.Body.Close()

	if err != nil {
		captured.BodyError = err.Error()
		// Restore empty body on error
		resp.Body = io.NopCloser(bytes.NewReader(nil))
		return captured, err
	}

	// Restore body with FULL buffer copy for downstream handlers
	resp.Body = io.NopCloser(&buf)

	// Store captured body
	captured.Body = body
	captured.BodyType = detectBodyType(body, resp.Header.Get("Content-Type"))

	return captured, nil
}

// CaptureEntry captures both request and response for a complete transaction.
func (e *Engine) CaptureEntry(req *http.Request, resp *http.Response, startTime, endTime time.Time) (*CapturedEntry, error) {
	capturedReq, reqErr := e.CaptureRequest(req, startTime)
	capturedResp, respErr := e.CaptureResponse(resp, endTime)

	entry := &CapturedEntry{
		Request:  capturedReq,
		Response: capturedResp,
	}

	// Calculate duration
	if capturedResp != nil && capturedReq != nil {
		capturedResp.DurationMS = endTime.Sub(startTime).Milliseconds()
	}

	// Return first error encountered
	if reqErr != nil {
		return entry, reqErr
	}
	return entry, respErr
}
