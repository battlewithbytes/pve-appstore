package devmode

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// ParseDockerfile unit tests
// ---------------------------------------------------------------------------

func TestParseDockerfileDebianBase(t *testing.T) {
	df := ParseDockerfile("FROM ubuntu:noble\nRUN apt-get update")
	if df.BaseOS != "debian" {
		t.Errorf("expected BaseOS=debian, got %q", df.BaseOS)
	}
	if df.BaseImage != "ubuntu:noble" {
		t.Errorf("expected BaseImage=ubuntu:noble, got %q", df.BaseImage)
	}
}

func TestParseDockerfileAlpineBase(t *testing.T) {
	df := ParseDockerfile("FROM alpine:3.21\n")
	if df.BaseOS != "alpine" {
		t.Errorf("expected BaseOS=alpine, got %q", df.BaseOS)
	}
}

func TestParseDockerfileLinuxserverBase(t *testing.T) {
	df := ParseDockerfile("FROM ghcr.io/linuxserver/baseimage-ubuntu:noble AS build\n")
	if df.BaseOS != "debian" {
		t.Errorf("expected BaseOS=debian for linuxserver ubuntu base, got %q", df.BaseOS)
	}
	if df.BaseImage != "ghcr.io/linuxserver/baseimage-ubuntu:noble" {
		t.Errorf("unexpected BaseImage %q", df.BaseImage)
	}
}

func TestParseDockerfileUnknownBase(t *testing.T) {
	df := ParseDockerfile("FROM scratch\n")
	if df.BaseOS != "unknown" {
		t.Errorf("expected BaseOS=unknown, got %q", df.BaseOS)
	}
}

func TestParseDockerfileScratchFallback(t *testing.T) {
	// Rootfs-builder pattern: first FROM is real OS, last FROM is scratch.
	// BaseImage/BaseOS should fall back to the previous FROM.
	content := `FROM alpine:3.21 AS rootfs-stage
RUN apk add --no-cache bash curl
FROM scratch
RUN apk add --no-cache shadow coreutils
`
	df := ParseDockerfile(content)
	if df.BaseImage != "alpine:3.21" {
		t.Errorf("expected BaseImage=alpine:3.21 (fallback from scratch), got %q", df.BaseImage)
	}
	if df.BaseOS != "alpine" {
		t.Errorf("expected BaseOS=alpine, got %q", df.BaseOS)
	}
	// Packages from all stages should still be collected
	pkgSet := map[string]bool{}
	for _, p := range df.Packages {
		pkgSet[p] = true
	}
	if !pkgSet["bash"] || !pkgSet["shadow"] {
		t.Errorf("expected packages from both stages, got %v", df.Packages)
	}
}

func TestParseDockerfileScratchFallbackDebian(t *testing.T) {
	content := `FROM ubuntu:noble AS builder
RUN apt-get update && apt-get install -y curl
FROM scratch
COPY --from=builder /root-out /
`
	df := ParseDockerfile(content)
	if df.BaseImage != "ubuntu:noble" {
		t.Errorf("expected BaseImage=ubuntu:noble (fallback from scratch), got %q", df.BaseImage)
	}
	if df.BaseOS != "debian" {
		t.Errorf("expected BaseOS=debian, got %q", df.BaseOS)
	}
}

func TestParseDockerfileAptKey(t *testing.T) {
	content := `FROM ubuntu:22.04
RUN curl -fsSL https://example.com/key.gpg | gpg --dearmor > /usr/share/keyrings/example.gpg
`
	df := ParseDockerfile(content)
	if len(df.AptKeys) != 1 {
		t.Fatalf("expected 1 AptKey, got %d", len(df.AptKeys))
	}
	if df.AptKeys[0].URL != "https://example.com/key.gpg" {
		t.Errorf("expected key URL https://example.com/key.gpg, got %q", df.AptKeys[0].URL)
	}
	if df.AptKeys[0].Keyring != "/usr/share/keyrings/example.gpg" {
		t.Errorf("expected keyring /usr/share/keyrings/example.gpg, got %q", df.AptKeys[0].Keyring)
	}
}

func TestParseDockerfileAptKeyTee(t *testing.T) {
	content := `FROM debian:bookworm
RUN curl -fsSL https://repo.example.com/keys/archive.asc | gpg --dearmor | tee /usr/share/keyrings/repo.gpg > /dev/null
`
	df := ParseDockerfile(content)
	if len(df.AptKeys) != 1 {
		t.Fatalf("expected 1 AptKey, got %d", len(df.AptKeys))
	}
	if df.AptKeys[0].URL != "https://repo.example.com/keys/archive.asc" {
		t.Errorf("unexpected key URL %q", df.AptKeys[0].URL)
	}
	if df.AptKeys[0].Keyring != "/usr/share/keyrings/repo.gpg" {
		t.Errorf("unexpected keyring %q", df.AptKeys[0].Keyring)
	}
}

func TestParseDockerfileAptRepo(t *testing.T) {
	content := `FROM ubuntu:22.04
RUN echo "deb [signed-by=/usr/share/keyrings/example.gpg] https://repo.example.com/apt stable main" > /etc/apt/sources.list.d/example.list
`
	df := ParseDockerfile(content)
	if len(df.AptRepos) != 1 {
		t.Fatalf("expected 1 AptRepo, got %d", len(df.AptRepos))
	}
	if !strings.Contains(df.AptRepos[0].Line, "https://repo.example.com/apt") {
		t.Errorf("unexpected repo line %q", df.AptRepos[0].Line)
	}
	if df.AptRepos[0].File != "/etc/apt/sources.list.d/example.list" {
		t.Errorf("unexpected file %q", df.AptRepos[0].File)
	}
}

func TestParseDockerfilePackages(t *testing.T) {
	content := `FROM debian:bookworm
RUN apt-get update && apt-get install -y --no-install-recommends nginx curl ca-certificates
`
	df := ParseDockerfile(content)
	expected := map[string]bool{"nginx": true, "curl": true, "ca-certificates": true}
	for _, pkg := range df.Packages {
		delete(expected, pkg)
	}
	if len(expected) > 0 {
		t.Errorf("missing packages: %v (got %v)", expected, df.Packages)
	}
}

func TestParseDockerfilePackagesVersionStripped(t *testing.T) {
	content := `FROM debian:bookworm
RUN apt-get install -y nginx=1.24.0-1 curl>=7.88
`
	df := ParseDockerfile(content)
	for _, pkg := range df.Packages {
		if strings.ContainsAny(pkg, "=<>") {
			t.Errorf("version qualifier not stripped from package %q", pkg)
		}
	}
	found := map[string]bool{}
	for _, pkg := range df.Packages {
		found[pkg] = true
	}
	if !found["nginx"] || !found["curl"] {
		t.Errorf("expected nginx and curl, got %v", df.Packages)
	}
}

func TestParseDockerfileApkPackages(t *testing.T) {
	content := `FROM alpine:3.21
RUN apk add --no-cache nginx curl openssl
`
	df := ParseDockerfile(content)
	expected := map[string]bool{"nginx": true, "curl": true, "openssl": true}
	for _, pkg := range df.Packages {
		delete(expected, pkg)
	}
	if len(expected) > 0 {
		t.Errorf("missing packages: %v (got %v)", expected, df.Packages)
	}
}

func TestParseDockerfileExpose(t *testing.T) {
	content := `FROM ubuntu:22.04
EXPOSE 8080 443/tcp 9090/udp
`
	df := ParseDockerfile(content)
	if len(df.Ports) != 3 {
		t.Fatalf("expected 3 ports, got %d: %v", len(df.Ports), df.Ports)
	}
	expected := []string{"8080", "443", "9090"}
	for i, p := range expected {
		if df.Ports[i] != p {
			t.Errorf("port[%d]: expected %q, got %q", i, p, df.Ports[i])
		}
	}
}

func TestParseDockerfileExposeSkipsVars(t *testing.T) {
	content := `FROM ubuntu:22.04
EXPOSE $PORT 8080
`
	df := ParseDockerfile(content)
	if len(df.Ports) != 1 || df.Ports[0] != "8080" {
		t.Errorf("expected [8080], got %v", df.Ports)
	}
}

func TestParseDockerfileVolumeJSON(t *testing.T) {
	content := `FROM ubuntu:22.04
VOLUME ["/config", "/data"]
`
	df := ParseDockerfile(content)
	if len(df.Volumes) != 2 {
		t.Fatalf("expected 2 volumes, got %d: %v", len(df.Volumes), df.Volumes)
	}
	if df.Volumes[0] != "/config" || df.Volumes[1] != "/data" {
		t.Errorf("unexpected volumes: %v", df.Volumes)
	}
}

func TestParseDockerfileVolumeSpaceSeparated(t *testing.T) {
	content := `FROM ubuntu:22.04
VOLUME /config /data /logs
`
	df := ParseDockerfile(content)
	if len(df.Volumes) != 3 {
		t.Fatalf("expected 3 volumes, got %d: %v", len(df.Volumes), df.Volumes)
	}
	if df.Volumes[0] != "/config" || df.Volumes[1] != "/data" || df.Volumes[2] != "/logs" {
		t.Errorf("unexpected volumes: %v", df.Volumes)
	}
}

func TestParseDockerfileContinuationLines(t *testing.T) {
	content := `FROM debian:bookworm
RUN apt-get update && \
    apt-get install -y \
    nginx \
    curl \
    ca-certificates
`
	df := ParseDockerfile(content)
	expected := map[string]bool{"nginx": true, "curl": true, "ca-certificates": true}
	for _, pkg := range df.Packages {
		delete(expected, pkg)
	}
	if len(expected) > 0 {
		t.Errorf("continuation lines not joined; missing packages: %v (got %v)", expected, df.Packages)
	}
}

func TestParseDockerfileDedup(t *testing.T) {
	content := `FROM debian:bookworm
RUN apt-get install -y curl nginx
RUN apt-get install -y curl openssl
`
	df := ParseDockerfile(content)
	count := 0
	for _, pkg := range df.Packages {
		if pkg == "curl" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected curl to appear once (dedup), appeared %d times in %v", count, df.Packages)
	}
}

func TestParseDockerfileMultiStage(t *testing.T) {
	content := `FROM golang:1.21 AS builder
RUN go build -o /app
FROM ubuntu:noble
RUN apt-get install -y ca-certificates
EXPOSE 8080
`
	df := ParseDockerfile(content)
	// Last FROM wins for base detection
	if df.BaseOS != "debian" {
		t.Errorf("expected BaseOS=debian from final stage, got %q", df.BaseOS)
	}
}

