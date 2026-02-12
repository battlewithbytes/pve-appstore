package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (s *Server) handleBrowseStorages(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"storages": s.cfg.Storages,
	})
}

func (s *Server) handleBrowseMounts(w http.ResponseWriter, r *http.Request) {
	type mountInfo struct {
		Path   string `json:"path"`
		FsType string `json:"fs_type"`
		Device string `json:"device"`
	}

	var mounts []mountInfo
	for _, sm := range s.storageMetas {
		if sm.Browsable && sm.Path != "" {
			mounts = append(mounts, mountInfo{
				Path:   sm.Path,
				FsType: sm.Type,
				Device: sm.ID,
			})
		}
	}
	if mounts == nil {
		mounts = []mountInfo{}
	}
	sort.Slice(mounts, func(i, j int) bool { return mounts[i].Path < mounts[j].Path })

	writeJSON(w, http.StatusOK, map[string]interface{}{"mounts": mounts})
}

func (s *Server) handleBrowseMkdir(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	cleaned := filepath.Clean(req.Path)
	if !s.isPathAllowed(cleaned) {
		writeError(w, http.StatusForbidden, "path is not under a configured storage")
		return
	}

	if err := os.MkdirAll(cleaned, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("mkdir failed: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"path": cleaned, "created": true})
}

func (s *Server) handleBrowsePaths(w http.ResponseWriter, r *http.Request) {
	root := r.URL.Query().Get("path")
	if root == "" {
		root = "/"
	}
	root = filepath.Clean(root)

	if !s.isPathAllowed(root) {
		writeError(w, http.StatusForbidden, "path is not under a configured storage")
		return
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("cannot read %s: %v", root, err))
		return
	}

	type dirEntry struct {
		Name  string `json:"name"`
		Path  string `json:"path"`
		IsDir bool   `json:"is_dir"`
	}
	dirs := []dirEntry{}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		dirs = append(dirs, dirEntry{
			Name:  e.Name(),
			Path:  filepath.Join(root, e.Name()),
			IsDir: e.IsDir(),
		})
	}
	sort.Slice(dirs, func(i, j int) bool {
		if dirs[i].IsDir != dirs[j].IsDir {
			return dirs[i].IsDir
		}
		return dirs[i].Name < dirs[j].Name
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"path":    root,
		"entries": dirs,
	})
}
