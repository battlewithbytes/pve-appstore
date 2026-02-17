package devmode

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/battlewithbytes/pve-appstore/internal/catalog"
	"gopkg.in/yaml.v3"
)

//go:embed assets/default-icon.png
var defaultIconPNG []byte

// DevStackMeta is the summary for listing dev stacks.
type DevStackMeta struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Version        string    `json:"version"`
	Description    string    `json:"description"`
	Status         string    `json:"status"`
	AppCount       int       `json:"app_count"`
	HasIcon        bool      `json:"has_icon"`
	GitHubBranch   string    `json:"github_branch,omitempty"`
	GitHubPRURL    string    `json:"github_pr_url,omitempty"`
	GitHubPRNumber int       `json:"github_pr_number,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// DevStack is the full dev stack with file contents.
type DevStack struct {
	DevStackMeta
	Manifest string    `json:"manifest"` // stack.yml content
	Deployed bool      `json:"deployed"`
	Files    []DevFile `json:"files"`
}

// DevAppMeta is the summary for listing dev apps.
type DevAppMeta struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Version        string    `json:"version"`
	Description    string    `json:"description"`
	Status         string    `json:"status"` // "draft", "validated", "deployed"
	HasIcon        bool      `json:"has_icon"`
	HasReadme      bool      `json:"has_readme"`
	SourceAppID    string    `json:"source_app_id,omitempty"`
	GitHubBranch   string    `json:"github_branch,omitempty"`
	GitHubPRURL    string    `json:"github_pr_url,omitempty"`
	GitHubPRNumber int       `json:"github_pr_number,omitempty"`
	TestInstallID  string    `json:"test_install_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// DevApp is the full dev app with file contents.
type DevApp struct {
	DevAppMeta
	Manifest   string            `json:"manifest"`    // app.yml content
	Script     string            `json:"script"`      // install.py content
	Readme     string            `json:"readme"`      // README.md content
	Files      []DevFile         `json:"files"`       // all files in the app directory
	Deployed   bool              `json:"deployed"`
}

// DevFile represents a file in a dev app directory.
type DevFile struct {
	Path    string `json:"path"`     // relative path from app dir
	Size    int64  `json:"size"`
	IsDir   bool   `json:"is_dir"`
}

// DevStore manages dev apps on disk.
type DevStore struct {
	baseDir string
}

// NewDevStore creates a new DevStore at the given directory.
func NewDevStore(baseDir string) *DevStore {
	os.MkdirAll(baseDir, 0755)
	return &DevStore{baseDir: baseDir}
}

// List returns metadata for all dev apps.
func (d *DevStore) List() ([]DevAppMeta, error) {
	entries, err := os.ReadDir(d.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []DevAppMeta{}, nil
		}
		return nil, err
	}

	var apps []DevAppMeta
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Skip stack directories (contain stack.yml instead of app.yml)
		if _, err := os.Stat(filepath.Join(d.baseDir, entry.Name(), "stack.yml")); err == nil {
			continue
		}
		meta, err := d.readMeta(entry.Name())
		if err != nil {
			continue
		}
		apps = append(apps, *meta)
	}
	sort.Slice(apps, func(i, j int) bool { return apps[i].Name < apps[j].Name })
	return apps, nil
}

// Get returns the full dev app.
func (d *DevStore) Get(id string) (*DevApp, error) {
	if !isValidID(id) {
		return nil, fmt.Errorf("invalid app id")
	}
	appDir := filepath.Join(d.baseDir, id)
	if _, err := os.Stat(appDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("dev app %q not found", id)
	}

	meta, err := d.readMeta(id)
	if err != nil {
		return nil, err
	}

	app := &DevApp{DevAppMeta: *meta}

	// Read manifest
	if data, err := os.ReadFile(filepath.Join(appDir, "app.yml")); err == nil {
		app.Manifest = string(data)
	}

	// Read script
	scriptPath := filepath.Join(appDir, "provision", "install.py")
	if data, err := os.ReadFile(scriptPath); err == nil {
		app.Script = string(data)
	}

	// Read readme
	if data, err := os.ReadFile(filepath.Join(appDir, "README.md")); err == nil {
		app.Readme = string(data)
	}

	// List all files
	app.Files = d.listFiles(appDir, "")

	return app, nil
}

