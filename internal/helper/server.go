// Package helper implements a privileged helper daemon that exposes validated
// container operations over a Unix domain socket. It replaces the sudo-based
// privilege escalation pattern with structured JSON requests and server-side
// validation at the privilege boundary.
package helper

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"github.com/battlewithbytes/pve-appstore/internal/config"
)

// ServerConfig holds configuration for the helper server.
type ServerConfig struct {
	SocketPath string
	DBPath     string
	AuditPath  string
	Config     *config.Config
}

// Server is the privileged helper daemon.
type Server struct {
	mu           sync.RWMutex
	cfg          *config.Config
	db           *sql.DB
	auditFile    *os.File
	allowedPaths []string // resolved storage mount points for fs validation

	// Concurrency controls (S9)
	ctidLocks  sync.Map // per-CTID mutex for config-modifying ops
	termSem    chan struct{}
	execSem    chan struct{}
	updateLock sync.Mutex
}

// NewServer creates a new helper server. The database is opened read-only
// for CTID validation — the helper never writes to the install database.
func NewServer(cfg ServerConfig) (*Server, error) {
	db, err := sql.Open("sqlite", cfg.DBPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	// Verify read-only connection works
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	// Open audit log
	os.MkdirAll("/var/log/pve-appstore", 0750)
	auditFile, err := os.OpenFile(cfg.AuditPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("opening audit log: %w", err)
	}

	s := &Server{
		cfg:       cfg.Config,
		db:        db,
		auditFile: auditFile,
		termSem:   make(chan struct{}, 5),  // max 5 concurrent terminals
		execSem:   make(chan struct{}, 20), // max 20 concurrent execs
	}
	s.resolveAllowedPaths()

	return s, nil
}

// Close releases resources held by the server.
func (s *Server) Close() {
	if s.db != nil {
		s.db.Close()
	}
	if s.auditFile != nil {
		s.auditFile.Close()
	}
}

// ReloadConfig updates the server's configuration (called on SIGHUP).
func (s *Server) ReloadConfig(cfg *config.Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = cfg
	s.resolveAllowedPaths()
}

// resolveAllowedPaths builds the list of storage paths allowed for fs operations.
func (s *Server) resolveAllowedPaths() {
	// Storage paths from config (browsable storages) plus well-known paths
	s.allowedPaths = []string{
		"/var/lib/pve-appstore/tmp",
	}
	// Note: The actual storage mount points are resolved at request time by checking
	// the config. This list is supplemented by runtime storage resolution.
}

// Handler returns the HTTP handler for the helper server.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("GET /v1/health", s.handleHealth)

	// pct operations
	mux.HandleFunc("POST /v1/pct/exec", s.withAudit("pct/exec", s.handlePctExec))
	mux.HandleFunc("POST /v1/pct/exec-stream", s.withAudit("pct/exec-stream", s.handlePctExecStream))
	mux.HandleFunc("POST /v1/pct/push", s.withAudit("pct/push", s.handlePctPush))
	mux.HandleFunc("POST /v1/pct/set", s.withAudit("pct/set", s.handlePctSet))

	// Config operations
	mux.HandleFunc("POST /v1/conf/append", s.withAudit("conf/append", s.handleConfAppend))

	// Filesystem operations
	mux.HandleFunc("POST /v1/fs/mkdir", s.withAudit("fs/mkdir", s.handleFsMkdir))
	mux.HandleFunc("POST /v1/fs/chown", s.withAudit("fs/chown", s.handleFsChown))
	mux.HandleFunc("POST /v1/fs/rm", s.withAudit("fs/rm", s.handleFsRm))

	// Update operation
	mux.HandleFunc("POST /v1/update", s.withAudit("update", s.handleUpdate))

	// Terminal (hijacked connection)
	mux.HandleFunc("GET /v1/terminal", s.withAudit("terminal", s.handleTerminal))

	// Wrap with peer credential check and body size limit
	return s.peerCredMiddleware(mux)
}

