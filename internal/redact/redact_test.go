package redact

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The zero value must already enforce the positional policy: an unredacted
// recording path is unrepresentable, not a forgotten constructor argument.
func TestZeroValueRedactsCredentialPositions(t *testing.T) {
	var r Redactor

	h := http.Header{
		"Authorization": {"Bearer live-token"},
		"Cookie":        {"session=abc123"},
		"Content-Type":  {"application/json"},
	}

	out := r.Headers(h)

	assert.Equal(t, []string{Placeholder}, out["Authorization"])
	assert.Equal(t, []string{Placeholder}, out["Cookie"])
	assert.Equal(t, []string{"application/json"}, out["Content-Type"])
}

func TestHeadersRedactsPositionsCaseInsensitively(t *testing.T) {
	var r Redactor

	h := http.Header{}
	// Bypass canonicalization to simulate arbitrary client capitalization.
	h["authorization"] = []string{"Bearer x"}
	h["PROXY-AUTHORIZATION"] = []string{"Basic y"}
	h["Set-Cookie"] = []string{"sid=1; HttpOnly"}
	h["X-Api-Key"] = []string{"k-123"}

	out := r.Headers(h)

	for name, values := range out {
		assert.Equal(t, []string{Placeholder}, values, "header %s must be redacted", name)
	}
}

func TestHeadersScrubsKnownValuesFromNonCredentialPositions(t *testing.T) {
	r := NewRedactor(Static("sk-leaked-value"))

	h := http.Header{
		"X-Debug-Echo": {"prefix sk-leaked-value suffix"},
	}

	out := r.Headers(h)

	assert.Equal(t, []string{"prefix " + Placeholder + " suffix"}, out["X-Debug-Echo"])
}

func TestValueScrubsContainedSecretsLongestFirst(t *testing.T) {
	// The longer secret embeds the shorter one. Scrubbing the shorter first
	// would split the longer and leave recognizable fragments behind.
	r := NewRedactor(Static("token", "prefix-token-suffix"))

	out := r.Value("body holds prefix-token-suffix and token alone")

	assert.NotContains(t, out, "prefix-")
	assert.NotContains(t, out, "-suffix")
	assert.Equal(t, "body holds "+Placeholder+" and "+Placeholder+" alone", out)
}

func TestEmptyValuesAreNotScrubbed(t *testing.T) {
	r := NewRedactor(Static("", "real-secret"))

	out := r.Value("contains real-secret only")

	assert.Equal(t, "contains "+Placeholder+" only", out)
	assert.False(t, strings.Contains(out, Placeholder+Placeholder),
		"an empty source value must not shred the output")
}

func TestBytesDoesNotMutateInput(t *testing.T) {
	r := NewRedactor(Static("real-secret"))
	in := []byte(`{"key":"real-secret"}`)

	out := r.Bytes(in)

	assert.Equal(t, []byte(`{"key":"real-secret"}`), in)
	assert.Equal(t, `{"key":"`+Placeholder+`"}`, string(out))
}

// Sources are consulted at redaction time, so secrets learned after the
// Redactor was built (e.g. a runtime grant's credential) are still scrubbed.
func TestValueSourceIsConsultedAtRedactionTime(t *testing.T) {
	var known []string
	r := NewRedactor(func() []string { return known })

	assert.Equal(t, "late-secret", r.Value("late-secret"))

	known = append(known, "late-secret")
	assert.Equal(t, Placeholder, r.Value("late-secret"))
}
