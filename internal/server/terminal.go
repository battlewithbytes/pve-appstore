package server

import (
	"bufio"
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

	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	inst, err := s.engine.GetInstall(installID)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("install %q not found", installID))
		return
	}

	s.handleTerminalForCTID(w, r, inst.CTID)
}

// handleTerminalForCTID opens a WebSocket terminal to a container by CTID.
func (s *Server) handleTerminalForCTID(w http.ResponseWriter, r *http.Request, ctid int) {
	// Accept WebSocket
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	ctx := r.Context()

	// Spawn a root shell inside the container via pct exec
	cmd := pctpkg.SudoNsenterCmd("/usr/sbin/pct", "exec", strconv.Itoa(ctid), "--", "/bin/bash", "-l")
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

// handleJournalLogs streams journalctl output from inside a container via WebSocket.
// Unlike handleTerminal, this is read-only (no PTY, no stdin).
func (s *Server) handleJournalLogs(w http.ResponseWriter, r *http.Request) {
	installID := r.PathValue("id")

	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	inst, err := s.engine.GetInstall(installID)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("install %q not found", installID))
		return
	}

	s.handleJournalLogsForCTID(w, r, inst.CTID)
}

// handleJournalLogsForCTID streams journalctl output from a container by CTID via WebSocket.
func (s *Server) handleJournalLogsForCTID(w http.ResponseWriter, r *http.Request, ctid int) {
	// Accept WebSocket
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	ctx := r.Context()

	// Run journalctl inside the container via pct exec (no PTY needed)
	cmd := pctpkg.SudoNsenterCmd("/usr/sbin/pct", "exec", strconv.Itoa(ctid), "--",
		"journalctl", "-f", "--no-pager", "-n", "100", "--output=short-iso")
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