// Create scaffolds a new dev app from a template.
func (d *DevStore) Create(id, template string) error {
	if !isValidID(id) {
		return fmt.Errorf("invalid app id: must be kebab-case")
	}
	appDir := filepath.Join(d.baseDir, id)
	if _, err := os.Stat(appDir); err == nil {
		return fmt.Errorf("app %q already exists", id)
	}

	if err := os.MkdirAll(filepath.Join(appDir, "provision"), 0755); err != nil {
		return err
	}

	tmpl := GetTemplate(template)
	if tmpl == nil {
		tmpl = GetTemplate("blank")
	}

	// Generate manifest and script from template
	manifest := tmpl.GenerateManifest(id)
	script := tmpl.GenerateScript(id)

	if err := os.WriteFile(filepath.Join(appDir, "app.yml"), []byte(manifest), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(appDir, "provision", "install.py"), []byte(script), 0644); err != nil {
		return err
	}

	// Create default icon
	if err := os.WriteFile(filepath.Join(appDir, "icon.png"), defaultIconPNG, 0644); err != nil {
		return err
	}

	// Create empty README
	readme := fmt.Sprintf("# %s\n\nTODO: Add description for your app.\n", titleFromID(id))
	if err := os.WriteFile(filepath.Join(appDir, "README.md"), []byte(readme), 0644); err != nil {
		return err
	}

	return nil
}

// SaveManifest writes app.yml.
func (d *DevStore) SaveManifest(id string, manifest []byte) error {
	if !isValidID(id) {
		return fmt.Errorf("invalid app id")
	}
	appDir := filepath.Join(d.baseDir, id)
	if _, err := os.Stat(appDir); os.IsNotExist(err) {
		return fmt.Errorf("dev app %q not found", id)
	}
	return os.WriteFile(filepath.Join(appDir, "app.yml"), manifest, 0644)
}

// SaveScript writes provision/install.py.
func (d *DevStore) SaveScript(id string, script []byte) error {
	if !isValidID(id) {
		return fmt.Errorf("invalid app id")
	}
	appDir := filepath.Join(d.baseDir, id)
	provDir := filepath.Join(appDir, "provision")
	os.MkdirAll(provDir, 0755)
	return os.WriteFile(filepath.Join(provDir, "install.py"), script, 0644)
}

// SaveFile writes an arbitrary file.
func (d *DevStore) SaveFile(id, relPath string, data []byte) error {
	if !isValidID(id) {
		return fmt.Errorf("invalid app id")
	}
	// Prevent path traversal
	clean := filepath.Clean(relPath)
	if strings.Contains(clean, "..") {
		return fmt.Errorf("invalid path")
	}
	fullPath := filepath.Join(d.baseDir, id, clean)
	os.MkdirAll(filepath.Dir(fullPath), 0755)
	return os.WriteFile(fullPath, data, 0644)
}

// ReadFile reads an arbitrary file from a dev app.
func (d *DevStore) ReadFile(id, relPath string) ([]byte, error) {
	if !isValidID(id) {
		return nil, fmt.Errorf("invalid app id")
	}
	clean := filepath.Clean(relPath)
	if strings.Contains(clean, "..") {
		return nil, fmt.Errorf("invalid path")
	}
	return os.ReadFile(filepath.Join(d.baseDir, id, clean))
}

// DeleteFile removes a single file from a dev app directory.
// Core files (app.yml, provision/install.py) cannot be deleted.
func (d *DevStore) DeleteFile(id, relPath string) error {
	if !isValidID(id) {
		return fmt.Errorf("invalid app id")
	}
	clean := filepath.Clean(relPath)
	if strings.Contains(clean, "..") {
		return fmt.Errorf("invalid path")
	}
	if clean == "app.yml" || clean == filepath.Join("provision", "install.py") {
		return fmt.Errorf("cannot delete core file %q", clean)
	}
	fullPath := filepath.Join(d.baseDir, id, clean)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return fmt.Errorf("file %q not found", clean)
	}
	return os.Remove(fullPath)
}

