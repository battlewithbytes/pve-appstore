package catalog

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Catalog manages the local cache of the app catalog.
type Catalog struct {
	mu          sync.RWMutex
	repoURL     string
	branch      string
	localDir    string
	apps        map[string]*AppManifest
	stacks      map[string]*StackManifest
	shadowed    map[string]*AppManifest // original apps displaced by dev apps
	lastRefresh time.Time
}

// New creates a new Catalog instance.
func New(repoURL, branch, dataDir string) *Catalog {
	return &Catalog{
		repoURL:  repoURL,
		branch:   branch,
		localDir: filepath.Join(dataDir, "catalog"),
		apps:     make(map[string]*AppManifest),
		stacks:   make(map[string]*StackManifest),
		shadowed: make(map[string]*AppManifest),
	}
}

// Refresh clones or pulls the catalog repo and re-indexes all apps.
// Developer apps are preserved across refreshes.
func (c *Catalog) Refresh() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Snapshot dev apps, stacks, and shadowed originals before re-indexing
	savedApps := c.devApps()
	savedStacks := c.devStacks()
	savedShadowed := make(map[string]*AppManifest, len(c.shadowed))
	for k, v := range c.shadowed {
		savedShadowed[k] = v
	}

	if err := c.fetchRepo(); err != nil {
		return fmt.Errorf("fetching catalog: %w", err)
	}

	if err := c.indexApps(); err != nil {
		return fmt.Errorf("indexing catalog: %w", err)
	}

	c.indexStacks()

	// Restore shadowed map first — re-index may have updated the official app
	c.shadowed = make(map[string]*AppManifest)
	for id := range savedShadowed {
		// If the re-indexed catalog has a fresh version, shadow that instead
		if fresh, ok := c.apps[id]; ok {
			c.shadowed[id] = fresh
		} else {
			c.shadowed[id] = savedShadowed[id]
		}
	}

	// Restore dev apps and stacks (overwrite re-indexed entries)
	for _, app := range savedApps {
		c.apps[app.ID] = app
	}
	for _, stack := range savedStacks {
		c.stacks[stack.ID] = stack
	}

	c.lastRefresh = time.Now()
	return nil
}

// LoadLocal loads apps from a local directory (no git). Useful for testing
// or when the catalog is already on disk.
func (c *Catalog) LoadLocal(dir string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolving catalog path: %w", err)
	}
	c.localDir = abs
	if err := c.indexApps(); err != nil {
		return err
	}
	c.indexStacks()
	c.lastRefresh = time.Now()
	return nil
}

// List returns all validated apps.
func (c *Catalog) List() []*AppManifest {
	c.mu.RLock()
	defer c.mu.RUnlock()

	apps := make([]*AppManifest, 0, len(c.apps))
	for _, app := range c.apps {
		apps = append(apps, app)
	}
	return apps
}

// Get returns a single app by ID.
func (c *Catalog) Get(id string) (*AppManifest, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	app, ok := c.apps[id]
	return app, ok
}

// Search returns apps matching the query string against name, description, tags, and categories.
func (c *Catalog) Search(query string) []*AppManifest {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if query == "" {
		return c.List()
	}

	q := strings.ToLower(query)
	var results []*AppManifest

	for _, app := range c.apps {
		if matches(app, q) {
			results = append(results, app)
		}
	}
	return results
}

// Filter returns apps matching the given category.
func (c *Catalog) Filter(category string) []*AppManifest {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if category == "" {
		return c.List()
	}

	cat := strings.ToLower(category)
	var results []*AppManifest

	for _, app := range c.apps {
		for _, c := range app.Categories {
			if strings.ToLower(c) == cat {
				results = append(results, app)
				break
			}
		}
	}
	return results
}

// Categories returns a deduplicated list of all categories across apps.
func (c *Catalog) Categories() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	seen := make(map[string]bool)
	var cats []string
	for _, app := range c.apps {
		for _, cat := range app.Categories {
			lower := strings.ToLower(cat)
			if !seen[lower] {
				seen[lower] = true
				cats = append(cats, cat)
			}
		}
	}
	return cats
}

