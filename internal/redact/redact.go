// Package redact removes credential material from data on its way into a
// durable artifact. It is the single redaction policy for recorders: which
// header positions carry credentials is defined once here, and the secret
// values the process actually holds are scrubbed from every recorded string.
// [LAW:single-enforcer]
package redact

import (
	"net/http"
	"sort"
	"strings"

	"github.com/bmf/chaperone/internal/auth"
)

// Placeholder replaces every redacted value in recorded output.
const Placeholder = "[REDACTED]"

// recordedCredentialPositions is the set of header names whose values are
// credentials and must never persist in a recording. It is derived from
// auth.KnownAuthHeaders — the canonical list of app credential headers —
// extended with positions only a recording must cover: the proxy's own
// access credential and session cookies, which the proxy forwards live but
// must never write to disk. [LAW:one-source-of-truth]
var recordedCredentialPositions = func() map[string]struct{} {
	positions := map[string]struct{}{
		"proxy-authorization": {},
		"cookie":              {},
		"set-cookie":          {},
	}
	for _, h := range auth.KnownAuthHeaders {
		positions[h] = struct{}{}
	}
	return positions
}()

// ValueSource returns the secret values currently known to the process.
// It is consulted at redaction time, not construction time, so values
// learned later (e.g. a credential first resolved for a runtime grant)
// are still scrubbed. [LAW:no-ambient-temporal-coupling]
type ValueSource func() []string

// Static returns a ValueSource over a fixed set of values.
func Static(values ...string) ValueSource {
	return func() []string { return values }
}

// Redactor scrubs credential material from data bound for a recording.
// The zero value still enforces the full positional policy (with no known
// values to scrub), so a recording path with redaction "off" is
// unrepresentable. [LAW:types-are-the-program]
type Redactor struct {
	sources []ValueSource
}

// NewRedactor builds a Redactor that, beyond the positional policy,
// scrubs every value reported by the given sources from recorded strings.
func NewRedactor(sources ...ValueSource) Redactor {
	return Redactor{sources: sources}
}

// Headers returns a copy of h that is safe to persist: values in credential
// positions are replaced wholesale, and known secret values are scrubbed
// from everything else.
func (r Redactor) Headers(h http.Header) http.Header {
	values := r.knownValues()
	out := make(http.Header, len(h))
	for name, vals := range h {
		if _, isCredential := recordedCredentialPositions[strings.ToLower(name)]; isCredential {
			out[name] = []string{Placeholder}
			continue
		}
		scrubbed := make([]string, len(vals))
		for i, v := range vals {
			scrubbed[i] = scrub(v, values)
		}
		out[name] = scrubbed
	}
	return out
}

// Value scrubs every known secret value from s.
func (r Redactor) Value(s string) string {
	return scrub(s, r.knownValues())
}

// Bytes scrubs every known secret value from b. The input is never mutated.
func (r Redactor) Bytes(b []byte) []byte {
	if len(b) == 0 {
		return b
	}
	return []byte(scrub(string(b), r.knownValues()))
}

func scrub(s string, values []string) string {
	for _, v := range values {
		s = strings.ReplaceAll(s, v, Placeholder)
	}
	return s
}

// knownValues snapshots the current secret values, longest first so a
// secret that embeds another is replaced whole before the shorter one can
// split it and leave recognizable fragments behind.
func (r Redactor) knownValues() []string {
	var values []string
	for _, source := range r.sources {
		for _, v := range source() {
			// An empty "secret" is not a value to scrub, and replacing the
			// empty string would shred the output.
			if v != "" {
				values = append(values, v)
			}
		}
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	return values
}