// RenameFile renames (or moves) a file within a dev app directory.
// Core files (app.yml, provision/install.py) cannot be renamed.
func (d *DevStore) RenameFile(id, oldPath, newPath string) error {
	if !isValidID(id) {
		return fmt.Errorf("invalid app id")
	}
	oldClean := filepath.Clean(oldPath)
	newClean := filepath.Clean(newPath)
	if strings.Contains(oldClean, "..") || strings.Contains(newClean, "..") {
		return fmt.Errorf("invalid path")
	}
	if oldClean == "app.yml" || oldClean == filepath.Join("provision", "install.py") {
		return fmt.Errorf("cannot rename core file %q", oldClean)
	}
	oldFull := filepath.Join(d.baseDir, id, oldClean)
	if _, err := os.Stat(oldFull); os.IsNotExist(err) {
		return fmt.Errorf("file %q not found", oldClean)
	}
	newFull := filepath.Join(d.baseDir, id, newClean)
	if _, err := os.Stat(newFull); err == nil {
		return fmt.Errorf("file %q already exists", newClean)
	}
	os.MkdirAll(filepath.Dir(newFull), 0755)
	return os.Rename(oldFull, newFull)
}

// RenameApp renames a dev app directory (changes its ID).
func (d *DevStore) RenameApp(oldID, newID string) error {
	if !isValidID(oldID) || !isValidID(newID) {
		return fmt.Errorf("invalid app id")
	}
	if oldID == newID {
		return nil
	}
	oldDir := filepath.Join(d.baseDir, oldID)
	if _, err := os.Stat(oldDir); os.IsNotExist(err) {
		return fmt.Errorf("dev app %q not found", oldID)
	}
	newDir := filepath.Join(d.baseDir, newID)
	if _, err := os.Stat(newDir); err == nil {
		return fmt.Errorf("dev app %q already exists", newID)
	}
	return os.Rename(oldDir, newDir)
}

// Delete removes a dev app directory.
func (d *DevStore) Delete(id string) error {
	if !isValidID(id) {
		return fmt.Errorf("invalid app id")
	}
	appDir := filepath.Join(d.baseDir, id)
	if _, err := os.Stat(appDir); os.IsNotExist(err) {
		return fmt.Errorf("dev app %q not found", id)
	}
	return os.RemoveAll(appDir)
}

// ParseManifest reads and parses the app.yml for a dev app.
func (d *DevStore) ParseManifest(id string) (*catalog.AppManifest, error) {
	if !isValidID(id) {
		return nil, fmt.Errorf("invalid app id")
	}
	manifestPath := filepath.Join(d.baseDir, id, "app.yml")
	return catalog.ParseManifest(manifestPath)
}

// AppDir returns the filesystem path for a dev app.
func (d *DevStore) AppDir(id string) string {
	return filepath.Join(d.baseDir, id)
}

