package filetrust

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testFile = File{
	Desc:   "test file",
	Stakes: "an attacker gains the thing",
	Bar:    NoUntrustedWrites,
}

// writeWithMode writes a file and then chmods it explicitly, so the test's
// mode is exact rather than umask-filtered.
func writeWithMode(t *testing.T, content string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "f")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	require.NoError(t, os.Chmod(path, mode))
	return path
}

func statOf(t *testing.T, path string) os.FileInfo {
	t.Helper()
	fi, err := os.Stat(path)
	require.NoError(t, err)
	return fi
}

// TestVerify_Bars proves the two canonical bars draw the line where their
// file classes need it: integrity-only files tolerate being readable,
// secret-bearing files tolerate no group/world access at all.
func TestVerify_Bars(t *testing.T) {
	cases := []struct {
		name   string
		bar    Bar
		mode   os.FileMode
		reject bool
	}{
		{"NoUntrustedWrites accepts 0600", NoUntrustedWrites, 0o600, false},
		{"NoUntrustedWrites accepts 0644", NoUntrustedWrites, 0o644, false},
		{"NoUntrustedWrites rejects 0666", NoUntrustedWrites, 0o666, true},
		{"NoUntrustedWrites rejects group-writable 0620", NoUntrustedWrites, 0o620, true},
		{"OwnerOnly accepts 0600", OwnerOnly, 0o600, false},
		{"OwnerOnly accepts 0400", OwnerOnly, 0o400, false},
		{"OwnerOnly rejects world-readable 0644", OwnerOnly, 0o644, true},
		{"OwnerOnly rejects group-readable 0640", OwnerOnly, 0o640, true},
		{"OwnerOnly rejects world-readable-only 0604", OwnerOnly, 0o604, true},
		{"OwnerOnly rejects group-executable 0610", OwnerOnly, 0o610, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeWithMode(t, "content", tc.mode)
			f := File{Desc: "test file", Stakes: "stakes", Bar: tc.bar}
			err := f.Verify(statOf(t, path), os.Getuid(), path)
			if tc.reject {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "by group or others")
				assert.Contains(t, err.Error(), "chmod")
				assert.Contains(t, err.Error(), "stakes")
			} else {
				assert.NoError(t, err)
			}
		})
	}
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

func TestVerify_Ownership(t *testing.T) {
	path := writeWithMode(t, "content", 0o600)
	fi := statOf(t, path)

	t.Run("file owned by another uid rejected", func(t *testing.T) {
		other := uint32(os.Getuid() + 1) //nolint:gosec // test uid arithmetic
		err := testFile.Verify(fakeOwnerInfo{fi, other}, os.Getuid(), path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "owned by uid")
		assert.Contains(t, err.Error(), "another user controls")
	})

	t.Run("file owned by running user accepted", func(t *testing.T) {
		assert.NoError(t, testFile.Verify(fi, os.Getuid(), path))
	})

	t.Run("unknown stat shape rejected, not skipped", func(t *testing.T) {
		err := testFile.Verify(fakeNoSysInfo{fi}, os.Getuid(), path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot determine file owner")
	})

	t.Run("non-regular file rejected", func(t *testing.T) {
		dirInfo := statOf(t, t.TempDir())
		err := testFile.Verify(dirInfo, os.Getuid(), "somedir")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a regular file")
	})

	t.Run("zero Bar refuses everything", func(t *testing.T) {
		f := File{Desc: "misbuilt file"}
		err := f.Verify(fi, os.Getuid(), path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no trust bar configured")
	})
}

func TestReadFile(t *testing.T) {
	t.Run("trusted file is read", func(t *testing.T) {
		path := writeWithMode(t, "hello", 0o600)
		data, err := testFile.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "hello", string(data))
	})

	t.Run("untrusted file is refused unread", func(t *testing.T) {
		path := writeWithMode(t, "hello", 0o666)
		_, err := testFile.ReadFile(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "writable by group or others")
	})

	t.Run("symlink is judged by its target", func(t *testing.T) {
		target := writeWithMode(t, "hello", 0o666)
		link := filepath.Join(t.TempDir(), "link")
		require.NoError(t, os.Symlink(target, link))
		_, err := testFile.ReadFile(link)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "writable by group or others")
	})

	t.Run("symlink to a trusted target is read", func(t *testing.T) {
		target := writeWithMode(t, "hello", 0o600)
		link := filepath.Join(t.TempDir(), "link")
		require.NoError(t, os.Symlink(target, link))
		data, err := testFile.ReadFile(link)
		require.NoError(t, err)
		assert.Equal(t, "hello", string(data))
	})

	t.Run("absence surfaces as fs.ErrNotExist", func(t *testing.T) {
		_, err := testFile.ReadFile(filepath.Join(t.TempDir(), "missing"))
		require.Error(t, err)
		assert.True(t, errors.Is(err, fs.ErrNotExist))
	})
}
