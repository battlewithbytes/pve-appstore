package catalog

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// AppManifest represents an app.yml manifest from the catalog.
type AppManifest struct {
	ID          string   `yaml:"id" json:"id"`
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description" json:"description"`
	Overview    string   `yaml:"overview,omitempty" json:"overview,omitempty"`
	Version     string   `yaml:"version" json:"version"`
	Categories  []string `yaml:"categories" json:"categories"`
	Tags        []string `yaml:"tags" json:"tags"`
	Homepage    string   `yaml:"homepage,omitempty" json:"homepage,omitempty"`
	License     string   `yaml:"license,omitempty" json:"license,omitempty"`
	Maintainers []string `yaml:"maintainers,omitempty" json:"maintainers,omitempty"`

	LXC          LXCConfig        `yaml:"lxc" json:"lxc"`
	Inputs       []InputSpec      `yaml:"inputs,omitempty" json:"inputs,omitempty"`
	Provisioning ProvisioningSpec `yaml:"provisioning,omitempty" json:"provisioning,omitempty"`
	Permissions  PermissionsSpec  `yaml:"permissions,omitempty" json:"permissions,omitempty"`
	Outputs      []OutputSpec     `yaml:"outputs,omitempty" json:"outputs,omitempty"`
	GPU          GPUSpec          `yaml:"gpu,omitempty" json:"gpu,omitempty"`

	// Computed fields (not from YAML)
	IconPath   string `yaml:"-" json:"icon_path,omitempty"`
	ReadmePath string `yaml:"-" json:"readme_path,omitempty"`
	DirPath    string `yaml:"-" json:"dir_path,omitempty"`
}

type LXCConfig struct {
	OSTemplate string      `yaml:"ostemplate" json:"ostemplate"`
	Defaults   LXCDefaults `yaml:"defaults" json:"defaults"`
}

type LXCDefaults struct {
	Unprivileged bool     `yaml:"unprivileged" json:"unprivileged"`
	Cores        int      `yaml:"cores" json:"cores"`
	MemoryMB     int      `yaml:"memory_mb" json:"memory_mb"`
	DiskGB       int      `yaml:"disk_gb" json:"disk_gb"`
	Bridge       string   `yaml:"bridge,omitempty" json:"bridge,omitempty"`
	Storage      string   `yaml:"storage,omitempty" json:"storage,omitempty"`
	Features     []string `yaml:"features,omitempty" json:"features,omitempty"`
	OnBoot       bool     `yaml:"onboot,omitempty" json:"onboot,omitempty"`
}

type InputSpec struct {
	Key         string          `yaml:"key" json:"key"`
	Label       string          `yaml:"label" json:"label"`
	Type        string          `yaml:"type" json:"type"`
	Default     interface{}     `yaml:"default,omitempty" json:"default,omitempty"`
	Required    bool            `yaml:"required" json:"required"`
	Validation  *InputValidation `yaml:"validation,omitempty" json:"validation,omitempty"`
	Help        string          `yaml:"help,omitempty" json:"help,omitempty"`
	Description string          `yaml:"description,omitempty" json:"description,omitempty"`
	Group       string          `yaml:"group,omitempty" json:"group,omitempty"`
}

type InputValidation struct {
	Regex string   `yaml:"regex,omitempty" json:"regex,omitempty"`
	Min   *float64 `yaml:"min,omitempty" json:"min,omitempty"`
	Max   *float64 `yaml:"max,omitempty" json:"max,omitempty"`
	Enum  []string `yaml:"enum,omitempty" json:"enum,omitempty"`
}

type ProvisioningSpec struct {
	Script     string            `yaml:"script" json:"script"`
	Env        map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	Assets     []string          `yaml:"assets,omitempty" json:"assets,omitempty"`
	TimeoutSec int               `yaml:"timeout_sec,omitempty" json:"timeout_sec,omitempty"`
	User       string            `yaml:"user,omitempty" json:"user,omitempty"`
	RedactKeys []string          `yaml:"redact_keys,omitempty" json:"redact_keys,omitempty"`
}

type OutputSpec struct {
	Key   string `yaml:"key" json:"key"`
	Label string `yaml:"label" json:"label"`
	Value string `yaml:"value" json:"value"`
}

