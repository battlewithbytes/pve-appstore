package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/creack/pty"
	"nhooyr.io/websocket"

	pctpkg "github.com/battlewithbytes/pve-appstore/internal/pct"
)

// terminalResize is sent by the frontend to resize the terminal.
type terminalResize struct {
	Type string `json:"type"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

func (s *Server) handleTerminal(w http.ResponseWriter, r *http.Request) {
	installID := r.PathValue("id")

	if s.engineInstallSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	inst, err := s.engineInstallSvc.GetInstall(installID)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("install %q not found", installID))
		return
	}

	s.handleTerminalForCTID(w, r, inst.CTID)
}

// handleTerminalForCTID opens a WebSocket terminal to a container by CTID.
func (s *Server) handleTerminalForCTID(w http.ResponseWriter, r *http.Request, ctid int) {
	// Accept WebSocket — restrict to same host + localhost for dev
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: s.allowedOriginPatterns(r),
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	ctx := r.Context()

	// Detect available shell: prefer bash, fall back to sh (Alpine has no bash)
	shell := "/bin/bash"
	if result, err := pctpkg.Exec(ctid, []string{"test", "-x", "/bin/bash"}); err != nil || result.ExitCode != 0 {
		shell = "/bin/sh"
	}

	// Use helper daemon if available for terminal
	if pctpkg.Helper != nil {
		s.handleTerminalViaHelper(conn, ctx, ctid, shell)
		return
	}

	cmd := pctpkg.SudoNsenterCmd("/usr/sbin/pct", "exec", strconv.Itoa(ctid), "--", shell, "-l")
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.Start(cmd)
	if err != nil {
		conn.Close(websocket.StatusInternalError, fmt.Sprintf("failed to start shell: %v", err))
		return
	}
	defer ptmx.Close()

	// Set initial size
	pty.Setsize(ptmx, &pty.Winsize{Rows: 24, Cols: 80})

	var wg sync.WaitGroup

	// PTY -> WebSocket
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				if writeErr := conn.Write(ctx, websocket.MessageBinary, buf[:n]); writeErr != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
	}()

	// WebSocket -> PTY
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			msgType, data, err := conn.Read(ctx)
			if err != nil {
				break
			}
			if msgType == websocket.MessageText {
				// Check if it's a resize message
				var resize terminalResize
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
		// Signal EOF to the PTY so the shell exits
		ptmx.Write([]byte{4}) // Ctrl-D
	}()

	// Wait for process to exit
	cmd.Wait()
	// Close PTY read side so the reader goroutine exits
	ptmx.Close()

	// Clean shutdown
	conn.Close(websocket.StatusNormalClosure, "shell exited")
	wg.Wait()
}

// handleTerminalViaHelper bridges a WebSocket to the helper daemon's terminal.
func (s *Server) handleTerminalViaHelper(conn *websocket.Conn, ctx context.Context, ctid int, shell string) {
	termConn, err := pctpkg.Helper.StartTerminal(ctid, shell)
	if err != nil {
		conn.Close(websocket.StatusInternalError, fmt.Sprintf("helper terminal: %v", err))
		return
	}
	defer termConn.Close()

	var wg sync.WaitGroup

	// Helper -> WebSocket
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := termConn.Read(buf)
			if n > 0 {
				if writeErr := conn.Write(ctx, websocket.MessageBinary, buf[:n]); writeErr != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
	}()

	// WebSocket -> Helper
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			msgType, data, err := conn.Read(ctx)
			if err != nil {
				break
			}
			if msgType == websocket.MessageText {
				// Check if it's a resize message — forward as JSON to helper
				var resize terminalResize
				if json.Unmarshal(data, &resize) == nil && resize.Type == "resize" {
					// Forward resize to helper
					termConn.Write(data)
					continue
				}
			}
			if _, err := termConn.Write(data); err != nil {
				break
			}
		}
	}()

	wg.Wait()
	conn.Close(websocket.StatusNormalClosure, "shell exited")
}

// handleJournalLogs streams journalctl output from inside a container via WebSocket.
// Unlike handleTerminal, this is read-only (no PTY, no stdin).
func (s *Server) handleJournalLogs(w http.ResponseWriter, r *http.Request) {
	installID := r.PathValue("id")

	if s.engineInstallSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	inst, err := s.engineInstallSvc.GetInstall(installID)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("install %q not found", installID))
		return
	}

	s.handleJournalLogsForCTID(w, r, inst.CTID)
}

// handleJournalLogsForCTID streams journalctl output from a container by CTID via WebSocket.
func (s *Server) handleJournalLogsForCTID(w http.ResponseWriter, r *http.Request, ctid int) {
	// Accept WebSocket — restrict to same host + localhost for dev
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: s.allowedOriginPatterns(r),
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	ctx := r.Context()

	// Detect OS to pick the right log command: journalctl (Debian) or tail (Alpine)
	logArgs := []string{"journalctl", "-f", "--no-pager", "-n", "100", "--output=short-iso"}
	if result, err := pctpkg.Exec(ctid, []string{"test", "-f", "/etc/alpine-release"}); err == nil && result.ExitCode == 0 {
		// Alpine/BusyBox: tail the syslog file (logread requires syslogd -C which isn't default)
		logArgs = []string{"tail", "-n", "100", "-f", "/var/log/messages"}
	}

	// Use helper daemon for streaming if available — the exec-stream endpoint
	// handles pct exec directly, but for journal logs we need the streaming variant.
	// Since the helper's exec runs pct directly (no sudo/nsenter needed), we fall through
	// to the same SudoNsenterCmd path when helper is not available.
	pctArgs := append([]string{"exec", strconv.Itoa(ctid), "--"}, logArgs...)
	cmd := pctpkg.SudoNsenterCmd("/usr/sbin/pct", pctArgs...)
	cmd.Env = append(os.Environ(), "TERM=dumb")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		conn.Close(websocket.StatusInternalError, fmt.Sprintf("pipe: %v", err))
		return
	}
	cmd.Stderr = cmd.Stdout // merge stderr into stdout

	if err := cmd.Start(); err != nil {
		conn.Close(websocket.StatusInternalError, fmt.Sprintf("start: %v", err))
		return
	}

	// stdout -> WebSocket (line-buffered text)
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 64*1024)
		for scanner.Scan() {
			line := scanner.Text() + "\n"
			if writeErr := conn.Write(ctx, websocket.MessageText, []byte(line)); writeErr != nil {
				break
			}
		}
	}()

	// Read from WebSocket only to detect client disconnect
	go func() {
		for {
			_, _, err := conn.Read(ctx)
			if err != nil {
				cmd.Process.Kill()
				return
			}
		}
	}()

	cmd.Wait()
	conn.Close(websocket.StatusNormalClosure, "log stream ended")
}
