package server

import (
	"archive/zip"
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

func (s *Server) handleDevListApps(w http.ResponseWriter, r *http.Request) {
	apps, err := s.devStore.List()
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

	if err := s.devStore.Create(req.ID, req.Template); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	app, err := s.devStore.Get(req.ID)
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
	catApp, ok := s.catalog.Get(req.SourceID)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("catalog app %q not found", req.SourceID))
		return
	}
	if catApp.DirPath == "" {
		writeError(w, http.StatusBadRequest, "catalog app has no source directory")
		return
	}

	if err := s.devStore.Fork(req.NewID, catApp.DirPath); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	app, err := s.devStore.Get(req.NewID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, app)
}

func (s *Server) handleDevGetApp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	app, err := s.devStore.Get(id)
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
	if err := s.devStore.SaveManifest(id, data); err != nil {
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
	if err := s.devStore.SaveScript(id, data); err != nil {
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
	if !s.devStore.IsDeployed(id) {
		return
	}
	manifest, err := s.devStore.ParseManifest(id)
	if err != nil {
		return // silently skip — invalid manifest won't break anything
	}
	appDir := s.devStore.AppDir(id)
	manifest.DirPath = appDir
	if _, err := os.Stat(filepath.Join(appDir, "icon.png")); err == nil {
		manifest.IconPath = filepath.Join(appDir, "icon.png")
	}
	if _, err := os.Stat(filepath.Join(appDir, "README.md")); err == nil {
		manifest.ReadmePath = filepath.Join(appDir, "README.md")
	}
	s.catalog.MergeDevApp(manifest)
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

	if err := s.devStore.SaveFile(id, req.Path, []byte(req.Content)); err != nil {
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
	data, err := s.devStore.ReadFile(id, path)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"content": string(data)})
}

func (s *Server) handleDevGetIcon(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	appDir := s.devStore.AppDir(id)
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
	appDir := s.devStore.AppDir(id)
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

	if err := os.WriteFile(filepath.Join(appDir, "icon.png"), iconData, 0644); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save icon")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (s *Server) handleDevDeleteApp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// Undeploy first if deployed
	s.catalog.RemoveDevApp(id)

	if err := s.devStore.Delete(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleDevValidate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	appDir := s.devStore.AppDir(id)
	if _, err := os.Stat(appDir); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("dev app %q not found", id))
		return
	}

	result := devmode.Validate(appDir)
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleDevDeploy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	appDir := s.devStore.AppDir(id)

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
	manifest, err := s.devStore.ParseManifest(id)
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
	s.catalog.MergeDevApp(manifest)
	s.devStore.SetStatus(id, "deployed")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "deployed",
		"app_id":  manifest.ID,
		"message": fmt.Sprintf("App %q is now available in the catalog", manifest.Name),
	})
}

func (s *Server) handleDevUndeploy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.catalog.RemoveDevApp(id)
	s.devStore.SetStatus(id, "draft")
	writeJSON(w, http.StatusOK, map[string]string{"status": "undeployed"})
}

func (s *Server) handleDevExport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	appDir := s.devStore.AppDir(id)
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

