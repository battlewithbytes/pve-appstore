package server

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/battlewithbytes/pve-appstore/internal/config"
	"github.com/battlewithbytes/pve-appstore/internal/devmode"
	"github.com/battlewithbytes/pve-appstore/internal/engine"
	gh "github.com/battlewithbytes/pve-appstore/internal/github"
)

func (s *Server) handleDevValidate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	appDir := s.devSvc.AppDir(id)
	if _, err := os.Stat(appDir); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("dev app %q not found", id))
		return
	}

	result := devmode.Validate(appDir)

	// If this app ID exists in the official catalog, the dev version must have a different version number
	if s.catalogSvc != nil {
		if official, ok := s.catalogSvc.GetApp(id); ok && official.Source != "developer" {
			devManifest, err := s.devSvc.ParseManifest(id)
			if err == nil && devManifest.Version == official.Version {
				result.Errors = append(result.Errors, devmode.ValidationMsg{
					File:    "app.yml",
					Message: fmt.Sprintf("Version %q is the same as the official catalog app — bump the version to differentiate your branch", devManifest.Version),
					Code:    "VERSION_SAME_AS_OFFICIAL",
				})
				result.Valid = false
			}
		}
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleDevDeploy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	appDir := s.devSvc.AppDir(id)

	// Validate first
	result := devmode.Validate(appDir)

	// Block deploy if version matches the official catalog app
	if s.catalogSvc != nil {
		if official, ok := s.catalogSvc.GetApp(id); ok && official.Source != "developer" {
			devManifest, parseErr := s.devSvc.ParseManifest(id)
			if parseErr == nil && devManifest.Version == official.Version {
				result.Errors = append(result.Errors, devmode.ValidationMsg{
					File:    "app.yml",
					Message: fmt.Sprintf("Version %q is the same as the official catalog app — bump the version to differentiate your branch", devManifest.Version),
					Code:    "VERSION_SAME_AS_OFFICIAL",
				})
				result.Valid = false
			}
		}
	}

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

func (s *Server) handleDevImportZip(w http.ResponseWriter, r *http.Request) {
	// Accept up to 10 MB for ZIP uploads
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file field is required (multipart form)")
		return
	}
	defer file.Close()

	// Read the entire ZIP into memory (already limited to 10 MB)
	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read uploaded file")
		return
	}

	zr, err := zip.NewReader(strings.NewReader(string(data)), int64(len(data)))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid ZIP file")
		return
	}

	// Determine the single top-level directory
	topDirs := map[string]bool{}
	for _, f := range zr.File {
		parts := strings.SplitN(filepath.ToSlash(f.Name), "/", 2)
		if len(parts) > 0 && parts[0] != "" {
			topDirs[parts[0]] = true
		}
	}
	if len(topDirs) != 1 {
		writeError(w, http.StatusBadRequest, "ZIP must contain exactly one top-level directory")
		return
	}

	var topDir string
	for d := range topDirs {
		topDir = d
	}

	// Validate the ID
	id := topDir
	if id == "" || len(id) > 64 {
		writeError(w, http.StatusBadRequest, "invalid app id from ZIP directory name")
		return
	}

	// Check if already exists
	appDir := s.devSvc.AppDir(id)
	if _, err := os.Stat(appDir); err == nil {
		writeError(w, http.StatusConflict, fmt.Sprintf("dev app %q already exists", id))
		return
	}

	// Determine type: app.yml → app, stack.yml → stack
	hasAppYml := false
	hasStackYml := false
	for _, f := range zr.File {
		rel := strings.TrimPrefix(filepath.ToSlash(f.Name), topDir+"/")
		if rel == "app.yml" {
			hasAppYml = true
		}
		if rel == "stack.yml" {
			hasStackYml = true
		}
	}
	if !hasAppYml && !hasStackYml {
		writeError(w, http.StatusBadRequest, "ZIP must contain app.yml or stack.yml")
		return
	}

	// Extract files to dev-apps/{id}/
	if err := os.MkdirAll(appDir, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create app directory")
		return
	}

	for _, f := range zr.File {
		rel := strings.TrimPrefix(filepath.ToSlash(f.Name), topDir+"/")
		if rel == "" || rel == "." {
			continue
		}
		// Skip dotfiles
		if strings.HasPrefix(filepath.Base(rel), ".") {
			continue
		}
		// Prevent path traversal
		if strings.Contains(rel, "..") {
			continue
		}

		target := filepath.Join(appDir, rel)
		if f.FileInfo().IsDir() {
			os.MkdirAll(target, 0755)
			continue
		}

		os.MkdirAll(filepath.Dir(target), 0755)
		rc, err := f.Open()
		if err != nil {
			continue
		}
		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			continue
		}
		io.Copy(out, rc)
		out.Close()
		rc.Close()
	}

	// Return the appropriate response
	if hasStackYml {
		stack, err := s.devSvc.GetStack(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("imported but failed to read stack: %v", err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"type":  "stack",
			"id":    id,
			"stack": stack,
		})
		return
	}

	// Ensure icon exists
	s.devSvc.EnsureIcon(id)

	app, err := s.devSvc.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("imported but failed to read app: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"type": "app",
		"id":   id,
		"app":  app,
	})
}