// peerCredMiddleware verifies the connecting peer is the appstore user.
func (s *Server) peerCredMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Health endpoint is allowed from any local user
		if r.URL.Path == "/v1/health" {
			next.ServeHTTP(w, r)
			return
		}

		// Extract peer credentials from the connection
		cred, err := getPeerCred(r)
		if err != nil {
			writeHelperError(w, http.StatusForbidden, "cannot verify peer credentials")
			return
		}

		// Verify UID matches the appstore user (or root for testing)
		if cred.Uid != 0 && !isAppstoreUser(cred.Uid) {
			writeHelperError(w, http.StatusForbidden, fmt.Sprintf("peer UID %d is not authorized", cred.Uid))
			return
		}

		// Store cred in context for audit logging
		ctx := context.WithValue(r.Context(), peerCredKey, cred)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type contextKey string

const peerCredKey contextKey = "peercred"

// getPeerCred extracts Unix peer credentials from the HTTP request's underlying connection.
func getPeerCred(r *http.Request) (*unix.Ucred, error) {
	// The http.Server wraps the connection; we need to access it via hijack
	// or via the connection state. For Unix sockets, we store creds in a
	// ConnContext callback. Here we get them from the request context.
	cred, ok := r.Context().Value(peerCredKey).(*unix.Ucred)
	if ok && cred != nil {
		return cred, nil
	}
	return nil, fmt.Errorf("no peer credentials available")
}

// ConnContext returns a context with peer credentials attached.
// This is used as http.Server.ConnContext to inject creds on every new connection.
func ConnContext(ctx context.Context, c net.Conn) context.Context {
	unixConn, ok := c.(*net.UnixConn)
	if !ok {
		return ctx
	}
	raw, err := unixConn.SyscallConn()
	if err != nil {
		return ctx
	}
	var cred *unix.Ucred
	raw.Control(func(fd uintptr) {
		cred, _ = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	})
	if cred != nil {
		ctx = context.WithValue(ctx, peerCredKey, cred)
	}
	return ctx
}

// isAppstoreUser checks if a UID belongs to the appstore service user.
func isAppstoreUser(uid uint32) bool {
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return false
	}
	target := fmt.Sprintf("%d", uid)
	for _, line := range splitPasswdLines(data) {
		parts := splitPasswdColon(line)
		if len(parts) >= 3 && parts[0] == config.ServiceUser && parts[2] == target {
			return true
		}
	}
	return false
}

func splitPasswdLines(data []byte) []string {
	var lines []string
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, string(data[start:i]))
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, string(data[start:]))
	}
	return lines
}

func splitPasswdColon(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// withAudit wraps a handler with audit logging.
func (s *Server) withAudit(endpoint string, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Apply request body size limit (1 MB)
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

		// Create a response wrapper to capture status code
		rw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
		handler(rw, r)

		// Log audit entry
		cred, _ := r.Context().Value(peerCredKey).(*unix.Ucred)
		entry := auditEntry{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Endpoint:  "/v1/" + endpoint,
			Result:    "ok",
			Duration:  time.Since(start).Milliseconds(),
		}
		if cred != nil {
			entry.PeerUID = int(cred.Uid)
			entry.PeerPID = int(cred.Pid)
		}
		if rw.code >= 400 {
			entry.Result = "error"
		}
		s.writeAudit(entry)
	}
}

type auditEntry struct {
	Timestamp string `json:"ts"`
	PeerUID   int    `json:"peer_uid"`
	PeerPID   int    `json:"peer_pid"`
	Endpoint  string `json:"endpoint"`
	CTID      int    `json:"ctid,omitempty"`
	Params    string `json:"params,omitempty"`
	Result    string `json:"result"`
	Duration  int64  `json:"duration_ms"`
}

func (s *Server) writeAudit(entry auditEntry) {
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	data = append(data, '\n')
	s.auditFile.Write(data)
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

// handleHealth returns a simple health check response.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeHelperJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// getCTIDLock returns a per-CTID mutex for serializing config operations.
func (s *Server) getCTIDLock(ctid int) *sync.Mutex {
	val, _ := s.ctidLocks.LoadOrStore(ctid, &sync.Mutex{})
	return val.(*sync.Mutex)
}

// getConfig returns the current config under read lock.
func (s *Server) getConfig() *config.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// Helper functions for JSON responses.

func writeHelperJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeHelperError(w http.ResponseWriter, code int, msg string) {
	writeHelperJSON(w, code, map[string]string{"error": msg})
}

func decodeBody(r *http.Request, v interface{}) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	return nil
}

func init() {
	// Suppress default log prefix for helper
	log.SetFlags(0)
}
