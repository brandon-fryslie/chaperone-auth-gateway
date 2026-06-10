// Package filetrust decides whether a file on local disk can be trusted
// before its contents are used. Trust means: no other local user could have
// tampered with the file — and, for files whose contents are themselves
// sensitive, no other local user could have read it.
//
// [LAW:one-type-per-behavior] every file-trust decision in the codebase goes
// through this one package; a second hand-rolled mode/owner check would drift
// from this one and is a bug.
package filetrust

import (
	"fmt"
	"io"
	"os"
	"syscall"
)

// Bar is the exposure a file may tolerate from other local users before it is
// refused. The fields are unexported so the only constructible Bars are the
// canonical ones below: a caller-invented Bar (or the zero value, which would
// forbid nothing) is unrepresentable. [LAW:types-are-the-program]
type Bar struct {
	forbidden os.FileMode
	violation string
	fix       string
}

var (
	// NoUntrustedWrites rejects group/world-writable files. The bar for files
	// whose integrity matters but whose contents are not themselves secret —
	// a config file holds credential references and steers behavior, but
	// reading it reveals no secret value.
	NoUntrustedWrites = Bar{forbidden: 0o022, violation: "writable by group or others", fix: "go-w"}

	// OwnerOnly rejects any group/world access. The bar for files whose
	// contents ARE the sensitive value — secret files, private keys — where
	// another user reading the file is as bad as writing it.
	OwnerOnly = Bar{forbidden: 0o077, violation: "readable or writable by group or others", fix: "go-rwx"}
)

// File names one class of protected file: what to call it in errors, what an
// attacker gains if the bar is not met, and which Bar applies.
type File struct {
	Desc   string // e.g. "config file", "secret file"
	Stakes string // what an attacker gains; spliced into the permission error
	Bar    Bar
}

// Verify rejects fi when anyone other than the running user could have
// written it — or, under OwnerOnly, read it: a non-regular file, forbidden
// permission bits, or ownership by a different uid. A pure decision over
// (FileInfo, uid) so the ownership branch is testable without root.
// [LAW:effects-at-boundaries]
// [LAW:no-silent-failure] rejection is a hard error carrying the remediation
// — never a warning, never a fallback.
func (f File) Verify(fi os.FileInfo, uid int, path string) error {
	if f.Bar.forbidden == 0 {
		// The zero Bar would forbid nothing — a trust check that is silently
		// off. Refusing everything is the only safe meaning for it.
		return fmt.Errorf("%s %s: no trust bar configured; refusing to load", f.Desc, path)
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%s %s is not a regular file (mode %s); refusing to load", f.Desc, path, fi.Mode())
	}
	if perm := fi.Mode().Perm(); perm&f.Bar.forbidden != 0 {
		return fmt.Errorf("%s %s is %s (mode %04o): %s — fix with: chmod %s %s", f.Desc, path, f.Bar.violation, perm, f.Stakes, f.Bar.fix, path)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		// Only darwin/linux are supported targets; an unknown stat shape means
		// ownership cannot be proven, and unprovable trust is a refusal, not a
		// pass. [LAW:no-silent-failure]
		return fmt.Errorf("%s %s: cannot determine file owner on this platform; refusing to load", f.Desc, path)
	}
	if int(st.Uid) != uid {
		return fmt.Errorf("%s %s is owned by uid %d but chaperone is running as uid %d: refusing to load a file another user controls — fix with: chown %d %s", f.Desc, path, st.Uid, uid, uid, path)
	}
	return nil
}

// ReadFile opens, verifies, and reads path as one owned sequence: the
// FileInfo handed to Verify is fstat'd from the same open handle the bytes
// are read from, so the inode that passed verification is the inode whose
// contents are returned (no check-then-use race), and a symlinked path is
// judged by the target actually read.
// [LAW:no-ambient-temporal-coupling] the safe ordering lives here, once;
// callers cannot reassemble it wrong.
//
// Every error is framed with the file's Desc; absence stays detectable
// through the wrapping via errors.Is(err, fs.ErrNotExist), so callers can map
// it to their own domain error.
func (f File) ReadFile(path string) ([]byte, error) {
	file, err := os.Open(path) //nolint:gosec // path is operator-provided configuration, gated by Verify below
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", f.Desc, err)
	}
	defer func() { _ = file.Close() }()

	fi, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat %s %s: %w", f.Desc, path, err)
	}
	if err := f.Verify(fi, os.Getuid(), path); err != nil {
		return nil, err
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s %s: %w", f.Desc, path, err)
	}
	return data, nil
}