func TestMultiStageResetsRunCommands(t *testing.T) {
	// Build stage commands should NOT appear in the final stage's RunCommands.
	// This prevents rootfs-builder commands (e.g. sed on /root-out/) from
	// leaking into the install script.
	content := `FROM alpine:3.22 AS rootfs-builder
RUN mkdir -p /root-out/etc
RUN sed -i 's/^root::/root:!:/' /root-out/etc/shadow
RUN useradd -r builder

FROM scratch
COPY --from=rootfs-builder /root-out/ /
`
	df := ParseDockerfile(content)
	// RunCommands from the rootfs-builder stage should be reset
	if len(df.RunCommands) != 0 {
		t.Errorf("expected 0 RunCommands from scratch stage, got %d: %v", len(df.RunCommands), df.RunCommands)
	}
	if len(df.Users) != 0 {
		t.Errorf("expected 0 Users from scratch stage, got %d", len(df.Users))
	}
	// Packages should still accumulate across stages
	if len(df.Packages) != 0 {
		// rootfs-builder has no package installs in this test
	}
}

func TestMultiStageKeepsPortsAndVolumes(t *testing.T) {
	// EXPOSE and VOLUME from earlier stages should persist (Docker inherits them).
	content := `FROM alpine:3.22 AS base
RUN apk add --no-cache nginx
EXPOSE 80 443
VOLUME /config

FROM base AS final
RUN sed -i 's/foo/bar/' /etc/nginx/nginx.conf
`
	df := ParseDockerfile(content)
	if len(df.Ports) != 2 {
		t.Errorf("expected 2 ports from base stage, got %d: %v", len(df.Ports), df.Ports)
	}
	if len(df.Volumes) != 1 {
		t.Errorf("expected 1 volume from base stage, got %d", len(df.Volumes))
	}
	// Only the final stage's RunCommands should remain
	if len(df.RunCommands) != 1 {
		t.Errorf("expected 1 RunCommand from final stage, got %d", len(df.RunCommands))
	}
	// Packages from all stages should accumulate
	if len(df.Packages) != 1 || df.Packages[0] != "nginx" {
		t.Errorf("expected [nginx] packages, got %v", df.Packages)
	}
}

// ---------------------------------------------------------------------------
// InferDockerfileURL tests
// ---------------------------------------------------------------------------

func TestInferDockerfileURLFromGitHub(t *testing.T) {
	url, branch := InferDockerfileURL("", "https://github.com/linuxserver/docker-resilio-sync")
	if branch != "master" {
		t.Errorf("expected branch=master, got %q", branch)
	}
	if !strings.Contains(url, "raw.githubusercontent.com/linuxserver/docker-resilio-sync") {
		t.Errorf("unexpected URL %q", url)
	}
	if !strings.Contains(url, "{branch}") {
		t.Errorf("URL should contain {branch} placeholder: %q", url)
	}
}

func TestInferDockerfileURLFromGitHubWithFragment(t *testing.T) {
	url, _ := InferDockerfileURL("", "https://github.com/linuxserver/docker-qbittorrent#application-setup")
	if !strings.Contains(url, "docker-qbittorrent") {
		t.Errorf("fragment should be stripped; got %q", url)
	}
}

func TestInferDockerfileURLFromLinuxServer(t *testing.T) {
	url, branch := InferDockerfileURL("lscr.io/linuxserver/resilio-sync:latest", "")
	if branch != "master" {
		t.Errorf("expected branch=master, got %q", branch)
	}
	expected := "https://raw.githubusercontent.com/linuxserver/docker-resilio-sync/{branch}/Dockerfile"
	if url != expected {
		t.Errorf("expected %q, got %q", expected, url)
	}
}

func TestInferDockerfileURLFromLinuxServerGHCR(t *testing.T) {
	url, _ := InferDockerfileURL("ghcr.io/linuxserver/qbittorrent", "")
	if !strings.Contains(url, "docker-qbittorrent") {
		t.Errorf("unexpected URL %q", url)
	}
}

func TestInferDockerfileURLEmpty(t *testing.T) {
	url, branch := InferDockerfileURL("myregistry.com/custom-app:v1", "")
	if url != "" || branch != "" {
		t.Errorf("expected empty for non-linuxserver/github repo; got url=%q branch=%q", url, branch)
	}
}

func TestInferDockerfileURLGitHubPreferred(t *testing.T) {
	// When both GitHub and Repository are provided, GitHub URL takes precedence
	url, _ := InferDockerfileURL("lscr.io/linuxserver/something", "https://github.com/owner/custom-repo")
	if !strings.Contains(url, "owner/custom-repo") {
		t.Errorf("GitHub URL should take precedence; got %q", url)
	}
}

// ---------------------------------------------------------------------------
// InferS6RunURL tests
// ---------------------------------------------------------------------------

func TestInferS6RunURL(t *testing.T) {
	base := "https://raw.githubusercontent.com/linuxserver/docker-resilio-sync/{branch}/Dockerfile"
	result := InferS6RunURL(base, "resilio-sync")
	expected := "https://raw.githubusercontent.com/linuxserver/docker-resilio-sync/{branch}/root/etc/s6-overlay/s6-rc.d/svc-resilio-sync/run"
	if result != expected {
		t.Errorf("expected\n  %q\ngot\n  %q", expected, result)
	}
}

func TestInferS6RunURLEmpty(t *testing.T) {
	result := InferS6RunURL("", "anything")
	if result != "" {
		t.Errorf("expected empty for empty base, got %q", result)
	}
}

// ---------------------------------------------------------------------------
// ParseS6RunScript tests
// ---------------------------------------------------------------------------

func TestParseS6RunScriptSimpleExec(t *testing.T) {
	script := `#!/usr/bin/with-contenv bash
# shellcheck shell=bash
exec /usr/bin/rslsync --nodaemon --config /config/sync.conf
`
	result := ParseS6RunScript(script)
	if result != "/usr/bin/rslsync --nodaemon --config /config/sync.conf" {
		t.Errorf("unexpected exec cmd: %q", result)
	}
}

func TestParseS6RunScriptWithS6Wrapper(t *testing.T) {
	script := `#!/usr/bin/with-contenv bash
cd /app || exit
exec s6-setuidgid abc -- /usr/bin/myapp --port 8080
`
	result := ParseS6RunScript(script)
	if result != "/usr/bin/myapp --port 8080" {
		t.Errorf("expected s6 wrapper stripped; got %q", result)
	}
}

func TestParseS6RunScriptEmpty(t *testing.T) {
	script := `#!/bin/bash
# just comments
`
	result := ParseS6RunScript(script)
	if result != "" {
		t.Errorf("expected empty for comment-only script, got %q", result)
	}
}

func TestParseS6RunScriptNonExecCommand(t *testing.T) {
	// Last meaningful line that isn't exec, cd, umask, chown, or if
	script := `#!/bin/bash
umask 022
/usr/bin/myapp --serve
`
	result := ParseS6RunScript(script)
	if result != "/usr/bin/myapp --serve" {
		t.Errorf("expected last meaningful line; got %q", result)
	}
}

// ---------------------------------------------------------------------------
// ConvertUnraidToScaffold tests — nil Dockerfile (fallback)
// ---------------------------------------------------------------------------

func TestConvertWithDockerfileNil(t *testing.T) {
	c := &UnraidContainer{
		Name:       "TestApp",
		Repository: "testimage:latest",
		Overview:   "A test application",
		Configs: []UnraidConfig{
			{Name: "WebUI Port", Target: "8080", Default: "8080", Type: "Port", Mode: "tcp"},
		},
	}
	id, manifest, script := ConvertUnraidToScaffold(c, nil)
	if id != "testapp" {
		t.Errorf("expected id=testapp, got %q", id)
	}
	if !strings.Contains(manifest, "SCAFFOLD") {
		t.Error("nil df should produce scaffold manifest with SCAFFOLD comment")
	}
	if !strings.Contains(script, "TODO") {
		t.Error("nil df should produce script with TODOs")
	}
	if !strings.Contains(script, "class Testapp(BaseApp)") {
		t.Errorf("script should contain Testapp class, got:\n%s", script)
	}
}

func TestConvertWithDockerfileEmptyPackages(t *testing.T) {
	// Empty Packages slice should also fall back to scaffold
	c := &UnraidContainer{
		Name:       "TestApp",
		Repository: "testimage:latest",
	}
	df := &DockerfileInfo{BaseOS: "debian"}
	_, _, script := ConvertUnraidToScaffold(c, df)
	if !strings.Contains(script, "TODO") {
		t.Error("empty packages should fall back to scaffold with TODOs")
	}
}

// ---------------------------------------------------------------------------
// ConvertUnraidToScaffold tests — with Dockerfile data
// ---------------------------------------------------------------------------

func TestConvertWithDockerfileBasic(t *testing.T) {
	c := &UnraidContainer{
		Name:       "MyApp",
		Repository: "lscr.io/linuxserver/myapp:latest",
		Overview:   "My application",
		Configs: []UnraidConfig{
			{Name: "WebUI Port", Target: "8080", Default: "8080", Type: "Port", Mode: "tcp"},
		},
	}
	df := &DockerfileInfo{
		BaseOS:   "debian",
		Packages: []string{"nginx", "curl"},
	}

	id, manifest, script := ConvertUnraidToScaffold(c, df)
	if id != "myapp" {
		t.Errorf("expected id=myapp, got %q", id)
	}

	// Manifest should have permissions section with packages
	if !strings.Contains(manifest, "permissions:") {
		t.Error("manifest missing permissions section")
	}
	if !strings.Contains(manifest, "nginx") {
		t.Error("manifest missing nginx in packages")
	}

	// Script should use pkg_install, not TODO
	if !strings.Contains(script, "self.pkg_install(") {
		t.Error("script should contain self.pkg_install()")
	}
	if strings.Contains(script, "# TODO: Replace with") {
		t.Error("script should NOT contain generic TODO when packages are known")
	}
	// Should have enable_service (no ExecCmd)
	if !strings.Contains(script, "self.enable_service(") {
		t.Error("script should call enable_service when no ExecCmd")
	}
}

func TestConvertWithDockerfileAlpine(t *testing.T) {
	c := &UnraidContainer{
		Name:       "AlpineApp",
		Repository: "alpine-based:latest",
	}
	df := &DockerfileInfo{
		BaseOS:   "alpine",
		Packages: []string{"nginx"},
	}
	_, manifest, _ := ConvertUnraidToScaffold(c, df)
	if !strings.Contains(manifest, "alpine-3.22") {
		t.Error("alpine base should use alpine-3.22 template")
	}
}

