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
	if !strings.Contains(manifest, "alpine-3.21") {
		t.Error("alpine base should use alpine-3.21 template")
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
	if !strings.Contains(manifest, "alpine-3.21") {
		t.Error("alpine base should use alpine-3.21 template")
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
