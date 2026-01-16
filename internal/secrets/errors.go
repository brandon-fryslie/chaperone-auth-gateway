package secrets

import (
	"fmt"
)

// ErrSecretNotFound is returned when a secret cannot be found in its source.
var ErrSecretNotFound = fmt.Errorf("secret not found")