func TestConvertWithDockerfileAptKeyRepo(t *testing.T) {
	c := &UnraidContainer{
		Name:       "RepoApp",
		Repository: "repo-app:latest",
	}
	df := &DockerfileInfo{
		BaseOS: "debian",
		AptKeys: []AptKey{
			{URL: "https://example.com/key.asc", Keyring: "/usr/share/keyrings/example.gpg"},
		},
		AptRepos: []AptRepo{
			{Line: "deb [signed-by=/usr/share/keyrings/example.gpg] https://repo.example.com/apt stable main", File: "/etc/apt/sources.list.d/example.list"},
		},
		Packages: []string{"example-app"},
	}

	_, manifest, script := ConvertUnraidToScaffold(c, df)

	// Manifest should have URLs in permissions
	if !strings.Contains(manifest, "https://example.com/key.asc") {
		t.Error("manifest should list key URL in permissions")
	}

	// Script should have add_apt_key and add_apt_repo
	if !strings.Contains(script, "self.add_apt_key(") {
		t.Error("script should call add_apt_key")
	}
	if !strings.Contains(script, "self.add_apt_repo(") {
		t.Error("script should call add_apt_repo")
	}
}

func TestConvertWithDockerfileExecCmd(t *testing.T) {
	c := &UnraidContainer{
		Name:       "ExecApp",
		Repository: "exec-app:latest",
	}
	df := &DockerfileInfo{
		BaseOS:   "debian",
		Packages: []string{"myapp"},
		ExecCmd:  "/usr/bin/myapp --serve",
	}

	_, _, script := ConvertUnraidToScaffold(c, df)
	if !strings.Contains(script, "self.create_service(") {
		t.Error("script should use create_service when ExecCmd is set")
	}
	if !strings.Contains(script, "/usr/bin/myapp --serve") {
		t.Error("script should contain the exec command")
	}
}

func TestConvertWithDockerfilePrivileged(t *testing.T) {
	c := &UnraidContainer{
		Name:       "PrivApp",
		Repository: "priv-app:latest",
		Privileged: "true",
	}
	df := &DockerfileInfo{
		BaseOS:   "debian",
		Packages: []string{"something"},
	}
	_, manifest, _ := ConvertUnraidToScaffold(c, df)
	if !strings.Contains(manifest, "unprivileged: false") {
		t.Error("Privileged=true should map to unprivileged: false")
	}
}

func TestConvertWithDockerfileVolumes(t *testing.T) {
	c := &UnraidContainer{
		Name:       "VolApp",
		Repository: "vol-app:latest",
		Configs: []UnraidConfig{
			{Name: "Config", Target: "/config", Type: "Path", Mode: "rw"},
		},
	}
	df := &DockerfileInfo{
		BaseOS:   "debian",
		Packages: []string{"myapp"},
		Volumes:  []string{"/config", "/data"}, // /config overlaps XML, /data is new
	}

	_, _, script := ConvertUnraidToScaffold(c, df)
	// Both /config and /data should be in script
	if !strings.Contains(script, `"/config"`) {
		t.Error("script should create /config directory")
	}
	if !strings.Contains(script, `"/data"`) {
		t.Error("script should create /data directory (from Dockerfile VOLUME)")
	}
}

func TestConvertSkipsDockerVars(t *testing.T) {
	c := &UnraidContainer{
		Name:       "VarApp",
		Repository: "var-app:latest",
		Configs: []UnraidConfig{
			{Name: "PUID", Target: "PUID", Default: "1000", Type: "Variable"},
			{Name: "PGID", Target: "PGID", Default: "1000", Type: "Variable"},
			{Name: "TZ", Target: "TZ", Default: "America/New_York", Type: "Variable"},
			{Name: "CUSTOM_VAR", Target: "CUSTOM_VAR", Default: "hello", Type: "Variable"},
		},
	}
	df := &DockerfileInfo{BaseOS: "debian", Packages: []string{"pkg"}}
	_, manifest, _ := ConvertUnraidToScaffold(c, df)
	// PUID/PGID/TZ should be skipped
	if strings.Contains(manifest, "key: puid") {
		t.Error("PUID should be skipped")
	}
	if strings.Contains(manifest, "key: pgid") {
		t.Error("PGID should be skipped")
	}
	if strings.Contains(manifest, "key: tz") {
		t.Error("TZ should be skipped")
	}
	// CUSTOM_VAR should be present
	if !strings.Contains(manifest, "custom_var") {
		t.Error("CUSTOM_VAR should be present in inputs")
	}
}

// ---------------------------------------------------------------------------
// Helper function tests
// ---------------------------------------------------------------------------

func TestExtractURLFromRepoLine(t *testing.T) {
	tests := []struct {
		line     string
		expected string
	}{
		{
			"deb [signed-by=/usr/share/keyrings/foo.gpg] https://repo.example.com/apt stable main",
			"https://repo.example.com/",
		},
		{
			"deb http://ppa.launchpad.net/foo/ppa/ubuntu focal main",
			"http://ppa.launchpad.net/",
		},
	}
	for _, tc := range tests {
		result := extractURLFromRepoLine(tc.line)
		if result != tc.expected {
			t.Errorf("extractURLFromRepoLine(%q) = %q, want %q", tc.line, result, tc.expected)
		}
	}
}

func TestInferMainService(t *testing.T) {
	tests := []struct {
		appID    string
		packages []string
		expected string
	}{
		{"nginx", []string{"curl", "nginx", "ca-certificates"}, "nginx"},
		{"my-app", []string{"libssl-dev", "curl", "my-app"}, "my-app"},
		{"custom", []string{"libfoo", "curl", "ca-certificates", "wget", "actual-pkg"}, "actual-pkg"},
		{"fallback", []string{"libfoo", "curl", "ca-certificates"}, "fallback"},
	}
	for _, tc := range tests {
		result := inferMainService(tc.appID, tc.packages)
		if result != tc.expected {
			t.Errorf("inferMainService(%q, %v) = %q, want %q", tc.appID, tc.packages, result, tc.expected)
		}
	}
}

// ---------------------------------------------------------------------------
// End-to-end test with realistic Dockerfile content
// ---------------------------------------------------------------------------

func TestEndToEndLinuxserverDockerfile(t *testing.T) {
	// Simplified but realistic linuxserver-style Dockerfile
	dockerfile := `FROM ghcr.io/linuxserver/baseimage-ubuntu:noble AS buildstage

RUN \
  echo "**** install build packages ****" && \
  apt-get update && \
  apt-get install -y \
    build-essential \
    cmake \
    curl \
    libssl-dev && \
  echo "**** build app ****" && \
  mkdir -p /build

FROM ghcr.io/linuxserver/baseimage-ubuntu:noble

ARG DEBIAN_FRONTEND=noninteractive

RUN \
  echo "**** install runtime packages ****" && \
  apt-get update && \
  apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    libssl3 \
    resilio-sync && \
  apt-get clean && \
  rm -rf /var/lib/apt/lists/*

EXPOSE 8888 55555

VOLUME /config /sync
`

	df := ParseDockerfile(dockerfile)

	// Base detection
	if df.BaseOS != "debian" {
		t.Errorf("expected debian, got %q", df.BaseOS)
	}

	// Packages should be deduplicated and include all from both stages
	pkgSet := map[string]bool{}
	for _, p := range df.Packages {
		pkgSet[p] = true
	}
	for _, expected := range []string{"ca-certificates", "curl", "resilio-sync", "build-essential", "cmake", "libssl-dev", "libssl3"} {
		if !pkgSet[expected] {
			t.Errorf("missing package %q (got %v)", expected, df.Packages)
		}
	}
	// curl should appear only once
	curlCount := 0
	for _, p := range df.Packages {
		if p == "curl" {
			curlCount++
		}
	}
	if curlCount != 1 {
		t.Errorf("curl appeared %d times, expected 1", curlCount)
	}

	// Ports
	if len(df.Ports) != 2 {
		t.Fatalf("expected 2 ports, got %d: %v", len(df.Ports), df.Ports)
	}
	portSet := map[string]bool{}
	for _, p := range df.Ports {
		portSet[p] = true
	}
	if !portSet["8888"] || !portSet["55555"] {
		t.Errorf("expected ports 8888 and 55555, got %v", df.Ports)
	}

	// Volumes
	if len(df.Volumes) != 2 {
		t.Fatalf("expected 2 volumes, got %d: %v", len(df.Volumes), df.Volumes)
	}
	volSet := map[string]bool{}
	for _, v := range df.Volumes {
		volSet[v] = true
	}
	if !volSet["/config"] || !volSet["/sync"] {
		t.Errorf("expected /config and /sync, got %v", df.Volumes)
	}

	// Now convert via Unraid scaffold
	c := &UnraidContainer{
		Name:       "Resilio Sync",
		Repository: "lscr.io/linuxserver/resilio-sync:latest",
		Overview:   "Resilio Sync is a file synchronization tool.",
		Project:    "https://www.resilio.com",
		Configs: []UnraidConfig{
			{Name: "WebUI Port", Target: "8888", Default: "8888", Type: "Port", Mode: "tcp"},
			{Name: "Listening Port", Target: "55555", Default: "55555", Type: "Port", Mode: "tcp"},
			{Name: "Config", Target: "/config", Default: "/mnt/user/appdata/resilio-sync", Type: "Path", Mode: "rw"},
			{Name: "PUID", Target: "PUID", Default: "1000", Type: "Variable"},
			{Name: "PGID", Target: "PGID", Default: "1000", Type: "Variable"},
			{Name: "TZ", Target: "TZ", Default: "Etc/UTC", Type: "Variable"},
		},
	}

	id, manifest, script := ConvertUnraidToScaffold(c, df)

	if id != "resilio-sync" {
		t.Errorf("expected id=resilio-sync, got %q", id)
	}

	// Manifest checks
	if !strings.Contains(manifest, `name: "Resilio Sync"`) {
		t.Error("manifest missing app name")
	}
	if !strings.Contains(manifest, "permissions:") {
		t.Error("manifest missing permissions section")
	}
	if !strings.Contains(manifest, "resilio-sync") {
		t.Error("manifest should reference resilio-sync in packages")
	}

	// Script checks — should have real SDK calls
	if !strings.Contains(script, "class ResilioSync(BaseApp)") {
		t.Error("script should have ResilioSync class")
	}
	if !strings.Contains(script, "self.pkg_install(") {
		t.Error("script should call pkg_install")
	}
	if !strings.Contains(script, `self.create_dir("/config")`) {
		t.Error("script should create /config directory")
	}
	if !strings.Contains(script, `self.create_dir("/sync")`) {
		t.Error("script should create /sync directory from Dockerfile VOLUME")
	}
	// Should NOT have TODOs — we have real data
	if strings.Contains(script, "# TODO: Replace with the actual packages") {
		t.Error("script should NOT have generic TODO when Dockerfile data is available")
	}
	// PUID/PGID/TZ should be skipped
	if strings.Contains(script, "puid") {
		t.Error("PUID should be filtered out of script")
	}
}

// ---------------------------------------------------------------------------
// ConvertDockerfileToScaffold tests (no Unraid XML)
// ---------------------------------------------------------------------------

