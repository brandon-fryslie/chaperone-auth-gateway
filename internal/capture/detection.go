package capture

import (
	"net/http"
	"strings"
)

// MaxNonStreamingBodySize is the maximum body size we'll capture in full.
// Larger bodies are treated as streaming to avoid memory issues.
const MaxNonStreamingBodySize = 10 * 1024 * 1024 // 10MB

// isStreamingResponse detects if a response is streaming based on headers.
// Streaming responses are not fully captured to avoid memory issues and blocking.
func isStreamingResponse(resp *http.Response) bool {
	contentType := resp.Header.Get("Content-Type")
	transferEncoding := resp.Header.Get("Transfer-Encoding")
	contentLength := resp.ContentLength

	// SSE (Server-Sent Events) - always streaming
	if contentType == "text/event-stream" {
		return true
	}

	// Chunked transfer encoding - likely streaming
	if transferEncoding == "chunked" {
		return true
	}

	// Unknown content length - assume streaming
	if contentLength < 0 {
		return true
	}

	// Very large responses - treat as streaming to avoid memory issues
	if contentLength > MaxNonStreamingBodySize {
		return true
	}

	return false
}

// isStreamingRequest detects if a request is streaming.
// This is less common but possible (e.g., large file uploads).
func isStreamingRequest(req *http.Request) bool {
	contentType := req.Header.Get("Content-Type")
	transferEncoding := req.Header.Get("Transfer-Encoding")
	contentLength := req.ContentLength

	// Chunked transfer encoding
	if transferEncoding == "chunked" {
		return true
	}

	// Unknown content length
	if contentLength < 0 {
		return true
	}

	// Very large requests
	if contentLength > MaxNonStreamingBodySize {
		return true
	}

	// Multipart uploads can be large
	if strings.HasPrefix(contentType, "multipart/") && contentLength > 1024*1024 {
		return true
	}

	return false
}

// detectBodyType determines the body type from content and headers.
func detectBodyType(body []byte, contentType string) BodyType {
	if len(body) == 0 {
		return BodyTypeEmpty
	}

	// JSON content types
	if isJSONContentType(contentType) {
		return BodyTypeJSON // Will be validated during formatting
	}

	// Binary content types
	if isBinaryContentType(contentType) {
		return BodyTypeBinary
	}

	// Default to text
	return BodyTypeText
}

// isJSONContentType checks if content-type indicates JSON.
func isJSONContentType(contentType string) bool {
	return strings.Contains(contentType, "application/json") ||
		strings.Contains(contentType, "application/ld+json") ||
		strings.Contains(contentType, "application/vnd.api+json") ||
		strings.HasSuffix(contentType, "+json")
}

// isBinaryContentType checks if content-type indicates binary data.
func isBinaryContentType(contentType string) bool {
	binaryPrefixes := []string{
		"image/",
		"video/",
		"audio/",
		"application/octet-stream",
		"application/pdf",
		"application/zip",
		"application/gzip",
	}

	for _, prefix := range binaryPrefixes {
		if strings.HasPrefix(contentType, prefix) {
			return true
		}
	}

	return false
}
