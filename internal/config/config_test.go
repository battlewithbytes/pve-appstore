package config

import (
	"os"
	"path/filepath"
	"testing"
)

func validConfig() *Config {
	return &Config{
		NodeName: "zeus",
		Pool:     "appstore",
		Storage:  "local-lvm",
		Bridge:   "vmbr0",
		Defaults: ResourceConfig{
			Cores:    DefaultCores,
			MemoryMB: DefaultMemoryMB,
			DiskGB:   DefaultDiskGB,
		},
		Security: SecurityConfig{
			UnprivilegedOnly: true,
			AllowedFeatures:  []string{"nesting"},
		},
		Service: ServiceConfig{
			BindAddress: DefaultBindAddress,
			Port:        DefaultPort,
		},
		Auth: AuthConfig{
			Mode: AuthModeNone,
		},
		Proxmox: ProxmoxConfig{
			AutoCreated: true,
			TokenID:     "appstore@pve!appstore",
			TokenSecret: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
			BaseURL:     "https://localhost:8006",
		},
		Catalog: CatalogConfig{
			URL:     DefaultCatalogURL,
			Branch:  DefaultCatalogBranch,
			Refresh: RefreshDaily,
		},
		GPU: GPUConfig{
			Enabled: false,
			Policy:  GPUPolicyNone,
		},
	}
}

func TestValidateValid(t *testing.T) {
	cfg := validConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateMissingNodeName(t *testing.T) {
	cfg := validConfig()
	cfg.NodeName = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for missing node_name")
	}
}

func TestValidateMissingPool(t *testing.T) {
	cfg := validConfig()
	cfg.Pool = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for missing pool")
	}
}

func TestValidateMissingStorage(t *testing.T) {
	cfg := validConfig()
	cfg.Storage = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for missing storage")
	}
}

func TestValidateMissingBridge(t *testing.T) {
	cfg := validConfig()
	cfg.Bridge = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for missing bridge")
	}
}

func TestValidateLowCores(t *testing.T) {
	cfg := validConfig()
	cfg.Defaults.Cores = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for cores < 1")
	}
}

func TestValidateLowMemory(t *testing.T) {
	cfg := validConfig()
	cfg.Defaults.MemoryMB = 64
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for memory_mb < 128")
	}
}

func TestValidateLowDisk(t *testing.T) {
	cfg := validConfig()
	cfg.Defaults.DiskGB = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for disk_gb < 1")
	}
}

func TestValidatePortRange(t *testing.T) {
	cfg := validConfig()

	cfg.Service.Port = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for port 0")
	}

	cfg.Service.Port = 70000
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for port > 65535")
	}
}

func TestValidateInvalidAuthMode(t *testing.T) {
	cfg := validConfig()
	cfg.Auth.Mode = "oauth"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for invalid auth mode")
	}
}

func TestValidateInvalidRefresh(t *testing.T) {
	cfg := validConfig()
	cfg.Catalog.Refresh = "hourly"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for invalid refresh schedule")
	}
}

func TestValidateInvalidGPUPolicy(t *testing.T) {
	cfg := validConfig()
	cfg.GPU.Policy = "deny"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for invalid GPU policy")
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "config.yml")

	cfg := validConfig()
	if err := cfg.Save(path); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Verify file exists
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("config file not created: %v", err)
	}
	if info.Mode().Perm() != 0640 {
		t.Fatalf("expected 0640 permissions, got %o", info.Mode().Perm())
	}

	// Load it back
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	// Spot-check fields
	if loaded.NodeName != cfg.NodeName {
		t.Errorf("node_name: got %q, want %q", loaded.NodeName, cfg.NodeName)
	}
	if loaded.Pool != cfg.Pool {
		t.Errorf("pool: got %q, want %q", loaded.Pool, cfg.Pool)
	}
	if loaded.Defaults.Cores != cfg.Defaults.Cores {
		t.Errorf("cores: got %d, want %d", loaded.Defaults.Cores, cfg.Defaults.Cores)
	}
	if loaded.Service.Port != cfg.Service.Port {
		t.Errorf("port: got %d, want %d", loaded.Service.Port, cfg.Service.Port)
	}
	if loaded.Auth.Mode != cfg.Auth.Mode {
		t.Errorf("auth.mode: got %q, want %q", loaded.Auth.Mode, cfg.Auth.Mode)
	}
	if loaded.GPU.Policy != cfg.GPU.Policy {
		t.Errorf("gpu.policy: got %q, want %q", loaded.GPU.Policy, cfg.GPU.Policy)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yml")
	os.WriteFile(path, []byte("{{invalid yaml"), 0644)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}
