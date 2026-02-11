package devmode

import (
	"fmt"
	"testing"
)

// mockFetcher maps URL templates to Dockerfile content for testing.
type mockFetcher struct {
	files map[string]string // urlTmpl -> content
}

func (m *mockFetcher) FetchDockerfile(urlTmpl, branch string) (content, usedURL string, err error) {
	if c, ok := m.files[urlTmpl]; ok {
		return c, urlTmpl, nil
	}
	return "", "", fmt.Errorf("not found: %s", urlTmpl)
}

func TestResolveChain_SingleLayer(t *testing.T) {
	fetcher := &mockFetcher{files: map[string]string{}}
	info := ResolveDockerfileChain(fetcher, "FROM alpine:3.23\nRUN apk add --no-cache curl\n", 5, nil)

	if len(info.Packages) != 1 || info.Packages[0] != "curl" {
		t.Errorf("expected [curl], got %v", info.Packages)
	}
	if info.BaseOS != "alpine" {
		t.Errorf("expected alpine, got %q", info.BaseOS)
	}
}

func TestResolveChain_TwoLayers(t *testing.T) {
	// Parent: baseimage-alpine has bash and coreutils
	parentDF := `FROM alpine:3.23
RUN apk add --no-cache bash coreutils curl shadow
`
	fetcher := &mockFetcher{files: map[string]string{
		"https://raw.githubusercontent.com/linuxserver/docker-baseimage-alpine/{branch}/Dockerfile": parentDF,
	}}

	appDF := `FROM ghcr.io/linuxserver/baseimage-alpine:3.23
RUN apk add --no-cache nginx fail2ban
EXPOSE 80 443
`
	info := ResolveDockerfileChain(fetcher, appDF, 5, nil)

	// Should have packages from both layers
	pkgSet := map[string]bool{}
	for _, p := range info.Packages {
		pkgSet[p] = true
	}
	for _, expected := range []string{"bash", "coreutils", "curl", "shadow", "nginx", "fail2ban"} {
		if !pkgSet[expected] {
			t.Errorf("missing package %q, got %v", expected, info.Packages)
		}
	}

	// Ports from app layer
	if len(info.Ports) != 2 {
		t.Errorf("expected 2 ports, got %d: %v", len(info.Ports), info.Ports)
	}

	// BaseOS from deepest parent
	if info.BaseOS != "alpine" {
		t.Errorf("expected alpine, got %q", info.BaseOS)
	}
}

func TestResolveChain_ThreeLayers(t *testing.T) {
	// SWAG pattern: app -> nginx-base -> alpine-base -> alpine
	alpineBaseDF := `FROM alpine:3.23
RUN apk add --no-cache bash coreutils curl shadow tzdata
`
	nginxBaseDF := `FROM ghcr.io/linuxserver/baseimage-alpine:3.23
RUN apk add --no-cache nginx nginx-mod-http-brotli apache2-utils
EXPOSE 80 443
`
	fetcher := &mockFetcher{files: map[string]string{
		"https://raw.githubusercontent.com/linuxserver/docker-baseimage-alpine/{branch}/Dockerfile":       alpineBaseDF,
		"https://raw.githubusercontent.com/linuxserver/docker-baseimage-alpine-nginx/{branch}/Dockerfile": nginxBaseDF,
	}}

	swagDF := `FROM ghcr.io/linuxserver/baseimage-alpine-nginx:3.22
RUN apk add --no-cache certbot fail2ban nginx-mod-http-geoip2
EXPOSE 80 443
VOLUME /config
`
	info := ResolveDockerfileChain(fetcher, swagDF, 5, nil)

	pkgSet := map[string]bool{}
	for _, p := range info.Packages {
		pkgSet[p] = true
	}

	// From alpine-base
	for _, expected := range []string{"bash", "coreutils", "curl", "shadow", "tzdata"} {
		if !pkgSet[expected] {
			t.Errorf("missing alpine-base package %q", expected)
		}
	}
	// From nginx-base
	for _, expected := range []string{"nginx", "nginx-mod-http-brotli", "apache2-utils"} {
		if !pkgSet[expected] {
			t.Errorf("missing nginx-base package %q", expected)
		}
	}
	// From app
	for _, expected := range []string{"certbot", "fail2ban", "nginx-mod-http-geoip2"} {
		if !pkgSet[expected] {
			t.Errorf("missing app package %q", expected)
		}
	}

	// Total should be at least 11 unique packages
	if len(info.Packages) < 11 {
		t.Errorf("expected at least 11 merged packages, got %d: %v", len(info.Packages), info.Packages)
	}
}

