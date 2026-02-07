package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validManifestYAML() string {
	return `id: test-app
name: Test App
description: A test application for unit tests
version: 1.0.0
categories:
  - testing
tags:
  - test
  - example
lxc:
  ostemplate: debian-12
  defaults:
    unprivileged: true
    cores: 1
    memory_mb: 512
    disk_gb: 4
    features:
      - nesting
inputs:
  - key: domain
    label: Domain Name
    type: string
    required: true
    help: The domain name for the app
  - key: port
    label: Port
    type: number
    default: 8080
    required: false
permissions:
  packages:
    - nginx
  paths:
    - /var/www/
    - /etc/nginx/
  services:
    - nginx
provisioning:
  script: provision/install.py
  timeout_sec: 600
outputs:
  - key: url
    label: App URL
    value: "http://{{ip}}:{{port}}"
gpu:
  supported:
    - intel
    - nvidia
  required: false
  profiles:
    - dri-render
`
}

func writeManifest(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "app.yml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseAndValidateValid(t *testing.T) {
	dir := t.TempDir()
	path := writeManifest(t, dir, validManifestYAML())

	m, err := ParseManifest(path)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if err := m.Validate(); err != nil {
		t.Fatalf("validate failed: %v", err)
	}

	if m.ID != "test-app" {
		t.Errorf("id: got %q, want %q", m.ID, "test-app")
	}
	if m.Name != "Test App" {
		t.Errorf("name: got %q, want %q", m.Name, "Test App")
	}
	if m.LXC.Defaults.Cores != 1 {
		t.Errorf("cores: got %d, want 1", m.LXC.Defaults.Cores)
	}
	if len(m.Inputs) != 2 {
		t.Errorf("inputs: got %d, want 2", len(m.Inputs))
	}
	if m.GPU.Required != false {
		t.Error("gpu.required should be false")
	}
	if len(m.GPU.Supported) != 2 {
		t.Errorf("gpu.supported: got %d, want 2", len(m.GPU.Supported))
	}
}

func TestValidateMissingID(t *testing.T) {
	m := &AppManifest{Name: "X", Description: "X", Version: "1.0.0"}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for missing id")
	}
}

func TestValidateNonKebabID(t *testing.T) {
	m := &AppManifest{ID: "TestApp", Name: "X", Description: "X", Version: "1.0.0"}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for non-kebab-case id")
	}
}

func TestValidateMissingName(t *testing.T) {
	m := &AppManifest{ID: "test-app", Description: "X", Version: "1.0.0"}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestValidateMissingVersion(t *testing.T) {
	m := &AppManifest{ID: "test-app", Name: "X", Description: "X"}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for missing version")
	}
}

func TestValidateMissingCategories(t *testing.T) {
	m := &AppManifest{
		ID: "test-app", Name: "X", Description: "X", Version: "1.0.0",
	}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for missing categories")
	}
}

func TestValidateLowMemory(t *testing.T) {
	m := &AppManifest{
		ID: "test-app", Name: "X", Description: "X", Version: "1.0.0",
		Categories: []string{"test"},
		LXC: LXCConfig{
			OSTemplate: "debian-12",
			Defaults:   LXCDefaults{Cores: 1, MemoryMB: 64, DiskGB: 4},
		},
		Provisioning: ProvisioningSpec{Script: "install.sh"},
	}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for memory < 128")
	}
}

func TestValidateInvalidInputType(t *testing.T) {
	m := &AppManifest{
		ID: "test-app", Name: "X", Description: "X", Version: "1.0.0",
		Categories: []string{"test"},
		LXC: LXCConfig{
			OSTemplate: "debian-12",
			Defaults:   LXCDefaults{Cores: 1, MemoryMB: 512, DiskGB: 4},
		},
		Inputs: []InputSpec{
			{Key: "x", Label: "X", Type: "invalid"},
		},
		Provisioning: ProvisioningSpec{Script: "install.sh"},
	}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for invalid input type")
	}
}

func TestValidateInvalidGPUType(t *testing.T) {
	m := &AppManifest{
		ID: "test-app", Name: "X", Description: "X", Version: "1.0.0",
		Categories: []string{"test"},
		LXC: LXCConfig{
			OSTemplate: "debian-12",
			Defaults:   LXCDefaults{Cores: 1, MemoryMB: 512, DiskGB: 4},
		},
		Provisioning: ProvisioningSpec{Script: "install.sh"},
		GPU:          GPUSpec{Supported: []string{"tpu"}},
	}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for invalid GPU type")
	}
}

func TestValidateMissingProvisioningScript(t *testing.T) {
	m := &AppManifest{
		ID: "test-app", Name: "X", Description: "X", Version: "1.0.0",
		Categories: []string{"test"},
		LXC: LXCConfig{
			OSTemplate: "debian-12",
			Defaults:   LXCDefaults{Cores: 1, MemoryMB: 512, DiskGB: 4},
		},
	}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for missing provisioning script")
	}
}

