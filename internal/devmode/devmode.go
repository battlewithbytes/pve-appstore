package devmode

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/battlewithbytes/pve-appstore/internal/catalog"
	"gopkg.in/yaml.v3"
)

//go:embed assets/default-icon.png
var defaultIconPNG []byte

// DevAppMeta is the summary for listing dev apps.
type DevAppMeta struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Description string    `json:"description"`
	Status      string    `json:"status"` // "draft", "validated", "deployed"
	HasIcon     bool      `json:"has_icon"`
	HasReadme   bool      `json:"has_readme"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
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
	if statusData, err := os.ReadFile(filepath.Join(appDir, ".devstatus")); err == nil {
		var s struct{ Status string `json:"status"` }
		if json.Unmarshal(statusData, &s) == nil && s.Status != "" {
			status = s.Status
		}
	}

	return &DevAppMeta{
		ID:          manifest.ID,
		Name:        manifest.Name,
		Version:     manifest.Version,
		Description: manifest.Description,
		Status:      status,
		HasIcon:     hasIcon == nil,
		HasReadme:   hasReadme == nil,
		CreatedAt:   createdAt,
		UpdatedAt:   info.ModTime(),
	}, nil
}

// SetStatus writes the dev app status file.
func (d *DevStore) SetStatus(id, status string) error {
	data, _ := json.Marshal(map[string]string{"status": status})
	return os.WriteFile(filepath.Join(d.baseDir, id, ".devstatus"), data, 0644)
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
		if strings.HasPrefix(e.Name(), ".") {
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

// EnsureIcon writes the default icon.png if one doesn't already exist.
func (d *DevStore) EnsureIcon(id string) {
	iconPath := filepath.Join(d.baseDir, id, "icon.png")
	if _, err := os.Stat(iconPath); os.IsNotExist(err) {
		os.WriteFile(iconPath, defaultIconPNG, 0644)
	}
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
