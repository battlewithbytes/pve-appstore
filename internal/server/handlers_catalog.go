package server

import (
	_ "embed"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/battlewithbytes/pve-appstore/internal/catalog"
	"github.com/battlewithbytes/pve-appstore/internal/version"
)

//go:embed assets/default-icon.png
var defaultIconPNG []byte

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if s.catalogSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "catalog service not available")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "ok",
		"version":   version.Version,
		"node":      s.cfg.NodeName,
		"app_count": s.catalogSvc.AppCount(),
	})
}

func (s *Server) handleListApps(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	category := r.URL.Query().Get("category")
	sortBy := r.URL.Query().Get("sort")

	if s.catalogSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "catalog service not available")
		return
	}

	var apps = s.catalogSvc.ListApps()

	// Apply search filter
	if query != "" {
		apps = s.catalogSvc.SearchApps(query)
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
	if s.catalogSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "catalog service not available")
		return
	}
	app, ok := s.catalogSvc.GetApp(id)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("app %q not found", id))
		return
	}

	writeJSON(w, http.StatusOK, app)
}

func (s *Server) handleGetAppReadme(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.catalogSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "catalog service not available")
		return
	}
	app, ok := s.catalogSvc.GetApp(id)
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
	if s.catalogSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "catalog service not available")
		return
	}
	app, ok := s.catalogSvc.GetApp(id)
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
	if s.catalogSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "catalog service not available")
		return
	}
	cats := s.catalogSvc.Categories()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"categories": cats,
	})
}

func (s *Server) handleCatalogRefresh(w http.ResponseWriter, r *http.Request) {
	if s.catalogSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "catalog service not available")
		return
	}
	if err := s.catalogSvc.Refresh(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("refresh failed: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "refreshed",
		"app_count": s.catalogSvc.AppCount(),
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
	Official    bool     `json:"official"`
	GPURequired bool     `json:"gpu_required"`
	GPUSupport  []string `json:"gpu_support,omitempty"`
	Source      string   `json:"source,omitempty"`
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
		Official:    app.Official,
		GPURequired: app.GPU.Required,
		GPUSupport:  app.GPU.Supported,
		Source:      app.Source,
	}
}
