package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/battlewithbytes/pve-appstore/internal/config"
	gh "github.com/battlewithbytes/pve-appstore/internal/github"
)

// --- GitHub PAT endpoints ---

func (s *Server) handleDevGitHubStatus(w http.ResponseWriter, r *http.Request) {
	if s.githubStore == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"connected": false})
		return
	}

	resp := map[string]interface{}{"connected": false}

	tokenEnc, _ := s.githubStore.GetGitHubState("github_token")
	if tokenEnc == "" {
		writeJSON(w, http.StatusOK, resp)
		return
	}

	resp["connected"] = true

	if userJSON, _ := s.githubStore.GetGitHubState("github_user"); userJSON != "" {
		var user gh.GitHubUser
		if json.Unmarshal([]byte(userJSON), &user) == nil {
			resp["user"] = user
		}
	}

	if forkJSON, _ := s.githubStore.GetGitHubState("github_fork"); forkJSON != "" {
		var fork gh.ForkResult
		if json.Unmarshal([]byte(forkJSON), &fork) == nil {
			resp["fork"] = fork
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleDevGitHubConnect(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	token := strings.TrimSpace(req.Token)
	if token == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}

	// Validate token format
	if !strings.HasPrefix(token, "ghp_") && !strings.HasPrefix(token, "github_pat_") {
		writeError(w, http.StatusBadRequest, "token must start with ghp_ or github_pat_")
		return
	}

	// Validate token by calling GitHub /user API
	client := gh.NewClient(token)
	user, err := client.User()
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid token: %v", err))
		return
	}

	// Encrypt and store token
	hmacSecret := s.cfg.Auth.HMACSecret
	if hmacSecret == "" {
		hmacSecret = "pve-appstore-default-key"
	}

	encrypted, err := gh.EncryptToken(token, hmacSecret)
	if err != nil {
		log.Printf("[github] token encryption failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to store token")
		return
	}

	if s.githubStore == nil {
		writeError(w, http.StatusInternalServerError, "store not available")
		return
	}

	if err := s.githubStore.SetGitHubState("github_token", encrypted); err != nil {
		log.Printf("[github] store token failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to store token")
		return
	}

	// Store user info
	userJSON, _ := json.Marshal(user)
	s.githubStore.SetGitHubState("github_user", string(userJSON))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "connected",
		"user":   user,
	})
}

func (s *Server) handleDevGitHubDisconnect(w http.ResponseWriter, r *http.Request) {
	if s.githubStore == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	s.githubStore.DeleteGitHubState("github_token")
	s.githubStore.DeleteGitHubState("github_user")
	s.githubStore.DeleteGitHubState("github_fork")

	writeJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
}

// --- Publish endpoints ---

func (s *Server) handleDevPublishStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	checks := map[string]bool{
		"github_connected":  false,
		"validation_passed": false,
		"test_installed":    false,
		"fork_exists":       false,
	}

	// GitHub connected?
	if s.githubStore != nil {
		if tokenEnc, _ := s.githubStore.GetGitHubState("github_token"); tokenEnc != "" {
			checks["github_connected"] = true
		}
		if forkJSON, _ := s.githubStore.GetGitHubState("github_fork"); forkJSON != "" {
			checks["fork_exists"] = true
		}
	}

	// Validation passed?
	appDir := s.devSvc.AppDir(id)
	if appDir != "" {
		if m, err := s.devSvc.ParseManifest(id); err == nil && m != nil {
			checks["validation_passed"] = true
		}
	}

	// Test install exists?
	if s.engineInstallSvc != nil {
		if _, exists := s.engineInstallSvc.HasActiveInstallForApp(id); exists {
			checks["test_installed"] = true
		}
	}

	ready := checks["github_connected"] && checks["validation_passed"] &&
		checks["test_installed"] && checks["fork_exists"]

	// Check if already published
	published := false
	prURL := ""
	prState := ""
	if meta, err := s.devSvc.Get(id); err == nil && meta != nil {
		if meta.GitHubPRURL != "" {
			published = true
			prURL = meta.GitHubPRURL
			prState = s.getPRState(prURL)
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

func (s *Server) handleDevPublish(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if s.githubStore == nil {
		writeError(w, http.StatusServiceUnavailable, "store not available")
		return
	}

	// Check GitHub connected
	tokenEnc, _ := s.githubStore.GetGitHubState("github_token")
	if tokenEnc == "" {
		writeError(w, http.StatusBadRequest, "GitHub not connected")
		return
	}

	// Decrypt token
	hmacSecret := s.cfg.Auth.HMACSecret
	if hmacSecret == "" {
		hmacSecret = "pve-appstore-default-key"
	}
	token, err := gh.DecryptToken(tokenEnc, hmacSecret)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to decrypt GitHub token")
		return
	}

	// Validate the app
	manifest, err := s.devSvc.ParseManifest(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("manifest validation failed: %v", err))
		return
	}

	// Check test install
	if s.engineInstallSvc != nil {
		if _, exists := s.engineInstallSvc.HasActiveInstallForApp(id); !exists {
			writeError(w, http.StatusBadRequest, "app must have a successful test install before publishing")
			return
		}
	}

	// Parse catalog repo
	catalogOwner, catalogRepo, err := config.ParseGitHubRepo(s.cfg.Catalog.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("cannot parse catalog URL: %v", err))
		return
	}

	client := gh.NewClient(token)

	// Fork the catalog repo (idempotent)
	forkResult, err := client.ForkRepo(catalogOwner, catalogRepo)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("failed to fork catalog repo: %v", err))
		return
	}

	// Store fork info
	forkJSON, _ := json.Marshal(forkResult)
	s.githubStore.SetGitHubState("github_fork", string(forkJSON))

	// Get default branch SHA from the fork
	baseSHA, defaultBranch, err := client.GetDefaultBranchSHA(forkResult.Owner, catalogRepo)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("failed to get branch SHA: %v", err))
		return
	}

	// Create branch on fork
	branchName := "app/" + id
	if err := client.CreateBranch(forkResult.Owner, catalogRepo, branchName, baseSHA); err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("failed to create branch: %v", err))
		return
	}

	// Push app files to the fork branch
	appDir := s.devSvc.AppDir(id)
	prefix := "apps/" + id
	if err := pushDirToGitHub(client, forkResult.Owner, catalogRepo, branchName, appDir, prefix); err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("failed to push files: %v", err))
		return
	}

	// Create PR
	title := fmt.Sprintf("Add %s v%s", manifest.Name, manifest.Version)
	body := fmt.Sprintf("## New App: %s\n\n%s\n\n- Version: %s\n- Category: %s\n\nSubmitted via PVE App Store Developer Mode.",
		manifest.Name, manifest.Description, manifest.Version,
		strings.Join(manifest.Categories, ", "))

	head := fmt.Sprintf("%s:%s", forkResult.Owner, branchName)
	pr, err := client.CreatePullRequest(catalogOwner, catalogRepo, title, body, head, defaultBranch)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("failed to create PR: %v", err))
		return
	}

	// Store PR URL in devstatus
	s.devSvc.SetGitHubMeta(id, map[string]string{
		"status":        "published",
		"github_branch": branchName,
		"github_pr_url": pr.HTMLURL,
	})

	log.Printf("[github] PR created: %s", pr.HTMLURL)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"pr_url":    pr.HTMLURL,
		"pr_number": pr.Number,
	})
}

