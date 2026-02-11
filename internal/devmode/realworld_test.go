package devmode

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// testDockerfile defines a real-world Dockerfile test case.
type testDockerfile struct {
	name     string // app name for converter
	file     string // path to Dockerfile (empty = inline content)
	content  string // inline content (used if file is empty)
	wantOS   string // expected BaseOS
	wantPkgs int    // minimum expected package count (0 = skip check)
	wantPip  int    // minimum expected pip package count
	wantPort int    // minimum expected port count
}

// readOrInline reads the file or returns inline content.
func (td testDockerfile) dockerfile() string {
	if td.file != "" {
		data, err := os.ReadFile(td.file)
		if err != nil {
			return "" // file not found — test will handle
		}
		return string(data)
	}
	return td.content
}

func TestRealWorldDockerfilesSuite(t *testing.T) {
	cases := []testDockerfile{
		{
			name:     "Plex Media Server",
			file:     "/tmp/dockerfiles/plex.Dockerfile",
			wantOS:   "debian",
			wantPkgs: 1,
			wantPort: 2,
		},
		{
			name:     "Sonarr",
			file:     "/tmp/dockerfiles/sonarr.Dockerfile",
			wantOS:   "alpine",
			wantPkgs: 2,
			wantPort: 1,
		},
		{
			name:     "Radarr",
			file:     "/tmp/dockerfiles/radarr.Dockerfile",
			wantOS:   "alpine",
			wantPkgs: 2,
			wantPort: 1,
		},
		{
			name:     "Jellyfin",
			file:     "/tmp/dockerfiles/jellyfin.Dockerfile",
			wantOS:   "debian",
			wantPkgs: 2,
			wantPort: 2,
		},
		{
			name:     "Home Assistant",
			file:     "/tmp/dockerfiles/homeassistant.Dockerfile",
			wantOS:   "alpine",
			wantPkgs: 10,
			wantPort: 1,
		},
		{
			name:     "Nextcloud",
			file:     "/tmp/dockerfiles/nextcloud.Dockerfile",
			wantOS:   "alpine",
			wantPkgs: 5,
			wantPort: 2,
		},
		{
			name:     "Nginx",
			file:     "/tmp/dockerfiles/nginx.Dockerfile",
			wantOS:   "alpine",
			wantPkgs: 5,
			wantPort: 2,
		},
		{
			name:     "WireGuard",
			file:     "/tmp/dockerfiles/wireguard.Dockerfile",
			wantOS:   "alpine",
			wantPkgs: 5,
			wantPort: 1,
		},
		{
			name:     "Pi-hole",
			file:     "/tmp/dockerfiles/pihole.Dockerfile",
			wantOS:   "alpine", // Resolved: ARG FTL_SOURCE=remote → FROM ${FTL_SOURCE}-ftl-install → remote-ftl-install → base → alpine:3.22
			wantPkgs: 5,
			wantPort: 2,
		},
		{
			name:     "Portainer",
			file:     "/tmp/dockerfiles/portainer.Dockerfile",
			wantOS:   "unknown", // FROM portainer/base — unknown custom image
			wantPort: 3,
		},
		{
			name:   "Grafana",
			file:   "/tmp/dockerfiles/grafana.Dockerfile",
			wantOS: "alpine", // Resolved: ARG BASE_IMAGE=alpine-base → FROM alpine:3.23.3 AS alpine-base → alpine
		},
		{
			name:   "Ollama",
			file:   "/tmp/dockerfiles/ollama.Dockerfile",
			wantOS: "debian", // Final FROM is ubuntu:24.04 (in the runtime stage)
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			content := tc.dockerfile()
			if content == "" {
				t.Skipf("Dockerfile not found: %s", tc.file)
				return
			}

			// Parse
			df := ParseDockerfile(content)

			// Check OS detection
			if tc.wantOS != "" && df.BaseOS != tc.wantOS {
				t.Errorf("BaseOS: got %q, want %q (BaseImage: %s)", df.BaseOS, tc.wantOS, df.BaseImage)
			}

			// Check minimum packages
			if tc.wantPkgs > 0 && len(df.Packages) < tc.wantPkgs {
				t.Errorf("Packages: got %d, want >= %d: %v", len(df.Packages), tc.wantPkgs, df.Packages)
			}

			// Check minimum pip packages
			if tc.wantPip > 0 && len(df.PipPackages) < tc.wantPip {
				t.Errorf("PipPackages: got %d, want >= %d: %v", len(df.PipPackages), tc.wantPip, df.PipPackages)
			}

			// Check minimum ports
			if tc.wantPort > 0 && len(df.Ports) < tc.wantPort {
				t.Errorf("Ports: got %d, want >= %d: %v", len(df.Ports), tc.wantPort, df.Ports)
			}

			// Convert to scaffold
			id, manifest, script := ConvertDockerfileToScaffold(tc.name, df)

			// Basic sanity checks
			if id == "" {
				t.Error("ConvertDockerfileToScaffold returned empty id")
			}
			if manifest == "" {
				t.Error("ConvertDockerfileToScaffold returned empty manifest")
			}
			if script == "" {
				t.Error("ConvertDockerfileToScaffold returned empty script")
			}

			// Manifest should have app name
			if !strings.Contains(manifest, fmt.Sprintf(`name: "%s"`, tc.name)) {
				t.Errorf("manifest missing app name %q", tc.name)
			}

			// Script should compile (has class + BaseApp + run)
			if !strings.Contains(script, "BaseApp") {
				t.Error("script missing BaseApp")
			}
			if !strings.Contains(script, "def install(self") {
				t.Error("script missing install method")
			}

			// Service inference should not pick a library/build package
			svc := inferMainService(id, df.Packages)
			badPrefixes := []string{"lib", "python3-", "qtbase", "qtscript", "qttools"}
			badSuffixes := []string{"-dev", "-devel", "-libs"}
			for _, pfx := range badPrefixes {
				if strings.HasPrefix(svc, pfx) {
					t.Errorf("service %q starts with utility prefix %q", svc, pfx)
				}
			}
			for _, sfx := range badSuffixes {
				if strings.HasSuffix(svc, sfx) {
					t.Errorf("service %q ends with utility suffix %q", svc, sfx)
				}
			}

			// Log results for review
			t.Logf("ID: %s", id)
			t.Logf("BaseOS: %s (image: %s)", df.BaseOS, df.BaseImage)
			t.Logf("Packages (%d): %v", len(df.Packages), df.Packages)
			if len(df.PipPackages) > 0 {
				t.Logf("Pip (%d): %v", len(df.PipPackages), df.PipPackages)
			}
			t.Logf("Ports: %v", df.Ports)
			t.Logf("Volumes: %v", df.Volumes)
			if len(df.AptKeys) > 0 {
				t.Logf("AptKeys: %d", len(df.AptKeys))
			}
			if len(df.AptRepos) > 0 {
				t.Logf("AptRepos: %d", len(df.AptRepos))
			}
			t.Logf("Service: %s", svc)
			t.Logf("ExecCmd: %s", df.ExecCmd)
		})
	}
}
