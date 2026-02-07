package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the full application configuration written to config.yml.
type Config struct {
	NodeName string          `yaml:"node_name"`
	Pool     string          `yaml:"pool"`
	Storage  string          `yaml:"storage"`
	Bridge   string          `yaml:"bridge"`
	Defaults ResourceConfig  `yaml:"defaults"`
	Security SecurityConfig  `yaml:"security"`
	Service  ServiceConfig   `yaml:"service"`
	Auth     AuthConfig      `yaml:"auth"`
	Proxmox  ProxmoxConfig   `yaml:"proxmox"`
	Catalog  CatalogConfig   `yaml:"catalog"`
	GPU      GPUConfig       `yaml:"gpu"`
}

type ResourceConfig struct {
	Cores    int `yaml:"cores"`
	MemoryMB int `yaml:"memory_mb"`
	DiskGB   int `yaml:"disk_gb"`
}

type SecurityConfig struct {
	UnprivilegedOnly bool     `yaml:"unprivileged_only"`
	AllowedFeatures  []string `yaml:"allowed_features"`
}

type ServiceConfig struct {
	BindAddress string `yaml:"bind_address"`
	Port        int    `yaml:"port"`
}

type AuthConfig struct {
	Mode         string `yaml:"mode"`
	PasswordHash string `yaml:"password_hash,omitempty"`
}

type ProxmoxConfig struct {
	AutoCreated   bool   `yaml:"auto_created"`
	TokenID       string `yaml:"token_id"`
	TokenSecret   string `yaml:"token_secret"`
	BaseURL       string `yaml:"base_url"`
	TLSSkipVerify bool   `yaml:"tls_skip_verify"`
	TLSCACertPath string `yaml:"tls_ca_cert,omitempty"`
}

type CatalogConfig struct {
	URL     string `yaml:"url"`
	Branch  string `yaml:"branch"`
	Refresh string `yaml:"refresh"`
}

type GPUConfig struct {
	Enabled        bool     `yaml:"enabled"`
	Policy         string   `yaml:"policy"`
	AllowedDevices []string `yaml:"allowed_devices,omitempty"`
}

// Load reads and parses a config file from the given path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &cfg, nil
}

// Validate checks that all required fields are present and values are in range.
func (c *Config) Validate() error {
	if c.NodeName == "" {
		return fmt.Errorf("node_name is required")
	}
	if c.Pool == "" {
		return fmt.Errorf("pool is required")
	}
	if c.Storage == "" {
		return fmt.Errorf("storage is required")
	}
	if c.Bridge == "" {
		return fmt.Errorf("bridge is required")
	}

	// Resource defaults
	if c.Defaults.Cores < 1 {
		return fmt.Errorf("defaults.cores must be >= 1")
	}
	if c.Defaults.MemoryMB < 128 {
		return fmt.Errorf("defaults.memory_mb must be >= 128")
	}
	if c.Defaults.DiskGB < 1 {
		return fmt.Errorf("defaults.disk_gb must be >= 1")
	}

	// Service
	if c.Service.Port < 1 || c.Service.Port > 65535 {
		return fmt.Errorf("service.port must be between 1 and 65535")
	}
	if c.Service.BindAddress == "" {
		return fmt.Errorf("service.bind_address is required")
	}

	// Auth mode
	switch c.Auth.Mode {
	case AuthModeNone, AuthModePassword:
		// ok
	default:
		return fmt.Errorf("auth.mode must be %q or %q", AuthModeNone, AuthModePassword)
	}

	// Catalog refresh
	switch c.Catalog.Refresh {
	case RefreshDaily, RefreshWeekly, RefreshManual:
		// ok
	default:
		return fmt.Errorf("catalog.refresh must be %q, %q, or %q", RefreshDaily, RefreshWeekly, RefreshManual)
	}

	// GPU policy
	switch c.GPU.Policy {
	case GPUPolicyNone, GPUPolicyAllow, GPUPolicyAllowlist:
		// ok
	default:
		return fmt.Errorf("gpu.policy must be %q, %q, or %q", GPUPolicyNone, GPUPolicyAllow, GPUPolicyAllowlist)
	}

	return nil
}

// Save writes the config to the given path, creating parent directories as needed.
func (c *Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(path, data, 0640); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}
