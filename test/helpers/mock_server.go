package helpers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
)

// RecordedRequest captures details of an HTTP request for testing.
type RecordedRequest struct {
	Method  string
	Path    string
	Headers http.Header
	Body    []byte
}

// MockResponse defines a configured response for mock servers.
type MockResponse struct {
	StatusCode int
	Body       []byte
	Headers    map[string]string
}

// MockServer wraps httptest.Server and provides request recording.
type MockServer struct {
	Server   *httptest.Server
	Requests []RecordedRequest
	mu       sync.Mutex
}

// NewMockServer creates a mock HTTP server with the given handler.
func NewMockServer(handler http.Handler) *MockServer {
	mock := &MockServer{
		Requests: []RecordedRequest{},
	}

	// Wrap handler to record requests
	wrappedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Record request
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close() // Best effort cleanup in test helper

		mock.mu.Lock()
		mock.Requests = append(mock.Requests, RecordedRequest{
			Method:  r.Method,
			Path:    r.URL.Path,
			Headers: r.Header.Clone(),
			Body:    body,
		})
		mock.mu.Unlock()

		// Call original handler
		handler.ServeHTTP(w, r)
	})

	mock.Server = httptest.NewServer(wrappedHandler)
	return mock
}

// NewMockTLSServer creates a mock HTTPS server with the given handler.
func NewMockTLSServer(handler http.Handler) *MockServer {
	mock := &MockServer{
		Requests: []RecordedRequest{},
	}

	// Wrap handler to record requests
	wrappedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Record request
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close() // Best effort cleanup in test helper

		mock.mu.Lock()
		mock.Requests = append(mock.Requests, RecordedRequest{
			Method:  r.Method,
			Path:    r.URL.Path,
			Headers: r.Header.Clone(),
			Body:    body,
		})
		mock.mu.Unlock()

		// Call original handler
		handler.ServeHTTP(w, r)
	})

	mock.Server = httptest.NewTLSServer(wrappedHandler)
	return mock
}

// RecordRequests creates a handler that records all requests and returns configured responses.
// The responses map uses "METHOD /path" as keys.
func RecordRequests(responses map[string]MockResponse) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Build lookup key
		key := r.Method + " " + r.URL.Path

		// Find matching response
		response, ok := responses[key]
		if !ok {
			// Default 404 if no matching response
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("Not Found")) // Ignore error in test helper
			return
		}

		// Set headers
		for k, v := range response.Headers {
			w.Header().Set(k, v)
		}

		// Write response
		w.WriteHeader(response.StatusCode)
		_, _ = w.Write(response.Body) // Ignore error in test helper
	})
}