func (s *Server) handleDevImportUnraid(w http.ResponseWriter, r *http.Request) {
	var req struct {
		XML string `json:"xml"`
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	xmlData := req.XML

	// If URL provided, fetch the XML from it
	if xmlData == "" && req.URL != "" {
		u := req.URL
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			writeError(w, http.StatusBadRequest, "url must start with http:// or https://")
			return
		}
		client := &http.Client{Timeout: 15 * time.Second}
		resp, err := client.Get(u)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to fetch URL: %v", err))
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("URL returned status %d", resp.StatusCode))
			return
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to read URL response: %v", err))
			return
		}
		xmlData = string(body)
	}

	if xmlData == "" {
		writeError(w, http.StatusBadRequest, "xml or url is required")
		return
	}

	container, err := devmode.ParseUnraidXML([]byte(xmlData))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Auto-fetch Dockerfile from GitHub to generate better install scripts
	var dfInfo *devmode.DockerfileInfo
	ghURL := container.GitHub
	if ghURL == "" {
		ghURL = container.Project // fallback to Project URL
	}
	if dockerfileURLTmpl, branch := devmode.InferDockerfileURL(container.Repository, ghURL); dockerfileURLTmpl != "" {
		dfInfo = s.fetchAndParseDockerfile(dockerfileURLTmpl, branch, container.Name)
	}

	// Set RepoURL from available GitHub URLs
	if dfInfo != nil && dfInfo.RepoURL == "" {
		for _, candidate := range []string{ghURL, container.Project} {
			if repo := devmode.ExtractGitHubRepoURL(candidate); repo != "" {
				dfInfo.RepoURL = repo
				break
			}
		}
	}

	id, manifest, script := devmode.ConvertUnraidToScaffold(container, dfInfo)

	// Create the dev app
	appDir := s.devStore.AppDir(id)
	os.MkdirAll(filepath.Join(appDir, "provision"), 0755)
	os.WriteFile(filepath.Join(appDir, "app.yml"), []byte(manifest), 0644)
	os.WriteFile(filepath.Join(appDir, "provision", "install.py"), []byte(script), 0644)
	desc := container.Overview
	if desc == "" {
		desc = container.Description
	}
	desc = devmode.StripHTML(desc)
	if len(desc) > 300 {
		desc = desc[:300] + "..."
	}
	readme := devmode.GenerateReadme(container.Name, "Unraid template", desc, dfInfo, container.Project)
	os.WriteFile(filepath.Join(appDir, "README.md"), []byte(readme), 0644)

	// Download icon if available
	if container.Icon != "" && strings.HasPrefix(container.Icon, "http") {
		client := &http.Client{Timeout: 15 * time.Second}
		resp, err := client.Get(container.Icon)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				iconData, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20)) // 2MB limit
				if err == nil && len(iconData) > 0 {
					os.WriteFile(filepath.Join(appDir, "icon.png"), iconData, 0644)
				}
			}
		}
	}

	// Ensure default icon exists if download failed or no icon URL
	s.devStore.EnsureIcon(id)

	app, err := s.devStore.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, app)
}

// fetchAndParseDockerfile fetches a Dockerfile from GitHub and parses it.
// Also attempts to fetch the s6 run script. Returns nil on any failure.
func (s *Server) fetchAndParseDockerfile(urlTmpl, branch, appName string) *devmode.DockerfileInfo {
	client := &http.Client{Timeout: 10 * time.Second}

	// Try the given branch first, then fallback
	branches := []string{branch}
	if branch == "master" {
		branches = append(branches, "main")
	} else if branch == "main" {
		branches = append(branches, "master")
	}

	var dockerfileContent string
	var usedURL string
	for _, b := range branches {
		u := strings.Replace(urlTmpl, "{branch}", b, 1)
		resp, err := client.Get(u)
		if err != nil {
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024)) // 512KB limit
			if err == nil && len(body) > 0 {
				dockerfileContent = string(body)
				usedURL = u
				break
			}
		}
	}

	if dockerfileContent == "" {
		return nil
	}

	dfInfo := devmode.ParseDockerfile(dockerfileContent)

	// Try to fetch s6 run script
	s6URLTmpl := devmode.InferS6RunURL(usedURL, appName)
	if s6URLTmpl != "" {
		resp, err := client.Get(s6URLTmpl)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
				if len(body) > 0 {
					dfInfo.ExecCmd = devmode.ParseS6RunScript(string(body))
				}
			}
		}
	}

	return dfInfo
}

