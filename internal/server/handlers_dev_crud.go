package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg" // register JPEG decoder for image.Decode
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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

func (s *Server) handleDevDeleteFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, "path query parameter required")
		return
	}
	if err := s.devSvc.DeleteFile(id, path); err != nil {
		if strings.Contains(err.Error(), "cannot delete core file") {
			writeError(w, http.StatusBadRequest, err.Error())
		} else if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
		} else {
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	// Auto-refresh catalog if deployed
	s.refreshDeployedDevApp(id)

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "path": path})
}

func (s *Server) handleDevRenameFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.From == "" || req.To == "" {
		writeError(w, http.StatusBadRequest, "from and to are required")
		return
	}
	if err := s.devSvc.RenameFile(id, req.From, req.To); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Auto-refresh catalog if deployed
	s.refreshDeployedDevApp(id)

	writeJSON(w, http.StatusOK, map[string]string{"status": "renamed"})
}

func (s *Server) handleDevRenameApp(w http.ResponseWriter, r *http.Request) {
	oldID := r.PathValue("id")

	var req struct {
		NewID string `json:"new_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.NewID == "" {
		writeError(w, http.StatusBadRequest, "new_id is required")
		return
	}

	// Undeploy from catalog under old ID first
	if s.catalogSvc != nil {
		s.catalogSvc.RemoveDevApp(oldID)
	}

	if err := s.devSvc.RenameApp(oldID, req.NewID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "renamed", "new_id": req.NewID})
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

func (s *Server) handleDevUploadFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// 2 MB upload limit
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "file too large (max 2MB)")
		return
	}

	destPath := r.FormValue("path")
	if destPath == "" {
		writeError(w, http.StatusBadRequest, "path field is required")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file field is required")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read uploaded file")
		return
	}

	resized := false
	if destPath == "icon.png" {
		data, resized, err = processIcon(data)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("icon processing failed: %v", err))
			return
		}
	}

	if err := s.devSvc.SaveFile(id, destPath, data); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Auto-refresh catalog if deployed
	s.refreshDeployedDevApp(id)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "saved",
		"path":    destPath,
		"resized": resized,
	})
}

// processIcon decodes a PNG or JPEG image, resizes it to fit within 256x256
// if larger, and re-encodes as PNG. Returns the processed bytes and whether
// a resize occurred.
func processIcon(data []byte) ([]byte, bool, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, false, fmt.Errorf("unsupported image format: %w", err)
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	const maxDim = 256
	resized := false

	if w > maxDim || h > maxDim {
		resized = true
		newW, newH := w, h
		if w >= h {
			newW = maxDim
			newH = h * maxDim / w
		} else {
			newH = maxDim
			newW = w * maxDim / h
		}
		if newW < 1 {
			newW = 1
		}
		if newH < 1 {
			newH = 1
		}

		dst := image.NewNRGBA(image.Rect(0, 0, newW, newH))
		for y := 0; y < newH; y++ {
			srcY := bounds.Min.Y + y*h/newH
			for x := 0; x < newW; x++ {
				srcX := bounds.Min.X + x*w/newW
				dst.Set(x, y, img.At(srcX, srcY))
			}
		}
		img = dst
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, false, fmt.Errorf("PNG encode failed: %w", err)
	}
	return buf.Bytes(), resized, nil
}

func (s *Server) handleDevBranchApp(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SourceID string `json:"source_id"` // catalog app to branch
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.SourceID == "" {
		writeError(w, http.StatusBadRequest, "source_id is required")
		return
	}

	// Dev app ID = source ID (no rename)
	newID := req.SourceID

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

	if err := s.devSvc.Fork(newID, catApp.DirPath); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			writeError(w, http.StatusConflict, fmt.Sprintf("dev copy of %q already exists — open it from the Developer Dashboard", newID))
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Track which catalog app was branched
	s.devSvc.SetGitHubMeta(newID, map[string]string{
		"source_app_id": req.SourceID,
	})

	app, err := s.devSvc.Get(newID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, app)
}
