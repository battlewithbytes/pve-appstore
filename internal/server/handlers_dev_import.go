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
)

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
	appDir := s.devSvc.AppDir(id)
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
	s.devSvc.EnsureIcon(id)

	app, err := s.devSvc.Get(id)
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
	appDir := s.devSvc.AppDir(id)
	os.MkdirAll(filepath.Join(appDir, "provision"), 0755)
	os.WriteFile(filepath.Join(appDir, "app.yml"), []byte(manifest), 0644)
	os.WriteFile(filepath.Join(appDir, "provision", "install.py"), []byte(script), 0644)
	readme := devmode.GenerateReadme(req.Name, "Dockerfile", "", dfInfo, "")
	os.WriteFile(filepath.Join(appDir, "README.md"), []byte(readme), 0644)

	// Write default icon
	s.devSvc.EnsureIcon(id)

	app, err := s.devSvc.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, app)
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
	appDir := s.devSvc.AppDir(id)
	os.MkdirAll(filepath.Join(appDir, "provision"), 0755)
	os.WriteFile(filepath.Join(appDir, "app.yml"), []byte(manifest), 0644)
	os.WriteFile(filepath.Join(appDir, "provision", "install.py"), []byte(script), 0644)
	readme := devmode.GenerateReadme(req.Name, "Dockerfile", "", dfInfo, "")
	os.WriteFile(filepath.Join(appDir, "README.md"), []byte(readme), 0644)
	s.devSvc.EnsureIcon(id)

	// Emit complete event
	writeSSE(devmode.ChainEvent{
		Type:    "complete",
		AppID:   id,
		Message: fmt.Sprintf("App %q created with %d packages", req.Name, len(dfInfo.Packages)),
	})
}
