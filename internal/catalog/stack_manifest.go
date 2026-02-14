package catalog

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// StackManifest represents a stack.yml manifest from the catalog.
type StackManifest struct {
	ID          string             `yaml:"id" json:"id"`
	Name        string             `yaml:"name" json:"name"`
	Description string             `yaml:"description" json:"description"`
	Version     string             `yaml:"version" json:"version"`
	Categories  []string           `yaml:"categories" json:"categories"`
	Tags        []string           `yaml:"tags" json:"tags"`
	Icon        string             `yaml:"icon,omitempty" json:"icon,omitempty"`
	Apps        []StackManifestApp `yaml:"apps" json:"apps"`
	LXC         LXCConfig          `yaml:"lxc" json:"lxc"`

	// Computed fields (not from YAML)
	IconPath string `yaml:"-" json:"icon_path,omitempty"`
	DirPath  string `yaml:"-" json:"dir_path,omitempty"`
	Source   string `yaml:"-" json:"source,omitempty"` // "official", "community", "developer"
}

// StackManifestApp is a single app reference within a stack definition.
type StackManifestApp struct {
	AppID  string            `yaml:"app_id" json:"app_id"`
	Inputs map[string]string `yaml:"inputs,omitempty" json:"inputs,omitempty"`
}

// ParseStackManifest reads and parses a stack.yml file.
func ParseStackManifest(path string) (*StackManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading stack manifest: %w", err)
	}

	var sm StackManifest
	if err := yaml.Unmarshal(data, &sm); err != nil {
		return nil, fmt.Errorf("parsing stack manifest %s: %w", path, err)
	}

	return &sm, nil
}

// ValidateStackManifest checks that a stack manifest has all required fields.
func ValidateStackManifest(sm *StackManifest) error {
	if sm.ID == "" {
		return fmt.Errorf("stack manifest: id is required")
	}
	if !kebabCaseRe.MatchString(sm.ID) {
		return fmt.Errorf("stack manifest: id %q must be kebab-case", sm.ID)
	}
	if sm.Name == "" {
		return fmt.Errorf("stack manifest %s: name is required", sm.ID)
	}
	if sm.Version == "" {
		return fmt.Errorf("stack manifest %s: version is required", sm.ID)
	}
	if len(sm.Apps) == 0 {
		return fmt.Errorf("stack manifest %s: at least one app is required", sm.ID)
	}

	for i, sa := range sm.Apps {
		if sa.AppID == "" {
			return fmt.Errorf("stack manifest %s: apps[%d].app_id is required", sm.ID, i)
		}
		if !kebabCaseRe.MatchString(sa.AppID) {
			return fmt.Errorf("stack manifest %s: apps[%d].app_id %q must be kebab-case", sm.ID, i, sa.AppID)
		}
	}

	// LXC defaults validation (optional for stacks — can inherit from apps)
	if sm.LXC.OSTemplate != "" {
		if sm.LXC.Defaults.Cores < 1 {
			sm.LXC.Defaults.Cores = 2 // default
		}
		if sm.LXC.Defaults.MemoryMB < 128 {
			sm.LXC.Defaults.MemoryMB = 1024 // default
		}
		if sm.LXC.Defaults.DiskGB < 1 {
			sm.LXC.Defaults.DiskGB = 8 // default
		}
	}

	return nil
}