func (s *Server) handleDevExportStack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	appDir := s.devSvc.AppDir(id)
	if _, err := os.Stat(filepath.Join(appDir, "stack.yml")); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("dev stack %q not found", id))
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

func (s *Server) handleDevValidateStack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	appDir := s.devSvc.AppDir(id)
	if _, err := os.Stat(filepath.Join(appDir, "stack.yml")); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("dev stack %q not found", id))
		return
	}

	result := devmode.ValidateStack(appDir)
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleDevDeployStack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	appDir := s.devSvc.AppDir(id)

	// Validate first
	result := devmode.ValidateStack(appDir)
	if !result.Valid {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error":      "stack has validation errors",
			"validation": result,
		})
		return
	}

	// Parse stack manifest
	sm, err := s.devSvc.ParseStackManifest(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Set computed paths
	sm.DirPath = appDir
	if _, err := os.Stat(filepath.Join(appDir, "icon.png")); err == nil {
		sm.IconPath = filepath.Join(appDir, "icon.png")
	}

	// Merge into catalog
	if s.catalogSvc != nil {
		s.catalogSvc.MergeDevStack(sm)
	}
	s.devSvc.SetStackStatus(id, "deployed")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":   "deployed",
		"stack_id": sm.ID,
		"message":  fmt.Sprintf("Stack %q is now available in the catalog", sm.Name),
	})
}

func (s *Server) handleDevUndeployStack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.catalogSvc != nil {
		s.catalogSvc.RemoveDevStack(id)
	}
	s.devSvc.SetStackStatus(id, "draft")
	writeJSON(w, http.StatusOK, map[string]string{"status": "undeployed"})
}

// handleDevPublishStack publishes a dev stack to GitHub as a PR.
func (s *Server) handleDevPublishStack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if s.githubStore == nil {
		writeError(w, http.StatusServiceUnavailable, "store not available")
		return
	}

	tokenEnc, _ := s.githubStore.GetGitHubState("github_token")
	if tokenEnc == "" {
		writeError(w, http.StatusBadRequest, "GitHub not connected")
		return
	}

	hmacSecret := s.cfg.Auth.HMACSecret
	if hmacSecret == "" {
		hmacSecret = "pve-appstore-default-key"
	}
	token, err := gh.DecryptToken(tokenEnc, hmacSecret)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to decrypt GitHub token")
		return
	}

	sm, err := s.devSvc.ParseStackManifest(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("stack manifest validation failed: %v", err))
		return
	}

	// Verify all referenced apps exist in the catalog (not just dev-deployed)
	if s.catalogSvc != nil {
		for _, sa := range sm.Apps {
			if app, ok := s.catalogSvc.GetApp(sa.AppID); !ok {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("referenced app %q not found in catalog", sa.AppID))
				return
			} else if app.Source == "developer" {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("referenced app %q must be published first (still in dev mode)", sa.AppID))
				return
			}
		}
	}

	catalogOwner, catalogRepo, err := config.ParseGitHubRepo(s.cfg.Catalog.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("cannot parse catalog URL: %v", err))
		return
	}

	// Determine PR title
	title := fmt.Sprintf("Add stack %s v%s", sm.Name, sm.Version)
	if s.catalogSvc != nil {
		if existing, ok := s.catalogSvc.GetStack(id); ok && existing.Source != "developer" {
			title = fmt.Sprintf("Update stack %s to v%s", sm.Name, sm.Version)
		}
	}

	result, err := s.publishToGitHub(token, catalogOwner, catalogRepo, publishParams{
		ID:         id,
		BranchName: "stack/" + id,
		DirPrefix:  "stacks/" + id,
		LocalDir:   s.devSvc.AppDir(id),
		Title:      title,
		Body:       buildStackPRBody(sm),
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	s.devSvc.SetGitHubMeta(id, map[string]string{
		"status":           "published",
		"github_branch":    "stack/" + id,
		"github_pr_url":    result.PRURL,
		"github_pr_number": strconv.Itoa(result.PRNumber),
	})

	log.Printf("[github] stack PR %s: %s", result.Action, result.PRURL)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"pr_url":    result.PRURL,
		"pr_number": result.PRNumber,
		"action":    result.Action,
	})
}

