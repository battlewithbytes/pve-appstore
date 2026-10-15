package engine

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/battlewithbytes/pve-appstore/internal/catalog"
	"github.com/battlewithbytes/pve-appstore/internal/config"
)

func newUpdateTestEngine(t *testing.T) *Engine {
	t.Helper()
	store, err := NewStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return &Engine{
		cfg:     &config.Config{NodeName: "zeus"},
		catalog: catalog.New("", "", t.TempDir()),
		store:   store,
	}
}

func createTestInstall(t *testing.T, e *Engine, id, status string) {
	t.Helper()
	inst := &Install{
		ID: id, AppID: "missing-app", AppName: "Missing", AppVersion: "1.0.0",
		CTID: 100, Node: "zeus", Pool: "appstore", Status: status, CreatedAt: time.Now(),
	}
	if err := e.store.CreateInstall(inst); err != nil {
		t.Fatalf("CreateInstall: %v", err)
	}
}

func TestUpdateRejectsUninstalled(t *testing.T) {
	e := newUpdateTestEngine(t)
	createTestInstall(t, e, "inst-gone", "uninstalled")

	_, err := e.Update("inst-gone", ReinstallRequest{})
	if err == nil || !strings.Contains(err.Error(), "use reinstall instead") {
		t.Fatalf("Update(uninstalled) err = %v, want 'use reinstall instead'", err)
	}
}

func TestUpdateAcceptsActiveInstalls(t *testing.T) {
	for _, status := range []string{"running", "stopped"} {
		t.Run(status, func(t *testing.T) {
			e := newUpdateTestEngine(t)
			createTestInstall(t, e, "inst-"+status, status)

			// The app isn't in the catalog, so Update must get past the status
			// guard and fail on the catalog lookup instead.
			_, err := e.Update("inst-"+status, ReinstallRequest{})
			if err == nil || !strings.Contains(err.Error(), "not found in catalog") {
				t.Fatalf("Update(%s) err = %v, want catalog lookup error", status, err)
			}
		})
	}
}

func TestNewUpdateJobCarriesInstallIdentity(t *testing.T) {
	e := newUpdateTestEngine(t)
	app := &catalog.AppManifest{
		ID:      "gitlab",
		Version: "1.1.4",
		Inputs: []catalog.InputSpec{
			{Key: "gitlab_port", Default: 80},
			{Key: "ssh_port", Default: 22},
			{Key: "new_option", Default: "on"},
		},
	}
	inst := &Install{
		ID: "inst-1", AppID: "gitlab", AppVersion: "1.1.3", Storage: "fast-pool", Bridge: "vmbr0",
		Cores: 4, MemoryMB: 8192, DiskGB: 20,
		Hostname: "gitlab", IPAddress: "192.168.1.117/24", MACAddress: "BC:24:11:37:DC:92",
		OnBoot: true, Unprivileged: true,
		Inputs:      map[string]string{"gitlab_port": "8080", "ssh_port": "2222"},
		MountPoints: []MountPoint{{Index: 1, Name: "data", Type: "volume", MountPath: "/var/opt/gitlab"}},
		Devices:     []DevicePassthrough{{Path: "/dev/dri/renderD128"}},
		EnvVars:     map[string]string{"TZ": "America/New_York"},
		CPUPin:      "2-5",
	}

	job := e.newUpdateJob(inst, app, ReinstallRequest{Cores: 8, Inputs: map[string]string{"ssh_port": "2022"}})

	if job.Hostname != "gitlab" || job.IPAddress != inst.IPAddress || job.MACAddress != inst.MACAddress {
		t.Errorf("network identity not carried: hostname=%q ip=%q mac=%q", job.Hostname, job.IPAddress, job.MACAddress)
	}
	if !job.OnBoot || !job.Unprivileged {
		t.Errorf("onboot=%v unprivileged=%v, want both true", job.OnBoot, job.Unprivileged)
	}
	if job.Cores != 8 || job.MemoryMB != 8192 || job.Storage != "fast-pool" {
		t.Errorf("sizing: cores=%d mem=%d storage=%q", job.Cores, job.MemoryMB, job.Storage)
	}
	want := map[string]string{"gitlab_port": "8080", "ssh_port": "2022", "new_option": "on"}
	for k, v := range want {
		if job.Inputs[k] != v {
			t.Errorf("input %s = %q, want %q", k, job.Inputs[k], v)
		}
	}
	if len(job.MountPoints) != 1 || len(job.Devices) != 1 || job.EnvVars["TZ"] == "" || job.CPUPin != "2-5" {
		t.Errorf("mounts/devices/env/cpupin not carried: %+v %+v %+v %q", job.MountPoints, job.Devices, job.EnvVars, job.CPUPin)
	}
	if inst.Inputs["ssh_port"] != "2222" {
		t.Errorf("newUpdateJob mutated the install's inputs: %v", inst.Inputs)
	}
}