func TestConvertDockerfileBasic(t *testing.T) {
	df := &DockerfileInfo{
		BaseOS:   "debian",
		Packages: []string{"nginx", "curl"},
		Ports:    []string{"8080"},
	}
	id, manifest, script := ConvertDockerfileToScaffold("My Web App", df)
	if id != "my-web-app" {
		t.Errorf("expected id=my-web-app, got %q", id)
	}
	if !strings.Contains(manifest, `name: "My Web App"`) {
		t.Error("manifest missing app name")
	}
	if !strings.Contains(manifest, "permissions:") {
		t.Error("manifest missing permissions section")
	}
	if !strings.Contains(manifest, "nginx") {
		t.Error("manifest missing nginx in packages")
	}
	if !strings.Contains(script, "self.pkg_install(") {
		t.Error("script should contain self.pkg_install()")
	}
	if !strings.Contains(script, "class MyWebApp(BaseApp)") {
		t.Errorf("script should contain MyWebApp class, got:\n%s", script)
	}
	if !strings.Contains(manifest, "dockerfile-import") {
		t.Error("manifest should have dockerfile-import tag")
	}
}

func TestConvertDockerfileWithPorts(t *testing.T) {
	df := &DockerfileInfo{
		BaseOS:   "debian",
		Packages: []string{"nginx"},
		Ports:    []string{"80", "443"},
	}
	_, manifest, script := ConvertDockerfileToScaffold("Nginx Server", df)
	// Should have port inputs
	if !strings.Contains(manifest, "key: port_80") {
		t.Error("manifest missing port_80 input")
	}
	if !strings.Contains(manifest, "key: port_443") {
		t.Error("manifest missing port_443 input")
	}
	// First port should be used for outputs
	if !strings.Contains(manifest, "{{port_80}}") {
		t.Error("manifest outputs should reference port_80")
	}
	// Script should read port inputs
	if !strings.Contains(script, `self.inputs.integer("port_80"`) {
		t.Error("script should read port_80 input")
	}
}

func TestConvertDockerfileWithAptKeyRepo(t *testing.T) {
	df := &DockerfileInfo{
		BaseOS: "debian",
		AptKeys: []AptKey{
			{URL: "https://example.com/key.asc", Keyring: "/usr/share/keyrings/example.gpg"},
		},
		AptRepos: []AptRepo{
			{Line: "deb [signed-by=/usr/share/keyrings/example.gpg] https://repo.example.com/apt stable main", File: "/etc/apt/sources.list.d/example.list"},
		},
		Packages: []string{"example-app"},
	}
	_, manifest, script := ConvertDockerfileToScaffold("Example App", df)

	if !strings.Contains(manifest, "https://example.com/key.asc") {
		t.Error("manifest should list key URL in permissions")
	}
	if !strings.Contains(script, "self.add_apt_key(") {
		t.Error("script should call add_apt_key")
	}
	if !strings.Contains(script, "self.add_apt_repo(") {
		t.Error("script should call add_apt_repo")
	}
}

func TestConvertDockerfileAlpine(t *testing.T) {
	df := &DockerfileInfo{
		BaseOS:   "alpine",
		Packages: []string{"nginx"},
	}
	_, manifest, _ := ConvertDockerfileToScaffold("Alpine App", df)
	if !strings.Contains(manifest, "alpine-3.22") {
		t.Error("alpine base should use alpine-3.22 template")
	}
}

func TestConvertDockerfileEmptyPackages(t *testing.T) {
	df := &DockerfileInfo{
		BaseOS: "debian",
		Ports:  []string{"8080"},
	}
	_, manifest, script := ConvertDockerfileToScaffold("Empty App", df)
	// Should fall back to scaffold with TODOs
	if !strings.Contains(script, "TODO") {
		t.Error("empty packages should produce scaffold with TODOs")
	}
	if !strings.Contains(manifest, "SCAFFOLD") {
		t.Error("empty packages manifest should have SCAFFOLD comment")
	}
	// Should still have port input
	if !strings.Contains(manifest, "port_8080") {
		t.Error("manifest should still have port input even with empty packages")
	}
}

func TestConvertDockerfileWithExecCmd(t *testing.T) {
	df := &DockerfileInfo{
		BaseOS:   "debian",
		Packages: []string{"myapp"},
		ExecCmd:  "/usr/bin/myapp --serve",
	}
	_, _, script := ConvertDockerfileToScaffold("Exec App", df)
	if !strings.Contains(script, "self.create_service(") {
		t.Error("script should use create_service when ExecCmd is set")
	}
	if !strings.Contains(script, "/usr/bin/myapp --serve") {
		t.Error("script should contain the exec command")
	}
}

func TestConvertDockerfileNilDf(t *testing.T) {
	id, manifest, script := ConvertDockerfileToScaffold("Nil App", nil)
	if id != "nil-app" {
		t.Errorf("expected id=nil-app, got %q", id)
	}
	if !strings.Contains(manifest, "SCAFFOLD") {
		t.Error("nil df should produce scaffold manifest")
	}
	if !strings.Contains(script, "TODO") {
		t.Error("nil df should produce script with TODOs")
	}
}

func TestParseDockerfileApkVirtualRemoved(t *testing.T) {
	content := `FROM alpine:3.22
RUN apk add --no-cache --virtual=build-dependencies \
    build-base \
    cargo \
    openssl-dev \
    python3-dev && \
    apk add --no-cache \
    fail2ban \
    nginx && \
    apk del --purge \
    build-dependencies
EXPOSE 80 443
VOLUME /config
`
	df := ParseDockerfile(content)
	// build-dependencies group should be removed
	pkgSet := map[string]bool{}
	for _, p := range df.Packages {
		pkgSet[p] = true
	}
	if pkgSet["build-base"] {
		t.Error("build-base should be excluded (virtual group was deleted)")
	}
	if pkgSet["cargo"] {
		t.Error("cargo should be excluded (virtual group was deleted)")
	}
	if pkgSet["openssl-dev"] {
		t.Error("openssl-dev should be excluded (virtual group was deleted)")
	}
	if !pkgSet["fail2ban"] {
		t.Error("fail2ban should be present (runtime package)")
	}
	if !pkgSet["nginx"] {
		t.Error("nginx should be present (runtime package)")
	}
}

func TestParseDockerfilePipInstall(t *testing.T) {
	content := `FROM ubuntu:22.04
RUN pip install -U --no-cache-dir certbot certbot-dns-cloudflare requests
`
	df := ParseDockerfile(content)
	if len(df.PipPackages) < 3 {
		t.Fatalf("expected at least 3 pip packages, got %d: %v", len(df.PipPackages), df.PipPackages)
	}
	pipSet := map[string]bool{}
	for _, p := range df.PipPackages {
		pipSet[p] = true
	}
	if !pipSet["certbot"] {
		t.Error("missing pip package: certbot")
	}
	if !pipSet["certbot-dns-cloudflare"] {
		t.Error("missing pip package: certbot-dns-cloudflare")
	}
}

func TestInferMainServiceVariousCases(t *testing.T) {
	tests := []struct {
		name     string
		appID    string
		packages []string
		expected string
	}{
		// Tier 1: exact match
		{"exact match", "nginx", []string{"curl", "nginx", "ca-certificates"}, "nginx"},
		{"exact match kebab", "my-app", []string{"libssl-dev", "curl", "my-app"}, "my-app"},
		// Tier 2: partial match — package contains app ID
		{"partial qbittorrent-nox", "qbittorrent", []string{"curl", "qt6-qtbase-sqlite", "qbittorrent-nox"}, "qbittorrent-nox"},
		{"partial grafana-server", "grafana", []string{"libfontconfig1", "grafana-server"}, "grafana-server"},
		{"partial jellyfin-server", "jellyfin", []string{"curl", "jellyfin-server", "jellyfin-web"}, "jellyfin-server"},
		// Tier 3: first non-utility
		{"skip build deps", "swag", []string{"build-base", "cargo", "openssl-dev", "fail2ban", "nginx"}, "fail2ban"},
		{"skip cmake", "myapp", []string{"build-essential", "cmake", "nginx"}, "nginx"},
		{"skip python3", "myapp", []string{"python3", "python3-pip", "gunicorn"}, "gunicorn"},
		{"skip qt6", "myapp", []string{"qt6-qtbase-sqlite", "qt6-qtbase", "actual-app"}, "actual-app"},
		{"skip libs suffix", "myapp", []string{"icu-libs", "zlib-libs", "redis"}, "redis"},
		{"skip dev suffix", "myapp", []string{"libffi-dev", "openssl-dev", "postgresql"}, "postgresql"},
		// Fallback to app ID
		{"fallback", "custom", []string{"libfoo", "curl", "ca-certificates"}, "custom"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := inferMainService(tc.appID, tc.packages)
			if result != tc.expected {
				t.Errorf("inferMainService(%q, %v) = %q, want %q", tc.appID, tc.packages, result, tc.expected)
			}
		})
	}
}

func TestParseDockerfileExposeDedupProtocol(t *testing.T) {
	content := `FROM alpine:3.21
EXPOSE 6881/tcp 6881/udp 8080
`
	df := ParseDockerfile(content)
	if len(df.Ports) != 2 {
		t.Errorf("expected 2 ports (6881 deduped), got %d: %v", len(df.Ports), df.Ports)
	}
	portSet := map[string]bool{}
	for _, p := range df.Ports {
		portSet[p] = true
	}
	if !portSet["6881"] || !portSet["8080"] {
		t.Errorf("expected 6881 and 8080, got %v", df.Ports)
	}
}

func TestEndToEndWithAptKeyAndRepo(t *testing.T) {
	dockerfile := `FROM ubuntu:22.04

RUN apt-get update && \
    apt-get install -y curl gnupg && \
    curl -fsSL https://packages.grafana.com/gpg.key | gpg --dearmor > /usr/share/keyrings/grafana.gpg && \
    echo "deb [signed-by=/usr/share/keyrings/grafana.gpg] https://packages.grafana.com/oss/deb stable main" > /etc/apt/sources.list.d/grafana.list && \
    apt-get update && \
    apt-get install -y grafana

EXPOSE 3000
VOLUME /var/lib/grafana
`
	df := ParseDockerfile(dockerfile)

	if len(df.AptKeys) != 1 {
		t.Fatalf("expected 1 apt key, got %d", len(df.AptKeys))
	}
	if df.AptKeys[0].URL != "https://packages.grafana.com/gpg.key" {
		t.Errorf("unexpected key URL: %q", df.AptKeys[0].URL)
	}

	if len(df.AptRepos) != 1 {
		t.Fatalf("expected 1 apt repo, got %d", len(df.AptRepos))
	}
	if !strings.Contains(df.AptRepos[0].Line, "packages.grafana.com") {
		t.Errorf("unexpected repo line: %q", df.AptRepos[0].Line)
	}

	// Convert
	c := &UnraidContainer{
		Name:       "Grafana",
		Repository: "grafana/grafana:latest",
		Configs: []UnraidConfig{
			{Name: "WebUI Port", Target: "3000", Default: "3000", Type: "Port", Mode: "tcp"},
		},
	}
	_, _, script := ConvertUnraidToScaffold(c, df)

	if !strings.Contains(script, "self.add_apt_key(") {
		t.Error("script should call add_apt_key for Grafana")
	}
	if !strings.Contains(script, "self.add_apt_repo(") {
		t.Error("script should call add_apt_repo for Grafana")
	}
	if !strings.Contains(script, `"grafana"`) {
		t.Error("script should install grafana package")
	}
}