func (s *Server) handleDevImportDockerfile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string `json:"name"`
		Dockerfile string `json:"dockerfile"`
		URL        string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	dockerfileContent := req.Dockerfile

	// If URL provided, fetch the Dockerfile from it
	if dockerfileContent == "" && req.URL != "" {
		u := req.URL
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			writeError(w, http.StatusBadRequest, "url must start with http:// or https://")
			return
		}
		// If URL looks like a GitHub repo page, try to infer raw Dockerfile URL
		if strings.Contains(u, "github.com/") && !strings.Contains(u, "raw.githubusercontent.com") {
			if urlTmpl, branch := devmode.InferDockerfileURL("", u); urlTmpl != "" {
				// Try branches
				branches := []string{branch}
				if branch == "master" {
					branches = append(branches, "main")
				} else if branch == "main" {
					branches = append(branches, "master")
				}
				client := &http.Client{Timeout: 15 * time.Second}
				for _, b := range branches {
					rawURL := strings.Replace(urlTmpl, "{branch}", b, 1)
					resp, err := client.Get(rawURL)
					if err != nil {
						continue
					}
					defer resp.Body.Close()
					if resp.StatusCode == http.StatusOK {
						body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
						if err == nil && len(body) > 0 {
							dockerfileContent = string(body)
							break
						}
					}
				}
			}
		}
		// If still empty, fetch URL directly
		if dockerfileContent == "" {
			client := &http.Client{Timeout: 15 * time.Second}
			resp, err := client.Get(u)
			if err != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to fetch URL: %v", err))
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("URL returned status %d", resp.StatusCode))
				return
			}
			body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
			if err != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to read URL response: %v", err))
				return
			}
			dockerfileContent = string(body)
		}
	}

	if dockerfileContent == "" {
		writeError(w, http.StatusBadRequest, "dockerfile or url is required")
		return
	}

	// Parse the app's own Dockerfile (no parent chain resolution —
	// parent layers add Docker-specific packages that break LXC installs)
	dfInfo := devmode.ParseDockerfile(dockerfileContent)

	// Set RepoURL from the user-provided URL if it's a GitHub URL
	if req.URL != "" {
		if repo := devmode.ExtractGitHubRepoURL(req.URL); repo != "" {
			dfInfo.RepoURL = repo
		}
	}

	// Try to fetch s6 run script for exec command
	if dfInfo.ExecCmd == "" {
		if urlTmpl, _ := devmode.InferDockerfileURL(dfInfo.BaseImage, ""); urlTmpl != "" {
			s6URL := devmode.InferS6RunURL(urlTmpl, req.Name)
			if s6URL != "" {
				client := &http.Client{Timeout: 10 * time.Second}
				resp, err := client.Get(strings.Replace(s6URL, "{branch}", "master", 1))
				if err == nil {
					defer resp.Body.Close()
					if resp.StatusCode == http.StatusOK {
						body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
						if len(body) > 0 {
							dfInfo.ExecCmd = devmode.ParseS6RunScript(string(body))
						}
					}
				}
			}
		}
	}

	id, manifest, script := devmode.ConvertDockerfileToScaffold(req.Name, dfInfo)

	// Create the dev app
	appDir := s.devStore.AppDir(id)
	os.MkdirAll(filepath.Join(appDir, "provision"), 0755)
	os.WriteFile(filepath.Join(appDir, "app.yml"), []byte(manifest), 0644)
	os.WriteFile(filepath.Join(appDir, "provision", "install.py"), []byte(script), 0644)
	readme := devmode.GenerateReadme(req.Name, "Dockerfile", "", dfInfo, "")
	os.WriteFile(filepath.Join(appDir, "README.md"), []byte(readme), 0644)

	// Write default icon
	s.devStore.EnsureIcon(id)

	app, err := s.devStore.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, app)
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

	appDir := s.devStore.AppDir(id)
	os.WriteFile(filepath.Join(appDir, "icon.png"), iconData, 0644)
}

func (s *Server) handleDevListTemplates(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"templates": devmode.ListTemplates(),
	})
}

// httpDockerfileFetcher fetches Dockerfiles over HTTP, trying multiple branches.
type httpDockerfileFetcher struct {
	client *http.Client
}

func (f *httpDockerfileFetcher) FetchDockerfile(urlTmpl, branch string) (content, usedURL string, err error) {
	branches := []string{branch}
	if branch == "master" {
		branches = append(branches, "main")
	} else if branch == "main" {
		branches = append(branches, "master")
	}

	for _, b := range branches {
		u := strings.Replace(urlTmpl, "{branch}", b, 1)
		resp, err := f.client.Get(u)
		if err != nil {
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
			if err == nil && len(body) > 0 {
				return string(body), u, nil
			}
		}
	}
	return "", "", fmt.Errorf("could not fetch Dockerfile from %s", urlTmpl)
}

