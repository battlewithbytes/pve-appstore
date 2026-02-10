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
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
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

	id, manifest, script := devmode.ConvertUnraidToScaffold(container, dfInfo)

	// Create the dev app
	appDir := s.devStore.AppDir(id)
	os.MkdirAll(filepath.Join(appDir, "provision"), 0755)
	os.WriteFile(filepath.Join(appDir, "app.yml"), []byte(manifest), 0644)
	os.WriteFile(filepath.Join(appDir, "provision", "install.py"), []byte(script), 0644)
	os.WriteFile(filepath.Join(appDir, "README.md"), []byte(fmt.Sprintf("# %s\n\nImported from Unraid template.\n", container.Name)), 0644)

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

	dfInfo := devmode.ParseDockerfile(dockerfileContent)
	id, manifest, script := devmode.ConvertDockerfileToScaffold(req.Name, dfInfo)

	// Create the dev app
	appDir := s.devStore.AppDir(id)
	os.MkdirAll(filepath.Join(appDir, "provision"), 0755)
	os.WriteFile(filepath.Join(appDir, "app.yml"), []byte(manifest), 0644)
	os.WriteFile(filepath.Join(appDir, "provision", "install.py"), []byte(script), 0644)
	os.WriteFile(filepath.Join(appDir, "README.md"), []byte(fmt.Sprintf("# %s\n\nImported from Dockerfile.\n", req.Name)), 0644)

	app, err := s.devStore.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, app)
}

func (s *Server) handleDevListTemplates(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"templates": devmode.ListTemplates(),
	})
}