// AppCount returns the number of loaded apps.
func (c *Catalog) AppCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.apps)
}

// LastRefresh returns the time of the most recent successful refresh or load.
func (c *Catalog) LastRefresh() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastRefresh
}

// IsStale checks whether the remote catalog has new commits by comparing
// the local HEAD with the remote HEAD via git ls-remote. Returns false on
// error (network failure) so the caller can safely skip and retry later.
func (c *Catalog) IsStale() (bool, error) {
	gitDir := filepath.Join(c.localDir, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		return false, nil // no local repo yet — Refresh will clone
	}

	// Get local HEAD
	localCmd := exec.Command("git", "-C", c.localDir, "rev-parse", "HEAD")
	localOut, err := localCmd.Output()
	if err != nil {
		return false, fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	localSHA := strings.TrimSpace(string(localOut))

	// Get remote HEAD
	remoteCmd := exec.Command("git", "ls-remote", "--heads", c.repoURL, c.branch)
	remoteOut, err := remoteCmd.Output()
	if err != nil {
		return false, fmt.Errorf("git ls-remote: %w", err)
	}
	remoteLine := strings.TrimSpace(string(remoteOut))
	if remoteLine == "" {
		return false, nil // branch not found on remote
	}
	remoteSHA := strings.Fields(remoteLine)[0]

	return localSHA != remoteSHA, nil
}

func (c *Catalog) fetchRepo() error {
	if _, err := os.Stat(filepath.Join(c.localDir, ".git")); err == nil {
		// Repo exists — pull
		cmd := exec.Command("git", "-C", c.localDir, "pull", "--ff-only", "origin", c.branch)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git pull: %s: %w", string(out), err)
		}
		return nil
	}

	// Clone fresh
	if err := os.MkdirAll(filepath.Dir(c.localDir), 0755); err != nil {
		return err
	}

	cmd := exec.Command("git", "clone", "--branch", c.branch, "--depth", "1", c.repoURL, c.localDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone: %s: %w", string(out), err)
	}
	return nil
}

func (c *Catalog) indexApps() error {
	appsDir := filepath.Join(c.localDir, "apps")

	// Also check catalog/apps/ layout
	if _, err := os.Stat(appsDir); os.IsNotExist(err) {
		alt := filepath.Join(c.localDir, "catalog", "apps")
		if _, err := os.Stat(alt); err == nil {
			appsDir = alt
		}
	}

	entries, err := os.ReadDir(appsDir)
	if err != nil {
		return fmt.Errorf("reading apps directory %s: %w", appsDir, err)
	}

	newApps := make(map[string]*AppManifest)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		appDir := filepath.Join(appsDir, entry.Name())
		manifestPath := filepath.Join(appDir, "app.yml")

		if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
			continue // skip directories without app.yml
		}

		manifest, err := ParseManifest(manifestPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", entry.Name(), err)
			continue
		}

		if err := manifest.Validate(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", entry.Name(), err)
			continue
		}

		// Set computed paths
		manifest.DirPath = appDir
		if _, err := os.Stat(filepath.Join(appDir, "icon.png")); err == nil {
			manifest.IconPath = filepath.Join(appDir, "icon.png")
		}
		if _, err := os.Stat(filepath.Join(appDir, "README.md")); err == nil {
			manifest.ReadmePath = filepath.Join(appDir, "README.md")
		}

		newApps[manifest.ID] = manifest
	}

	c.apps = newApps
	return nil
}

// MergeDevApp adds or replaces a developer app in the catalog.
// If a non-dev app with the same ID exists, it is preserved in the shadow map.
func (c *Catalog) MergeDevApp(app *AppManifest) {
	c.mu.Lock()
	defer c.mu.Unlock()
	app.Source = "developer"
	if existing, ok := c.apps[app.ID]; ok && existing.Source != "developer" {
		c.shadowed[app.ID] = existing
	}
	c.apps[app.ID] = app
}

