// Package capture provides unified HTTP traffic capture for HAR and JSONL recording.
// It implements memory-safe body capture with streaming detection to avoid
// interference with proxy operations.
package capture

import (
	"net/http"
	"time"
)

// BodyType indicates how the body was captured and should be formatted.
type BodyType string

const (
	// BodyTypeJSON indicates a JSON body that was successfully parsed
	BodyTypeJSON BodyType = "json"
	// BodyTypeJSONInvalid indicates invalid JSON (stored as string)
	BodyTypeJSONInvalid BodyType = "json_invalid"
	// BodyTypeText indicates plain text body
	BodyTypeText BodyType = "text"
	// BodyTypeStreaming indicates a streaming response (SSE, chunked, etc.)
	BodyTypeStreaming BodyType = "streaming"
	// BodyTypeBinary indicates binary data (image, video, etc.)
	BodyTypeBinary BodyType = "binary"
	// BodyTypeEmpty indicates no body
	BodyTypeEmpty BodyType = "empty"
)

// CapturedRequest contains captured request data for recording.
type CapturedRequest struct {
	Method    string
	URL       string
	Host      string
	Path      string
	Headers   http.Header
	Body      []byte // nil for streaming or empty bodies
	BodyType  BodyType
	BodyInfo  string // Human-readable info (e.g., "streaming", "5MB binary")
	BodyError string // Error message if body capture failed
	StartTime time.Time
}

// CapturedResponse contains captured response data for recording.
type CapturedResponse struct {
	Status      int
	StatusText  string
	Headers     http.Header
	Body        []byte // nil for streaming or empty bodies
	BodyType    BodyType
	BodyInfo    string // Human-readable info
	BodyError   string // Error message if body capture failed
	EndTime     time.Time
	DurationMS  int64 // Duration in milliseconds
	ContentType string
}

// CapturedEntry is a complete request/response pair.
type CapturedEntry struct {
	Request  *CapturedRequest
	Response *CapturedResponse
}
