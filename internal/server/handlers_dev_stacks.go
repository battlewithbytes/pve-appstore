package server

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/battlewithbytes/pve-appstore/internal/devmode"
)

func (s *Server) handleDevListStacks(w http.ResponseWriter, r *http.Request) {
	stacks, err := s.devSvc.ListStacks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if stacks == nil {
		stacks = []devmode.DevStackMeta{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"stacks": stacks, "total": len(stacks)})
}

func (s *Server) handleDevCreateStack(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID       string `json:"id"`
		Template string `json:"template"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	if err := s.devSvc.CreateStack(req.ID, req.Template); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	stack, err := s.devSvc.GetStack(req.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, stack)
}

func (s *Server) handleDevGetStack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	stack, err := s.devSvc.GetStack(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stack)
}

func (s *Server) handleDevSaveStackManifest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	data, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	if err := s.devSvc.SaveStackManifest(id, data); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (s *Server) handleDevDeleteStack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// Undeploy from catalog first if deployed
	if s.devSvc.IsStackDeployed(id) {
		if s.catalogSvc != nil {
			s.catalogSvc.RemoveDevStack(id)
		}
	}

	if err := s.devSvc.DeleteStack(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handleDevGetStackIcon serves the icon for a dev stack.
func (s *Server) handleDevGetStackIcon(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	iconPath := filepath.Join(s.devSvc.AppDir(id), "icon.png")
	if _, err := os.Stat(iconPath); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, "icon not found")
		return
	}
	http.ServeFile(w, r, iconPath)
}