// readMeta reads summary info from a dev app directory.
func (d *DevStore) readMeta(id string) (*DevAppMeta, error) {
	appDir := filepath.Join(d.baseDir, id)
	manifestPath := filepath.Join(appDir, "app.yml")

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}

	var manifest struct {
		ID          string `yaml:"id"`
		Name        string `yaml:"name"`
		Version     string `yaml:"version"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}

	info, _ := os.Stat(manifestPath)
	createdAt := info.ModTime()

	// Check for icon and readme
	_, hasIcon := os.Stat(filepath.Join(appDir, "icon.png"))
	_, hasReadme := os.Stat(filepath.Join(appDir, "README.md"))

	// Check status file
	status := "draft"
	var githubBranch, githubPRURL, testInstallID, sourceAppID string
	var githubPRNumber int
	if statusData, err := os.ReadFile(filepath.Join(appDir, ".devstatus")); err == nil {
		// Use map[string]interface{} to handle mixed types (pr_number stored as string by SetGitHubMeta)
		var raw map[string]interface{}
		if json.Unmarshal(statusData, &raw) == nil {
			if v, ok := raw["status"].(string); ok && v != "" {
				status = v
			}
			if v, ok := raw["source_app_id"].(string); ok {
				sourceAppID = v
			}
			if v, ok := raw["github_branch"].(string); ok {
				githubBranch = v
			}
			if v, ok := raw["github_pr_url"].(string); ok {
				githubPRURL = v
			}
			if v, ok := raw["test_install_id"].(string); ok {
				testInstallID = v
			}
			// github_pr_number may be stored as string or number
			switch v := raw["github_pr_number"].(type) {
			case string:
				githubPRNumber, _ = strconv.Atoi(v)
			case float64:
				githubPRNumber = int(v)
			}
		}
	}

	return &DevAppMeta{
		ID:             manifest.ID,
		Name:           manifest.Name,
		Version:        manifest.Version,
		Description:    manifest.Description,
		Status:         status,
		HasIcon:        hasIcon == nil,
		HasReadme:      hasReadme == nil,
		SourceAppID:    sourceAppID,
		GitHubBranch:   githubBranch,
		GitHubPRURL:    githubPRURL,
		GitHubPRNumber: githubPRNumber,
		TestInstallID:  testInstallID,
		CreatedAt:      createdAt,
		UpdatedAt:      info.ModTime(),
	}, nil
}

// SetStatus writes the dev app status file, preserving other fields.
func (d *DevStore) SetStatus(id, status string) error {
	return d.SetGitHubMeta(id, map[string]string{"status": status})
}

// SetGitHubMeta merges key-value pairs into the .devstatus JSON file.
func (d *DevStore) SetGitHubMeta(id string, meta map[string]string) error {
	statusPath := filepath.Join(d.baseDir, id, ".devstatus")

	// Read existing
	existing := make(map[string]string)
	if data, err := os.ReadFile(statusPath); err == nil {
		json.Unmarshal(data, &existing)
	}

	// Merge
	for k, v := range meta {
		existing[k] = v
	}

	data, _ := json.Marshal(existing)
	return os.WriteFile(statusPath, data, 0644)
}

// IsDeployed returns true if the dev app is currently deployed to the catalog.
func (d *DevStore) IsDeployed(id string) bool {
	data, err := os.ReadFile(filepath.Join(d.baseDir, id, ".devstatus"))
	if err != nil {
		return false
	}
	var s struct{ Status string `json:"status"` }
	if json.Unmarshal(data, &s) != nil {
		return false
	}
	return s.Status == "deployed"
}

func (d *DevStore) listFiles(dir, prefix string) []DevFile {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []DevFile
	for _, e := range entries {
		if isSkippable(e.Name()) {
			continue
		}
		rel := filepath.Join(prefix, e.Name())
		if e.IsDir() {
			files = append(files, DevFile{Path: rel, IsDir: true})
			files = append(files, d.listFiles(filepath.Join(dir, e.Name()), rel)...)
		} else {
			info, _ := e.Info()
			size := int64(0)
			if info != nil {
				size = info.Size()
			}
			files = append(files, DevFile{Path: rel, Size: size})
		}
	}
	return files
}

// Fork copies a catalog app directory into the dev store under a new ID.
func (d *DevStore) Fork(newID, sourceDir string) error {
	if !isValidID(newID) {
		return fmt.Errorf("invalid app id: must be kebab-case")
	}
	destDir := filepath.Join(d.baseDir, newID)
	if _, err := os.Stat(destDir); err == nil {
		return fmt.Errorf("app %q already exists", newID)
	}
	if err := copyDir(sourceDir, destDir); err != nil {
		os.RemoveAll(destDir) // clean up partial copy
		return fmt.Errorf("copying app: %w", err)
	}
	// Update the ID in app.yml — use simple line replacement to preserve formatting
	manifestPath := filepath.Join(destDir, "app.yml")
	if data, err := os.ReadFile(manifestPath); err == nil {
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if strings.HasPrefix(line, "id:") {
				lines[i] = "id: " + newID
				break
			}
		}
		os.WriteFile(manifestPath, []byte(strings.Join(lines, "\n")), 0644)
	}
	return nil
}

// copyDir recursively copies src to dst, skipping dotfiles.
func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if isSkippable(e.Name()) {
			continue
		}
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return err
			}
			if err := os.WriteFile(dstPath, data, 0644); err != nil {
				return err
			}
		}
	}
	return nil
}

// ListStacks returns metadata for all dev stacks.
func (d *DevStore) ListStacks() ([]DevStackMeta, error) {
	entries, err := os.ReadDir(d.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []DevStackMeta{}, nil
		}
		return nil, err
	}

	var stacks []DevStackMeta
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Only include directories containing stack.yml
		if _, err := os.Stat(filepath.Join(d.baseDir, entry.Name(), "stack.yml")); err != nil {
			continue
		}
		meta, err := d.readStackMeta(entry.Name())
		if err != nil {
			continue
		}
		stacks = append(stacks, *meta)
	}
	sort.Slice(stacks, func(i, j int) bool { return stacks[i].Name < stacks[j].Name })
	return stacks, nil
}

// GetStack returns the full dev stack.
func (d *DevStore) GetStack(id string) (*DevStack, error) {
	if !isValidID(id) {
		return nil, fmt.Errorf("invalid stack id")
	}
	stackDir := filepath.Join(d.baseDir, id)
	if _, err := os.Stat(filepath.Join(stackDir, "stack.yml")); os.IsNotExist(err) {
		return nil, fmt.Errorf("dev stack %q not found", id)
	}

	meta, err := d.readStackMeta(id)
	if err != nil {
		return nil, err
	}

	stack := &DevStack{DevStackMeta: *meta}

	if data, err := os.ReadFile(filepath.Join(stackDir, "stack.yml")); err == nil {
		stack.Manifest = string(data)
	}

	stack.Files = d.listFiles(stackDir, "")
	stack.Deployed = d.IsStackDeployed(id)

	return stack, nil
}

// CreateStack scaffolds a new dev stack.
func (d *DevStore) CreateStack(id, template string) error {
	if !isValidID(id) {
		return fmt.Errorf("invalid stack id: must be kebab-case")
	}
	stackDir := filepath.Join(d.baseDir, id)
	if _, err := os.Stat(stackDir); err == nil {
		return fmt.Errorf("stack %q already exists", id)
	}

	if err := os.MkdirAll(stackDir, 0755); err != nil {
		return err
	}

	name := titleFromID(id)
	manifest := GenerateStackManifest(id, name, template)

	if err := os.WriteFile(filepath.Join(stackDir, "stack.yml"), []byte(manifest), 0644); err != nil {
		return err
	}

	// Create default icon
	if err := os.WriteFile(filepath.Join(stackDir, "icon.png"), defaultIconPNG, 0644); err != nil {
		return err
	}

	readme := fmt.Sprintf("# %s\n\nTODO: Add description for your stack.\n", name)
	if err := os.WriteFile(filepath.Join(stackDir, "README.md"), []byte(readme), 0644); err != nil {
		return err
	}

	return nil
}

// SaveStackManifest writes stack.yml.
func (d *DevStore) SaveStackManifest(id string, data []byte) error {
	if !isValidID(id) {
		return fmt.Errorf("invalid stack id")
	}
	stackDir := filepath.Join(d.baseDir, id)
	if _, err := os.Stat(stackDir); os.IsNotExist(err) {
		return fmt.Errorf("dev stack %q not found", id)
	}
	return os.WriteFile(filepath.Join(stackDir, "stack.yml"), data, 0644)
}

// ParseStackManifest reads and parses the stack.yml for a dev stack.
func (d *DevStore) ParseStackManifest(id string) (*catalog.StackManifest, error) {
	if !isValidID(id) {
		return nil, fmt.Errorf("invalid stack id")
	}
	manifestPath := filepath.Join(d.baseDir, id, "stack.yml")
	return catalog.ParseStackManifest(manifestPath)
}

// DeleteStack removes a dev stack directory.
func (d *DevStore) DeleteStack(id string) error {
	if !isValidID(id) {
		return fmt.Errorf("invalid stack id")
	}
	stackDir := filepath.Join(d.baseDir, id)
	if _, err := os.Stat(filepath.Join(stackDir, "stack.yml")); os.IsNotExist(err) {
		return fmt.Errorf("dev stack %q not found", id)
	}
	return os.RemoveAll(stackDir)
}

// IsStackDeployed returns true if the dev stack is currently deployed to the catalog.
func (d *DevStore) IsStackDeployed(id string) bool {
	data, err := os.ReadFile(filepath.Join(d.baseDir, id, ".devstatus"))
	if err != nil {
		return false
	}
	var s struct{ Status string `json:"status"` }
	if json.Unmarshal(data, &s) != nil {
		return false
	}
	return s.Status == "deployed"
}

// SetStackStatus writes the dev stack status file.
func (d *DevStore) SetStackStatus(id, status string) error {
	return d.SetGitHubMeta(id, map[string]string{"status": status})
}

// readStackMeta reads summary info from a dev stack directory.
func (d *DevStore) readStackMeta(id string) (*DevStackMeta, error) {
	stackDir := filepath.Join(d.baseDir, id)
	manifestPath := filepath.Join(stackDir, "stack.yml")

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}

	var manifest struct {
		ID          string `yaml:"id"`
		Name        string `yaml:"name"`
		Version     string `yaml:"version"`
		Description string `yaml:"description"`
		Apps        []struct {
			AppID string `yaml:"app_id"`
		} `yaml:"apps"`
	}
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}

	info, _ := os.Stat(manifestPath)
	createdAt := info.ModTime()

	_, hasIcon := os.Stat(filepath.Join(stackDir, "icon.png"))

	status := "draft"
	var githubBranch, githubPRURL string
	var githubPRNumber int
	if statusData, err := os.ReadFile(filepath.Join(stackDir, ".devstatus")); err == nil {
		var raw map[string]interface{}
		if json.Unmarshal(statusData, &raw) == nil {
			if v, ok := raw["status"].(string); ok && v != "" {
				status = v
			}
			if v, ok := raw["github_branch"].(string); ok {
				githubBranch = v
			}
			if v, ok := raw["github_pr_url"].(string); ok {
				githubPRURL = v
			}
			switch v := raw["github_pr_number"].(type) {
			case string:
				githubPRNumber, _ = strconv.Atoi(v)
			case float64:
				githubPRNumber = int(v)
			}
		}
	}

	return &DevStackMeta{
		ID:             manifest.ID,
		Name:           manifest.Name,
		Version:        manifest.Version,
		Description:    manifest.Description,
		Status:         status,
		AppCount:       len(manifest.Apps),
		HasIcon:        hasIcon == nil,
		GitHubBranch:   githubBranch,
		GitHubPRURL:    githubPRURL,
		GitHubPRNumber: githubPRNumber,
		CreatedAt:      createdAt,
		UpdatedAt:      info.ModTime(),
	}, nil
}

// EnsureIcon writes the default icon.png if one doesn't already exist.
func (d *DevStore) EnsureIcon(id string) {
	iconPath := filepath.Join(d.baseDir, id, "icon.png")
	if _, err := os.Stat(iconPath); os.IsNotExist(err) {
		os.WriteFile(iconPath, defaultIconPNG, 0644)
	}
}

// isSkippable returns true for files/dirs that should be excluded from copies and listings.
func isSkippable(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	if name == "__pycache__" || strings.HasSuffix(name, ".pyc") {
		return true
	}
	return false
}

func isValidID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, c := range id {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
			return false
		}
	}
	return !strings.HasPrefix(id, "-") && !strings.HasSuffix(id, "-")
}

func titleFromID(id string) string {
	words := strings.Split(id, "-")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