// RemoveDevApp removes a developer app from the catalog (only if source is "developer").
// If a shadowed original exists, it is restored.
func (c *Catalog) RemoveDevApp(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if app, ok := c.apps[id]; ok && app.Source == "developer" {
		if orig, ok := c.shadowed[id]; ok {
			c.apps[id] = orig
			delete(c.shadowed, id)
		} else {
			delete(c.apps, id)
		}
	}
}

// ListStacks returns all validated stacks.
func (c *Catalog) ListStacks() []*StackManifest {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stacks := make([]*StackManifest, 0, len(c.stacks))
	for _, s := range c.stacks {
		stacks = append(stacks, s)
	}
	return stacks
}

// GetStack returns a single stack by ID.
func (c *Catalog) GetStack(id string) (*StackManifest, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	s, ok := c.stacks[id]
	return s, ok
}

// MergeDevStack adds or replaces a developer stack in the catalog.
func (c *Catalog) MergeDevStack(s *StackManifest) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s.Source = "developer"
	c.stacks[s.ID] = s
}

// RemoveDevStack removes a developer stack from the catalog (only if source is "developer").
func (c *Catalog) RemoveDevStack(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if s, ok := c.stacks[id]; ok && s.Source == "developer" {
		delete(c.stacks, id)
	}
}

// devApps returns a snapshot of all developer-sourced apps (must be called with lock held).
func (c *Catalog) devApps() []*AppManifest {
	var devApps []*AppManifest
	for _, app := range c.apps {
		if app.Source == "developer" {
			devApps = append(devApps, app)
		}
	}
	return devApps
}

// devStacks returns a snapshot of all developer-sourced stacks (must be called with lock held).
func (c *Catalog) devStacks() []*StackManifest {
	var result []*StackManifest
	for _, s := range c.stacks {
		if s.Source == "developer" {
			result = append(result, s)
		}
	}
	return result
}

// indexStacks scans the stacks/ directory and loads stack manifests.
func (c *Catalog) indexStacks() {
	stacksDir := filepath.Join(c.localDir, "stacks")

	// Also check catalog/stacks/ layout
	if _, err := os.Stat(stacksDir); os.IsNotExist(err) {
		alt := filepath.Join(c.localDir, "catalog", "stacks")
		if _, err := os.Stat(alt); err == nil {
			stacksDir = alt
		}
	}

	entries, err := os.ReadDir(stacksDir)
	if err != nil {
		// No stacks directory is fine — not all catalogs have stacks
		c.stacks = make(map[string]*StackManifest)
		return
	}

	newStacks := make(map[string]*StackManifest)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		stackDir := filepath.Join(stacksDir, entry.Name())
		manifestPath := filepath.Join(stackDir, "stack.yml")

		if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
			continue
		}

		sm, err := ParseStackManifest(manifestPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping stack %s: %v\n", entry.Name(), err)
			continue
		}

		if err := ValidateStackManifest(sm); err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping stack %s: %v\n", entry.Name(), err)
			continue
		}

		sm.DirPath = stackDir
		if _, err := os.Stat(filepath.Join(stackDir, "icon.png")); err == nil {
			sm.IconPath = filepath.Join(stackDir, "icon.png")
		}

		newStacks[sm.ID] = sm
	}

	c.stacks = newStacks
}

func matches(app *AppManifest, query string) bool {
	if strings.Contains(strings.ToLower(app.Name), query) {
		return true
	}
	if strings.Contains(strings.ToLower(app.Description), query) {
		return true
	}
	if strings.Contains(strings.ToLower(app.ID), query) {
		return true
	}
	for _, tag := range app.Tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}
	for _, cat := range app.Categories {
		if strings.Contains(strings.ToLower(cat), query) {
			return true
		}
	}
	return false
}