// handleDevStackPublishStatus checks the publish readiness and PR state for a dev stack.
func (s *Server) handleDevStackPublishStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	checks := map[string]bool{
		"github_connected":  false,
		"validation_passed": false,
	}

	if s.githubStore != nil {
		if tokenEnc, _ := s.githubStore.GetGitHubState("github_token"); tokenEnc != "" {
			checks["github_connected"] = true
		}
	}

	appDir := s.devSvc.AppDir(id)
	if _, err := os.Stat(filepath.Join(appDir, "stack.yml")); err == nil {
		if _, err := s.devSvc.ParseStackManifest(id); err == nil {
			checks["validation_passed"] = true
		}
	}

	// Check all referenced apps are published
	appsPublished := true
	if sm, err := s.devSvc.ParseStackManifest(id); err == nil && s.catalogSvc != nil {
		for _, sa := range sm.Apps {
			if app, ok := s.catalogSvc.GetApp(sa.AppID); !ok || app.Source == "developer" {
				appsPublished = false
				break
			}
		}
	} else {
		appsPublished = false
	}
	checks["apps_published"] = appsPublished

	ready := checks["github_connected"] && checks["validation_passed"] && checks["apps_published"]

	published := false
	prURL := ""
	prState := ""
	if stack, err := s.devSvc.GetStack(id); err == nil && stack != nil {
		if stack.GitHubPRURL != "" {
			published = true
			prURL = stack.GitHubPRURL
			prState = s.getPRState(prURL)
		}
	}

	// Auto-refresh catalog when PR is merged so the stack appears immediately
	if prState == "pr_merged" {
		if stack, err := s.devSvc.GetStack(id); err == nil && stack != nil && (stack.Status == "published" || stack.Status == "deployed") {
			s.tryRefreshInBackground("PR merged for stack " + id)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ready":     ready,
		"checks":    checks,
		"published": published,
		"pr_url":    prURL,
		"pr_state":  prState,
	})
}

// Catalog stack handlers

func (s *Server) handleListCatalogStacks(w http.ResponseWriter, r *http.Request) {
	if s.catalogSvc == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"stacks": []interface{}{}, "total": 0})
		return
	}
	stacks := s.catalogSvc.ListStacks()
	writeJSON(w, http.StatusOK, map[string]interface{}{"stacks": stacks, "total": len(stacks)})
}

func (s *Server) handleGetCatalogStack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.catalogSvc == nil {
		writeError(w, http.StatusNotFound, "catalog not available")
		return
	}
	sm, ok := s.catalogSvc.GetStack(id)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("stack %q not found", id))
		return
	}

	// Read README if available
	readme := ""
	if sm.DirPath != "" {
		if data, err := os.ReadFile(filepath.Join(sm.DirPath, "README.md")); err == nil {
			readme = string(data)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"stack":  sm,
		"readme": readme,
	})
}

func (s *Server) handleInstallCatalogStack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.catalogSvc == nil {
		writeError(w, http.StatusNotFound, "catalog not available")
		return
	}
	sm, ok := s.catalogSvc.GetStack(id)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("stack %q not found", id))
		return
	}

	// Validate all referenced apps exist
	for _, sa := range sm.Apps {
		if _, ok := s.catalogSvc.GetApp(sa.AppID); !ok {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("stack references app %q which is not in catalog", sa.AppID))
			return
		}
	}

	// Parse user overrides from body
	var overrides struct {
		Storage  string `json:"storage"`
		Bridge   string `json:"bridge"`
		Cores    int    `json:"cores"`
		MemoryMB int    `json:"memory_mb"`
		DiskGB   int    `json:"disk_gb"`
		Hostname string `json:"hostname"`
	}
	json.NewDecoder(r.Body).Decode(&overrides)

	// Build stack create request from manifest + overrides
	apps := make([]engine.StackAppRequest, len(sm.Apps))
	for i, sa := range sm.Apps {
		apps[i] = engine.StackAppRequest{
			AppID:  sa.AppID,
			Inputs: sa.Inputs,
		}
	}

	cores := sm.LXC.Defaults.Cores
	if overrides.Cores > 0 {
		cores = overrides.Cores
	}
	memoryMB := sm.LXC.Defaults.MemoryMB
	if overrides.MemoryMB > 0 {
		memoryMB = overrides.MemoryMB
	}
	diskGB := sm.LXC.Defaults.DiskGB
	if overrides.DiskGB > 0 {
		diskGB = overrides.DiskGB
	}
	storage := overrides.Storage
	if storage == "" && len(s.cfg.Storages) > 0 {
		storage = s.cfg.Storages[0]
	}
	bridge := overrides.Bridge
	if bridge == "" && len(s.cfg.Bridges) > 0 {
		bridge = s.cfg.Bridges[0]
	}

	req := engine.StackCreateRequest{
		Name:     sm.Name,
		Apps:     apps,
		Storage:  storage,
		Bridge:   bridge,
		Cores:    cores,
		MemoryMB: memoryMB,
		DiskGB:   diskGB,
		Hostname: overrides.Hostname,
	}

	job, err := s.engineStackSvc.StartStack(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}
