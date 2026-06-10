package recorder

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/bmf/chaperone/internal/redact"
)

// HAREntry represents a single HTTP request/response pair in HAR format
type HAREntry struct {
	StartedDateTime string      `json:"startedDateTime"`
	Time            int         `json:"time"`
	Request         HARRequest  `json:"request"`
	Response        HARResponse `json:"response"`
	Cache           HARCache    `json:"cache"`
	Timings         HARTimings  `json:"timings"`
	Comment         string      `json:"comment,omitempty"`
}

// HARRequest represents an HTTP request in HAR format
type HARRequest struct {
	Method      string           `json:"method"`
	URL         string           `json:"url"`
	HTTPVersion string           `json:"httpVersion"`
	Cookies     []HARCookie      `json:"cookies"`
	Headers     []HARHeader      `json:"headers"`
	QueryString []HARQueryString `json:"queryString"`
	PostData    *HARPostData     `json:"postData,omitempty"`
	HeadersSize int              `json:"headersSize"`
	BodySize    int              `json:"bodySize"`
}

// HARResponse represents an HTTP response in HAR format
type HARResponse struct {
	Status      int         `json:"status"`
	StatusText  string      `json:"statusText"`
	HTTPVersion string      `json:"httpVersion"`
	Cookies     []HARCookie `json:"cookies"`
	Headers     []HARHeader `json:"headers"`
	Content     HARContent  `json:"content"`
	RedirectURL string      `json:"redirectURL"`
	HeadersSize int         `json:"headersSize"`
	BodySize    int         `json:"bodySize"`
}

// HARContent represents response content in HAR format
type HARContent struct {
	Size     int    `json:"size"`
	MIMEType string `json:"mimeType"`
	Text     string `json:"text,omitempty"`
	Encoding string `json:"encoding,omitempty"`
	Comment  string `json:"comment,omitempty"`
}

// HARHeader represents an HTTP header in HAR format
type HARHeader struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	Comment string `json:"comment,omitempty"`
}

// HARCookie represents an HTTP cookie in HAR format
type HARCookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Path     string `json:"path,omitempty"`
	Domain   string `json:"domain,omitempty"`
	Expires  string `json:"expires,omitempty"`
	HTTPOnly bool   `json:"httpOnly,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	Comment  string `json:"comment,omitempty"`
}

// HARQueryString represents a query string parameter in HAR format
type HARQueryString struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	Comment string `json:"comment,omitempty"`
}

// HARPostData represents POST data in HAR format
type HARPostData struct {
	MIMEType string             `json:"mimeType"`
	Text     string             `json:"text"`
	Params   []HARPostDataParam `json:"params,omitempty"`
	Comment  string             `json:"comment,omitempty"`
}

// HARPostDataParam represents a POST data parameter in HAR format
type HARPostDataParam struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	FileName    string `json:"fileName,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Comment     string `json:"comment,omitempty"`
}

// HARCache represents cache information in HAR format
type HARCache struct {
}

// HARTimings represents timing information in HAR format
type HARTimings struct {
	Blocked int    `json:"blocked,omitempty"`
	DNS     int    `json:"dns,omitempty"`
	Connect int    `json:"connect,omitempty"`
	Send    int    `json:"send,omitempty"`
	Wait    int    `json:"wait,omitempty"`
	Receive int    `json:"receive,omitempty"`
	SSL     int    `json:"ssl,omitempty"`
	Comment string `json:"comment,omitempty"`
}

// HAR represents a complete HAR archive
type HAR struct {
	Version string     `json:"version"`
	Creator HARCreator `json:"creator"`
	Browser HARBrowser `json:"browser,omitempty"`
	Pages   []HARPage  `json:"pages,omitempty"`
	Entries []HAREntry `json:"entries"`
	Comment string     `json:"comment,omitempty"`
}

// HARCreator represents the creator of the HAR file
type HARCreator struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Comment string `json:"comment,omitempty"`
}

// HARBrowser represents the browser that generated the HAR
type HARBrowser struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Comment string `json:"comment,omitempty"`
}

// HARPage represents a page in the HAR file
type HARPage struct {
	StartedDateTime string         `json:"startedDateTime"`
	ID              string         `json:"id"`
	Title           string         `json:"title"`
	PageTimings     HARPageTimings `json:"pageTimings"`
	Comment         string         `json:"comment,omitempty"`
}

// HARPageTimings represents page timing information
type HARPageTimings struct {
	OnContentLoad int    `json:"onContentLoad,omitempty"`
	OnLoad        int    `json:"onLoad,omitempty"`
	Comment       string `json:"comment,omitempty"`
}

