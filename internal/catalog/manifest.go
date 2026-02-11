package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
	Icon        string   `yaml:"icon,omitempty" json:"icon,omitempty"` // URL to app icon
	Official    bool     `yaml:"official,omitempty" json:"official"`
	Maintainers []string `yaml:"maintainers,omitempty" json:"maintainers,omitempty"`

	LXC          LXCConfig        `yaml:"lxc" json:"lxc"`
	Inputs       []InputSpec      `yaml:"inputs,omitempty" json:"inputs,omitempty"`
	Provisioning ProvisioningSpec `yaml:"provisioning,omitempty" json:"provisioning,omitempty"`
	Permissions  PermissionsSpec  `yaml:"permissions,omitempty" json:"permissions,omitempty"`
	Outputs      []OutputSpec     `yaml:"outputs,omitempty" json:"outputs,omitempty"`
	GPU          GPUSpec          `yaml:"gpu,omitempty" json:"gpu,omitempty"`
	Volumes      []VolumeSpec     `yaml:"volumes,omitempty" json:"volumes,omitempty"`

	// Computed fields (not from YAML)
	IconPath   string `yaml:"-" json:"icon_path,omitempty"`
	ReadmePath string `yaml:"-" json:"readme_path,omitempty"`
	DirPath    string `yaml:"-" json:"dir_path,omitempty"`
	Source     string `yaml:"-" json:"source,omitempty"` // "official", "community", "developer"
}

type LXCConfig struct {
	OSTemplate string      `yaml:"ostemplate" json:"ostemplate"`
	Defaults   LXCDefaults `yaml:"defaults" json:"defaults"`
	ExtraConfig []string   `yaml:"extra_config,omitempty" json:"extra_config,omitempty"` // raw LXC config lines
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
	Key            string          `yaml:"key" json:"key"`
	Label          string          `yaml:"label" json:"label"`
	Type           string          `yaml:"type" json:"type"`
	Default        interface{}     `yaml:"default,omitempty" json:"default,omitempty"`
	Required       bool            `yaml:"required" json:"required"`
	Reconfigurable bool            `yaml:"reconfigurable,omitempty" json:"reconfigurable,omitempty"`
	Validation     *InputValidation `yaml:"validation,omitempty" json:"validation,omitempty"`
	Help           string          `yaml:"help,omitempty" json:"help,omitempty"`
	Description    string          `yaml:"description,omitempty" json:"description,omitempty"`
	Group          string          `yaml:"group,omitempty" json:"group,omitempty"`
	ShowWhen       *ShowWhenSpec   `yaml:"show_when,omitempty" json:"show_when,omitempty"`
}

// ShowWhenSpec controls conditional visibility of an input.
// The input is shown only when the referenced input's value matches one of the given values.
type ShowWhenSpec struct {
	Input  string   `yaml:"input" json:"input"`   // key of another input to check
	Values []string `yaml:"values" json:"values"` // show when value is one of these
}

type InputValidation struct {
	Regex     string   `yaml:"regex,omitempty" json:"regex,omitempty"`
	Min       *float64 `yaml:"min,omitempty" json:"min,omitempty"`
	Max       *float64 `yaml:"max,omitempty" json:"max,omitempty"`
	MinLength *int     `yaml:"min_length,omitempty" json:"min_length,omitempty"`
	MaxLength *int     `yaml:"max_length,omitempty" json:"max_length,omitempty"`
	Enum      []string `yaml:"enum,omitempty" json:"enum,omitempty"`
	EnumDir   string   `yaml:"enum_dir,omitempty" json:"-"` // directory of .yml files to build enum from
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

// VolumeSpec declares a persistent volume mount for an app.
// Type "volume" (default) creates a Proxmox managed disk image.
// Type "bind" mounts a host directory into the container.
type VolumeSpec struct {
	Name            string `yaml:"name" json:"name"`
	Type            string `yaml:"type" json:"type"`                                       // "volume" or "bind"
	MountPath       string `yaml:"mount_path" json:"mount_path"`
	SizeGB          int    `yaml:"size_gb,omitempty" json:"size_gb,omitempty"`              // only for type=volume
	Label           string `yaml:"label" json:"label"`
	DefaultHostPath string `yaml:"default_host_path,omitempty" json:"default_host_path,omitempty"` // suggested default for bind
	Required        bool   `yaml:"required" json:"required"`
	ReadOnly        bool   `yaml:"read_only,omitempty" json:"read_only,omitempty"`
	Description     string `yaml:"description,omitempty" json:"description,omitempty"`
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

	// Resolve enum_dir references relative to the manifest's directory.
	baseDir := filepath.Dir(path)
	if err := m.resolveEnumDirs(baseDir); err != nil {
		return nil, fmt.Errorf("manifest %s: %w", path, err)
	}

	return &m, nil
}

// resolveEnumDirs reads .yml files from any enum_dir directories and
// populates the corresponding Enum slices from their id fields.
func (m *AppManifest) resolveEnumDirs(baseDir string) error {
	for i := range m.Inputs {
		v := m.Inputs[i].Validation
		if v == nil || v.EnumDir == "" {
			continue
		}
		enumDir := filepath.Join(baseDir, v.EnumDir)
		entries, err := os.ReadDir(enumDir)
		if err != nil {
			return fmt.Errorf("reading enum_dir %q: %w", v.EnumDir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yml") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(enumDir, entry.Name()))
			if err != nil {
				return fmt.Errorf("reading enum file %s: %w", entry.Name(), err)
			}
			var e struct {
				ID string `yaml:"id"`
			}
			if err := yaml.Unmarshal(data, &e); err != nil {
				return fmt.Errorf("parsing enum file %s: %w", entry.Name(), err)
			}
			if e.ID != "" {
				v.Enum = append(v.Enum, e.ID)
			}
		}
		sort.Strings(v.Enum)
		if len(v.Enum) == 0 {
			return fmt.Errorf("enum_dir %q produced no entries", v.EnumDir)
		}
	}
	return nil
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

	// Volumes
	for i := range m.Volumes {
		vol := &m.Volumes[i]
		// Default type to "volume" for backward compat
		if vol.Type == "" {
			vol.Type = "volume"
		}
		if vol.Name == "" {
			return fmt.Errorf("manifest %s: volume name is required", m.ID)
		}
		if vol.Type != "volume" && vol.Type != "bind" {
			return fmt.Errorf("manifest %s: volume %s type must be \"volume\" or \"bind\"", m.ID, vol.Name)
		}
		if vol.MountPath == "" || vol.MountPath[0] != '/' {
			return fmt.Errorf("manifest %s: volume %s mount_path must be an absolute path", m.ID, vol.Name)
		}
		if vol.Type == "volume" && vol.SizeGB < 1 {
			return fmt.Errorf("manifest %s: volume %s size_gb must be >= 1", m.ID, vol.Name)
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
