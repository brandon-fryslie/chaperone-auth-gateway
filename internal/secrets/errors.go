package secrets

import (
	cherrors "github.com/bmf/chaperone/internal/errors"
)

// ErrSecretNotFound is an alias for the canonical sentinel in internal/errors
// — the same value, not a second sentinel — so errors.Is matches no matter
// which package name a caller reaches it through. [LAW:one-source-of-truth]
var ErrSecretNotFound = cherrors.ErrSecretNotFound
