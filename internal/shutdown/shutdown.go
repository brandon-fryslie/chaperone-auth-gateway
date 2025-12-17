package shutdown

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// Manager coordinates graceful shutdown of application components.
type Manager struct {
	shutdownFuncs []func(ctx context.Context) error
	shutdownOnce  sync.Once
	mu            sync.Mutex
	logger        *slog.Logger
	lastError     error
}

// NewManager creates a new shutdown manager.
// Logger can be nil for silent operation.
func NewManager(logger *slog.Logger) *Manager {
	return &Manager{
		logger: logger,
	}
}

// Register adds a shutdown function to be called during shutdown.
// Functions are executed in LIFO order (last registered, first executed).
func (m *Manager) Register(fn func(ctx context.Context) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shutdownFuncs = append(m.shutdownFuncs, fn)
}

// WaitForShutdown blocks until SIGTERM or SIGINT is received.
func (m *Manager) WaitForShutdown() error {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
	<-sigChan
	return nil
}

// Shutdown executes all registered shutdown functions with the given timeout.
// Functions are executed in LIFO order (last registered, first executed).
// Returns an error if any shutdown function fails or timeout is exceeded.
func (m *Manager) Shutdown(timeout time.Duration) error {
	m.shutdownOnce.Do(func() {
		// Handle negative/zero timeout
		if timeout <= 0 {
			timeout = time.Millisecond // minimal timeout
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		m.mu.Lock()
		funcs := make([]func(ctx context.Context) error, len(m.shutdownFuncs))
		copy(funcs, m.shutdownFuncs)
		m.mu.Unlock()

		// Channel to collect results
		type result struct {
			err error
		}
		resultChan := make(chan result, 1)

		// Execute in LIFO order (reverse) - but run in a goroutine to respect timeout
		go func() {
			var errs []error

			// Execute in LIFO order (reverse)
			for i := len(funcs) - 1; i >= 0; i-- {
				fn := funcs[i]

				// Execute with panic recovery
				func() {
					defer func() {
						if r := recover(); r != nil {
							if m.logger != nil {
								m.logger.Error("panic in shutdown function", "panic", r)
							}
						}
					}()

					if err := fn(ctx); err != nil {
						errs = append(errs, err)
						if m.logger != nil {
							m.logger.Error("shutdown function error", "error", err)
						}
					}
				}()
			}

			if len(errs) > 0 {
				resultChan <- result{err: errors.Join(errs...)}
			} else {
				resultChan <- result{err: nil}
			}
		}()

		// Wait for either completion or timeout
		select {
		case res := <-resultChan:
			m.lastError = res.err
		case <-ctx.Done():
			// Context timed out, but give functions a brief moment to detect cancellation
			// and complete their cleanup
			select {
			case res := <-resultChan:
				// Functions completed during grace period
				if res.err != nil {
					m.lastError = res.err
				} else {
					m.lastError = ctx.Err()
				}
			case <-time.After(10 * time.Millisecond):
				// Grace period expired, return timeout error
				m.lastError = ctx.Err()
			}
		}
	})

	return m.lastError
}
