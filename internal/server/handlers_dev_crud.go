package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/battlewithbytes/pve-appstore/internal/devmode"
)

func (s *Server) handleDevListApps(w http.ResponseWriter, r *http.Request) {
	apps, err := s.devSvc.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if apps == nil {
		apps = []devmode.DevAppMeta{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"apps":  apps,
		"total": len(apps),
	})
}

func (s *Server) handleDevCreateApp(w http.ResponseWriter, r *http.Request) {
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

	if err := s.devSvc.Create(req.ID, req.Template); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	app, err := s.devSvc.Get(req.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, app)
}

func (s *Server) handleDevForkApp(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SourceID string `json:"source_id"` // catalog app to fork from
		NewID    string `json:"new_id"`    // new dev app ID
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.SourceID == "" || req.NewID == "" {
		writeError(w, http.StatusBadRequest, "source_id and new_id are required")
		return
	}

	// Look up the catalog app to get its directory
	if s.catalogSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "catalog service not available")
		return
	}
	catApp, ok := s.catalogSvc.GetApp(req.SourceID)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("catalog app %q not found", req.SourceID))
		return
	}
	if catApp.DirPath == "" {
		writeError(w, http.StatusBadRequest, "catalog app has no source directory")
		return
	}

	if err := s.devSvc.Fork(req.NewID, catApp.DirPath); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	app, err := s.devSvc.Get(req.NewID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, app)
}

func (s *Server) handleDevGetApp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	app, err := s.devSvc.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, app)
}

func (s *Server) handleDevSaveManifest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	data, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read body")
		return
	}
	if err := s.devSvc.SaveManifest(id, data); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// If manifest has an icon URL, auto-download it
	s.syncDevIcon(id, data)

	// Auto-refresh catalog if app is deployed
	s.refreshDeployedDevApp(id)

	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (s *Server) handleDevSaveScript(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	data, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read body")
		return
	}
	if err := s.devSvc.SaveScript(id, data); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Auto-refresh catalog if app is deployed
	s.refreshDeployedDevApp(id)

	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

// refreshDeployedDevApp re-merges a dev app into the catalog if it's currently deployed.
// This means edits to app.yml or install.py take effect immediately without undeploy/redeploy.
func (s *Server) refreshDeployedDevApp(id string) {
	if !s.devSvc.IsDeployed(id) {
		return
	}
	manifest, err := s.devSvc.ParseManifest(id)
	if err != nil {
		return // silently skip — invalid manifest won't break anything
	}
	appDir := s.devSvc.AppDir(id)
	manifest.DirPath = appDir
	if _, err := os.Stat(filepath.Join(appDir, "icon.png")); err == nil {
		manifest.IconPath = filepath.Join(appDir, "icon.png")
	}
	if _, err := os.Stat(filepath.Join(appDir, "README.md")); err == nil {
		manifest.ReadmePath = filepath.Join(appDir, "README.md")
	}
	if s.catalogSvc != nil {
		s.catalogSvc.MergeDevApp(manifest)
	}
}

func (s *Server) handleDevSaveFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	if err := s.devSvc.SaveFile(id, req.Path, []byte(req.Content)); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (s *Server) handleDevGetFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, "path query parameter required")
		return
	}
	data, err := s.devSvc.ReadFile(id, path)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"content": string(data)})
}

func (s *Server) handleDevDeleteApp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// Undeploy first if deployed
	if s.catalogSvc != nil {
		s.catalogSvc.RemoveDevApp(id)
	}

	if err := s.devSvc.Delete(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
