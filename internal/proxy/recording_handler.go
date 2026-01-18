// Package proxy provides HTTP/HTTPS proxy handlers for MITM and credential injection.
// This file contains HAR recording handlers for request/response capture.
package proxy

import (
	"net/http"
	"time"

	"github.com/bmf/chaperone/internal/log"
	"github.com/bmf/chaperone/internal/recorder"
	"github.com/elazarl/goproxy"
)

// recordRequestHandler creates a handler that captures request start time for HAR recording.
func recordRequestHandler(rec *recorder.Recorder) func(*http.Request, *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	return func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		// Get or create metadata
		if meta, ok := ctx.UserData.(*requestMetadata); ok {
			meta.request = r.Clone(r.Context())
			if meta.startTime.IsZero() {
				meta.startTime = time.Now()
			}
		} else {
			ctx.UserData = &requestMetadata{
				startTime: time.Now(),
				request:   r.Clone(r.Context()),
				requestID: log.RequestID(r.Context()),
			}
		}

		return r, nil
	}
}

// recordResponseHandler creates a handler that completes HAR entry with response data.
// It also logs the response status code for the request.
func recordResponseHandler(rec *recorder.Recorder) func(*http.Response, *goproxy.ProxyCtx) *http.Response {
	return func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
		if ctx.UserData == nil {
			return resp
		}

		meta, ok := ctx.UserData.(*requestMetadata)
		if !ok || meta.request == nil {
			return resp
		}

		// Record the request and response
		endTime := time.Now()
		recordResponse := rec.RecordRequest(meta.request, meta.startTime)
		recordResponse(resp, nil, endTime)

		// Log request completion with response status
		// Use stored request ID for log correlation with the request line
		reqCtx := meta.request.Context()
		duration := endTime.Sub(meta.startTime)

		logArgs := []any{
			"method", meta.request.Method,
			"host", meta.request.Host,
			"path", meta.request.URL.Path,
			"status", resp.StatusCode,
			"duration_ms", duration.Milliseconds(),
		}

		// Include stripped headers if any
		if len(meta.strippedHeaders) > 0 {
			logArgs = append(logArgs, "stripped_headers", meta.strippedHeaders)
		}

		log.Info(reqCtx, "request completed", logArgs...)

		return resp
	}
}
