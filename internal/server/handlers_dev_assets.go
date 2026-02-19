package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/battlewithbytes/pve-appstore/internal/devmode"
	"gopkg.in/yaml.v3"
)

func (s *Server) handleDevGetIcon(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	appDir := s.devSvc.AppDir(id)
	iconPath := filepath.Join(appDir, "icon.png")
	if data, err := os.ReadFile(iconPath); err == nil {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(data)
		return
	}
	// Fall back to default icon
	w.Header().Set("Content-Type", "image/png")
	w.Write(defaultIconPNG)
}

func (s *Server) handleDevSetIcon(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	appDir := s.devSvc.AppDir(id)
	if _, err := os.Stat(appDir); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("dev app %q not found", id))
		return
	}

	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}
	if !strings.HasPrefix(req.URL, "http://") && !strings.HasPrefix(req.URL, "https://") {
		writeError(w, http.StatusBadRequest, "url must start with http:// or https://")
		return
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(req.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to fetch icon: %v", err))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("icon URL returned status %d", resp.StatusCode))
		return
	}

	iconData, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20)) // 2MB limit
	if err != nil || len(iconData) == 0 {
		writeError(w, http.StatusBadRequest, "failed to read icon data")
		return
	}

	processed, resized, err := processIcon(iconData)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("icon processing failed: %v", err))
		return
	}

	if err := os.WriteFile(filepath.Join(appDir, "icon.png"), processed, 0644); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save icon")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "saved", "resized": resized})
}

// syncDevIcon checks the manifest for an icon URL and downloads it if present.
func (s *Server) syncDevIcon(id string, manifestData []byte) {
	var manifest struct {
		Icon string `yaml:"icon"`
	}
	if err := yaml.Unmarshal(manifestData, &manifest); err != nil || manifest.Icon == "" {
		return
	}
	if !strings.HasPrefix(manifest.Icon, "http://") && !strings.HasPrefix(manifest.Icon, "https://") {
		return
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(manifest.Icon)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}

	iconData, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil || len(iconData) == 0 {
		return
	}

	processed, _, err := processIcon(iconData)
	if err != nil {
		return
	}

	appDir := s.devSvc.AppDir(id)
	os.WriteFile(filepath.Join(appDir, "icon.png"), processed, 0644)
}

func (s *Server) handleDevListTemplates(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"templates": devmode.ListTemplates(),
	})
}

func (s *Server) handleDevListOSTemplates(w http.ResponseWriter, r *http.Request) {
	// Return cached result if available
	s.osTemplatesMu.Lock()
	cached := s.osTemplatesJSON
	s.osTemplatesMu.Unlock()
	if cached != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Write(cached)
		return
	}

	templates, err := s.engine.ListOSTemplates(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"templates": []string{}})
		return
	}
	data, _ := json.Marshal(map[string]interface{}{"templates": templates})

	s.osTemplatesMu.Lock()
	s.osTemplatesJSON = data
	s.osTemplatesMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(data)
}