// ---------------------------------------------------------------------------
// Real-world Dockerfile integration tests
// ---------------------------------------------------------------------------

func TestRealWorldBestBuyFinder(t *testing.T) {
	dockerfile := `FROM python:3.11-slim

RUN apt-get update && apt-get install -y \
    wget \
    gnupg \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt requests

RUN playwright install chromium
RUN playwright install-deps

COPY stock_checker.py .
CMD ["python", "stock_checker.py"]
`
	df := ParseDockerfile(dockerfile)

	if df.BaseOS != "debian" {
		t.Errorf("expected debian (python:3.11-slim is debian), got %q", df.BaseOS)
	}
	if len(df.Packages) == 0 {
		t.Fatal("expected some packages")
	}
	// pip packages from -r file won't be captured, but "requests" should be
	pipSet := map[string]bool{}
	for _, p := range df.PipPackages {
		pipSet[p] = true
	}
	if !pipSet["requests"] {
		t.Errorf("expected pip package 'requests', got %v", df.PipPackages)
	}

	// Convert
	id, manifest, script := ConvertDockerfileToScaffold("BestBuy Finder", df)
	if id != "bestbuy-finder" {
		t.Errorf("expected id=bestbuy-finder, got %q", id)
	}
	if !strings.Contains(manifest, `name: "BestBuy Finder"`) {
		t.Error("manifest missing app name")
	}
	if !strings.Contains(script, "class BestbuyFinder(BaseApp)") {
		t.Errorf("script should have BestbuyFinder class, got:\n%s", script[:200])
	}
	if !strings.Contains(script, "self.pkg_install(") {
		t.Error("script should have pkg_install")
	}
	t.Logf("BestBuy Finder packages: %v", df.Packages)
	t.Logf("BestBuy Finder pip packages: %v", df.PipPackages)
	t.Logf("BestBuy Finder service inferred: %s", inferMainService("bestbuy-finder", df.Packages))
}

func TestRealWorldWallcadeFPGA(t *testing.T) {
	dockerfile := `FROM ubuntu:22.04

RUN apt-get update && apt-get install -y \
    wget \
    xz-utils \
    python3 \
    python3-pip \
    python3-venv \
    python3-dev \
    libffi-dev \
    libreadline-dev \
    tcl-dev \
    graphviz \
    xdot \
    libboost-system-dev \
    libboost-python-dev \
    libboost-filesystem-dev \
    libeigen3-dev \
    udev \
    git \
    make \
    && rm -rf /var/lib/apt/lists/*

RUN pip3 install --no-cache-dir \
    migen \
    litex \
    litedram \
    liteeth

RUN pip3 install --no-cache-dir \
    meson \
    ninja \
    cocotb \
    pytest \
    numpy \
    pyserial

CMD ["/bin/bash"]
`
	df := ParseDockerfile(dockerfile)

	if df.BaseOS != "debian" {
		t.Errorf("expected debian, got %q", df.BaseOS)
	}

	// Should have many packages
	if len(df.Packages) < 5 {
		t.Errorf("expected many packages, got %d: %v", len(df.Packages), df.Packages)
	}

	// Should have pip packages
	if len(df.PipPackages) < 5 {
		t.Errorf("expected many pip packages, got %d: %v", len(df.PipPackages), df.PipPackages)
	}

	// Convert
	_, manifest, script := ConvertDockerfileToScaffold("Wallcade FPGA", df)

	// Service should not be a lib or dev package
	svc := inferMainService("wallcade-fpga", df.Packages)
	if strings.HasPrefix(svc, "lib") || strings.HasSuffix(svc, "-dev") {
		t.Errorf("service should not be a library/dev package, got %q", svc)
	}
	t.Logf("Wallcade service: %s", svc)
	t.Logf("Wallcade packages (%d): %v", len(df.Packages), df.Packages)
	t.Logf("Wallcade pip packages (%d): %v", len(df.PipPackages), df.PipPackages)

	if !strings.Contains(manifest, `name: "Wallcade FPGA"`) {
		t.Error("manifest missing app name")
	}
	if !strings.Contains(script, "class WallcadeFpga(BaseApp)") {
		t.Errorf("script should have WallcadeFpga class, got:\n%s", script[:200])
	}
}

func TestRealWorldDIEEngine(t *testing.T) {
	dockerfile := `FROM ubuntu:24.04

RUN apt update -qq && apt upgrade -y  && apt install -y wget && \
    wget https://github.com/horsicq/DIE-engine/releases/download/Beta/die_3.10_Ubuntu_24.04_amd64.deb  && \
    apt install -y ./die_3.10_Ubuntu_24.04_amd64.deb && \
    rm die_3.10_Ubuntu_24.04_amd64.deb && rm -rf /usr/lib/die/db

ENTRYPOINT ["/usr/bin/diec"]
`
	df := ParseDockerfile(dockerfile)

	if df.BaseOS != "debian" {
		t.Errorf("expected debian, got %q", df.BaseOS)
	}

	// Should have wget at minimum
	pkgSet := map[string]bool{}
	for _, p := range df.Packages {
		pkgSet[p] = true
	}
	if !pkgSet["wget"] {
		t.Errorf("expected wget, got %v", df.Packages)
	}

	// Convert — should not crash
	id, manifest, script := ConvertDockerfileToScaffold("Detect It Easy", df)
	if id != "detect-it-easy" {
		t.Errorf("expected id=detect-it-easy, got %q", id)
	}
	if !strings.Contains(manifest, `name: "Detect It Easy"`) {
		t.Error("manifest missing app name")
	}
	if !strings.Contains(script, "class DetectItEasy(BaseApp)") {
		t.Errorf("script should have DetectItEasy class")
	}
	t.Logf("DIE packages: %v", df.Packages)
	t.Logf("DIE service: %s", inferMainService("detect-it-easy", df.Packages))
}

func TestRealWorldDIEEngineBuildStage(t *testing.T) {
	// Multi-stage build Dockerfile
	dockerfile := `ARG image=ubuntu:bionic

FROM ${image} as source-internet

FROM source-internet as builder

RUN apt-get update --quiet
RUN apt-get install --quiet --assume-yes \
      git  \
      build-essential \
      qt5-default \
      qtbase5-dev \
      qtscript5-dev \
      qttools5-dev-tools

RUN git clone --recursive https://github.com/horsicq/DIE-engine.git
RUN cd DIE-engine && bash -x build_dpkg.sh && bash -x install.sh
`
	df := ParseDockerfile(dockerfile)

	// ARG image=ubuntu:bionic → resolves through alias chain to ubuntu:bionic
	if df.BaseOS != "debian" {
		t.Errorf("expected BaseOS=debian (resolved through ARG+alias chain), got %q", df.BaseOS)
	}
	if df.BaseImage != "ubuntu:bionic" {
		t.Errorf("expected BaseImage=ubuntu:bionic, got %q", df.BaseImage)
	}
	t.Logf("DIE build stage BaseOS: %s, BaseImage: %s", df.BaseOS, df.BaseImage)

	// Packages should include qt5 and build-essential
	pkgSet := map[string]bool{}
	for _, p := range df.Packages {
		pkgSet[p] = true
	}
	if !pkgSet["build-essential"] {
		t.Errorf("expected build-essential, got %v", df.Packages)
	}
	if !pkgSet["qt5-default"] {
		t.Errorf("expected qt5-default, got %v", df.Packages)
	}

	// Service inference should skip build-essential and qt5 packages
	svc := inferMainService("die-engine", df.Packages)
	if svc == "build-essential" || strings.HasPrefix(svc, "qt5") || strings.HasPrefix(svc, "qt") {
		t.Errorf("service should not be a build tool, got %q", svc)
	}
	t.Logf("DIE-engine build service: %s", svc)

	// Convert should not crash
	_, _, script := ConvertDockerfileToScaffold("DIE Engine", df)
	if !strings.Contains(script, "class DieEngine(BaseApp)") {
		t.Errorf("script should have DieEngine class")
	}
}

func TestRealWorldMultiStageFrigate(t *testing.T) {
	// Simplified Frigate-style multi-stage build
	dockerfile := `FROM debian:12 AS base
RUN apt-get update && apt-get install -y python3 python3-dev gcc

FROM debian:12-slim AS wget
RUN apt-get update && apt-get install -y wget xz-utils

FROM base AS nginx
RUN apt-get install -y nginx libpcre3-dev

FROM debian:12-slim AS runtime
RUN apt-get update && apt-get install -y \
    python3 \
    ffmpeg \
    libopencv-core-dev \
    nginx \
    curl

EXPOSE 5000 8971 1984
VOLUME /config /media
`
	df := ParseDockerfile(dockerfile)

	// Last FROM should win
	if df.BaseOS != "debian" {
		t.Errorf("expected debian, got %q", df.BaseOS)
	}

	// All ports should be captured
	if len(df.Ports) < 3 {
		t.Errorf("expected 3 ports, got %d: %v", len(df.Ports), df.Ports)
	}

	// Volumes
	if len(df.Volumes) < 2 {
		t.Errorf("expected 2 volumes, got %d: %v", len(df.Volumes), df.Volumes)
	}

	// Service should be something reasonable from the last stage
	svc := inferMainService("frigate", df.Packages)
	// Should not be a -dev package or build tool
	if strings.HasSuffix(svc, "-dev") || svc == "gcc" {
		t.Errorf("service should not be a dev package, got %q", svc)
	}
	t.Logf("Frigate packages: %v", df.Packages)
	t.Logf("Frigate service: %s", svc)

	// Convert should produce valid output
	_, manifest, script := ConvertDockerfileToScaffold("Frigate NVR", df)
	if !strings.Contains(manifest, `name: "Frigate NVR"`) {
		t.Error("manifest missing app name")
	}
	if !strings.Contains(script, "class FrigateNvr(BaseApp)") {
		t.Errorf("script should have FrigateNvr class")
	}
	// Should have port inputs for all 3 ports
	if !strings.Contains(manifest, "port_5000") {
		t.Error("manifest should have port_5000 input")
	}
}

// ---------------------------------------------------------------------------
// ARG + Stage Alias Resolution tests
// ---------------------------------------------------------------------------

func TestParseDockerfileArgResolution(t *testing.T) {
	content := `ARG BASE=alpine:3.22
FROM ${BASE}
RUN apk add --no-cache curl
`
	df := ParseDockerfile(content)
	if df.BaseImage != "alpine:3.22" {
		t.Errorf("expected BaseImage=alpine:3.22, got %q", df.BaseImage)
	}
	if df.BaseOS != "alpine" {
		t.Errorf("expected BaseOS=alpine, got %q", df.BaseOS)
	}
}

