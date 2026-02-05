package recorder

import (
	"net/http"
	"time"

	"github.com/bmf/chaperone/internal/capture"
)

// MultiRecorder records HTTP traffic to multiple formats simultaneously.
type MultiRecorder struct {
	harRecorder   *Recorder
	jsonlRecorder *JSONLRecorder
	engine        *capture.Engine
}

// NewMultiRecorder creates a recorder that can write to HAR, JSONL, or both.
func NewMultiRecorder(harRecorder *Recorder, jsonlRecorder *JSONLRecorder) *MultiRecorder {
	return &MultiRecorder{
		harRecorder:   harRecorder,
		jsonlRecorder: jsonlRecorder,
		engine:        capture.NewEngine(10 * 1024 * 1024),
	}
}

// RecordRequest captures the request and returns a callback to record the response.
// This method uses the unified capture engine and writes to all enabled formats.
func (m *MultiRecorder) RecordRequest(req *http.Request, started time.Time) func(resp *http.Response, err error, end time.Time) {
	// Capture request using unified engine
	capturedReq, _ := m.engine.CaptureRequest(req, started)

	// Get request ID from context (if available)
	requestID := getRequestIDFromContext(req.Context())

	return func(resp *http.Response, err error, end time.Time) {
		// Handle error case
		if err != nil {
			// Write to HAR if enabled
			if m.harRecorder != nil {
				harCallback := m.harRecorder.RecordRequest(req, started)
				harCallback(resp, err, end)
			}
			// JSONL doesn't record errors currently
			return
		}

		// Capture response using unified engine
		capturedResp, _ := m.engine.CaptureResponse(resp, end)
		if capturedResp != nil {
			capturedResp.DurationMS = end.Sub(started).Milliseconds()
		}

		// Write to HAR format
		if m.harRecorder != nil {
			harCallback := m.harRecorder.RecordRequest(req, started)
			harCallback(resp, nil, end)
		}

		// Write to JSONL format
		if m.jsonlRecorder != nil && capturedReq != nil && capturedResp != nil {
			responseCallback := m.jsonlRecorder.RecordRequest(capturedReq, requestID)
			responseCallback(capturedResp)
		}
	}
}

// Close closes all underlying recorders.
func (m *MultiRecorder) Close() error {
	if m.jsonlRecorder != nil {
		if err := m.jsonlRecorder.Close(); err != nil {
			return err
		}
	}
	return nil
}

// Helper to extract request ID from context
func getRequestIDFromContext(ctx interface{}) string {
	// TODO: Extract actual request ID from context
	// For now, return empty string
	return ""
}
