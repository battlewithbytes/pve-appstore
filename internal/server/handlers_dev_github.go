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

	"github.com/battlewithbytes/pve-appstore/internal/catalog"
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

	// Auto-fork catalog repo (one-time, idempotent)
	resp := map[string]interface{}{
		"status": "connected",
		"user":   user,
	}
	if forkResult, err := s.ensureCatalogFork(token); err != nil {
		log.Printf("[github] auto-fork on connect failed (non-fatal): %v", err)
	} else {
		resp["fork"] = forkResult
	}

	writeJSON(w, http.StatusOK, resp)
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

// --- Repository info endpoint ---

func (s *Server) handleDevGitHubRepoInfo(w http.ResponseWriter, r *http.Request) {
	if s.githubStore == nil {
		writeError(w, http.StatusServiceUnavailable, "store not available")
		return
	}

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
		writeError(w, http.StatusInternalServerError, "failed to decrypt token")
		return
	}

	// Get fork info from cache
	var fork gh.ForkResult
	if forkJSON, _ := s.githubStore.GetGitHubState("github_fork"); forkJSON != "" {
		json.Unmarshal([]byte(forkJSON), &fork)
	}
	if fork.FullName == "" {
		writeError(w, http.StatusBadRequest, "no fork info available — reconnect GitHub")
		return
	}

	// Parse upstream catalog URL
	_, catalogRepo, err := config.ParseGitHubRepo(s.cfg.Catalog.URL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot parse catalog URL")
		return
	}

	// Build upstream info
	upstream := map[string]string{
		"url":    s.cfg.Catalog.URL,
		"branch": s.cfg.Catalog.Branch,
	}
	if upstream["branch"] == "" {
		upstream["branch"] = "main"
	}

	// Build fork info
	forkURL := "https://github.com/" + fork.FullName
	forkInfo := map[string]string{
		"full_name": fork.FullName,
		"url":       forkURL,
		"clone_url": fork.CloneURL,
	}

	// Fetch branches from fork
	client := gh.NewClient(token)
	allBranches, err := client.ListBranches(fork.Owner, catalogRepo)
	if err != nil {
		log.Printf("[github] ListBranches failed: %v", err)
		// Non-fatal: return empty branches
		allBranches = nil
	}

	// Build dev app/stack metadata lookup: github_branch → (pr_url, pr_state)
	type branchMeta struct {
		prURL string
		prState string
	}
	metaByBranch := make(map[string]branchMeta)

	if apps, err := s.devSvc.List(); err == nil {
		for _, a := range apps {
			if a.GitHubBranch != "" {
				m := branchMeta{prURL: a.GitHubPRURL}
				if a.GitHubPRURL != "" {
					m.prState = s.getPRState(a.GitHubPRURL)
				}
				metaByBranch[a.GitHubBranch] = m
			}
		}
	}
	if stacks, err := s.devSvc.ListStacks(); err == nil {
		for _, st := range stacks {
			if st.GitHubBranch != "" {
				m := branchMeta{prURL: st.GitHubPRURL}
				if st.GitHubPRURL != "" {
					m.prState = s.getPRState(st.GitHubPRURL)
				}
				metaByBranch[st.GitHubBranch] = m
			}
		}
	}

	// Filter to app/* and stack/* branches and enrich with PR info
	type branchEntry struct {
		Name    string `json:"name"`
		AppID   string `json:"app_id"`
		PRURL   string `json:"pr_url"`
		PRState string `json:"pr_state"`
	}
	var branches []branchEntry
	for _, b := range allBranches {
		var appID string
		if strings.HasPrefix(b.Name, "app/") {
			appID = strings.TrimPrefix(b.Name, "app/")
		} else if strings.HasPrefix(b.Name, "stack/") {
			appID = strings.TrimPrefix(b.Name, "stack/")
		} else {
			continue
		}
		entry := branchEntry{Name: b.Name, AppID: appID}
		if m, ok := metaByBranch[b.Name]; ok {
			entry.PRURL = m.prURL
			entry.PRState = m.prState
		}
		branches = append(branches, entry)
	}

	// Local paths
	local := map[string]string{
		"catalog_path":  filepath.Join(config.DefaultDataDir, "catalog"),
		"dev_apps_path": filepath.Join(config.DefaultDataDir, "dev-apps"),
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"upstream": upstream,
		"fork":     forkInfo,
		"branches": branches,
		"local":    local,
	})
}

// --- Branch management ---

