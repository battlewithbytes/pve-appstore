package server

import (
	"net/http"
	"time"

	"github.com/battlewithbytes/pve-appstore/internal/updater"
	"github.com/battlewithbytes/pve-appstore/internal/version"
)

func (s *Server) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	status, err := s.updater.CheckLatestRelease(version.Version)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to check for updates: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleApplyUpdate(w http.ResponseWriter, r *http.Request) {
	status, err := s.updater.CheckLatestRelease(version.Version)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to check for updates: "+err.Error())
		return
	}
	if !status.Available || status.Release == nil {
		writeError(w, http.StatusBadRequest, "no update available")
		return
	}

	if !updater.ScriptExists() {
		writeError(w, http.StatusBadRequest, "Update script not found. Run `sudo pve-appstore self-update` from CLI for the first update.")
		return
	}

	// Download the new binary
	if err := updater.DownloadBinary(status.Release.DownloadURL, updater.TempBinary); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to download update: "+err.Error())
		return
	}

	// Send response before restarting
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "updating",
		"version": status.Release.Version,
	})

	// Flush response then apply update in background
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		updater.ApplyUpdateSudo(updater.TempBinary)
	}()
}
