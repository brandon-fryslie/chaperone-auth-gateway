package secrets

import "strings"

// builtinProviders maps each canonical built-in scheme name to its provider
// constructor. It is the [LAW:one-source-of-truth] for which "provider:" prefixes
// a credential reference may use: both provider registration (RegisterBuiltins)
// and credential-ref validation (IsKnownScheme) derive from this one map, so
// adding a provider is a single edit and the two can never drift.
func builtinProviders() map[string]func() SecretProvider {
	return map[string]func() SecretProvider{
		"env":      func() SecretProvider { return NewEnvProvider() },
		"file":     func() SecretProvider { return NewFileProvider() },
		"keychain": func() SecretProvider { return NewKeychainProvider() },
	}
}

// RegisterBuiltins registers every built-in secret provider on the registry.
func RegisterBuiltins(r *Registry) {
	for name, newProvider := range builtinProviders() {
		r.Register(name, newProvider())
	}
}

// IsKnownScheme reports whether ref begins with a known "provider:" scheme from
// the built-in set. It validates only the scheme, not the path; callers that need
// the secret resolved still go through Registry.Fetch.
func IsKnownScheme(ref string) bool {
	name, _, ok := strings.Cut(ref, ":")
	if !ok {
		return false
	}
	_, known := builtinProviders()[name]
	return known
}
