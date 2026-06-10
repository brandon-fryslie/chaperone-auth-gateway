package recorder

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/bmf/chaperone/internal/capture"
	"github.com/bmf/chaperone/internal/redact"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The JSONL recorder is the one sink that persists bodies, so a known secret
// placed in the URL, a non-credential header, and both bodies must be gone
// from the written bytes — and credential-position headers redacted wholesale.
func TestJSONLRecorderRedactsEntries(t *testing.T) {
	const secret = "sk-jsonl-canary-secret-91xb"

	var buf bytes.Buffer
	r := NewJSONLRecorder(&buf, redact.NewRedactor(redact.Static(secret)))

	capturedReq := &capture.CapturedRequest{
		Method: "POST",
		URL:    "https://api.example.com/v1/complete?key=" + secret,
		Host:   "api.example.com",
		Path:   "/v1/complete",
		Headers: http.Header{
			"Authorization": {"Bearer " + secret},
			"Cookie":        {"session=live-session-cookie"},
			"X-Debug-Echo":  {"echo " + secret},
			"Content-Type":  {"application/json"},
		},
		Body:      []byte(`{"token":"` + secret + `","prompt":"hi"}`),
		BodyType:  capture.BodyTypeJSON,
		StartTime: time.Now(),
	}

	respond := r.RecordRequest(capturedReq, "req-1")
	respond(&capture.CapturedResponse{
		Status:     200,
		StatusText: "OK",
		Headers: http.Header{
			"Set-Cookie":   {"sid=server-set-cookie; HttpOnly"},
			"Content-Type": {"application/json"},
		},
		Body:     []byte(`{"echo":"` + secret + `"}`),
		BodyType: capture.BodyTypeJSON,
		EndTime:  time.Now(),
	})
	require.NoError(t, r.Close())

	out := buf.String()
	assert.NotContains(t, out, secret, "secret must not appear anywhere in JSONL output")
	assert.NotContains(t, out, "live-session-cookie", "cookie value must not persist")
	assert.NotContains(t, out, "server-set-cookie", "set-cookie value must not persist")
	assert.Contains(t, out, redact.Placeholder)
	assert.Contains(t, out, `"Content-Type":"application/json"`,
		"non-credential headers must survive redaction intact")
}

// The HAR recorder records transport errors as synthetic responses; error
// strings can echo request material, so they pass through the redactor too.
func TestHARRecorderRedactsErrorText(t *testing.T) {
	const secret = "sk-har-canary-error-secret"

	rec := NewRecorder(redact.NewRedactor(redact.Static(secret)))

	req, err := http.NewRequest("GET", "https://api.example.com/v1?key="+secret, nil)
	require.NoError(t, err)
	req.Header.Set("Cookie", "session=har-cookie-value")

	respond := rec.RecordRequest(req, time.Now())
	respond(nil, fmt.Errorf("connect to https://api.example.com/v1?key=%s: refused", secret), time.Now())

	out, err := rec.ToJSON()
	require.NoError(t, err)
	assert.NotContains(t, string(out), secret)
	assert.NotContains(t, string(out), "har-cookie-value")
}