// tryGitHubFork attempts to fork the catalog repo on GitHub if the user is connected.
// Returns fork info on success, nil if not connected or on failure.
func (s *Server) tryGitHubFork(appID string) map[string]string {
	if s.githubStore == nil {
		return nil
	}

	tokenEnc, _ := s.githubStore.GetGitHubState("github_token")
	if tokenEnc == "" {
		return nil
	}

	hmacSecret := s.cfg.Auth.HMACSecret
	if hmacSecret == "" {
		hmacSecret = "pve-appstore-default-key"
	}

	token, err := gh.DecryptToken(tokenEnc, hmacSecret)
	if err != nil {
		log.Printf("[github] failed to decrypt token for fork: %v", err)
		return nil
	}

	catalogOwner, catalogRepo, err := config.ParseGitHubRepo(s.cfg.Catalog.URL)
	if err != nil {
		log.Printf("[github] cannot parse catalog URL for fork: %v", err)
		return nil
	}

	client := gh.NewClient(token)
	forkResult, err := client.ForkRepo(catalogOwner, catalogRepo)
	if err != nil {
		log.Printf("[github] fork failed: %v", err)
		return nil
	}

	// Store fork info
	forkJSON, _ := json.Marshal(forkResult)
	s.githubStore.SetGitHubState("github_fork", string(forkJSON))

	// Set the branch name in devstatus
	branchName := "app/" + appID
	s.devSvc.SetGitHubMeta(appID, map[string]string{
		"github_branch": branchName,
	})

	log.Printf("[github] forked catalog repo to %s", forkResult.FullName)
	return map[string]string{
		"full_name": forkResult.FullName,
		"branch":    branchName,
	}
}

// pushDirToGitHub walks a local directory and pushes each file to a GitHub repo via the Contents API.
func pushDirToGitHub(client *gh.Client, owner, repo, branch, localDir, remotePrefix string) error {
	return filepath.Walk(localDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Skip dot dirs
			if strings.HasPrefix(info.Name(), ".") && path != localDir {
				return filepath.SkipDir
			}
			return nil
		}
		// Skip dot files
		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}
		// Skip __pycache__ and .pyc
		if info.Name() == "__pycache__" || strings.HasSuffix(info.Name(), ".pyc") {
			return nil
		}

		relPath, _ := filepath.Rel(localDir, path)
		remotePath := remotePrefix + "/" + filepath.ToSlash(relPath)

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		// Check if file already exists (for update)
		sha, _ := client.GetFileSHA(owner, repo, remotePath, branch)

		msg := fmt.Sprintf("Add %s", remotePath)
		if sha != "" {
			msg = fmt.Sprintf("Update %s", remotePath)
		}

		return client.CreateOrUpdateFile(owner, repo, remotePath, branch, content, msg, sha)
	})
}

// prURLRe matches GitHub PR URLs like https://github.com/owner/repo/pull/123
var prURLRe = regexp.MustCompile(`github\.com/([^/]+)/([^/]+)/pull/(\d+)`)

// getPRState polls the GitHub API to get the current state of a PR from its URL.
// Returns "pr_open", "pr_merged", "pr_closed", or "" on error.
func (s *Server) getPRState(prURL string) string {
	if prURL == "" || s.githubStore == nil {
		return ""
	}

	m := prURLRe.FindStringSubmatch(prURL)
	if m == nil {
		return ""
	}
	owner, repo := m[1], m[2]
	number, err := strconv.Atoi(m[3])
	if err != nil {
		return ""
	}

	tokenEnc, _ := s.githubStore.GetGitHubState("github_token")
	if tokenEnc == "" {
		return ""
	}

	hmacSecret := s.cfg.Auth.HMACSecret
	if hmacSecret == "" {
		hmacSecret = "pve-appstore-default-key"
	}
	token, err := gh.DecryptToken(tokenEnc, hmacSecret)
	if err != nil {
		return ""
	}

	client := gh.NewClient(token)
	state, err := client.GetPRState(owner, repo, number)
	if err != nil {
		log.Printf("[github] failed to get PR state for %s: %v", prURL, err)
		return ""
	}
	return state
}
