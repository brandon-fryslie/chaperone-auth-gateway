package config

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const minimalConfig = `
[services.openai]
host_pattern = "api.openai.com"
auth_strategy = "bearer"
credential_ref = "env:OPENAI_API_KEY"
`

// writeConfigWithMode writes a config file and then chmods it explicitly, so
// the test's mode is exact rather than umask-filtered.
func writeConfigWithMode(t *testing.T, content string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "chaperone.toml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	require.NoError(t, os.Chmod(path, mode))
	return path
}

// TestLoad_RejectsUntrustedConfig is the acceptance test for the
// permission/ownership gate: a config any other local user could write is
// refused with a loud error before parsing. Every mode loads through
// config.Load, so rejection here IS startup failure for inject/run/examine/
// check alike.
func TestLoad_RejectsUntrustedConfig(t *testing.T) {
	t.Run("world-writable (0666) rejected", func(t *testing.T) {
		path := writeConfigWithMode(t, minimalConfig, 0o666)
		_, err := Load(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "writable by group or others")
		assert.Contains(t, err.Error(), "chmod go-w")
	})

	t.Run("group-writable (0620) rejected", func(t *testing.T) {
		path := writeConfigWithMode(t, minimalConfig, 0o620)
		_, err := Load(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "writable by group or others")
	})

	t.Run("rejection happens before any TOML is parsed", func(t *testing.T) {
		// Invalid TOML in a world-writable file: if the error is the trust
		// rejection (not a parse error), nothing in the file — including any
		// credential_ref — was ever interpreted.
		path := writeConfigWithMode(t, "this is { not [ toml", 0o666)
		_, err := Load(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "writable by group or others")
		assert.NotContains(t, err.Error(), "TOML")
	})

	t.Run("symlink to an untrusted file rejected", func(t *testing.T) {
		target := writeConfigWithMode(t, minimalConfig, 0o666)
		link := filepath.Join(t.TempDir(), "chaperone.toml")
		require.NoError(t, os.Symlink(target, link))
		_, err := Load(link)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "writable by group or others")
	})

	t.Run("owner-only (0600) accepted", func(t *testing.T) {
		path := writeConfigWithMode(t, minimalConfig, 0o600)
		cfg, err := Load(path)
		require.NoError(t, err)
		assert.Equal(t, "api.openai.com", cfg.Services["openai"].HostPattern)
	})

	t.Run("world-readable but not writable (0644) accepted", func(t *testing.T) {
		path := writeConfigWithMode(t, minimalConfig, 0o644)
		_, err := Load(path)
		require.NoError(t, err)
	})
}

// fakeOwnerInfo wraps a real FileInfo but reports a chosen owner uid: tests
// cannot chown a file to another user without root, but the trust decision is
// pure over (FileInfo, uid), so ownership rejection is provable with a fake.
type fakeOwnerInfo struct {
	os.FileInfo
	uid uint32
}

func (f fakeOwnerInfo) Sys() any { return &syscall.Stat_t{Uid: f.uid} }

// fakeNoSysInfo reports a stat shape the trust check cannot interpret.
type fakeNoSysInfo struct{ os.FileInfo }

func (f fakeNoSysInfo) Sys() any { return nil }

func TestVerifyConfigTrust_Ownership(t *testing.T) {
	path := writeConfigWithMode(t, minimalConfig, 0o600)
	fi, err := os.Stat(path)
	require.NoError(t, err)

	t.Run("file owned by another uid rejected", func(t *testing.T) {
		other := uint32(os.Getuid() + 1) //nolint:gosec // test uid arithmetic
		err := verifyConfigTrust(fakeOwnerInfo{fi, other}, os.Getuid(), path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "owned by uid")
		assert.Contains(t, err.Error(), "another user controls")
	})

	t.Run("file owned by running user accepted", func(t *testing.T) {
		err := verifyConfigTrust(fi, os.Getuid(), path)
		assert.NoError(t, err)
	})

	t.Run("unknown stat shape rejected, not skipped", func(t *testing.T) {
		err := verifyConfigTrust(fakeNoSysInfo{fi}, os.Getuid(), path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot determine file owner")
	})

	t.Run("non-regular file rejected", func(t *testing.T) {
		dirInfo, err := os.Stat(t.TempDir())
		require.NoError(t, err)
		err = verifyConfigTrust(dirInfo, os.Getuid(), "somedir")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a regular file")
	})
}
