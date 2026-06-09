package secrets

import "testing"

func TestIsKnownScheme(t *testing.T) {
	cases := map[string]bool{
		"env:OPENAI_API_KEY":   true,
		"file:/etc/secret":     true,
		"keychain:svc/account": true,
		"vault:secret/path":    false, // not a built-in scheme
		"OPENAI_API_KEY":       false, // no scheme separator
		"":                     false,
	}
	for ref, want := range cases {
		if got := IsKnownScheme(ref); got != want {
			t.Errorf("IsKnownScheme(%q) = %v, want %v", ref, got, want)
		}
	}
}

// RegisterBuiltins and IsKnownScheme must derive from the same set: every scheme
// the registry registers is a known scheme, and vice versa.
func TestRegisterBuiltinsMatchesKnownSchemes(t *testing.T) {
	r := NewRegistry()
	RegisterBuiltins(r)
	for name := range builtinProviders() {
		if !r.HasProvider(name) {
			t.Errorf("RegisterBuiltins did not register provider %q", name)
		}
		if !IsKnownScheme(name + ":path") {
			t.Errorf("IsKnownScheme rejects registered scheme %q", name)
		}
	}
}
