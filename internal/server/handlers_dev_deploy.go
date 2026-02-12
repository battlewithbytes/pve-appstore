package server

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/battlewithbytes/pve-appstore/internal/devmode"
)

func (s *Server) handleDevValidate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	appDir := s.devSvc.AppDir(id)
	if _, err := os.Stat(appDir); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("dev app %q not found", id))
		return
	}

	result := devmode.Validate(appDir)
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleDevDeploy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	appDir := s.devSvc.AppDir(id)

	// Validate first
	result := devmode.Validate(appDir)
	if !result.Valid {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error":      "app has validation errors",
			"validation": result,
		})
		return
	}

	// Parse manifest
	manifest, err := s.devSvc.ParseManifest(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Set computed paths
	manifest.DirPath = appDir
	if _, err := os.Stat(filepath.Join(appDir, "icon.png")); err == nil {
		manifest.IconPath = filepath.Join(appDir, "icon.png")
	}
	if _, err := os.Stat(filepath.Join(appDir, "README.md")); err == nil {
		manifest.ReadmePath = filepath.Join(appDir, "README.md")
	}

	// Merge into catalog
	if s.catalogSvc != nil {
		s.catalogSvc.MergeDevApp(manifest)
	}
	s.devSvc.SetStatus(id, "deployed")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "deployed",
		"app_id":  manifest.ID,
		"message": fmt.Sprintf("App %q is now available in the catalog", manifest.Name),
	})
}

func (s *Server) handleDevUndeploy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.catalogSvc != nil {
		s.catalogSvc.RemoveDevApp(id)
	}
	s.devSvc.SetStatus(id, "draft")
	writeJSON(w, http.StatusOK, map[string]string{"status": "undeployed"})
}

func (s *Server) handleDevExport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	appDir := s.devSvc.AppDir(id)
	if _, err := os.Stat(appDir); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("dev app %q not found", id))
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, id))

	zw := zip.NewWriter(w)
	defer zw.Close()

	filepath.Walk(appDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		// Skip dot files
		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		relPath, _ := filepath.Rel(appDir, path)
		zipPath := filepath.Join(id, relPath)

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return nil
		}
		header.Name = zipPath
		header.Method = zip.Deflate

		writer, err := zw.CreateHeader(header)
		if err != nil {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer file.Close()
		io.Copy(writer, file)
		return nil
	})
}