func TestResolveChain_FetchFailure(t *testing.T) {
	// Parent URL returns error — chain stops, returns app-layer packages only
	fetcher := &mockFetcher{files: map[string]string{}}

	appDF := `FROM ghcr.io/linuxserver/baseimage-alpine:3.23
RUN apk add --no-cache nginx
EXPOSE 80
`
	var events []ChainEvent
	info := ResolveDockerfileChain(fetcher, appDF, 5, func(e ChainEvent) {
		events = append(events, e)
	})

	if len(info.Packages) != 1 || info.Packages[0] != "nginx" {
		t.Errorf("expected [nginx] from app layer only, got %v", info.Packages)
	}

	// Should have an error event
	hasError := false
	for _, e := range events {
		if e.Type == "error" {
			hasError = true
		}
	}
	if !hasError {
		t.Error("expected an error event for fetch failure")
	}
}

func TestResolveChain_MaxDepth(t *testing.T) {
	// Circular reference: A -> B -> A (would loop forever without depth limit)
	fetcher := &mockFetcher{files: map[string]string{
		"https://raw.githubusercontent.com/linuxserver/docker-app-a/{branch}/Dockerfile": "FROM ghcr.io/linuxserver/app-b:latest\nRUN apk add --no-cache pkg-a\n",
		"https://raw.githubusercontent.com/linuxserver/docker-app-b/{branch}/Dockerfile": "FROM ghcr.io/linuxserver/app-a:latest\nRUN apk add --no-cache pkg-b\n",
	}}

	appDF := `FROM ghcr.io/linuxserver/app-a:latest
RUN apk add --no-cache my-app
`
	info := ResolveDockerfileChain(fetcher, appDF, 3, nil)

	// Should not panic or infinite loop
	if info == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestResolveChain_Events(t *testing.T) {
	parentDF := `FROM alpine:3.23
RUN apk add --no-cache bash curl
`
	fetcher := &mockFetcher{files: map[string]string{
		"https://raw.githubusercontent.com/linuxserver/docker-baseimage-alpine/{branch}/Dockerfile": parentDF,
	}}

	appDF := `FROM ghcr.io/linuxserver/baseimage-alpine:3.23
RUN apk add --no-cache nginx
EXPOSE 80
`
	var events []ChainEvent
	ResolveDockerfileChain(fetcher, appDF, 5, func(e ChainEvent) {
		events = append(events, e)
	})

	// Expected event sequence: parsed(app), fetching(parent), parsed(parent), terminal, merged
	if len(events) < 4 {
		t.Fatalf("expected at least 4 events, got %d: %+v", len(events), events)
	}

	expectedTypes := []string{"parsed", "fetching", "parsed", "terminal", "merged"}
	for i, expected := range expectedTypes {
		if i >= len(events) {
			t.Errorf("missing event[%d]: expected type %q", i, expected)
			continue
		}
		if events[i].Type != expected {
			t.Errorf("event[%d]: expected type %q, got %q (%s)", i, expected, events[i].Type, events[i].Message)
		}
	}
}

func TestMergeChain(t *testing.T) {
	parent := &DockerfileInfo{
		BaseOS:      "alpine",
		BaseImage:   "alpine:3.23",
		Packages:    []string{"bash", "curl"},
		Ports:       []string{"80"},
		Volumes:     []string{"/config"},
		EnvVars:     []EnvVar{{Key: "FOO", Default: "parent"}, {Key: "BAR", Default: "parent"}},
		PipPackages: []string{"requests"},
	}
	child := &DockerfileInfo{
		BaseOS:      "alpine",
		BaseImage:   "ghcr.io/linuxserver/myapp:latest",
		Packages:    []string{"curl", "nginx"}, // curl overlaps
		Ports:       []string{"80", "443"},      // 80 overlaps
		Volumes:     []string{"/config", "/data"},
		EnvVars:     []EnvVar{{Key: "BAR", Default: "child"}, {Key: "BAZ", Default: "child"}},
		PipPackages: []string{"requests", "flask"},
		ExecCmd:     "/usr/bin/myapp",
	}

	merged := MergeDockerfileInfoChain([]*DockerfileInfo{parent, child})

	// BaseOS from parent
	if merged.BaseOS != "alpine" {
		t.Errorf("expected BaseOS=alpine, got %q", merged.BaseOS)
	}

	// BaseImage from child (app layer)
	if merged.BaseImage != "ghcr.io/linuxserver/myapp:latest" {
		t.Errorf("expected BaseImage from child, got %q", merged.BaseImage)
	}

	// ExecCmd from child
	if merged.ExecCmd != "/usr/bin/myapp" {
		t.Errorf("expected ExecCmd from child, got %q", merged.ExecCmd)
	}

	// Packages: deduped, parent first
	if len(merged.Packages) != 3 {
		t.Errorf("expected 3 packages (bash, curl, nginx), got %d: %v", len(merged.Packages), merged.Packages)
	}
	// curl should appear only once
	curlCount := 0
	for _, p := range merged.Packages {
		if p == "curl" {
			curlCount++
		}
	}
	if curlCount != 1 {
		t.Errorf("curl appeared %d times, expected 1", curlCount)
	}

	// Ports: union
	if len(merged.Ports) != 2 {
		t.Errorf("expected 2 ports (80, 443), got %d: %v", len(merged.Ports), merged.Ports)
	}

	// Volumes: union
	if len(merged.Volumes) != 2 {
		t.Errorf("expected 2 volumes (/config, /data), got %d: %v", len(merged.Volumes), merged.Volumes)
	}

	// EnvVars: child overrides parent for BAR
	envMap := map[string]string{}
	for _, ev := range merged.EnvVars {
		envMap[ev.Key] = ev.Default
	}
	if envMap["FOO"] != "parent" {
		t.Errorf("FOO should be 'parent', got %q", envMap["FOO"])
	}
	if envMap["BAR"] != "child" {
		t.Errorf("BAR should be 'child' (child overrides), got %q", envMap["BAR"])
	}
	if envMap["BAZ"] != "child" {
		t.Errorf("BAZ should be 'child', got %q", envMap["BAZ"])
	}

	// PipPackages: deduped
	if len(merged.PipPackages) != 2 {
		t.Errorf("expected 2 pip packages, got %d: %v", len(merged.PipPackages), merged.PipPackages)
	}
}

func TestIsTerminalBaseImage(t *testing.T) {
	tests := []struct {
		image    string
		terminal bool
	}{
		{"alpine:3.23", true},
		{"ubuntu:noble", true},
		{"debian:12", true},
		{"python:3.11-slim", true},
		{"node:20-alpine", true},
		{"scratch", true},
		{"docker.io/library/alpine:3.23", true},
		{"nginx:latest", true},
		{"redis:7", true},
		{"ghcr.io/linuxserver/baseimage-alpine:3.23", false},
		{"ghcr.io/linuxserver/baseimage-ubuntu:noble", false},
		{"lscr.io/linuxserver/swag:latest", false},
		{"ghcr.io/someuser/myapp:v1", false},
		{"registry.example.com/myimage:latest", false},
	}
	for _, tc := range tests {
		t.Run(tc.image, func(t *testing.T) {
			got := IsTerminalBaseImage(tc.image)
			if got != tc.terminal {
				t.Errorf("IsTerminalBaseImage(%q) = %v, want %v", tc.image, got, tc.terminal)
			}
		})
	}
}

func TestMergeChain_Empty(t *testing.T) {
	merged := MergeDockerfileInfoChain(nil)
	if merged.BaseOS != "unknown" {
		t.Errorf("expected unknown, got %q", merged.BaseOS)
	}
}

func TestMergeChain_Single(t *testing.T) {
	info := &DockerfileInfo{
		BaseOS:   "alpine",
		Packages: []string{"curl"},
	}
	merged := MergeDockerfileInfoChain([]*DockerfileInfo{info})
	if merged != info {
		t.Error("single layer merge should return the same object")
	}
}

func TestResolveChain_NilCallback(t *testing.T) {
	// Verify nil callback doesn't panic
	fetcher := &mockFetcher{files: map[string]string{}}
	info := ResolveDockerfileChain(fetcher, "FROM alpine:3.23\nRUN apk add --no-cache curl\n", 5, nil)
	if info == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestResolveChain_PackageLayers(t *testing.T) {
	alpineBaseDF := `FROM alpine:3.23
RUN apk add --no-cache bash coreutils curl
`
	nginxBaseDF := `FROM ghcr.io/linuxserver/baseimage-alpine:3.23
RUN apk add --no-cache nginx apache2-utils
`
	fetcher := &mockFetcher{files: map[string]string{
		"https://raw.githubusercontent.com/linuxserver/docker-baseimage-alpine/{branch}/Dockerfile":       alpineBaseDF,
		"https://raw.githubusercontent.com/linuxserver/docker-baseimage-alpine-nginx/{branch}/Dockerfile": nginxBaseDF,
	}}

	appDF := `FROM ghcr.io/linuxserver/baseimage-alpine-nginx:3.22
RUN apk add --no-cache certbot fail2ban
`
	info := ResolveDockerfileChain(fetcher, appDF, 5, nil)

	// Should have 3 PackageLayers (alpine-base, nginx-base, app)
	if len(info.PackageLayers) != 3 {
		t.Fatalf("expected 3 PackageLayers, got %d: %+v", len(info.PackageLayers), info.PackageLayers)
	}

	// Layer 0: alpine-base (deepest parent)
	if len(info.PackageLayers[0].Packages) != 3 {
		t.Errorf("layer 0 (alpine-base) expected 3 packages, got %d: %v",
			len(info.PackageLayers[0].Packages), info.PackageLayers[0].Packages)
	}

	// Layer 1: nginx-base
	if len(info.PackageLayers[1].Packages) != 2 {
		t.Errorf("layer 1 (nginx-base) expected 2 packages, got %d: %v",
			len(info.PackageLayers[1].Packages), info.PackageLayers[1].Packages)
	}

	// Layer 2: app
	if len(info.PackageLayers[2].Packages) != 2 {
		t.Errorf("layer 2 (app) expected 2 packages, got %d: %v",
			len(info.PackageLayers[2].Packages), info.PackageLayers[2].Packages)
	}

	// Total should be 7
	if len(info.Packages) != 7 {
		t.Errorf("expected 7 total packages, got %d: %v", len(info.Packages), info.Packages)
	}
}

func TestMergeChain_CopyInstructions(t *testing.T) {
	parent := &DockerfileInfo{
		BaseOS: "debian",
		CopyInstructions: []CopyInstruction{
			{Src: "parent-root/", Dest: "/"},
		},
	}
	child := &DockerfileInfo{
		BaseOS: "debian",
		CopyInstructions: []CopyInstruction{
			{Src: "root/", Dest: "/"},
			{Src: "/app", Dest: "/usr/local/bin/app", FromStage: "builder"},
		},
	}
	merged := MergeDockerfileInfoChain([]*DockerfileInfo{parent, child})
	// Only app layer (child) COPY instructions kept — parent COPYs
	// deploy Docker-specific s6 scripts we don't need in LXC
	if len(merged.CopyInstructions) != 2 {
		t.Errorf("expected 2 COPY instructions (app layer only), got %d", len(merged.CopyInstructions))
	}
}

func TestMergeChain_RepoURL(t *testing.T) {
	parent := &DockerfileInfo{
		BaseOS:  "debian",
		RepoURL: "https://github.com/linuxserver/docker-baseimage-alpine",
	}
	child := &DockerfileInfo{
		BaseOS:  "debian",
		RepoURL: "https://github.com/linuxserver/docker-swag",
	}
	merged := MergeDockerfileInfoChain([]*DockerfileInfo{parent, child})
	if merged.RepoURL != "https://github.com/linuxserver/docker-swag" {
		t.Errorf("expected app layer RepoURL, got %q", merged.RepoURL)
	}
}

func TestMergeChain_NewFields(t *testing.T) {
	parent := &DockerfileInfo{
		BaseOS:      "debian",
		Users:       []string{"root"},
		Directories: []string{"/config"},
		Downloads:   []DownloadAction{{URL: "https://example.com/a", Dest: "/tmp/a"}},
		Symlinks:    []SymlinkAction{{Target: "/a", Link: "/b"}},
		RunCommands: []RunCommand{{Type: "mkdir", Original: "mkdir -p /config"}},
	}
	child := &DockerfileInfo{
		BaseOS:        "debian",
		Users:         []string{"abc", "root"}, // root duplicated
		Directories:   []string{"/config", "/data"},
		Downloads:     []DownloadAction{{URL: "https://example.com/b", Dest: "/tmp/b"}},
		Symlinks:      []SymlinkAction{{Target: "/c", Link: "/d"}},
		RunCommands:   []RunCommand{{Type: "useradd", Original: "useradd abc"}},
		StartupCmd:    "python app.py",
		EntrypointCmd: "/usr/bin/entry",
	}
	merged := MergeDockerfileInfoChain([]*DockerfileInfo{parent, child})

	// Users, Directories, Downloads, Symlinks merge from all layers
	if len(merged.Users) != 2 {
		t.Errorf("expected 2 users (root, abc), got %d: %v", len(merged.Users), merged.Users)
	}
	if len(merged.Directories) != 2 {
		t.Errorf("expected 2 directories (/config, /data), got %d: %v", len(merged.Directories), merged.Directories)
	}
	if len(merged.Downloads) != 2 {
		t.Errorf("expected 2 downloads (all layers), got %d", len(merged.Downloads))
	}
	if len(merged.Symlinks) != 2 {
		t.Errorf("expected 2 symlinks (all layers), got %d", len(merged.Symlinks))
	}
	// RunCommands: parent "mkdir" (typed) + child "useradd" (typed) = 2
	// Parent "unknown" type would be dropped, but mkdir is actionable
	if len(merged.RunCommands) != 2 {
		t.Errorf("expected 2 run commands (typed from all layers), got %d", len(merged.RunCommands))
	}

	// StartupCmd from child
	if merged.StartupCmd != "python app.py" {
		t.Errorf("expected StartupCmd from child, got %q", merged.StartupCmd)
	}
	if merged.EntrypointCmd != "/usr/bin/entry" {
		t.Errorf("expected EntrypointCmd from child, got %q", merged.EntrypointCmd)
	}
}

func TestRepoURLFromTemplate(t *testing.T) {
	tests := []struct {
		url, want string
	}{
		{"https://raw.githubusercontent.com/linuxserver/docker-swag/master/Dockerfile", "https://github.com/linuxserver/docker-swag"},
		{"https://raw.githubusercontent.com/owner/repo/{branch}/Dockerfile", "https://github.com/owner/repo"},
		{"https://example.com/not-github", ""},
	}
	for _, tc := range tests {
		got := repoURLFromTemplate(tc.url)
		if got != tc.want {
			t.Errorf("repoURLFromTemplate(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

func TestExtractGitHubRepoURL(t *testing.T) {
	tests := []struct {
		url, want string
	}{
		{"https://github.com/linuxserver/docker-swag", "https://github.com/linuxserver/docker-swag"},
		{"https://github.com/linuxserver/docker-swag/tree/master/root", "https://github.com/linuxserver/docker-swag"},
		{"https://raw.githubusercontent.com/owner/repo/main/Dockerfile", "https://github.com/owner/repo"},
		{"https://example.com/not-github", ""},
		{"", ""},
	}
	for _, tc := range tests {
		got := ExtractGitHubRepoURL(tc.url)
		if got != tc.want {
			t.Errorf("ExtractGitHubRepoURL(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

func TestMergeChain_AptKeysAndRepos(t *testing.T) {
	parent := &DockerfileInfo{
		BaseOS: "debian",
		AptKeys: []AptKey{
			{URL: "https://example.com/key1.asc", Keyring: "/usr/share/keyrings/key1.gpg"},
		},
		AptRepos: []AptRepo{
			{Line: "deb [signed-by=/usr/share/keyrings/key1.gpg] https://repo1.example.com stable main", File: "/etc/apt/sources.list.d/repo1.list"},
		},
	}
	child := &DockerfileInfo{
		BaseOS: "debian",
		AptKeys: []AptKey{
			{URL: "https://example.com/key1.asc", Keyring: "/usr/share/keyrings/key1.gpg"}, // duplicate
			{URL: "https://example.com/key2.asc", Keyring: "/usr/share/keyrings/key2.gpg"},
		},
		AptRepos: []AptRepo{
			{Line: "deb [signed-by=/usr/share/keyrings/key2.gpg] https://repo2.example.com stable main", File: "/etc/apt/sources.list.d/repo2.list"},
		},
	}

	merged := MergeDockerfileInfoChain([]*DockerfileInfo{parent, child})

	if len(merged.AptKeys) != 2 {
		t.Errorf("expected 2 apt keys (deduped), got %d", len(merged.AptKeys))
	}
	if len(merged.AptRepos) != 2 {
		t.Errorf("expected 2 apt repos, got %d", len(merged.AptRepos))
	}
}
