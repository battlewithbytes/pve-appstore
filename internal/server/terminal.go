package server

import (
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

	// Accept WebSocket
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	ctx := r.Context()

	// Spawn pct enter <ctid> with a PTY for proper terminal emulation
	cmd := pctpkg.SudoNsenterCmd("/usr/sbin/pct", "enter", strconv.Itoa(inst.CTID))
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