func TestValidateShellScriptRejected(t *testing.T) {
	m := &AppManifest{
		ID: "test-app", Name: "X", Description: "X", Version: "1.0.0",
		Categories: []string{"test"},
		LXC: LXCConfig{
			OSTemplate: "debian-12",
			Defaults:   LXCDefaults{Cores: 1, MemoryMB: 512, DiskGB: 4},
		},
		Provisioning: ProvisioningSpec{Script: "provision/install.sh"},
	}
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for .sh provisioning script")
	}
	if !strings.Contains(err.Error(), ".py") {
		t.Errorf("error %q should mention .py requirement", err.Error())
	}
}

func TestValidatePythonScriptAccepted(t *testing.T) {
	m := &AppManifest{
		ID: "test-app", Name: "X", Description: "X", Version: "1.0.0",
		Categories: []string{"test"},
		LXC: LXCConfig{
			OSTemplate: "debian-12",
			Defaults:   LXCDefaults{Cores: 1, MemoryMB: 512, DiskGB: 4},
		},
		Provisioning: ProvisioningSpec{Script: "provision/install.py"},
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("expected valid manifest: %v", err)
	}
}

func TestPermissionsSpecParsed(t *testing.T) {
	dir := t.TempDir()
	path := writeManifest(t, dir, validManifestYAML())

	m, err := ParseManifest(path)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if len(m.Permissions.Packages) != 1 || m.Permissions.Packages[0] != "nginx" {
		t.Errorf("permissions.packages: got %v, want [nginx]", m.Permissions.Packages)
	}
	if len(m.Permissions.Paths) != 2 {
		t.Errorf("permissions.paths: got %v, want 2 entries", m.Permissions.Paths)
	}
	if len(m.Permissions.Services) != 1 || m.Permissions.Services[0] != "nginx" {
		t.Errorf("permissions.services: got %v, want [nginx]", m.Permissions.Services)
	}
}

func TestCatalogLoadLocal(t *testing.T) {
	// Create a mini catalog structure
	dir := t.TempDir()
	appsDir := filepath.Join(dir, "apps", "test-app")
	os.MkdirAll(appsDir, 0755)
	writeManifest(t, appsDir, validManifestYAML())

	cat := New("", "", "")
	if err := cat.LoadLocal(dir); err != nil {
		t.Fatalf("LoadLocal failed: %v", err)
	}

	if cat.AppCount() != 1 {
		t.Fatalf("expected 1 app, got %d", cat.AppCount())
	}

	app, ok := cat.Get("test-app")
	if !ok {
		t.Fatal("expected to find test-app")
	}
	if app.Name != "Test App" {
		t.Errorf("name: got %q, want %q", app.Name, "Test App")
	}
}

func TestCatalogSearch(t *testing.T) {
	dir := t.TempDir()
	appsDir := filepath.Join(dir, "apps", "test-app")
	os.MkdirAll(appsDir, 0755)
	writeManifest(t, appsDir, validManifestYAML())

	cat := New("", "", "")
	cat.LoadLocal(dir)

	// Search by name
	results := cat.Search("Test")
	if len(results) != 1 {
		t.Errorf("search 'Test': got %d, want 1", len(results))
	}

	// Search by tag
	results = cat.Search("example")
	if len(results) != 1 {
		t.Errorf("search 'example': got %d, want 1", len(results))
	}

	// No match
	results = cat.Search("nonexistent")
	if len(results) != 0 {
		t.Errorf("search 'nonexistent': got %d, want 0", len(results))
	}
}

func TestCatalogFilter(t *testing.T) {
	dir := t.TempDir()
	appsDir := filepath.Join(dir, "apps", "test-app")
	os.MkdirAll(appsDir, 0755)
	writeManifest(t, appsDir, validManifestYAML())

	cat := New("", "", "")
	cat.LoadLocal(dir)

	results := cat.Filter("testing")
	if len(results) != 1 {
		t.Errorf("filter 'testing': got %d, want 1", len(results))
	}

	results = cat.Filter("media")
	if len(results) != 0 {
		t.Errorf("filter 'media': got %d, want 0", len(results))
	}
}

func TestCatalogCategories(t *testing.T) {
	dir := t.TempDir()
	appsDir := filepath.Join(dir, "apps", "test-app")
	os.MkdirAll(appsDir, 0755)
	writeManifest(t, appsDir, validManifestYAML())

	cat := New("", "", "")
	cat.LoadLocal(dir)

	cats := cat.Categories()
	if len(cats) != 1 || cats[0] != "testing" {
		t.Errorf("categories: got %v, want [testing]", cats)
	}
}