func TestParseDockerfileStageAlias(t *testing.T) {
	content := `FROM alpine:3.21 AS base
FROM base
RUN apk add --no-cache curl
`
	df := ParseDockerfile(content)
	if df.BaseImage != "alpine:3.21" {
		t.Errorf("expected BaseImage=alpine:3.21 (resolved from alias), got %q", df.BaseImage)
	}
	if df.BaseOS != "alpine" {
		t.Errorf("expected BaseOS=alpine, got %q", df.BaseOS)
	}
}

func TestParseDockerfileChainedAliases(t *testing.T) {
	content := `FROM alpine:3.20 AS base
FROM base AS stage2
FROM stage2
RUN apk add --no-cache curl
`
	df := ParseDockerfile(content)
	if df.BaseImage != "alpine:3.20" {
		t.Errorf("expected BaseImage=alpine:3.20 (resolved through chain), got %q", df.BaseImage)
	}
	if df.BaseOS != "alpine" {
		t.Errorf("expected BaseOS=alpine, got %q", df.BaseOS)
	}
}

func TestParseDockerfileArgPlusAlias(t *testing.T) {
	// Pi-hole pattern: ARG selects which stage alias to use via variable concatenation
	content := `ARG FTL_SOURCE=remote
FROM alpine:3.22 AS base
FROM base AS remote-ftl-install
FROM base AS local-ftl-install
FROM ${FTL_SOURCE}-ftl-install AS final
`
	df := ParseDockerfile(content)
	if df.BaseImage != "alpine:3.22" {
		t.Errorf("expected BaseImage=alpine:3.22 (resolved through ARG+alias chain), got %q", df.BaseImage)
	}
	if df.BaseOS != "alpine" {
		t.Errorf("expected BaseOS=alpine, got %q", df.BaseOS)
	}
}

func TestParseDockerfileUnresolvableArg(t *testing.T) {
	content := `ARG TARGETARCH
FROM base-${TARGETARCH}
`
	df := ParseDockerfile(content)
	// TARGETARCH has no default, so ${TARGETARCH} remains unresolved
	if df.BaseOS != "unknown" {
		t.Errorf("expected BaseOS=unknown for unresolvable ARG, got %q", df.BaseOS)
	}
}

func TestParseDockerfilePlatformFlag(t *testing.T) {
	content := `FROM --platform=linux/amd64 ubuntu:22.04 AS builder
FROM --platform=${TARGETPLATFORM} alpine:3.21
`
	df := ParseDockerfile(content)
	if df.BaseImage != "alpine:3.21" {
		t.Errorf("expected BaseImage=alpine:3.21 (--platform skipped), got %q", df.BaseImage)
	}
	if df.BaseOS != "alpine" {
		t.Errorf("expected BaseOS=alpine, got %q", df.BaseOS)
	}
}

func TestParseDockerfileGrafanaPattern(t *testing.T) {
	// Grafana pattern: ARG defines alias name, FROM creates alias, final FROM uses ARG
	content := `ARG BASE_IMAGE=alpine-base
FROM alpine:3.23.3 AS alpine-base
FROM ubuntu:22.04 AS ubuntu-base
FROM ${BASE_IMAGE}
RUN apk add --no-cache curl
`
	df := ParseDockerfile(content)
	if df.BaseImage != "alpine:3.23.3" {
		t.Errorf("expected BaseImage=alpine:3.23.3 (ARG→alias→concrete), got %q", df.BaseImage)
	}
	if df.BaseOS != "alpine" {
		t.Errorf("expected BaseOS=alpine, got %q", df.BaseOS)
	}
}

// ---------------------------------------------------------------------------
// ENV parsing tests
// ---------------------------------------------------------------------------

func TestParseDockerfileEnvKeyValue(t *testing.T) {
	content := `FROM alpine:3.22
ENV OLLAMA_HOST=0.0.0.0
ENV OLLAMA_MODELS=/data/models
`
	df := ParseDockerfile(content)
	if len(df.EnvVars) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(df.EnvVars))
	}
	if df.EnvVars[0].Key != "OLLAMA_HOST" || df.EnvVars[0].Default != "0.0.0.0" {
		t.Errorf("env[0]: expected OLLAMA_HOST=0.0.0.0, got %s=%s", df.EnvVars[0].Key, df.EnvVars[0].Default)
	}
	if df.EnvVars[1].Key != "OLLAMA_MODELS" || df.EnvVars[1].Default != "/data/models" {
		t.Errorf("env[1]: expected OLLAMA_MODELS=/data/models, got %s=%s", df.EnvVars[1].Key, df.EnvVars[1].Default)
	}
}

func TestParseDockerfileEnvLegacyForm(t *testing.T) {
	content := `FROM debian:12
ENV MYAPP_PORT 8080
`
	df := ParseDockerfile(content)
	if len(df.EnvVars) != 1 {
		t.Fatalf("expected 1 env var, got %d", len(df.EnvVars))
	}
	if df.EnvVars[0].Key != "MYAPP_PORT" || df.EnvVars[0].Default != "8080" {
		t.Errorf("expected MYAPP_PORT=8080, got %s=%s", df.EnvVars[0].Key, df.EnvVars[0].Default)
	}
}

func TestParseDockerfileEnvMultipleOnLine(t *testing.T) {
	content := `FROM alpine:3.22
ENV FOO=bar BAZ=qux
`
	df := ParseDockerfile(content)
	if len(df.EnvVars) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(df.EnvVars))
	}
	if df.EnvVars[0].Key != "FOO" || df.EnvVars[0].Default != "bar" {
		t.Errorf("env[0]: expected FOO=bar, got %s=%s", df.EnvVars[0].Key, df.EnvVars[0].Default)
	}
	if df.EnvVars[1].Key != "BAZ" || df.EnvVars[1].Default != "qux" {
		t.Errorf("env[1]: expected BAZ=qux, got %s=%s", df.EnvVars[1].Key, df.EnvVars[1].Default)
	}
}

func TestParseDockerfileEnvQuotedValues(t *testing.T) {
	content := `FROM alpine:3.22
ENV GREETING="hello world"
`
	df := ParseDockerfile(content)
	if len(df.EnvVars) != 1 {
		t.Fatalf("expected 1 env var, got %d", len(df.EnvVars))
	}
	if df.EnvVars[0].Key != "GREETING" || df.EnvVars[0].Default != "hello world" {
		t.Errorf("expected GREETING='hello world', got %s=%s", df.EnvVars[0].Key, df.EnvVars[0].Default)
	}
}

func TestParseDockerfileEnvFiltersDockerSpecific(t *testing.T) {
	content := `FROM debian:12
ENV PUID=1000
ENV PGID=1000
ENV TZ=America/New_York
ENV DEBIAN_FRONTEND=noninteractive
ENV MYAPP_DEBUG=true
ENV HOME=/root
ENV PATH=/usr/local/bin:/usr/bin
`
	df := ParseDockerfile(content)
	// Only MYAPP_DEBUG should survive filtering
	if len(df.EnvVars) != 1 {
		t.Fatalf("expected 1 env var after filtering, got %d: %+v", len(df.EnvVars), df.EnvVars)
	}
	if df.EnvVars[0].Key != "MYAPP_DEBUG" {
		t.Errorf("expected MYAPP_DEBUG, got %s", df.EnvVars[0].Key)
	}
}

func TestParseDockerfileEnvFiltersVariableRefs(t *testing.T) {
	content := `FROM alpine:3.22
ENV GOPATH=/go
ENV APP_HOME=$GOPATH/src/app
ENV APP_PORT=3000
`
	df := ParseDockerfile(content)
	// GOPATH is filtered (in dockerSpecificEnvVars), APP_HOME is filtered ($ref), only APP_PORT remains
	if len(df.EnvVars) != 1 {
		t.Fatalf("expected 1 env var after filtering, got %d: %+v", len(df.EnvVars), df.EnvVars)
	}
	if df.EnvVars[0].Key != "APP_PORT" {
		t.Errorf("expected APP_PORT, got %s", df.EnvVars[0].Key)
	}
}

func TestParseDockerfileEnvDeduplicates(t *testing.T) {
	content := `FROM alpine:3.22
ENV APP_PORT=3000
ENV APP_PORT=4000
`
	df := ParseDockerfile(content)
	if len(df.EnvVars) != 1 {
		t.Fatalf("expected 1 env var after dedup, got %d", len(df.EnvVars))
	}
	// First occurrence wins
	if df.EnvVars[0].Default != "3000" {
		t.Errorf("expected first value 3000, got %s", df.EnvVars[0].Default)
	}
}

// ---------------------------------------------------------------------------
// README generation tests
// ---------------------------------------------------------------------------

func TestGenerateReadmeBasic(t *testing.T) {
	readme := GenerateReadme("TestApp", "Dockerfile", "A test application", nil, "")
	if !strings.Contains(readme, "# TestApp") {
		t.Error("README should contain app name as heading")
	}
	if !strings.Contains(readme, "A test application") {
		t.Error("README should contain description")
	}
	if !strings.Contains(readme, "Imported from Dockerfile") {
		t.Error("README should mention import source")
	}
}

func TestGenerateReadmeWithDockerfileInfo(t *testing.T) {
	df := &DockerfileInfo{
		BaseImage: "alpine:3.22",
		BaseOS:    "alpine",
		Ports:     []string{"8080", "443"},
		Volumes:   []string{"/data", "/config"},
		EnvVars:   []EnvVar{{Key: "APP_PORT", Default: "8080"}, {Key: "APP_DEBUG", Default: "false"}},
		Packages:  []string{"nginx", "curl"},
	}
	readme := GenerateReadme("MyApp", "Unraid template", "My cool app", df, "https://example.com")
	if !strings.Contains(readme, "alpine:3.22") {
		t.Error("README should contain base image")
	}
	if !strings.Contains(readme, "8080") {
		t.Error("README should list ports")
	}
	if !strings.Contains(readme, "/data") {
		t.Error("README should list volumes")
	}
	if !strings.Contains(readme, "APP_PORT") {
		t.Error("README should list env vars")
	}
	if !strings.Contains(readme, "nginx") {
		t.Error("README should list packages")
	}
	if !strings.Contains(readme, "https://example.com") {
		t.Error("README should include homepage")
	}
}

// ---------------------------------------------------------------------------
// envKeyToLabel tests
// ---------------------------------------------------------------------------

