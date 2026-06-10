package secrets

import "context"

// SecretProvider fetches secrets from a source.
// Implementations include EnvProvider, FileProvider, KeychainProvider.
type SecretProvider interface {
	// Fetch retrieves a secret value for the given path.
	// The path format is provider-specific.
	//
	// The contract splits along what each side can know: a provider detects
	// its own source's absence semantics (unset variable, missing file,
	// missing keychain item → ErrSecretNotFound) and returns the stored value
	// verbatim, including any whitespace its source or transport adds.
	// Registry.Fetch — the one boundary every fetch crosses — normalizes the
	// value (trims surrounding whitespace, rejects empty-after-trim), so no
	// provider carries its own copy of that rule. [LAW:single-enforcer]
	Fetch(ctx context.Context, path string) (string, error)
}