type GPUSpec struct {
	Supported []string `yaml:"supported,omitempty" json:"supported,omitempty"`
	Required  bool     `yaml:"required" json:"required"`
	Profiles  []string `yaml:"profiles,omitempty" json:"profiles,omitempty"`
	Notes     string   `yaml:"notes,omitempty" json:"notes,omitempty"`
}

// PermissionsSpec declares the allowlist of operations an app is permitted to
// perform during provisioning. The Python SDK enforces these at runtime.
type PermissionsSpec struct {
	Packages         []string `yaml:"packages,omitempty" json:"packages,omitempty"`
	Pip              []string `yaml:"pip,omitempty" json:"pip,omitempty"`
	URLs             []string `yaml:"urls,omitempty" json:"urls,omitempty"`
	Paths            []string `yaml:"paths,omitempty" json:"paths,omitempty"`
	Services         []string `yaml:"services,omitempty" json:"services,omitempty"`
	Users            []string `yaml:"users,omitempty" json:"users,omitempty"`
	Commands         []string `yaml:"commands,omitempty" json:"commands,omitempty"`
	InstallerScripts []string `yaml:"installer_scripts,omitempty" json:"installer_scripts,omitempty"`
	AptRepos         []string `yaml:"apt_repos,omitempty" json:"apt_repos,omitempty"`
}

var kebabCaseRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ParseManifest reads and parses an app.yml file.
func ParseManifest(path string) (*AppManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	var m AppManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest %s: %w", path, err)
	}

	return &m, nil
}

// Validate checks that a manifest has all required fields and valid values.
func (m *AppManifest) Validate() error {
	if m.ID == "" {
		return fmt.Errorf("manifest: id is required")
	}
	if !kebabCaseRe.MatchString(m.ID) {
		return fmt.Errorf("manifest: id %q must be kebab-case", m.ID)
	}
	if m.Name == "" {
		return fmt.Errorf("manifest %s: name is required", m.ID)
	}
	if m.Description == "" {
		return fmt.Errorf("manifest %s: description is required", m.ID)
	}
	if m.Version == "" {
		return fmt.Errorf("manifest %s: version is required", m.ID)
	}
	if len(m.Categories) == 0 {
		return fmt.Errorf("manifest %s: at least one category is required", m.ID)
	}

	// LXC
	if m.LXC.OSTemplate == "" {
		return fmt.Errorf("manifest %s: lxc.ostemplate is required", m.ID)
	}
	if m.LXC.Defaults.Cores < 1 {
		return fmt.Errorf("manifest %s: lxc.defaults.cores must be >= 1", m.ID)
	}
	if m.LXC.Defaults.MemoryMB < 128 {
		return fmt.Errorf("manifest %s: lxc.defaults.memory_mb must be >= 128", m.ID)
	}
	if m.LXC.Defaults.DiskGB < 1 {
		return fmt.Errorf("manifest %s: lxc.defaults.disk_gb must be >= 1", m.ID)
	}

	// Inputs
	validTypes := map[string]bool{
		"string": true, "number": true, "boolean": true, "secret": true, "select": true,
	}
	for _, inp := range m.Inputs {
		if inp.Key == "" {
			return fmt.Errorf("manifest %s: input key is required", m.ID)
		}
		if inp.Label == "" {
			return fmt.Errorf("manifest %s: input %s label is required", m.ID, inp.Key)
		}
		if !validTypes[inp.Type] {
			return fmt.Errorf("manifest %s: input %s type %q is invalid", m.ID, inp.Key, inp.Type)
		}
	}

	// GPU supported types
	validGPUTypes := map[string]bool{"intel": true, "amd": true, "nvidia": true}
	for _, t := range m.GPU.Supported {
		if !validGPUTypes[strings.ToLower(t)] {
			return fmt.Errorf("manifest %s: gpu.supported type %q is invalid", m.ID, t)
		}
	}

	// Provisioning
	if m.Provisioning.Script == "" {
		return fmt.Errorf("manifest %s: provisioning.script is required", m.ID)
	}
	if !strings.HasSuffix(m.Provisioning.Script, ".py") {
		return fmt.Errorf("manifest %s: provisioning.script must be a .py file, got %q", m.ID, m.Provisioning.Script)
	}

	return nil
}