func (s *Server) handleDevImportDockerfileStream(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string `json:"name"`
		Dockerfile string `json:"dockerfile"`
		URL        string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Extend write deadline
	rc := http.NewResponseController(w)
	rc.SetWriteDeadline(time.Now().Add(2 * time.Minute))

	flusher, _ := w.(http.Flusher)

	writeSSE := func(event devmode.ChainEvent) {
		data, _ := json.Marshal(event)
		fmt.Fprintf(w, "data: %s\n\n", data)
		if flusher != nil {
			flusher.Flush()
		}
	}

	dockerfileContent := req.Dockerfile
	client := &http.Client{Timeout: 15 * time.Second}

	// If URL provided, fetch the Dockerfile
	if dockerfileContent == "" && req.URL != "" {
		u := req.URL
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			writeSSE(devmode.ChainEvent{Type: "error", Message: "URL must start with http:// or https://"})
			return
		}

		writeSSE(devmode.ChainEvent{Type: "fetching", Layer: 0, URL: u, Message: "Fetching initial Dockerfile..."})

		// If GitHub repo page, infer raw URL
		if strings.Contains(u, "github.com/") && !strings.Contains(u, "raw.githubusercontent.com") {
			if urlTmpl, branch := devmode.InferDockerfileURL("", u); urlTmpl != "" {
				fetcher := &httpDockerfileFetcher{client: client}
				content, _, err := fetcher.FetchDockerfile(urlTmpl, branch)
				if err == nil {
					dockerfileContent = content
				}
			}
		}

		// If still empty, fetch URL directly
		if dockerfileContent == "" {
			resp, err := client.Get(u)
			if err != nil {
				writeSSE(devmode.ChainEvent{Type: "error", Message: fmt.Sprintf("Failed to fetch URL: %v", err)})
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				writeSSE(devmode.ChainEvent{Type: "error", Message: fmt.Sprintf("URL returned status %d", resp.StatusCode)})
				return
			}
			body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
			if err != nil {
				writeSSE(devmode.ChainEvent{Type: "error", Message: fmt.Sprintf("Failed to read response: %v", err)})
				return
			}
			dockerfileContent = string(body)
		}
	}

	if dockerfileContent == "" {
		writeSSE(devmode.ChainEvent{Type: "error", Message: "No Dockerfile content provided"})
		return
	}

	// Parse the app's own Dockerfile (no parent chain resolution —
	// parent layers add Docker-specific packages that break LXC installs)
	dfInfo := devmode.ParseDockerfile(dockerfileContent)

	writeSSE(devmode.ChainEvent{
		Type:    "parsed",
		Layer:   0,
		Image:   dfInfo.BaseImage,
		Message: fmt.Sprintf("Parsed Dockerfile: base=%s, %d packages, %d ports, %d volumes", dfInfo.BaseImage, len(dfInfo.Packages), len(dfInfo.Ports), len(dfInfo.Volumes)),
	})

	// Set RepoURL from the user-provided URL if it's a GitHub URL
	if req.URL != "" {
		if repo := devmode.ExtractGitHubRepoURL(req.URL); repo != "" {
			dfInfo.RepoURL = repo
		}
	}

	// Also try to fetch s6 run script for exec command
	if dfInfo.ExecCmd == "" {
		if urlTmpl, _ := devmode.InferDockerfileURL(dfInfo.BaseImage, ""); urlTmpl != "" {
			s6URL := devmode.InferS6RunURL(urlTmpl, req.Name)
			if s6URL != "" {
				writeSSE(devmode.ChainEvent{Type: "fetching", Layer: 0, URL: s6URL, Message: "Looking for service run script..."})
				resp, err := client.Get(strings.Replace(s6URL, "{branch}", "master", 1))
				if err == nil {
					defer resp.Body.Close()
					if resp.StatusCode == http.StatusOK {
						body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
						if len(body) > 0 {
							dfInfo.ExecCmd = devmode.ParseS6RunScript(string(body))
						}
					}
				}
			}
		}
	}

	writeSSE(devmode.ChainEvent{Type: "merged", Layer: 0, Message: "Generating app scaffold..."})

	// Generate scaffold
	id, manifest, script := devmode.ConvertDockerfileToScaffold(req.Name, dfInfo)

	// Create the dev app on disk
	appDir := s.devStore.AppDir(id)
	os.MkdirAll(filepath.Join(appDir, "provision"), 0755)
	os.WriteFile(filepath.Join(appDir, "app.yml"), []byte(manifest), 0644)
	os.WriteFile(filepath.Join(appDir, "provision", "install.py"), []byte(script), 0644)
	readme := devmode.GenerateReadme(req.Name, "Dockerfile", "", dfInfo, "")
	os.WriteFile(filepath.Join(appDir, "README.md"), []byte(readme), 0644)
	s.devStore.EnsureIcon(id)

	// Emit complete event
	writeSSE(devmode.ChainEvent{
		Type:    "complete",
		AppID:   id,
		Message: fmt.Sprintf("App %q created with %d packages", req.Name, len(dfInfo.Packages)),
	})
}
