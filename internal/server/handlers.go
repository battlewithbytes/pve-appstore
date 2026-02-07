package server

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/inertz/pve-appstore/internal/catalog"
	"github.com/inertz/pve-appstore/internal/engine"
	"github.com/inertz/pve-appstore/internal/version"
)

//go:embed assets/default-icon.png
var defaultIconPNG []byte

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "ok",
		"version":   version.Version,
		"node":      s.cfg.NodeName,
		"app_count": s.catalog.AppCount(),
	})
}

func (s *Server) handleListApps(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	category := r.URL.Query().Get("category")
	sortBy := r.URL.Query().Get("sort")

	var apps = s.catalog.List()

	// Apply search filter
	if query != "" {
		apps = s.catalog.Search(query)
	}

	// Apply category filter
	if category != "" {
		filtered := make([]*appResponse, 0)
		for _, app := range apps {
			for _, c := range app.Categories {
				if strings.EqualFold(c, category) {
					filtered = append(filtered, toAppResponse(app))
					break
				}
			}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"apps":  filtered,
			"total": len(filtered),
		})
		return
	}

	// Convert to response format
	resp := make([]*appResponse, 0, len(apps))
	for _, app := range apps {
		resp = append(resp, toAppResponse(app))
	}

	// Sort
	switch sortBy {
	case "name":
		sort.Slice(resp, func(i, j int) bool {
			return resp[i].Name < resp[j].Name
		})
	default: // "updated" or default — sort by name for now
		sort.Slice(resp, func(i, j int) bool {
			return resp[i].Name < resp[j].Name
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"apps":  resp,
		"total": len(resp),
	})
}

func (s *Server) handleGetApp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	app, ok := s.catalog.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("app %q not found", id))
		return
	}

	writeJSON(w, http.StatusOK, app)
}

func (s *Server) handleGetAppReadme(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	app, ok := s.catalog.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("app %q not found", id))
		return
	}

	if app.ReadmePath == "" {
		writeError(w, http.StatusNotFound, "no readme available")
		return
	}

	data, err := os.ReadFile(app.ReadmePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read readme")
		return
	}

	w.Header().Set("Content-Type", "text/markdown")
	w.Write(data)
}

func (s *Server) handleGetAppIcon(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	app, ok := s.catalog.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("app %q not found", id))
		return
	}

	if app.IconPath != "" {
		http.ServeFile(w, r, app.IconPath)
		return
	}

	// Serve embedded default icon
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(defaultIconPNG)
}

func (s *Server) handleCategories(w http.ResponseWriter, r *http.Request) {
	cats := s.catalog.Categories()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"categories": cats,
	})
}

func (s *Server) handleCatalogRefresh(w http.ResponseWriter, r *http.Request) {
	if err := s.catalog.Refresh(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("refresh failed: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "refreshed",
		"app_count": s.catalog.AppCount(),
	})
}

// appResponse is a lightweight summary for the app list.
type appResponse struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Categories  []string `json:"categories"`
	Tags        []string `json:"tags"`
	HasIcon     bool     `json:"has_icon"`
	GPURequired bool     `json:"gpu_required"`
	GPUSupport  []string `json:"gpu_support,omitempty"`
}

func toAppResponse(app *catalog.AppManifest) *appResponse {
	return &appResponse{
		ID:          app.ID,
		Name:        app.Name,
		Description: app.Description,
		Version:     app.Version,
		Categories:  app.Categories,
		Tags:        app.Tags,
		HasIcon:     true, // default icon served when app has none
		GPURequired: app.GPU.Required,
		GPUSupport:  app.GPU.Supported,
	}
}

func (s *Server) handleConfigDefaults(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"storage": s.cfg.Storage,
		"bridge":  s.cfg.Bridge,
		"defaults": map[string]interface{}{
			"cores":     s.cfg.Defaults.Cores,
			"memory_mb": s.cfg.Defaults.MemoryMB,
			"disk_gb":   s.cfg.Defaults.DiskGB,
		},
	})
}

// --- Install/Jobs handlers ---

func (s *Server) handleInstallApp(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	var req engine.InstallRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	req.AppID = appID

	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "install engine not available")
		return
	}

	job, err := s.engine.StartInstall(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	if s.engine == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"jobs": []interface{}{}, "total": 0})
		return
	}

	jobs, err := s.engine.ListJobs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if jobs == nil {
		jobs = []*engine.Job{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"jobs":  jobs,
		"total": len(jobs),
	})
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	job, err := s.engine.GetJob(id)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("job %q not found", id))
		return
	}

	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleGetJobLogs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	afterStr := r.URL.Query().Get("after")

	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	afterID := 0
	if afterStr != "" {
		afterID, _ = strconv.Atoi(afterStr)
	}

	logs, lastID, err := s.engine.GetLogsSince(id, afterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if logs == nil {
		logs = []*engine.LogEntry{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"logs":    logs,
		"last_id": lastID,
	})
}

func (s *Server) handleListInstalls(w http.ResponseWriter, r *http.Request) {
	if s.engine == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"installs": []interface{}{}, "total": 0})
		return
	}

	installs, err := s.engine.ListInstalls()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if installs == nil {
		installs = []*engine.Install{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"installs": installs,
		"total":    len(installs),
	})
}

func (s *Server) handleUninstall(w http.ResponseWriter, r *http.Request) {
	installID := r.PathValue("id")

	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	job, err := s.engine.Uninstall(installID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, job)
}