func TestEnvKeyToLabel(t *testing.T) {
	tests := []struct {
		key, want string
	}{
		{"OLLAMA_HOST", "Ollama Host"},
		{"APP_PORT", "App Port"},
		{"DEBUG", "Debug"},
		{"MY_COMPLEX_VAR_NAME", "My Complex Var Name"},
	}
	for _, tc := range tests {
		got := envKeyToLabel(tc.key)
		if got != tc.want {
			t.Errorf("envKeyToLabel(%q) = %q, want %q", tc.key, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// COPY/ADD parsing tests
// ---------------------------------------------------------------------------

func TestParseCOPY(t *testing.T) {
	content := `FROM ubuntu:22.04
COPY root/ /
`
	df := ParseDockerfile(content)
	if len(df.CopyInstructions) != 1 {
		t.Fatalf("expected 1 COPY, got %d", len(df.CopyInstructions))
	}
	ci := df.CopyInstructions[0]
	if ci.Src != "root/" || ci.Dest != "/" {
		t.Errorf("expected COPY root/ /, got %q -> %q", ci.Src, ci.Dest)
	}
	if ci.IsAdd || ci.IsURL || ci.FromStage != "" {
		t.Errorf("unexpected flags: IsAdd=%v IsURL=%v FromStage=%q", ci.IsAdd, ci.IsURL, ci.FromStage)
	}
}

func TestParseCOPYFromStage(t *testing.T) {
	content := `FROM golang:1.21 AS builder
RUN go build -o /app
FROM ubuntu:22.04
COPY --from=builder /app /usr/local/bin/app
`
	df := ParseDockerfile(content)
	if len(df.CopyInstructions) != 1 {
		t.Fatalf("expected 1 COPY, got %d", len(df.CopyInstructions))
	}
	ci := df.CopyInstructions[0]
	if ci.FromStage != "builder" {
		t.Errorf("expected FromStage=builder, got %q", ci.FromStage)
	}
	if ci.Src != "/app" || ci.Dest != "/usr/local/bin/app" {
		t.Errorf("expected /app -> /usr/local/bin/app, got %q -> %q", ci.Src, ci.Dest)
	}
}

func TestParseCOPYFlags(t *testing.T) {
	content := `FROM ubuntu:22.04
COPY --chown=1000:1000 --chmod=755 config.json /etc/app/
`
	df := ParseDockerfile(content)
	if len(df.CopyInstructions) != 1 {
		t.Fatalf("expected 1 COPY, got %d", len(df.CopyInstructions))
	}
	ci := df.CopyInstructions[0]
	if ci.Src != "config.json" || ci.Dest != "/etc/app/" {
		t.Errorf("expected config.json -> /etc/app/, got %q -> %q", ci.Src, ci.Dest)
	}
	if ci.FromStage != "" {
		t.Errorf("expected empty FromStage, got %q", ci.FromStage)
	}
}

func TestParseADD(t *testing.T) {
	content := `FROM ubuntu:22.04
ADD scripts/ /opt/scripts/
`
	df := ParseDockerfile(content)
	if len(df.CopyInstructions) != 1 {
		t.Fatalf("expected 1 ADD, got %d", len(df.CopyInstructions))
	}
	ci := df.CopyInstructions[0]
	if !ci.IsAdd {
		t.Error("expected IsAdd=true")
	}
	if ci.Src != "scripts/" || ci.Dest != "/opt/scripts/" {
		t.Errorf("expected scripts/ -> /opt/scripts/, got %q -> %q", ci.Src, ci.Dest)
	}
}

func TestParseADDWithURL(t *testing.T) {
	content := `FROM ubuntu:22.04
ADD https://example.com/file.tar.gz /tmp/
`
	df := ParseDockerfile(content)
	if len(df.CopyInstructions) != 1 {
		t.Fatalf("expected 1 ADD, got %d", len(df.CopyInstructions))
	}
	ci := df.CopyInstructions[0]
	if !ci.IsAdd || !ci.IsURL {
		t.Error("expected IsAdd=true, IsURL=true")
	}
	if ci.Src != "https://example.com/file.tar.gz" {
		t.Errorf("expected URL src, got %q", ci.Src)
	}
}

// ---------------------------------------------------------------------------
// CMD/ENTRYPOINT parsing tests
// ---------------------------------------------------------------------------

func TestParseCMDExecForm(t *testing.T) {
	content := `FROM ubuntu:22.04
CMD ["python", "app.py", "--port", "8080"]
`
	df := ParseDockerfile(content)
	if df.StartupCmd != "python app.py --port 8080" {
		t.Errorf("expected 'python app.py --port 8080', got %q", df.StartupCmd)
	}
}

func TestParseCMDShellForm(t *testing.T) {
	content := `FROM ubuntu:22.04
CMD python app.py --port 8080
`
	df := ParseDockerfile(content)
	if df.StartupCmd != "python app.py --port 8080" {
		t.Errorf("expected 'python app.py --port 8080', got %q", df.StartupCmd)
	}
}

func TestParseCMDLastOneWins(t *testing.T) {
	content := `FROM ubuntu:22.04
CMD ["first"]
CMD ["second"]
`
	df := ParseDockerfile(content)
	if df.StartupCmd != "second" {
		t.Errorf("expected last CMD wins, got %q", df.StartupCmd)
	}
}

func TestParseENTRYPOINTExecForm(t *testing.T) {
	content := `FROM ubuntu:22.04
ENTRYPOINT ["/usr/bin/diec"]
`
	df := ParseDockerfile(content)
	if df.EntrypointCmd != "/usr/bin/diec" {
		t.Errorf("expected '/usr/bin/diec', got %q", df.EntrypointCmd)
	}
}

func TestParseENTRYPOINTShellForm(t *testing.T) {
	content := `FROM ubuntu:22.04
ENTRYPOINT /usr/bin/myapp --serve
`
	df := ParseDockerfile(content)
	if df.EntrypointCmd != "/usr/bin/myapp --serve" {
		t.Errorf("expected '/usr/bin/myapp --serve', got %q", df.EntrypointCmd)
	}
}

// ---------------------------------------------------------------------------
// RUN command categorization tests
// ---------------------------------------------------------------------------

func TestRunCategorizeMkdir(t *testing.T) {
	content := `FROM ubuntu:22.04
RUN mkdir -p /opt/app/data /var/log/app
`
	df := ParseDockerfile(content)
	dirSet := map[string]bool{}
	for _, d := range df.Directories {
		dirSet[d] = true
	}
	if !dirSet["/opt/app/data"] || !dirSet["/var/log/app"] {
		t.Errorf("expected /opt/app/data and /var/log/app in Directories, got %v", df.Directories)
	}
	// Should have a "mkdir" RunCommand
	found := false
	for _, rc := range df.RunCommands {
		if rc.Type == "mkdir" {
			found = true
		}
	}
	if !found {
		t.Error("expected RunCommand with type 'mkdir'")
	}
}

func TestRunCategorizeUseradd(t *testing.T) {
	content := `FROM ubuntu:22.04
RUN useradd -r -s /bin/false abc
`
	df := ParseDockerfile(content)
	if len(df.Users) != 1 || df.Users[0] != "abc" {
		t.Errorf("expected Users=[abc], got %v", df.Users)
	}
}

func TestRunCategorizeAdduser(t *testing.T) {
	content := `FROM alpine:3.22
RUN adduser -D -H -s /sbin/nologin myuser
`
	df := ParseDockerfile(content)
	if len(df.Users) != 1 || df.Users[0] != "myuser" {
		t.Errorf("expected Users=[myuser], got %v", df.Users)
	}
}

func TestRunCategorizeDownloadCurl(t *testing.T) {
	content := `FROM ubuntu:22.04
RUN curl -fsSL -o /tmp/app.tar.gz https://example.com/app.tar.gz
`
	df := ParseDockerfile(content)
	if len(df.Downloads) != 1 {
		t.Fatalf("expected 1 download, got %d", len(df.Downloads))
	}
	dl := df.Downloads[0]
	if dl.URL != "https://example.com/app.tar.gz" || dl.Dest != "/tmp/app.tar.gz" {
		t.Errorf("unexpected download: URL=%q Dest=%q", dl.URL, dl.Dest)
	}
}

func TestRunCategorizeDownloadWget(t *testing.T) {
	content := `FROM ubuntu:22.04
RUN wget -O /tmp/app.deb https://example.com/app.deb
`
	df := ParseDockerfile(content)
	if len(df.Downloads) != 1 {
		t.Fatalf("expected 1 download, got %d", len(df.Downloads))
	}
	dl := df.Downloads[0]
	if dl.URL != "https://example.com/app.deb" || dl.Dest != "/tmp/app.deb" {
		t.Errorf("unexpected download: URL=%q Dest=%q", dl.URL, dl.Dest)
	}
}

func TestRunCategorizeSymlink(t *testing.T) {
	content := `FROM ubuntu:22.04
RUN ln -sf /usr/share/nginx/html /var/www
`
	df := ParseDockerfile(content)
	if len(df.Symlinks) != 1 {
		t.Fatalf("expected 1 symlink, got %d", len(df.Symlinks))
	}
	sl := df.Symlinks[0]
	if sl.Target != "/usr/share/nginx/html" || sl.Link != "/var/www" {
		t.Errorf("unexpected symlink: Target=%q Link=%q", sl.Target, sl.Link)
	}
}

func TestRunCategorizeSed(t *testing.T) {
	content := `FROM ubuntu:22.04
RUN sed -i 's/foo/bar/g' /etc/app/config.conf
`
	df := ParseDockerfile(content)
	found := false
	for _, rc := range df.RunCommands {
		if rc.Type == "sed" {
			found = true
		}
	}
	if !found {
		t.Error("expected RunCommand with type 'sed'")
	}
}

func TestRunCategorizeSkip(t *testing.T) {
	content := `FROM ubuntu:22.04
RUN apt-get update && apt-get install -y curl && rm -rf /var/lib/apt/lists/* && apt-get clean && echo "**** cleanup ****"
`
	df := ParseDockerfile(content)
	// Shell noise should NOT appear in RunCommands
	for _, rc := range df.RunCommands {
		if rc.Type != "skip" && strings.Contains(rc.Original, "rm -rf /var/lib/apt") {
			t.Errorf("'rm -rf /var/lib/apt' should be filtered as noise, got type=%q", rc.Type)
		}
	}
}

func TestRunCategorizeUnknown(t *testing.T) {
	content := `FROM ubuntu:22.04
RUN /opt/app/custom-setup.sh --init
`
	df := ParseDockerfile(content)
	found := false
	for _, rc := range df.RunCommands {
		if rc.Type == "unknown" && strings.Contains(rc.Original, "custom-setup.sh") {
			found = true
		}
	}
	if !found {
		t.Error("expected RunCommand with type 'unknown' for unrecognized command")
	}
}

func TestSplitRunSemicolon(t *testing.T) {
	content := `FROM ubuntu:22.04
RUN mkdir -p /data; useradd -r app; echo done
`
	df := ParseDockerfile(content)
	if len(df.Directories) == 0 || df.Directories[0] != "/data" {
		t.Errorf("expected /data from semicolon-split, got %v", df.Directories)
	}
	if len(df.Users) == 0 || df.Users[0] != "app" {
		t.Errorf("expected user 'app' from semicolon-split, got %v", df.Users)
	}
}

func TestRunCategorizeChmod(t *testing.T) {
	content := `FROM ubuntu:22.04
RUN chmod 755 /opt/app/run.sh
`
	df := ParseDockerfile(content)
	found := false
	for _, rc := range df.RunCommands {
		if rc.Type == "chmod" {
			found = true
		}
	}
	if !found {
		t.Error("expected RunCommand with type 'chmod'")
	}
}

func TestRunCategorizeChown(t *testing.T) {
	content := `FROM ubuntu:22.04
RUN chown -R app:app /opt/app
`
	df := ParseDockerfile(content)
	found := false
	for _, rc := range df.RunCommands {
		if rc.Type == "chown" {
			found = true
		}
	}
	if !found {
		t.Error("expected RunCommand with type 'chown'")
	}
}

func TestRunCategorizeGitClone(t *testing.T) {
	content := `FROM ubuntu:22.04
RUN git clone --depth 1 https://github.com/user/repo.git /opt/repo
`
	df := ParseDockerfile(content)
	found := false
	for _, rc := range df.RunCommands {
		if rc.Type == "git_clone" {
			found = true
		}
	}
	if !found {
		t.Error("expected RunCommand with type 'git_clone'")
	}
}

// ---------------------------------------------------------------------------
// Scaffold generation with new fields
// ---------------------------------------------------------------------------

func TestScaffoldWithPipPackages(t *testing.T) {
	df := &DockerfileInfo{
		BaseOS:      "debian",
		Packages:    []string{"python3"},
		PipPackages: []string{"certbot", "certbot-dns-cloudflare"},
	}
	_, manifest, script := ConvertDockerfileToScaffold("Certbot App", df)
	if !strings.Contains(script, "self.pip_install(") {
		t.Error("script should contain self.pip_install()")
	}
	if !strings.Contains(script, "certbot") {
		t.Error("script should contain certbot pip package")
	}
	if !strings.Contains(manifest, "pip:") {
		t.Error("manifest should contain pip in permissions")
	}
}

func TestScaffoldWithUsers(t *testing.T) {
	df := &DockerfileInfo{
		BaseOS:   "debian",
		Packages: []string{"nginx"},
		Users:    []string{"abc"},
	}
	_, _, script := ConvertDockerfileToScaffold("User App", df)
	if !strings.Contains(script, `self.create_user("abc")`) {
		t.Errorf("script should contain create_user(\"abc\"), got:\n%s", script)
	}
}

func TestScaffoldWithCOPYAndRepo(t *testing.T) {
	df := &DockerfileInfo{
		BaseOS:   "debian",
		Packages: []string{"nginx"},
		RepoURL:  "https://github.com/linuxserver/docker-swag",
		CopyInstructions: []CopyInstruction{
			{Src: "config/", Dest: "/etc/nginx/"},
		},
	}
	_, manifest, script := ConvertDockerfileToScaffold("SWAG", df)
	if !strings.Contains(script, `"git", "clone"`) {
		t.Logf("Script:\n%s", script)
		t.Error("script should contain git clone when RepoURL is available")
	}
	if !strings.Contains(script, "docker-swag.git") {
		t.Error("script should reference the repo URL")
	}
	if !strings.Contains(script, `"cp"`) {
		t.Error("script should contain cp command")
	}
	if !strings.Contains(manifest, "commands:") {
		t.Error("manifest should have commands in permissions")
	}
}

func TestScaffoldSkipsDockerInitCopy(t *testing.T) {
	df := &DockerfileInfo{
		BaseOS:   "debian",
		Packages: []string{"nginx"},
		RepoURL:  "https://github.com/linuxserver/docker-swag",
		CopyInstructions: []CopyInstruction{
			{Src: "root/", Dest: "/"},
		},
	}
	_, _, script := ConvertDockerfileToScaffold("SWAG", df)
	if strings.Contains(script, `"git", "clone"`) {
		t.Error("script should NOT git clone when only COPY is root/ -> / (Docker init scripts)")
	}
}

func TestScaffoldWithCOPYNoRepo(t *testing.T) {
	df := &DockerfileInfo{
		BaseOS:   "debian",
		Packages: []string{"nginx"},
		CopyInstructions: []CopyInstruction{
			{Src: "config.json", Dest: "/etc/app/"},
		},
	}
	_, _, script := ConvertDockerfileToScaffold("No Repo App", df)
	if !strings.Contains(script, "TODO: COPY instructions found but source repo URL unknown") {
		t.Error("script should have TODO comment when no RepoURL")
	}
	if !strings.Contains(script, "COPY config.json /etc/app/") {
		t.Error("script should list the COPY instructions in TODO comments")
	}
}

func TestScaffoldWithCOPYSkipsFromStage(t *testing.T) {
	df := &DockerfileInfo{
		BaseOS:   "debian",
		Packages: []string{"nginx"},
		RepoURL:  "https://github.com/user/repo",
		CopyInstructions: []CopyInstruction{
			{Src: "/app", Dest: "/usr/local/bin/app", FromStage: "builder"},
			{Src: "config/", Dest: "/etc/app/"},
		},
	}
	_, _, script := ConvertDockerfileToScaffold("Stage App", df)
	// Should only have git clone for the non-staged COPY
	if !strings.Contains(script, `"git", "clone"`) {
		t.Error("script should have git clone for non-staged COPY")
	}
	// Should NOT copy the --from=builder artifact
	if strings.Contains(script, "/usr/local/bin/app") {
		t.Error("script should skip COPY --from=builder")
	}
}

func TestScaffoldWithCMD(t *testing.T) {
	df := &DockerfileInfo{
		BaseOS:     "debian",
		Packages:   []string{"myapp"},
		StartupCmd: "python app.py --serve",
	}
	_, _, script := ConvertDockerfileToScaffold("CMD App", df)
	if !strings.Contains(script, "self.create_service(") {
		t.Error("script should use create_service when StartupCmd is set")
	}
	if !strings.Contains(script, "python app.py --serve") {
		t.Error("script should contain the startup command")
	}
}

func TestScaffoldWithEntrypoint(t *testing.T) {
	df := &DockerfileInfo{
		BaseOS:        "debian",
		Packages:      []string{"myapp"},
		EntrypointCmd: "/usr/bin/diec",
	}
	_, _, script := ConvertDockerfileToScaffold("EP App", df)
	if !strings.Contains(script, "self.create_service(") {
		t.Error("script should use create_service when EntrypointCmd is set")
	}
	if !strings.Contains(script, "/usr/bin/diec") {
		t.Error("script should contain the entrypoint command")
	}
}

func TestScaffoldStartupPriority(t *testing.T) {
	// ExecCmd > EntrypointCmd > StartupCmd
	df := &DockerfileInfo{
		BaseOS:        "debian",
		Packages:      []string{"myapp"},
		ExecCmd:       "/usr/bin/from-s6",
		EntrypointCmd: "/usr/bin/from-entrypoint",
		StartupCmd:    "/usr/bin/from-cmd",
	}
	_, _, script := ConvertDockerfileToScaffold("Priority App", df)
	if !strings.Contains(script, "/usr/bin/from-s6") {
		t.Error("ExecCmd should have highest priority")
	}
	if strings.Contains(script, "from-entrypoint") || strings.Contains(script, "from-cmd") {
		t.Error("lower priority commands should not appear when ExecCmd is set")
	}
}

func TestScaffoldWithTodoComments(t *testing.T) {
	df := &DockerfileInfo{
		BaseOS:   "debian",
		Packages: []string{"nginx"},
		RunCommands: []RunCommand{
			{Type: "sed", Original: "sed -i 's/foo/bar/' /etc/config"},
			{Type: "unknown", Original: "/opt/custom-setup.sh"},
		},
	}
	_, manifest, script := ConvertDockerfileToScaffold("Todo App", df)

	// sed is emitted as a real run_command() call
	if !strings.Contains(script, `run_command(["sed"`) {
		t.Error("script should emit sed as run_command()")
	}
	// unknown commands remain as TODO
	if !strings.Contains(script, "TODO: Review and adapt") {
		t.Error("script should have TODO section for unknown commands")
	}
	if !strings.Contains(script, "/opt/custom-setup.sh") {
		t.Error("script should include the unknown command in TODO")
	}
	// sed command name should be in permissions commands list
	if !strings.Contains(manifest, "  commands:\n") || !strings.Contains(manifest, "    - sed\n") {
		t.Error("manifest should include sed in commands permissions")
	}
	// /etc/ path from sed target should be in permissions paths
	if !strings.Contains(manifest, "/etc/") {
		t.Error("manifest should include /etc/ path from sed target in permissions")
	}
}

func TestScaffoldWithDownloads(t *testing.T) {
	df := &DockerfileInfo{
		BaseOS:   "debian",
		Packages: []string{"nginx"},
		Downloads: []DownloadAction{
			{URL: "https://example.com/app.tar.gz", Dest: "/tmp/app.tar.gz"},
		},
	}
	_, manifest, script := ConvertDockerfileToScaffold("DL App", df)
	if !strings.Contains(script, `self.download("https://example.com/app.tar.gz"`) {
		t.Error("script should contain self.download()")
	}
	if !strings.Contains(manifest, "https://example.com/app.tar.gz") {
		t.Error("manifest should list download URL in permissions")
	}
}

func TestScaffoldWithSymlinks(t *testing.T) {
	df := &DockerfileInfo{
		BaseOS:   "debian",
		Packages: []string{"nginx"},
		Symlinks: []SymlinkAction{
			{Target: "/usr/share/nginx/html", Link: "/var/www"},
		},
	}
	_, _, script := ConvertDockerfileToScaffold("Link App", df)
	if !strings.Contains(script, `self.run_command(["ln", "-sf"`) {
		t.Error("script should contain ln -sf command")
	}
}

func TestScaffoldDirectoriesMerged(t *testing.T) {
	df := &DockerfileInfo{
		BaseOS:      "debian",
		Packages:    []string{"nginx"},
		Volumes:     []string{"/config", "/data"},
		Directories: []string{"/data", "/var/log/app"}, // /data overlaps with Volumes
	}
	_, _, script := ConvertDockerfileToScaffold("Dir App", df)
	// Should have /config, /data, /var/log/app (deduped)
	if !strings.Contains(script, `"/config"`) {
		t.Error("script should create /config")
	}
	if !strings.Contains(script, `"/data"`) {
		t.Error("script should create /data")
	}
	if !strings.Contains(script, `"/var/log/app"`) {
		t.Error("script should create /var/log/app from Directories")
	}
	// /data should appear only once in create_dir calls
	count := strings.Count(script, `self.create_dir("/data")`)
	if count != 1 {
		t.Errorf("expected /data to appear once in create_dir, got %d times", count)
	}
}