// Recorder records HTTP requests and responses in HAR format.
// Every entry is redacted as it is built, so the in-memory archive never
// holds credential material — not only the serialized output. [LAW:single-enforcer]
type Recorder struct {
	mu        sync.Mutex
	entries   []HAREntry
	startTime time.Time
	maxBody   int64
	redactor  redact.Redactor
}

// NewRecorder creates a new HAR recorder. The redactor is taken by value:
// its zero value still enforces the positional credential policy, so an
// unredacted recorder cannot be constructed. [LAW:types-are-the-program]
func NewRecorder(redactor redact.Redactor) *Recorder {
	return &Recorder{
		startTime: time.Now(),
		maxBody:   100 * 1024, // 100KB max body size to prevent huge files
		redactor:  redactor,
	}
}

// RecordRequest records an HTTP request and returns a callback to record the response
func (r *Recorder) RecordRequest(req *http.Request, started time.Time) func(resp *http.Response, err error, end time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Create HAR request
	harReq := HARRequest{
		Method:      req.Method,
		URL:         r.redactor.Value(req.URL.String()),
		HTTPVersion: fmt.Sprintf("HTTP/%d.%d", req.ProtoMajor, req.ProtoMinor),
		Headers:     make([]HARHeader, 0, len(req.Header)),
		HeadersSize: -1, // Unknown
		BodySize:    -1, // Unknown
	}

	// Add headers
	for name, values := range r.redactor.Headers(req.Header) {
		for _, value := range values {
			harReq.Headers = append(harReq.Headers, HARHeader{
				Name:  name,
				Value: value,
			})
		}
	}

	// Return callback to record response
	return func(resp *http.Response, err error, end time.Time) {
		r.mu.Lock()
		defer r.mu.Unlock()

		entry := HAREntry{
			StartedDateTime: started.Format(time.RFC3339Nano),
			Time:            int(end.Sub(started).Milliseconds()),
			Request:         harReq,
			Cache:           HARCache{},
			Timings: HARTimings{
				Send:    -1,
				Wait:    int(end.Sub(started).Milliseconds()),
				Receive: -1,
			},
		}

		if err != nil {
			// Record error as response. Error strings can embed request
			// material (URLs, header echoes), so they pass through the
			// redactor like every other recorded string.
			entry.Response = HARResponse{
				Status:      0,
				StatusText:  r.redactor.Value(err.Error()),
				HTTPVersion: fmt.Sprintf("HTTP/%d.%d", req.ProtoMajor, req.ProtoMinor),
				Content: HARContent{
					Size:     0,
					MIMEType: "text/plain",
					Text:     r.redactor.Value(err.Error()),
				},
				HeadersSize: 0,
				BodySize:    -1,
			}
		} else if resp != nil {
			// Record successful response
			entry.Response = HARResponse{
				Status:      resp.StatusCode,
				StatusText:  http.StatusText(resp.StatusCode),
				HTTPVersion: fmt.Sprintf("HTTP/%d.%d", resp.ProtoMajor, resp.ProtoMinor),
				Headers:     make([]HARHeader, 0, len(resp.Header)),
				Content: HARContent{
					Size:     0,
					MIMEType: resp.Header.Get("Content-Type"),
				},
				HeadersSize: -1,
				BodySize:    int(resp.ContentLength),
			}

			// Add response headers
			for name, values := range r.redactor.Headers(resp.Header) {
				for _, value := range values {
					entry.Response.Headers = append(entry.Response.Headers, HARHeader{
						Name:  name,
						Value: value,
					})
				}
			}

			// Do not try to read response body as it is streamed to client
			// Just record the size from Content-Length header
			if resp.ContentLength > 0 {
				entry.Response.Content.Size = int(resp.ContentLength)
			}
		}

		r.entries = append(r.entries, entry)
	}
}

// ToJSON returns the HAR archive as JSON
func (r *Recorder) ToJSON() ([]byte, error) {
	har := HAR{
		Version: "1.2",
		Creator: HARCreator{
			Name:    "Chaperone Auth Gateway",
			Version: "1.0",
		},
		Entries: r.entries,
	}

	return json.MarshalIndent(har, "", "  ")
}

// WriteToFile writes the HAR archive to a file
func (r *Recorder) WriteToFile(filename string) error {
	data, err := r.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal HAR: %w", err)
	}

	// Write with proper formatting
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err != nil {
		return fmt.Errorf("failed to indent JSON: %w", err)
	}

	// Write to file with 0600 permissions (owner read/write only)
	if err := os.WriteFile(filename, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("failed to write HAR file: %w", err)
	}

	return nil
}

// String returns the HAR archive as a formatted JSON string
func (r *Recorder) String() string {
	data, _ := r.ToJSON()
	return string(data)
}
