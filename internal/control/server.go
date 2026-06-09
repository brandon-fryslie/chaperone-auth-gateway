package control

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"syscall"
	"time"

	"log/slog"
)

// Server exposes the control API over a localhost-only unix domain socket. It is
// the world-boundary half of the control plane: it owns the listener and the
// socket file lifecycle and translates bytes ↔ typed API calls. All policy lives
// in API; this part only moves requests and surfaces outcomes.
type Server struct {
	api        *API
	socketPath string
	httpServer *http.Server
	listener   net.Listener
	logger     *slog.Logger
}

// NewServer builds a control server bound to socketPath. It does not listen until
// Start is called.
func NewServer(api *API, socketPath string, logger *slog.Logger) (*Server, error) {
	if api == nil {
		return nil, fmt.Errorf("control: api is required")
	}
	if socketPath == "" {
		return nil, fmt.Errorf("control: socket path is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{api: api, socketPath: socketPath, logger: logger}
	s.httpServer = &http.Server{
		Handler:           s.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s, nil
}

// SocketPath is the path the server listens on (useful for clients and tests).
func (s *Server) SocketPath() string { return s.socketPath }

// Start binds the socket and serves in a background goroutine. It returns once the
// listener is bound (so callers know the control plane is reachable) or with a
// loud error if the socket cannot be claimed.
func (s *Server) Start() error {
	ln, err := listenUnix(s.socketPath)
	if err != nil {
		return err
	}
	s.listener = ln

	// Owner-only: even on a shared host, only this user may connect.
	if err := os.Chmod(s.socketPath, 0600); err != nil {
		_ = ln.Close()
		return fmt.Errorf("control: chmod socket: %w", err)
	}

	go func() {
		if err := s.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("control server stopped", "error", err)
		}
	}()
	s.logger.Info("control plane listening", "socket", s.socketPath)
	return nil
}

// Stop shuts the HTTP server down and removes the socket file. Safe to register
// directly with the shutdown manager.
func (s *Server) Stop(ctx context.Context) error {
	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(ctx); err != nil {
			return fmt.Errorf("control: shutdown: %w", err)
		}
	}
	// Remove the socket file so the next daemon starts clean. Absence is fine.
	if err := os.Remove(s.socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("control: remove socket: %w", err)
	}
	return nil
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(PathGrant, s.handleGrant)
	mux.HandleFunc(PathRevoke, s.handleRevoke)
	mux.HandleFunc(PathList, s.handleList)
	mux.HandleFunc(PathListGrantable, s.handleListGrantable)
	return mux
}

func (s *Server) handleGrant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("use POST"))
		return
	}
	var req GrantRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	res, err := s.api.Grant(req)
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("use POST"))
		return
	}
	var req RevokeRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	res, err := s.api.Revoke(req)
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("use GET"))
		return
	}
	writeJSON(w, http.StatusOK, s.api.List())
}

func (s *Server) handleListGrantable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("use GET"))
		return
	}
	writeJSON(w, http.StatusOK, s.api.ListGrantable())
}

// statusFor maps a control failure to its HTTP outcome contract. A non-apiError
// is treated as an internal fault (loud, 5xx) rather than guessed.
func statusFor(err error) int {
	var ae *apiError
	if errors.As(err, &ae) {
		switch ae.kind {
		case kindRejected:
			return http.StatusForbidden
		case kindBadRequest:
			return http.StatusBadRequest
		case kindInternal:
			return http.StatusInternalServerError
		}
	}
	return http.StatusInternalServerError
}

// listenUnix binds a unix socket, refusing to stomp a control plane that is
// already live. On "address already in use" it probes the existing socket: a
// successful dial means another daemon owns it (fail loudly); a failed dial means
// the file is stale residue (remove and retry once).
func listenUnix(path string) (net.Listener, error) {
	ln, err := net.Listen("unix", path)
	if err == nil {
		return ln, nil
	}
	if !errors.Is(err, syscall.EADDRINUSE) {
		// Not an in-use error: could be a missing parent dir or permissions.
		return nil, fmt.Errorf("control: listen on %s: %w", path, err)
	}

	// Probe liveness before touching the file.
	conn, dialErr := net.DialTimeout("unix", path, 500*time.Millisecond)
	if dialErr == nil {
		_ = conn.Close()
		return nil, fmt.Errorf("control: another chaperone daemon is already listening on %s", path)
	}

	// Stale socket: remove and retry once.
	if rmErr := os.Remove(path); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
		return nil, fmt.Errorf("control: remove stale socket %s: %w", path, rmErr)
	}
	ln, err = net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("control: listen on %s after clearing stale socket: %w", path, err)
	}
	return ln, nil
}
