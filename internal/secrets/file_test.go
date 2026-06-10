package secrets

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cherrors "github.com/bmf/chaperone/internal/errors"
)

// writeSecretWithMode writes a secret file and then chmods it explicitly, so
// the test's mode is exact rather than umask-filtered.
func writeSecretWithMode(t *testing.T, content string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	require.NoError(t, os.Chmod(path, mode))
	return path
}

// TestFileProvider_RejectsExposedSecretFile is the acceptance test for the
// trust gate: a secret file any other local user could read or write is
// refused with a loud error naming the fix — never read silently. The file
// holds the credential value itself, so the bar is stricter than config's:
// any group/world bit, including read, is a refusal.
func TestFileProvider_RejectsExposedSecretFile(t *testing.T) {
	provider := NewFileProvider()
	ctx := context.Background()

	rejected := []struct {
		name string
		mode os.FileMode
	}{
		{"world-readable (0644)", 0o644},
		{"group-readable (0640)", 0o640},
		{"world-writable (0666)", 0o666},
	}
	for _, tc := range rejected {
		t.Run(tc.name+" rejected", func(t *testing.T) {
			path := writeSecretWithMode(t, "sk-secret", tc.mode)
			_, err := provider.Fetch(ctx, path)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "readable or writable by group or others")
			assert.Contains(t, err.Error(), "chmod go-rwx")
		})
	}

	t.Run("owner-only (0600) accepted", func(t *testing.T) {
		path := writeSecretWithMode(t, "sk-secret", 0o600)
		secret, err := provider.Fetch(ctx, path)
		require.NoError(t, err)
		assert.Equal(t, "sk-secret", secret)
	})

	t.Run("owner-read-only (0400) accepted", func(t *testing.T) {
		path := writeSecretWithMode(t, "sk-secret", 0o400)
		secret, err := provider.Fetch(ctx, path)
		require.NoError(t, err)
		assert.Equal(t, "sk-secret", secret)
	})

	t.Run("symlink to an exposed file rejected", func(t *testing.T) {
		target := writeSecretWithMode(t, "sk-secret", 0o644)
		link := filepath.Join(t.TempDir(), "secret-link")
		require.NoError(t, os.Symlink(target, link))
		_, err := provider.Fetch(ctx, link)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "readable or writable by group or others")
	})

	t.Run("symlink to a trusted file accepted", func(t *testing.T) {
		target := writeSecretWithMode(t, "sk-secret", 0o600)
		link := filepath.Join(t.TempDir(), "secret-link")
		require.NoError(t, os.Symlink(target, link))
		secret, err := provider.Fetch(ctx, link)
		require.NoError(t, err)
		assert.Equal(t, "sk-secret", secret)
	})

	t.Run("missing file is ErrSecretNotFound", func(t *testing.T) {
		_, err := provider.Fetch(ctx, filepath.Join(t.TempDir(), "missing"))
		assert.ErrorIs(t, err, ErrSecretNotFound)
	})
}

// fileRegistry builds a registry with the file provider registered, mirroring
// production wiring: normalization lives in Registry.Fetch, so the contract
// is asserted through it.
func fileRegistry() *Registry {
	r := NewRegistry()
	r.Register("file", NewFileProvider())
	return r
}

// TestRegistryFetch_NormalizesSecrets is the acceptance test for the one
// whitespace/newline contract shared across providers: surrounding
// whitespace is trimmed, and empty-after-trim is not found.
func TestRegistryFetch_NormalizesSecrets(t *testing.T) {
	ctx := context.Background()

	t.Run("trailing newline trimmed from file secret", func(t *testing.T) {
		path := writeSecretWithMode(t, "sk-secret\n", 0o600)
		secret, err := fileRegistry().Fetch(ctx, "file:"+path)
		require.NoError(t, err)
		assert.Equal(t, "sk-secret", secret)
	})

	t.Run("surrounding whitespace trimmed from file secret", func(t *testing.T) {
		path := writeSecretWithMode(t, "  sk-secret \r\n", 0o600)
		secret, err := fileRegistry().Fetch(ctx, "file:"+path)
		require.NoError(t, err)
		assert.Equal(t, "sk-secret", secret)
	})

	t.Run("whitespace-only file is not found", func(t *testing.T) {
		path := writeSecretWithMode(t, " \n\t\n", 0o600)
		_, err := fileRegistry().Fetch(ctx, "file:"+path)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrSecretNotFound)
		assert.Contains(t, err.Error(), "empty value")
	})

	t.Run("env secret trimmed identically", func(t *testing.T) {
		t.Setenv("CHAPERONE_TEST_SECRET", " sk-from-env\n")
		r := NewRegistry()
		r.Register("env", NewEnvProvider())
		secret, err := r.Fetch(ctx, "env:CHAPERONE_TEST_SECRET")
		require.NoError(t, err)
		assert.Equal(t, "sk-from-env", secret)
	})

	t.Run("env var set to whitespace is not found", func(t *testing.T) {
		t.Setenv("CHAPERONE_TEST_EMPTY", "  ")
		r := NewRegistry()
		r.Register("env", NewEnvProvider())
		_, err := r.Fetch(ctx, "env:CHAPERONE_TEST_EMPTY")
		assert.ErrorIs(t, err, ErrSecretNotFound)
	})
}

// TestErrSecretNotFound_OneIdentity pins the sentinel alias: the secrets
// package re-exports the canonical sentinel from internal/errors, so
// errors.Is matches across both names. Two sentinels with the same text but
// different identities would make the error classifier silently miss
// provider errors.
func TestErrSecretNotFound_OneIdentity(t *testing.T) {
	assert.ErrorIs(t, ErrSecretNotFound, cherrors.ErrSecretNotFound)
}
