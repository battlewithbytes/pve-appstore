package engine

import (
	"strings"
	"testing"
)

// --- buildProvisionCommand ---

func TestBuildProvisionCommandBasic(t *testing.T) {
	cmd := buildProvisionCommand("install.py", "install", nil)

	// Should start with env and end with runner args
	if cmd[0] != "env" {
		t.Errorf("cmd[0] = %q, want %q", cmd[0], "env")
	}

	// Should contain python3 -u -m appstore.runner
	joined := strings.Join(cmd, " ")
	if !strings.Contains(joined, "python3 -u -m appstore.runner") {
		t.Errorf("cmd missing runner: %v", cmd)
	}

	// Should end with action and script
	if cmd[len(cmd)-2] != "install" {
		t.Errorf("action = %q, want %q", cmd[len(cmd)-2], "install")
	}
	if !strings.HasSuffix(cmd[len(cmd)-1], "install.py") {
		t.Errorf("script = %q, want suffix install.py", cmd[len(cmd)-1])
	}
}

func TestBuildProvisionCommandWithEnv(t *testing.T) {
	env := map[string]string{"DB_HOST": "localhost"}
	cmd := buildProvisionCommand("install.py", "install", env)

	found := false
	for _, arg := range cmd {
		if arg == "DB_HOST=localhost" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("cmd %v missing DB_HOST=localhost", cmd)
	}
}

func TestBuildProvisionCommandEmptyEnv(t *testing.T) {
	cmd := buildProvisionCommand("install.py", "install", map[string]string{})

	// Should still work fine, just no extra env vars
	if cmd[0] != "env" {
		t.Errorf("cmd[0] = %q, want %q", cmd[0], "env")
	}
	joined := strings.Join(cmd, " ")
	if !strings.Contains(joined, "install") {
		t.Errorf("cmd missing install action: %v", cmd)
	}
}

// --- buildStackProvisionCommand ---

func TestBuildStackProvisionCommandBasic(t *testing.T) {
	cmd := buildStackProvisionCommand("nginx", "install.py", "install", nil)

	if cmd[0] != "env" {
		t.Errorf("cmd[0] = %q, want %q", cmd[0], "env")
	}

	joined := strings.Join(cmd, " ")
	// Should use per-app paths
	if !strings.Contains(joined, "/opt/appstore/provision/nginx/install.py") {
		t.Errorf("cmd missing app-specific script path: %v", cmd)
	}
	if !strings.Contains(joined, "/opt/appstore/nginx/inputs.json") {
		t.Errorf("cmd missing app-specific inputs path: %v", cmd)
	}
}

func TestBuildStackProvisionCommandWithEnv(t *testing.T) {
	env := map[string]string{"REDIS_URL": "redis://host"}
	cmd := buildStackProvisionCommand("redis", "install.py", "install", env)

	found := false
	for _, arg := range cmd {
		if arg == "REDIS_URL=redis://host" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("cmd %v missing REDIS_URL env var", cmd)
	}
}
