package control

import (
	"context"
	"encoding/json"
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
		// Every connection reaching Serve has passed the peer gate; this lifts
		// the attested identity into each request's context for attribution.
		ConnContext: peerContext,
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
	// Authenticate every peer at the accept seam: the socket's 0600 mode is the
	// first layer, the kernel-attested uid check is the deciding one.
	gated := &peerGate{Listener: ln, ownUID: os.Geteuid(), creds: peerCred, logger: s.logger}
	s.listener = gated

	go func() {
		if err := s.httpServer.Serve(gated); err != nil && !errors.Is(err, http.ErrServerClosed) {
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

// writeJSON encodes v as the response body with the given status. The status and
// headers are already committed once encoding starts, so a failure can't be
// recovered — but it must not vanish: it is surfaced to the daemon's operational
// log ([LAW:no-silent-failure]).
func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.logger.Error("control: failed to encode response", "status", status, "error", err)
	}
}

// writeError sends a non-2xx response carrying err's message verbatim.
func (s *Server) writeError(w http.ResponseWriter, status int, err error) {
	s.writeJSON(w, status, errorBody{Error: err.Error()})
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
		s.writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("use POST"))
		return
	}
	peer, ok := peerFrom(r.Context())
	if !ok {
		s.writeError(w, http.StatusInternalServerError,
			fmt.Errorf("control: connection carries no attested peer identity"))
		return
	}
	var req GrantRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}
	res, err := s.api.Grant(req, peer)
	if err != nil {
		s.writeError(w, statusFor(err), err)
		return
	}
	s.writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("use POST"))
		return
	}
	peer, ok := peerFrom(r.Context())
	if !ok {
		s.writeError(w, http.StatusInternalServerError,
			fmt.Errorf("control: connection carries no attested peer identity"))
		return
	}
	var req RevokeRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, err)
		return
	}
	res, err := s.api.Revoke(req, peer)
	if err != nil {
		s.writeError(w, statusFor(err), err)
		return
	}
	s.writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("use GET"))
		return
	}
	s.writeJSON(w, http.StatusOK, s.api.List())
}

func (s *Server) handleListGrantable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("use GET"))
		return
	}
	s.writeJSON(w, http.StatusOK, s.api.ListGrantable())
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

// listenUnix binds a unix socket owner-only from the instant it exists,
// refusing to stomp a control plane that is already live. On "address already
// in use" it probes the existing socket: a successful dial means another daemon
// owns it (fail loudly); a failed dial means the file is stale residue (remove
// and retry once).
func listenUnix(path string) (net.Listener, error) {
	// The kernel fixes the socket file's mode at bind (0777 &^ umask): tightening
	// the umask across the bind is the only way the file is 0600 from its first
	// observable instant — a chmod afterwards leaves a window at the process
	// umask. [LAW:no-shared-mutable-globals] exception: the umask is process-wide
	// state, but the kernel offers no per-bind mode; the bracket is held only
	// across this function, and a concurrent file creation could only come out
	// stricter, never looser.
	oldMask := syscall.Umask(0o177)
	defer syscall.Umask(oldMask)

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
