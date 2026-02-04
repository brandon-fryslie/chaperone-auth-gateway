package recorder

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/bmf/chaperone/internal/capture"
)

// JSONLRecorder records HTTP traffic in JSON Lines format.
// Each line is a complete JSON object representing a request or response.
type JSONLRecorder struct {
	mu      sync.Mutex
	writer  io.Writer
	file    *os.File // If writing to file, for cleanup
	engine  *capture.Engine
}

// JSONLEntry represents a single JSONL entry (request or response).
type JSONLEntry struct {
	Timestamp   string                 `json:"ts"`
	RequestID   string                 `json:"req_id,omitempty"`
	Type        string                 `json:"type"` // "request" or "response"
	Method      string                 `json:"method,omitempty"`
	URL         string                 `json:"url,omitempty"`
	Host        string                 `json:"host,omitempty"`
	Path        string                 `json:"path,omitempty"`
	Status      int                    `json:"status,omitempty"`
	StatusText  string                 `json:"status_text,omitempty"`
	Headers     map[string]string      `json:"headers,omitempty"`
	Body        interface{}            `json:"body,omitempty"`
	BodyType    string                 `json:"body_type,omitempty"`
	BodyInfo    string                 `json:"body_info,omitempty"`
	BodyError   string                 `json:"body_error,omitempty"`
	DurationMS  int64                  `json:"duration_ms,omitempty"`
}

// NewJSONLRecorder creates a new JSONL recorder writing to the given writer.
func NewJSONLRecorder(writer io.Writer) *JSONLRecorder {
	return &JSONLRecorder{
		writer: writer,
		engine: capture.NewEngine(10 * 1024 * 1024), // 10MB max
	}
}

// NewJSONLFileRecorder creates a new JSONL recorder writing to a file.
func NewJSONLFileRecorder(path string) (*JSONLRecorder, error) {
	file, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("failed to create JSONL file: %w", err)
	}

	// Set permissions to 0600 (owner read/write only)
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to set file permissions: %w", err)
	}

	return &JSONLRecorder{
		writer: file,
		file:   file,
		engine: capture.NewEngine(10 * 1024 * 1024),
	}, nil
}

// RecordRequest records a request and returns a callback to record the response.
func (r *JSONLRecorder) RecordRequest(capturedReq *capture.CapturedRequest, requestID string) func(*capture.CapturedResponse) {
	// Write request entry immediately
	r.writeRequestEntry(capturedReq, requestID)

	// Return callback to write response
	return func(capturedResp *capture.CapturedResponse) {
		r.writeResponseEntry(capturedResp, requestID)
	}
}

// writeRequestEntry writes a request entry to JSONL.
func (r *JSONLRecorder) writeRequestEntry(req *capture.CapturedRequest, requestID string) {
	entry := JSONLEntry{
		Timestamp: req.StartTime.UTC().Format(time.RFC3339Nano),
		RequestID: requestID,
		Type:      "request",
		Method:    req.Method,
		URL:       req.URL,
		Host:      req.Host,
		Path:      req.Path,
		Headers:   flattenHeaders(req.Headers),
		BodyType:  string(req.BodyType),
		BodyInfo:  req.BodyInfo,
		BodyError: req.BodyError,
	}

	// Include body if captured
	if len(req.Body) > 0 {
		entry.Body = formatBody(req.Body, req.BodyType)
	}

	r.writeEntry(entry)
}

// writeResponseEntry writes a response entry to JSONL.
func (r *JSONLRecorder) writeResponseEntry(resp *capture.CapturedResponse, requestID string) {
	entry := JSONLEntry{
		Timestamp:  resp.EndTime.UTC().Format(time.RFC3339Nano),
		RequestID:  requestID,
		Type:       "response",
		Status:     resp.Status,
		StatusText: resp.StatusText,
		Headers:    flattenHeaders(resp.Headers),
		BodyType:   string(resp.BodyType),
		BodyInfo:   resp.BodyInfo,
		BodyError:  resp.BodyError,
		DurationMS: resp.DurationMS,
	}

	// Include body if captured
	if len(resp.Body) > 0 {
		entry.Body = formatBody(resp.Body, resp.BodyType)
	}

	r.writeEntry(entry)
}

// formatBody formats the body based on its type.
func formatBody(body []byte, bodyType capture.BodyType) interface{} {
	switch bodyType {
	case capture.BodyTypeJSON:
		// Try to parse as JSON for better querying
		var parsed interface{}
		if err := json.Unmarshal(body, &parsed); err == nil {
			return parsed
		}
		// If parse fails, return as string with invalid flag
		return string(body)

	case capture.BodyTypeBinary:
		// Don't include binary data - just metadata
		return map[string]interface{}{
			"_binary": true,
			"size":    len(body),
		}

	default:
		// Text, invalid JSON, etc. - return as string
		return string(body)
	}
}

// flattenHeaders converts http.Header to a simple map.
// Takes the first value for each header (most common case).
func flattenHeaders(headers map[string][]string) map[string]string {
	result := make(map[string]string, len(headers))
	for key, values := range headers {
		if len(values) > 0 {
			result[key] = values[0]
		}
	}
	return result
}

// writeEntry writes a JSON line to the output.
func (r *JSONLRecorder) writeEntry(entry JSONLEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Marshal to JSON (single line)
	data, err := json.Marshal(entry)
	if err != nil {
		// Log error but don't fail - recording is best-effort
		return
	}

	// Write JSON line + newline
	r.writer.Write(data)
	r.writer.Write([]byte("\n"))
}

// Close closes the underlying file if this recorder owns it.
func (r *JSONLRecorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.file != nil {
		return r.file.Close()
	}
	return nil
}

// GetEngine returns the capture engine for direct use.
func (r *JSONLRecorder) GetEngine() *capture.Engine {
	return r.engine
}