func (s *Server) handleDevGitHubDeleteBranch(w http.ResponseWriter, r *http.Request) {
	if s.githubStore == nil {
		writeError(w, http.StatusServiceUnavailable, "store not available")
		return
	}

	var req struct {
		Branch string `json:"branch"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	branch := strings.TrimSpace(req.Branch)
	if branch == "" || (!strings.HasPrefix(branch, "app/") && !strings.HasPrefix(branch, "stack/")) {
		writeError(w, http.StatusBadRequest, "branch must start with app/ or stack/")
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
		writeError(w, http.StatusInternalServerError, "failed to decrypt token")
		return
	}

	var fork gh.ForkResult
	if forkJSON, _ := s.githubStore.GetGitHubState("github_fork"); forkJSON != "" {
		json.Unmarshal([]byte(forkJSON), &fork)
	}
	if fork.Owner == "" {
		writeError(w, http.StatusBadRequest, "no fork info available")
		return
	}

	_, catalogRepo, err := config.ParseGitHubRepo(s.cfg.Catalog.URL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot parse catalog URL")
		return
	}

	client := gh.NewClient(token)
	if err := client.DeleteBranch(fork.Owner, catalogRepo, branch); err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("failed to delete branch: %v", err))
		return
	}

	log.Printf("[github] deleted branch %s from %s/%s", branch, fork.Owner, catalogRepo)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "branch": branch})
}

// --- Publish endpoints ---

func (s *Server) handleDevPublishStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	checks := map[string]bool{
		"github_connected":  false,
		"validation_passed": false,
		"test_installed":    false,
	}

	// GitHub connected?
	if s.githubStore != nil {
		if tokenEnc, _ := s.githubStore.GetGitHubState("github_token"); tokenEnc != "" {
			checks["github_connected"] = true
		}
	}

	// Validation passed?
	appDir := s.devSvc.AppDir(id)
	if appDir != "" {
		if m, err := s.devSvc.ParseManifest(id); err == nil && m != nil {
			checks["validation_passed"] = true
		}
	}

	// Test install exists? Only dev-sourced installs count.
	if s.engineInstallSvc != nil {
		if _, exists := s.engineInstallSvc.HasActiveDevInstallForApp(id); exists {
			checks["test_installed"] = true
		}
	}

	ready := checks["github_connected"] && checks["validation_passed"] &&
		checks["test_installed"]

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

	// Auto-refresh catalog when PR is merged so the app appears immediately
	if prState == "pr_merged" {
		if meta, err := s.devSvc.Get(id); err == nil && meta != nil && (meta.Status == "published" || meta.Status == "deployed") {
			s.tryRefreshInBackground("PR merged for app " + id)
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

	// Check test install — only dev-sourced installs count
	if s.engineInstallSvc != nil {
		if _, exists := s.engineInstallSvc.HasActiveDevInstallForApp(id); !exists {
			writeError(w, http.StatusBadRequest, "app must have a successful test install of the dev version before publishing")
			return
		}
	}

	// Parse catalog repo
	catalogOwner, catalogRepo, err := config.ParseGitHubRepo(s.cfg.Catalog.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("cannot parse catalog URL: %v", err))
		return
	}

	// Determine PR title
	title := fmt.Sprintf("Add %s v%s", manifest.Name, manifest.Version)
	if s.catalogSvc != nil {
		if existing, ok := s.catalogSvc.GetApp(id); ok && existing.Source != "developer" {
			title = fmt.Sprintf("Update %s to v%s", manifest.Name, manifest.Version)
		}
	}

	result, err := s.publishToGitHub(token, catalogOwner, catalogRepo, publishParams{
		ID:         id,
		BranchName: "app/" + id,
		DirPrefix:  "apps/" + id,
		LocalDir:   s.devSvc.AppDir(id),
		Title:      title,
		Body:       buildAppPRBody(manifest),
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	// Store PR info in devstatus
	s.devSvc.SetGitHubMeta(id, map[string]string{
		"status":           "published",
		"github_branch":    "app/" + id,
		"github_pr_url":    result.PRURL,
		"github_pr_number": strconv.Itoa(result.PRNumber),
	})

	log.Printf("[github] PR %s: %s", result.Action, result.PRURL)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"pr_url":    result.PRURL,
		"pr_number": result.PRNumber,
		"action":    result.Action,
	})
}

// ensureCatalogFork forks the catalog repo if not already forked.
// Called on GitHub connect and during publish.
func (s *Server) ensureCatalogFork(token string) (*gh.ForkResult, error) {
	// Check if fork info already cached
	if s.githubStore != nil {
		if forkJSON, _ := s.githubStore.GetGitHubState("github_fork"); forkJSON != "" {
			var cached gh.ForkResult
			if json.Unmarshal([]byte(forkJSON), &cached) == nil && cached.FullName != "" {
				return &cached, nil
			}
		}
	}

	catalogOwner, catalogRepo, err := config.ParseGitHubRepo(s.cfg.Catalog.URL)
	if err != nil {
		return nil, fmt.Errorf("cannot parse catalog URL: %w", err)
	}

	client := gh.NewClient(token)
	forkResult, err := client.ForkRepo(catalogOwner, catalogRepo)
	if err != nil {
		return nil, fmt.Errorf("fork failed: %w", err)
	}

	// Store fork info
	forkJSON, _ := json.Marshal(forkResult)
	if s.githubStore != nil {
		s.githubStore.SetGitHubState("github_fork", string(forkJSON))
	}

	log.Printf("[github] forked catalog repo to %s", forkResult.FullName)
	return forkResult, nil
}

// publishParams holds the parameters for publishToGitHub.
type publishParams struct {
	ID         string // app or stack ID
	BranchName string // e.g. "app/{id}" or "stack/{id}"
	DirPrefix  string // e.g. "apps/{id}" or "stacks/{id}"
	LocalDir   string // filesystem path to push
	Title      string // PR title
	Body       string // PR body markdown
}

// publishResult holds the result of publishToGitHub.
type publishResult struct {
	PRURL    string `json:"pr_url"`
	PRNumber int    `json:"pr_number"`
	Action   string `json:"action"` // "created" or "updated"
}

// publishToGitHub implements the fork→sync→branch→push→PR flow, reusing open PRs.
func (s *Server) publishToGitHub(token, catalogOwner, catalogRepo string, p publishParams) (*publishResult, error) {
	client := gh.NewClient(token)

	// 1. Ensure fork exists (uses cache from connect, or forks now)
	forkResult, err := s.ensureCatalogFork(token)
	if err != nil {
		return nil, fmt.Errorf("failed to fork catalog repo: %w", err)
	}

	// 2. Sync fork with upstream
	syncErr := client.SyncFork(forkResult.Owner, catalogRepo, "main")
	if syncErr != nil {
		// Fork may have been deleted externally — clear cache and re-fork once
		log.Printf("[github] sync fork failed (%v), re-forking...", syncErr)
		if s.githubStore != nil {
			s.githubStore.DeleteGitHubState("github_fork")
		}
		forkResult, err = client.ForkRepo(catalogOwner, catalogRepo)
		if err != nil {
			return nil, fmt.Errorf("failed to re-fork catalog repo: %w", err)
		}
		forkJSON, _ := json.Marshal(forkResult)
		if s.githubStore != nil {
			s.githubStore.SetGitHubState("github_fork", string(forkJSON))
		}
		// Don't retry sync — fresh fork is already current
	}

	// 3. Get synced default branch SHA
	baseSHA, defaultBranch, err := client.GetDefaultBranchSHA(forkResult.Owner, catalogRepo)
	if err != nil {
		return nil, fmt.Errorf("failed to get branch SHA: %w", err)
	}

	// 4. Check for existing open PR
	head := fmt.Sprintf("%s:%s", forkResult.Owner, p.BranchName)
	existingPR, err := client.FindOpenPR(catalogOwner, catalogRepo, head)
	if err != nil {
		log.Printf("[github] FindOpenPR failed (non-fatal): %v", err)
	}

	if existingPR != nil {
		// Update existing PR: push files to existing branch, update PR metadata
		if err := pushDirToGitHub(client, forkResult.Owner, catalogRepo, p.BranchName, p.LocalDir, p.DirPrefix); err != nil {
			return nil, fmt.Errorf("failed to push files: %w", err)
		}
		if err := client.UpdatePullRequest(catalogOwner, catalogRepo, existingPR.Number, p.Title, p.Body); err != nil {
			return nil, fmt.Errorf("failed to update PR: %w", err)
		}
		log.Printf("[github] PR updated: %s", existingPR.HTMLURL)
		return &publishResult{
			PRURL:    existingPR.HTMLURL,
			PRNumber: existingPR.Number,
			Action:   "updated",
		}, nil
	}

	// 5. No open PR — clean up stale branch, create fresh one
	_ = client.DeleteBranch(forkResult.Owner, catalogRepo, p.BranchName) // idempotent
	if err := client.CreateBranch(forkResult.Owner, catalogRepo, p.BranchName, baseSHA); err != nil {
		return nil, fmt.Errorf("failed to create branch: %w", err)
	}

	// 6. Push files
	if err := pushDirToGitHub(client, forkResult.Owner, catalogRepo, p.BranchName, p.LocalDir, p.DirPrefix); err != nil {
		return nil, fmt.Errorf("failed to push files: %w", err)
	}

	// 7. Create PR
	pr, err := client.CreatePullRequest(catalogOwner, catalogRepo, p.Title, p.Body, head, defaultBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to create PR: %w", err)
	}

	return &publishResult{
		PRURL:    pr.HTMLURL,
		PRNumber: pr.Number,
		Action:   "created",
	}, nil
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

// buildAppPRBody generates a structured PR body for an app submission.
func buildAppPRBody(m *catalog.AppManifest) string {
	var b strings.Builder

	b.WriteString("## Summary\n\n")
	b.WriteString("| Field | Value |\n")
	b.WriteString("|-------|-------|\n")
	b.WriteString(fmt.Sprintf("| App ID | `%s` |\n", m.ID))
	b.WriteString(fmt.Sprintf("| Version | `%s` |\n", m.Version))
	b.WriteString(fmt.Sprintf("| Category | %s |\n", strings.Join(m.Categories, ", ")))
	b.WriteString(fmt.Sprintf("| OS Template | `%s` |\n", m.LXC.OSTemplate))
	b.WriteString(fmt.Sprintf("| Resources | %d cores, %dMB RAM, %dGB disk |\n",
		m.LXC.Defaults.Cores, m.LXC.Defaults.MemoryMB, m.LXC.Defaults.DiskGB))

	b.WriteString("\n## Description\n\n")
	b.WriteString(m.Description)
	b.WriteString("\n")

	b.WriteString("\n## Permissions\n\n")
	b.WriteString(fmt.Sprintf("- **Packages:** %s\n", listOrNone(m.Permissions.Packages)))
	b.WriteString(fmt.Sprintf("- **Services:** %s\n", listOrNone(m.Permissions.Services)))
	b.WriteString(fmt.Sprintf("- **Paths:** %s\n", listOrNone(m.Permissions.Paths)))
	b.WriteString(fmt.Sprintf("- **URLs:** %s\n", listOrNone(m.Permissions.URLs)))

	b.WriteString("\n## Reviewer Checklist\n\n")
	b.WriteString("- [ ] Manifest fields are complete and accurate\n")
	b.WriteString("- [ ] Install script uses SDK methods (no raw subprocess/os.system)\n")
	b.WriteString("- [ ] Permissions section matches actual script usage\n")
	b.WriteString("- [ ] Version follows semver (X.Y.Z)\n")
	b.WriteString("- [ ] Description is meaningful (not TODO/placeholder)\n")

	b.WriteString("\n---\n*Submitted via [PVE App Store](https://github.com/battlewithbytes/pve-appstore) Developer Mode*\n")

	return b.String()
}

// buildStackPRBody generates a structured PR body for a stack submission.
func buildStackPRBody(sm *catalog.StackManifest) string {
	var b strings.Builder

	b.WriteString("## Summary\n\n")
	b.WriteString("| Field | Value |\n")
	b.WriteString("|-------|-------|\n")
	b.WriteString(fmt.Sprintf("| Stack ID | `%s` |\n", sm.ID))
	b.WriteString(fmt.Sprintf("| Version | `%s` |\n", sm.Version))
	b.WriteString(fmt.Sprintf("| Category | %s |\n", strings.Join(sm.Categories, ", ")))
	if sm.LXC.OSTemplate != "" {
		b.WriteString(fmt.Sprintf("| OS Template | `%s` |\n", sm.LXC.OSTemplate))
		b.WriteString(fmt.Sprintf("| Resources | %d cores, %dMB RAM, %dGB disk |\n",
			sm.LXC.Defaults.Cores, sm.LXC.Defaults.MemoryMB, sm.LXC.Defaults.DiskGB))
	}

	b.WriteString("\n## Description\n\n")
	b.WriteString(sm.Description)
	b.WriteString("\n")

	b.WriteString("\n## Apps\n\n")
	for _, sa := range sm.Apps {
		b.WriteString(fmt.Sprintf("- `%s`\n", sa.AppID))
	}

	b.WriteString("\n## Reviewer Checklist\n\n")
	b.WriteString("- [ ] All referenced apps exist in the catalog\n")
	b.WriteString("- [ ] Stack metadata is complete and accurate\n")
	b.WriteString("- [ ] Version follows semver (X.Y.Z)\n")
	b.WriteString("- [ ] Description is meaningful (not TODO/placeholder)\n")

	b.WriteString("\n---\n*Submitted via [PVE App Store](https://github.com/battlewithbytes/pve-appstore) Developer Mode*\n")

	return b.String()
}

// listOrNone formats a string slice as a comma-separated list, or "none declared".
func listOrNone(items []string) string {
	if len(items) == 0 {
		return "none declared"
	}
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = "`" + item + "`"
	}
	return strings.Join(quoted, ", ")
}
