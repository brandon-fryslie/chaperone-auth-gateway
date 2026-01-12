package run

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// FDConfig specifies how to handle file descriptors for a child process.
type FDConfig struct {
	Stdout     io.Writer // Where to send stdout
	Stderr     io.Writer // Where to send stderr
	closed     bool
	closeMutex sync.Mutex
}

// NewFDConfig creates a FDConfig from stdout/stderr mode strings.
// Modes: "inherit" (use parent's FD), "file:/path" (write to file), "discard" (send to /dev/null)
func NewFDConfig(stdoutMode, stderrMode string) (*FDConfig, error) {
	stdout, err := parseFDMode(stdoutMode, os.Stdout)
	if err != nil {
		return nil, fmt.Errorf("stdout: %w", err)
	}

	stderr, err := parseFDMode(stderrMode, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("stderr: %w", err)
	}

	return &FDConfig{
		Stdout: stdout,
		Stderr: stderr,
	}, nil
}

// parseFDMode parses a file descriptor mode string and returns the appropriate writer.
func parseFDMode(mode string, defaultWriter io.Writer) (io.Writer, error) {
	switch {
	case mode == "" || mode == "inherit":
		return defaultWriter, nil

	case mode == "discard":
		return io.Discard, nil

	case strings.HasPrefix(mode, "file:"):
		path := mode[5:] // Strip "file:" prefix
		if path == "" {
			return nil, fmt.Errorf("file path cannot be empty")
		}

		// Open file in append mode with 0644 permissions
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open file %q: %w", path, err)
		}

		return f, nil

	default:
		return nil, fmt.Errorf("invalid mode %q: must be 'inherit', 'discard', or 'file:/path'", mode)
	}
}

// Close closes any file handles opened by this FDConfig.
// Safe to call multiple times.
func (fc *FDConfig) Close() error {
	fc.closeMutex.Lock()
	defer fc.closeMutex.Unlock()

	if fc.closed {
		return nil // Already closed
	}

	var errs []error

	// Track which closers we've already closed (in case stdout and stderr point to the same file)
	closedFiles := make(map[io.Closer]bool)

	// Close stdout if it's a file
	if closer, ok := fc.Stdout.(io.Closer); ok && fc.Stdout != os.Stdout {
		if err := closer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("stdout: %w", err))
		}
		closedFiles[closer] = true
	}

	// Close stderr if it's a file and we haven't already closed it
	if closer, ok := fc.Stderr.(io.Closer); ok && fc.Stderr != os.Stderr {
		if !closedFiles[closer] {
			if err := closer.Close(); err != nil {
				errs = append(errs, fmt.Errorf("stderr: %w", err))
			}
		}
	}

	fc.closed = true

	if len(errs) > 0 {
		return fmt.Errorf("failed to close file descriptors: %v", errs)
	}

	return nil
}
