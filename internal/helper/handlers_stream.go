package helper

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"

	"github.com/creack/pty"
)

// --- POST /v1/pct/exec-stream ---
// Streams stdout/stderr line-by-line as chunked HTTP response.
// Returns exit code in X-Exit-Code trailer header.

func (s *Server) handlePctExecStream(w http.ResponseWriter, r *http.Request) {
	var req pctExecRequest
	if err := decodeBody(r, &req); err != nil {
		writeHelperError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.validateCTID(req.CTID); err != nil {
		writeHelperError(w, http.StatusForbidden, err.Error())
		return
	}
	if err := validateCommand(req.Command); err != nil {
		writeHelperError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Acquire exec semaphore
	select {
	case s.execSem <- struct{}{}:
		defer func() { <-s.execSem }()
	default:
		writeHelperError(w, http.StatusTooManyRequests, "too many concurrent exec operations")
		return
	}

	args := append([]string{"exec", strconv.Itoa(req.CTID), "--"}, req.Command...)
	cmd := exec.Command(pctBin, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		writeHelperError(w, http.StatusInternalServerError, fmt.Sprintf("stdout pipe: %v", err))
		return
	}
	cmd.Stderr = cmd.Stdout // merge stderr into stdout

	if err := cmd.Start(); err != nil {
		writeHelperError(w, http.StatusInternalServerError, fmt.Sprintf("start: %v", err))
		return
	}

	// Set up chunked response
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("Trailer", "X-Exit-Code")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text() + "\n"
		w.Write([]byte(line))
		if flusher != nil {
			flusher.Flush()
		}
	}

	exitCode := 0
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	// Set trailer
	w.Header().Set("X-Exit-Code", strconv.Itoa(exitCode))
	if flusher != nil {
		flusher.Flush()
	}
}

// --- GET /v1/terminal ---
// Hijacks the HTTP connection for bidirectional PTY communication.
// Query params: ctid, shell

type terminalResizeMsg struct {
	Type string `json:"type"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

func (s *Server) handleTerminal(w http.ResponseWriter, r *http.Request) {
	ctidStr := r.URL.Query().Get("ctid")
	shell := r.URL.Query().Get("shell")

	ctid, err := strconv.Atoi(ctidStr)
	if err != nil {
		writeHelperError(w, http.StatusBadRequest, "invalid ctid")
		return
	}
	if err := s.validateCTID(ctid); err != nil {
		writeHelperError(w, http.StatusForbidden, err.Error())
		return
	}
	if shell == "" {
		shell = "/bin/sh"
	}
	if err := validateShell(shell); err != nil {
		writeHelperError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Acquire terminal semaphore
	select {
	case s.termSem <- struct{}{}:
		defer func() { <-s.termSem }()
	default:
		writeHelperError(w, http.StatusTooManyRequests, "too many concurrent terminal sessions")
		return
	}

	// Hijack the connection
	hj, ok := w.(http.Hijacker)
	if !ok {
		writeHelperError(w, http.StatusInternalServerError, "hijack not supported")
		return
	}
	conn, bufrw, err := hj.Hijack()
	if err != nil {
		writeHelperError(w, http.StatusInternalServerError, fmt.Sprintf("hijack: %v", err))
		return
	}
	defer conn.Close()

	// Send HTTP 200 to indicate successful upgrade
	bufrw.WriteString("HTTP/1.1 200 OK\r\n")
	bufrw.WriteString("Content-Type: application/octet-stream\r\n")
	bufrw.WriteString("Connection: close\r\n")
	bufrw.WriteString("\r\n")
	bufrw.Flush()

	// Start PTY
	cmd := exec.Command(pctBin, "exec", strconv.Itoa(ctid), "--", shell, "-l")
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.Start(cmd)
	if err != nil {
		conn.Write([]byte(fmt.Sprintf("failed to start shell: %v\r\n", err)))
		return
	}
	defer ptmx.Close()

	// Set initial terminal size
	pty.Setsize(ptmx, &pty.Winsize{Rows: 24, Cols: 80})

	var wg sync.WaitGroup

	// PTY -> connection
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				if _, writeErr := conn.Write(buf[:n]); writeErr != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
	}()

	// Connection -> PTY (with resize message detection)
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				break
			}
			data := buf[:n]

			// Check for JSON resize message prefix
			if len(data) > 0 && data[0] == '{' {
				var resize terminalResizeMsg
				if json.Unmarshal(data, &resize) == nil && resize.Type == "resize" {
					pty.Setsize(ptmx, &pty.Winsize{
						Rows: uint16(resize.Rows),
						Cols: uint16(resize.Cols),
					})
					continue
				}
			}

			if _, err := ptmx.Write(data); err != nil {
				break
			}
		}
		ptmx.Write([]byte{4}) // Ctrl-D
	}()

	cmd.Wait()
	ptmx.Close()
	conn.Close()
	wg.Wait()
}

// TerminalConn wraps a net.Conn for terminal communication.
// It provides the same read/write interface used by the main server's
// WebSocket-to-PTY bridge.
type TerminalConn struct {
	conn   net.Conn
	reader *bufio.Reader
}

// NewTerminalConn creates a terminal connection from a raw net.Conn.
func NewTerminalConn(conn net.Conn) *TerminalConn {
	return &TerminalConn{
		conn:   conn,
		reader: bufio.NewReader(conn),
	}
}

func (tc *TerminalConn) Read(p []byte) (int, error)  { return tc.conn.Read(p) }
func (tc *TerminalConn) Write(p []byte) (int, error)  { return tc.conn.Write(p) }
func (tc *TerminalConn) Close() error                  { return tc.conn.Close() }

// SendResize sends a resize message to the terminal helper.
func (tc *TerminalConn) SendResize(cols, rows int) error {
	msg := terminalResizeMsg{Type: "resize", Cols: cols, Rows: rows}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = tc.conn.Write(data)
	return err
}

// DialTerminal connects to the helper daemon's terminal endpoint and returns
// a TerminalConn for bidirectional communication.
func DialTerminal(socketPath string, ctid int, shell string) (*TerminalConn, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("connecting to helper: %w", err)
	}

	// Send HTTP GET request to upgrade the connection
	reqLine := fmt.Sprintf("GET /v1/terminal?ctid=%d&shell=%s HTTP/1.1\r\n"+
		"Host: helper\r\n"+
		"Connection: close\r\n"+
		"\r\n", ctid, shell)
	if _, err := conn.Write([]byte(reqLine)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("writing request: %w", err)
	}

	// Read HTTP response
	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if !containsHTTP200(statusLine) {
		conn.Close()
		// Read remaining response for error message
		rest, _ := io.ReadAll(reader)
		return nil, fmt.Errorf("helper terminal failed: %s %s", statusLine, string(rest))
	}

	// Drain headers
	for {
		line, err := reader.ReadString('\n')
		if err != nil || line == "\r\n" {
			break
		}
	}

	return NewTerminalConn(conn), nil
}

func containsHTTP200(line string) bool {
	return len(line) >= 12 && line[9] == '2' && line[10] == '0' && line[11] == '0'
}
