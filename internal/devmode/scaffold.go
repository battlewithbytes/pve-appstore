package devmode

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ensurePipPrereqs adds pip prerequisite system packages (from the OS profile)
// to df.Packages and the last PackageLayer when pip packages are present.
func ensurePipPrereqs(df *DockerfileInfo) {
	if df == nil || len(df.PipPackages) == 0 {
		return
	}
	profile := ProfileFor(df.BaseOS)
	existing := make(map[string]bool, len(df.Packages))
	for _, p := range df.Packages {
		existing[p] = true
	}
	for _, p := range profile.PipPrereqs {
		if !existing[p] {
			df.Packages = append(df.Packages, p)
			// Also add to the last PackageLayer so the script installs them
			if len(df.PackageLayers) > 0 {
				last := &df.PackageLayers[len(df.PackageLayers)-1]
				last.Packages = append(last.Packages, p)
			}
		}
	}
}

// ConvertDockerfileToScaffold generates an app ID, app.yml, and install.py from
// a user-provided name and parsed DockerfileInfo (no Unraid XML required).
func ConvertDockerfileToScaffold(name string, df *DockerfileInfo) (id, manifest, script string) {
	id = toKebabCase(name)

	description := "Imported from Dockerfile: " + name
	if len(description) > 200 {
		description = description[:200] + "..."
	}

	// Ensure pip prerequisites are in the package list
	ensurePipPrereqs(df)

	// Determine OS template from profile
	profile := ProfileFor("")
	if df != nil {
		profile = ProfileFor(df.BaseOS)
	}
	osTemplate := profile.OSTemplate

	// If no Dockerfile data and nothing meaningful extracted, generate a simple scaffold
	if df == nil || (len(df.Packages) == 0 && len(df.AptKeys) == 0 && len(df.AptRepos) == 0 &&
		len(df.PipPackages) == 0 && len(df.RunCommands) == 0 && len(df.CopyInstructions) == 0) {
		return convertDockerfileScaffold(id, name, description, osTemplate, df)
	}

	// Collect ports and volumes from Dockerfile
	var portInputs []portInputInfo
	for _, p := range df.Ports {
		key := "port_" + p
		portInputs = append(portInputs, portInputInfo{key: key, port: p, defaultVal: p})
	}

	// Volumes
	var paths []volumePathInfo
	for _, v := range df.Volumes {
		paths = append(paths, volumePathInfo{name: "Data", target: v})
	}

	// Infer main service name from the app layer packages only (not base layers)
	mainService := inferMainService(id, appLayerPackages(df))

	// Build manifest
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`id: %s
name: "%s"
description: "%s"
version: "0.1.0"
categories:
  - utilities
tags:
  - dockerfile-import`, id, name, description))
	sb.WriteString("\nmaintainers:\n  - \"Your Name\"\n")
	sb.WriteString("icon: \"\"  # Paste icon URL or use icon editor in header\n")

	sb.WriteString(fmt.Sprintf(`
lxc:
  ostemplate: "%s"
  defaults:
    unprivileged: true
    cores: 2
    memory_mb: 1024
    disk_gb: 8
    onboot: true
`, osTemplate))

	// Inputs from ports + env vars
	if len(portInputs) > 0 || len(df.EnvVars) > 0 {
		sb.WriteString("\ninputs:\n")
		for _, p := range portInputs {
			sb.WriteString(fmt.Sprintf(`  - key: %s
    label: "Port %s"
    type: number
    default: %s
    required: true
    validation:
      min: 1
      max: 65535
    help: "Container port %s"
`, p.key, p.port, p.defaultVal, p.port))
		}
		for _, ev := range df.EnvVars {
			key := toSnakeCase(ev.Key)
			label := envKeyToLabel(ev.Key)
			sb.WriteString(fmt.Sprintf(`  - key: %s
    label: "%s"
    type: string
    default: "%s"
    required: false
    help: "Environment variable %s"
`, key, label, strings.ReplaceAll(ev.Default, `"`, `\"`), ev.Key))
		}
	}

	// Volumes section from Dockerfile VOLUME directives
	if len(df.Volumes) > 0 {
		sb.WriteString("\nvolumes:\n")
		namesSeen := map[string]int{}
		for _, v := range df.Volumes {
			name := volumeNameFromPath(v)
			namesSeen[name]++
			if namesSeen[name] > 1 {
				name = fmt.Sprintf("%s-%d", name, namesSeen[name])
			}
			label := volumeLabelFromPath(v)
			sb.WriteString(fmt.Sprintf(`  - name: %s
    type: volume
    mount_path: %s
    size_gb: 8
    label: "%s"
    required: true
    description: "Data stored at %s"
`, name, v, label, v))
		}
	}

	sb.WriteString(`
provisioning:
  script: provision/install.py
  timeout_sec: 600

`)

	// Permissions section
	sb.WriteString("permissions:\n")
	allPermPkgs := df.Packages
	if len(df.PipPackages) > 0 {
		sb.WriteString("  packages:\n")
		for _, pkg := range allPermPkgs {
			sb.WriteString(fmt.Sprintf("    - %s\n", pkg))
		}
		sb.WriteString("  pip:\n")
		for _, pkg := range df.PipPackages {
			sb.WriteString(fmt.Sprintf("    - %s\n", pkg))
		}
	} else if len(allPermPkgs) > 0 {
		sb.WriteString("  packages:\n")
		for _, pkg := range allPermPkgs {
			sb.WriteString(fmt.Sprintf("    - %s\n", pkg))
		}
	}
	if len(df.AptRepos) > 0 {
		sb.WriteString("  apt_repos:\n")
		for _, r := range df.AptRepos {
			// Emit just the repo URL, not the full deb line — the permission
			// check matches on URL, so this is less fragile than exact-line match.
			if u := extractURLFromRepoLine(r.Line); u != "" {
				sb.WriteString(fmt.Sprintf("    - \"%s\"\n", strings.TrimRight(u, "/")))
			} else {
				sb.WriteString(fmt.Sprintf("    - \"%s\"\n", r.Line))
			}
		}
	}
	var urls []string
	for _, k := range df.AptKeys {
		urls = append(urls, k.URL)
	}
	for _, r := range df.AptRepos {
		if u := extractURLFromRepoLine(r.Line); u != "" {
			urls = append(urls, u+"*")
		}
	}
	for _, dl := range df.Downloads {
		urls = append(urls, dl.URL)
	}
	if df.RepoURL != "" {
		urls = append(urls, df.RepoURL+"*")
	}
	if len(urls) > 0 {
		sb.WriteString("  urls:\n")
		for _, u := range dedup(urls) {
			sb.WriteString(fmt.Sprintf("    - \"%s\"\n", u))
		}
	}
	var permPaths []string
	for _, p := range paths {
		permPaths = append(permPaths, p.target)
	}
	for _, d := range df.Directories {
		permPaths = append(permPaths, d)
	}
	if len(df.AptRepos) > 0 {
		permPaths = append(permPaths, "/etc/apt/sources.list.d/")
	}
	if len(df.AptKeys) > 0 {
		permPaths = append(permPaths, "/usr/share/keyrings/")
	}
	permPaths = append(permPaths, "/etc/systemd/system/")
	// Extract paths referenced by run_command() calls (sed targets, mv destinations, etc.)
	permPaths = append(permPaths, extractPathsFromRunCommands(df.RunCommands)...)
	if len(permPaths) > 0 {
		sb.WriteString("  paths:\n")
		for _, p := range dedup(permPaths) {
			sb.WriteString(fmt.Sprintf("    - %s\n", p))
		}
	}
	// Commands — extract from what the script will actually call via run_command()
	cmds := collectScriptCommands(df)
	if len(cmds) > 0 {
		sb.WriteString("  commands:\n")
		for _, c := range cmds {
			sb.WriteString(fmt.Sprintf("    - %s\n", c))
		}
	}
	if len(df.Users) > 0 {
		sb.WriteString("  users:\n")
		for _, u := range df.Users {
			sb.WriteString(fmt.Sprintf("    - %s\n", u))
		}
	}
	sb.WriteString("  services:\n")
	for _, svc := range inferImpliedServices(df.Packages, mainService, df.BaseImage) {
		sb.WriteString(fmt.Sprintf("    - %s\n", svc))
	}
	sb.WriteString(fmt.Sprintf("    - %s\n", mainService))
	sb.WriteString("\n")

	// Outputs
	if len(portInputs) > 0 {
		sb.WriteString(fmt.Sprintf(`outputs:
  - key: url
    label: "Web UI"
    value: "http://{{IP}}:{{%s}}"
  - key: webui_port
    label: "Web UI Port"
    value: "{{%s}}"
`, portInputs[0].key, portInputs[0].key))
	} else {
		sb.WriteString(`outputs:
  - key: url
    label: "Access URL"
    value: "http://{{IP}}/"
`)
	}

	manifest = sb.String()

	// Build script with real SDK v2 calls
	script = buildInstallScript(buildScriptParams{
		name:        name,
		className:   toPascalCase(id),
		docstring:   fmt.Sprintf("Provisioning script for %s.\nConverted from Dockerfile analysis.", name),
		df:          df,
		portInputs:  portInputs,
		envInputs:   df.EnvVars,
		volumePaths: paths,
		mainService: mainService,
	})

	return id, manifest, script
}

// convertDockerfileScaffold generates a simple scaffold when Dockerfile has no packages.
func convertDockerfileScaffold(id, name, description, osTemplate string, df *DockerfileInfo) (string, string, string) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`id: %s
name: "%s"
description: "%s"
version: "0.1.0"
categories:
  - utilities
tags:
  - dockerfile-import`, id, name, description))
	sb.WriteString("\nmaintainers:\n  - \"Your Name\"\n")
	sb.WriteString("icon: \"\"  # Paste icon URL or use icon editor in header\n")
	sb.WriteString("# This is a SCAFFOLD — you must implement the provisioning logic.\n")

	sb.WriteString(fmt.Sprintf(`
lxc:
  ostemplate: "%s"
  defaults:
    unprivileged: true
    cores: 2
    memory_mb: 1024
    disk_gb: 8
    onboot: true
`, osTemplate))

	// Inputs from ports
	if df != nil && len(df.Ports) > 0 {
		sb.WriteString("\ninputs:\n")
		for _, p := range df.Ports {
			key := "port_" + p
			sb.WriteString(fmt.Sprintf(`  - key: %s
    label: "Port %s"
    type: number
    default: %s
    required: true
    validation:
      min: 1
      max: 65535
    help: "Container port %s"
`, key, p, p, p))
		}
	}

	sb.WriteString(`
provisioning:
  script: provision/install.py
  timeout_sec: 600

`)

	// Outputs
	if df != nil && len(df.Ports) > 0 {
		key := "port_" + df.Ports[0]
		sb.WriteString(fmt.Sprintf(`outputs:
  - key: url
    label: "Web UI"
    value: "http://{{IP}}:{{%s}}"
  - key: webui_port
    label: "Web UI Port"
    value: "{{%s}}"
`, key, key))
	} else {
		sb.WriteString(`outputs:
  - key: url
    label: "Access URL"
    value: "http://{{IP}}/"
`)
	}

	manifest := sb.String()

	// Build script
	className := toPascalCase(id)
	var scriptParts []string
	scriptParts = append(scriptParts, fmt.Sprintf(`#!/usr/bin/env python3
"""
Provisioning script for %s.
Imported from Dockerfile — implement the provisioning logic below.
"""
from appstore import BaseApp, run


class %s(BaseApp):
    def install(self):`, name, className))

	// Port inputs
	if df != nil && len(df.Ports) > 0 {
		scriptParts = append(scriptParts, "        # Read inputs")
		for _, p := range df.Ports {
			key := "port_" + p
			varName := toSnakeCase(key)
			scriptParts = append(scriptParts, fmt.Sprintf(`        %s = self.inputs.integer("%s", %s)`, varName, key, p))
		}
		scriptParts = append(scriptParts, "")
	}

	scriptParts = append(scriptParts, `        # Step 1: Install packages
        # TODO: Replace with the actual packages needed
        # self.apt_install(["package1", "package2"])`)

	if df != nil && len(df.Volumes) > 0 {
		scriptParts = append(scriptParts, "\n        # Step 2: Create data directories")
		for _, v := range df.Volumes {
			scriptParts = append(scriptParts, fmt.Sprintf(`        self.create_dir("%s")`, v))
		}
	}

	scriptParts = append(scriptParts, `
        # Step 3: Configure the application
        # TODO: Write config files, set up users, etc.

        # Step 4: Create and enable systemd service
        # TODO: Create a service unit for the application

        self.log.info("Installation complete — configure the application manually")`)

	scriptParts = append(scriptParts, fmt.Sprintf(`

run(%s)
`, className))

	script := strings.Join(scriptParts, "\n")
	return id, manifest, script
}

// GenerateReadme creates a README.md from import data.
// name is the app name, source describes the import origin (e.g. "Unraid template", "Dockerfile"),
// description is a short text about the app, and df provides parsed Dockerfile data (may be nil).
func GenerateReadme(name, source, description string, df *DockerfileInfo, homepage string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s\n\n", name))

	if description != "" {
		sb.WriteString(description + "\n\n")
	}

	sb.WriteString(fmt.Sprintf("*Imported from %s.*\n\n", source))

	if homepage != "" {
		sb.WriteString(fmt.Sprintf("**Homepage:** %s\n\n", homepage))
	}

	if df == nil {
		sb.WriteString("## Setup\n\nTODO: Add setup instructions.\n")
		return sb.String()
	}

	// Base image info
	sb.WriteString("## Details\n\n")
	sb.WriteString(fmt.Sprintf("| Property | Value |\n|---|---|\n"))
	if df.BaseImage != "" {
		sb.WriteString(fmt.Sprintf("| Base Image | `%s` |\n", df.BaseImage))
	}
	if df.BaseOS != "" && df.BaseOS != "unknown" {
		sb.WriteString(fmt.Sprintf("| Base OS | %s |\n", df.BaseOS))
	}

	// Ports
	if len(df.Ports) > 0 {
		sb.WriteString("\n## Ports\n\n")
		sb.WriteString("| Port | Description |\n|---|---|\n")
		for _, p := range df.Ports {
			sb.WriteString(fmt.Sprintf("| %s | |\n", p))
		}
	}

	// Volumes
	if len(df.Volumes) > 0 {
		sb.WriteString("\n## Volumes\n\n")
		sb.WriteString("| Path | Description |\n|---|---|\n")
		for _, v := range df.Volumes {
			sb.WriteString(fmt.Sprintf("| `%s` | |\n", v))
		}
	}

	// Environment variables
	if len(df.EnvVars) > 0 {
		sb.WriteString("\n## Environment Variables\n\n")
		sb.WriteString("| Variable | Default | Description |\n|---|---|---|\n")
		for _, ev := range df.EnvVars {
			def := ev.Default
			if def == "" {
				def = "*required*"
			}
			sb.WriteString(fmt.Sprintf("| `%s` | `%s` | |\n", ev.Key, def))
		}
	}

	// Packages
	if len(df.Packages) > 0 {
		sb.WriteString("\n## Packages\n\n")
		sb.WriteString("The following packages are installed during provisioning:\n\n")
		for _, pkg := range df.Packages {
			sb.WriteString(fmt.Sprintf("- `%s`\n", pkg))
		}
	}

	sb.WriteString("\n## Setup\n\nTODO: Add setup instructions.\n")
	return sb.String()
}

// appLayerPackages returns only the app layer's packages (last layer) when
// PackageLayers is available, falling back to the full package list.
// This prevents base-image system packages (xz, alpine-release, shadow, etc.)
// from being mistakenly identified as the main application service.
func appLayerPackages(df *DockerfileInfo) []string {
	if len(df.PackageLayers) > 1 {
		return df.PackageLayers[len(df.PackageLayers)-1].Packages
	}
	return df.Packages
}

// shellSplit splits a shell command string into arguments, handling basic quoting.
func shellSplit(cmd string) []string {
	var args []string
	var current strings.Builder
	inSingle, inDouble := false, false

	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		switch {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
		case c == '"' && !inSingle:
			inDouble = !inDouble
		case c == ' ' && !inSingle && !inDouble:
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(c)
		}
	}
	if current.Len() > 0 {
		args = append(args, current.String())
	}
	return args
}

// writeArgList writes a Python list literal from args, e.g. ["sed", "-i", "s|...|...|"].
func writeArgList(sp *strings.Builder, args []string) {
	sp.WriteString("[")
	for i, a := range args {
		if i > 0 {
			sp.WriteString(", ")
		}
		sp.WriteString(fmt.Sprintf("%q", a))
	}
	sp.WriteString("]")
}

// sdkHandledSet() returns the set of RunCommand types handled by dedicated SDK
// methods (loaded from command_rules.yml) — NOT emitted as run_command().
func sdkHandledSet() map[string]bool {
	m := make(map[string]bool, len(cmdRules.SDKHandled))
	for _, t := range cmdRules.SDKHandled {
		m[t] = true
	}
	return m
}

// containsShellOperator returns true if the command contains shell operators
// (>>, > /, |, $()) that can't be split into subprocess arguments.
// These commands must be wrapped as ["sh", "-c", "..."] instead of shellSplit.
func containsShellOperator(cmd string) bool {
	for _, op := range cmdRules.ShellOperators {
		if strings.Contains(cmd, op) {
			return true
		}
	}
	return false
}


// collectScriptCommands returns the deduplicated list of command names that the
// generated install.py will call via self.run_command(). Extracts the first word
// of every RunCommand that isn't handled by a dedicated SDK method.
func collectScriptCommands(df *DockerfileInfo) []string {
	seen := map[string]bool{}
	var cmds []string
	add := func(name string) {
		if !seen[name] {
			seen[name] = true
			cmds = append(cmds, name)
		}
	}

	// COPY handling generates git/cp/rm
	hasCopy := false
	for _, ci := range df.CopyInstructions {
		if ci.FromStage == "" && !ci.IsURL && !strings.Contains(ci.Src, "$") {
			hasCopy = true
			break
		}
	}
	if hasCopy && df.RepoURL != "" {
		add("git")
		add("cp")
		add("rm")
	}

	// Symlinks generate "ln"
	if len(df.Symlinks) > 0 {
		add("ln")
	}

	// All emitted RunCommands — extract first word (command name)
	for _, rc := range df.RunCommands {
		if sdkHandledSet()[rc.Type] || rc.Type == "git_clone" || rc.Type == "unknown" {
			continue
		}
		if containsShellOperator(rc.Original) {
			add("sh")
		} else {
			args := shellSplit(rc.Original)
			if len(args) > 0 {
				add(args[0])
			}
		}
	}
	return cmds
}

// extractPathsFromRunCommands scans emitted RunCommand arguments for absolute paths
// and returns their parent directories (e.g. "/etc/nginx/" from "sed -i ... /etc/nginx/nginx.conf").
func extractPathsFromRunCommands(cmds []RunCommand) []string {
	seen := map[string]bool{}
	var paths []string
	for _, rc := range cmds {
		if sdkHandledSet()[rc.Type] || rc.Type == "git_clone" || rc.Type == "unknown" {
			continue
		}
		for _, arg := range shellSplit(rc.Original) {
			if !strings.HasPrefix(arg, "/") || strings.HasPrefix(arg, "/tmp") {
				continue
			}
			// Skip flags
			if strings.HasPrefix(arg, "-") {
				continue
			}
			// Get parent directory
			dir := arg
			if idx := strings.LastIndex(arg, "/"); idx > 0 {
				dir = arg[:idx+1] // keep trailing slash
			}
			if dir != "/" && !seen[dir] {
				seen[dir] = true
				paths = append(paths, dir)
			}
		}
	}
	return paths
}

// isDockerInit returns true if the command is a Docker container init system
// and not a real service command (should not be used as exec_start).
func isDockerInit(cmd string) bool {
	trimmed := strings.TrimSpace(cmd)
	switch trimmed {
	case "/init", "/sbin/init", "/usr/bin/tini", "tini",
		"/sbin/tini", "/bin/tini", "/docker-entrypoint.sh",
		"s6-svscan /var/run/s6/services":
		return true
	}
	// s6-overlay patterns
	if strings.HasPrefix(trimmed, "/init ") || strings.HasPrefix(trimmed, "s6-") {
		return true
	}
	return false
}

// inferMainService guesses the main service name from the app ID and package list.
func inferMainService(appID string, packages []string) string {
	kebab := toKebabCase(appID)

	// Tier 1: exact match — app ID matches a package name
	for _, pkg := range packages {
		if pkg == kebab {
			return pkg
		}
	}

	// Tier 2: partial match — package contains the app ID or vice versa
	for _, pkg := range packages {
		if strings.Contains(pkg, kebab) || strings.Contains(kebab, pkg) {
			if !isUtilityPackage(pkg) {
				return pkg
			}
		}
	}

	// Tier 3: first non-utility package
	for _, pkg := range packages {
		if isUtilityPackage(pkg) {
			continue
		}
		return pkg
	}
	return kebab
}

// inferImpliedServices detects services that should be enabled by checking:
// 1. The base image name for known service names (e.g. "baseimage-alpine-nginx")
// 2. Package prefixes that imply a parent service (e.g. "nginx-mod-*" → nginx)
// Both lists are loaded from base_images.yml.
// Returns deduplicated service names excluding the main service.
func inferImpliedServices(packages []string, mainService string, baseImage string) []string {
	seen := map[string]bool{mainService: true}
	var services []string

	// Check base image name for known service names
	lower := strings.ToLower(baseImage)
	for _, svc := range knownServices {
		if strings.Contains(lower, svc) && !seen[svc] {
			seen[svc] = true
			services = append(services, svc)
		}
	}

	// Check package prefixes for implied services
	for _, pkg := range packages {
		for prefix, svc := range impliedServiceMap {
			if strings.HasPrefix(pkg, prefix) && !seen[svc] {
				seen[svc] = true
				services = append(services, svc)
			}
		}
	}
	return services
}

// isUtilityPackage uses pattern-based heuristics to detect packages that are
// build tools, libraries, or system utilities rather than application services.
func isUtilityPackage(pkg string) bool {
	// Prefix patterns: libraries and language toolchains
	utilPrefixes := []string{
		"lib",      // libssl3, libffi-dev, libasound2, etc.
		"ca-",      // ca-certificates
		"qt5-",     // Qt5 library deps (qt5-default)
		"qt6-",     // Qt6 library deps (qt6-qtbase-sqlite)
		"qtbase",   // qtbase5-dev
		"qtscript", // qtscript5-dev
		"qttools",  // qttools5-dev-tools
		"python",   // python3, python3-dev, python3-pip
		"php",      // php84-bcmath, php84-gd (modules, not services)
	}
	for _, pfx := range utilPrefixes {
		if strings.HasPrefix(pkg, pfx) {
			return true
		}
	}

	// Suffix patterns: build deps, libraries
	utilSuffixes := []string{
		"-dev",       // build headers (openssl-dev, libffi-dev)
		"-dev-tools", // qttools5-dev-tools, etc.
		"-libs",      // library packages (icu-libs)
		"-devel",     // RPM-style build deps
		"-common",    // common/shared packages
		"-utils",     // utility subpackages
	}
	for _, sfx := range utilSuffixes {
		if strings.HasSuffix(pkg, sfx) {
			return true
		}
	}

	// Exact matches: common toolchain/utility packages found in many Dockerfiles
	switch pkg {
	case "curl", "wget", "gnupg", "gpg",
		"build-essential", "build-base",
		"make", "cmake", "gcc", "g++", "cargo", "git", "ccache",
		"apt-transport-https", "software-properties-common",
		"udev", "coreutils", "bash", "bash-completion",
		"binutils", "grep", "xz-utils", "openssl",
		"ca-certificates", "at", "bc", "sudo",
		// Compression utilities (not services)
		"xz", "gzip", "bzip2", "zstd", "unzip", "zip", "tar", "pigz",
		// Base system packages common in Docker base layers
		"shadow", "tzdata", "s6-overlay", "logrotate", "patch",
		"file", "findutils", "ncurses-terminfo-base", "ncurses-terminfo",
		"sed", "gawk", "less", "procps", "net-tools", "iproute2",
		"jq", "nano", "vim", "tree", "hostname", "iputils",
		// Alpine base packages
		"alpine-baselayout", "alpine-keys", "apk-tools", "musl", "musl-utils",
		"busybox-extras", "scanelf", "ssl_client", "alpine-release":
		return true
	}

	return false
}

// volumePathInfo matches the struct used in scaffold code for volume/directory paths.
type volumePathInfo struct {
	name, target string
}

// portInputInfo matches the struct used in scaffold code for port inputs.
type portInputInfo struct {
	key        string
	port       string
	defaultVal string
}

// buildScriptParams are the parameters for building a complete install.py script body.
type buildScriptParams struct {
	name        string
	className   string
	docstring   string
	df          *DockerfileInfo
	portInputs  []portInputInfo
	envInputs   []EnvVar
	secretVars  []struct{ key, name string }
	stringVars  []struct{ key, name, defaultVal string }
	volumePaths []volumePathInfo
	mainService string
}

// buildInstallScript generates a complete install.py script from DockerfileInfo
// and input configuration. Used by both ConvertDockerfileToScaffold and
// convertWithDockerfile (Unraid path) to avoid duplication.
func buildInstallScript(p buildScriptParams) string {
	var sp strings.Builder
	sp.WriteString(fmt.Sprintf("#!/usr/bin/env python3\n\"\"\"\n%s\n\"\"\"\nfrom appstore import BaseApp, run\n\n\nclass %s(BaseApp):\n    def install(self):", p.docstring, p.className))

	// Read inputs
	hasInputs := len(p.portInputs) > 0 || len(p.envInputs) > 0 || len(p.secretVars) > 0 || len(p.stringVars) > 0
	if hasInputs {
		sp.WriteString("\n        # Read inputs\n")
		for _, pi := range p.portInputs {
			varName := toSnakeCase(pi.key)
			sp.WriteString(fmt.Sprintf("        %s = self.inputs.integer(\"%s\", %s)\n", varName, pi.key, pi.defaultVal))
		}
		for _, v := range p.secretVars {
			sp.WriteString(fmt.Sprintf("        %s = self.inputs.secret(\"%s\")\n", v.key, v.key))
		}
		for _, v := range p.stringVars {
			sp.WriteString(fmt.Sprintf("        %s = self.inputs.string(\"%s\", \"%s\")\n", v.key, v.key, v.defaultVal))
		}
		for _, ev := range p.envInputs {
			varName := toSnakeCase(ev.Key)
			sp.WriteString(fmt.Sprintf("        %s = self.inputs.string(\"%s\", \"%s\")\n",
				varName, toSnakeCase(ev.Key), strings.ReplaceAll(ev.Default, `"`, `\"`)))
		}
	}

	sp.WriteString("\n")
	df := p.df

	// APT keys
	for _, k := range df.AptKeys {
		sp.WriteString(fmt.Sprintf("        self.add_apt_key(\"%s\",\n                         \"%s\")\n", k.URL, k.Keyring))
	}

	// APT repos
	for _, r := range df.AptRepos {
		filename := r.File
		if idx := strings.LastIndex(filename, "/"); idx >= 0 {
			filename = filename[idx+1:]
		}
		sp.WriteString(fmt.Sprintf("        self.add_apt_repo(\"%s\",\n                          \"%s\")\n", r.Line, filename))
	}

	// Package install — grouped by layer if chain was resolved
	if len(df.PackageLayers) > 1 {
		sp.WriteString("\n        # Install packages by layer (base -> app)\n")
		for _, layer := range df.PackageLayers {
			sp.WriteString(fmt.Sprintf("        # --- %s ---\n", layer.Image))
			writePkgInstall(&sp, layer.Packages)
			sp.WriteString("\n")
		}
	} else if len(df.Packages) > 0 {
		writePkgInstall(&sp, df.Packages)
	}

	// Pip packages
	if len(df.PipPackages) > 0 {
		sp.WriteString("\n        # Install Python packages\n")
		if len(df.PipPackages) <= 3 {
			sp.WriteString("        self.pip_install(")
			for i, pkg := range df.PipPackages {
				if i > 0 {
					sp.WriteString(", ")
				}
				sp.WriteString(fmt.Sprintf("\"%s\"", pkg))
			}
			sp.WriteString(")\n")
		} else {
			sp.WriteString("        self.pip_install(\n")
			for i, pkg := range df.PipPackages {
				sp.WriteString(fmt.Sprintf("            \"%s\"", pkg))
				if i < len(df.PipPackages)-1 {
					sp.WriteString(",")
				}
				sp.WriteString("\n")
			}
			sp.WriteString("        )\n")
		}
	}

	// Create users
	if len(df.Users) > 0 {
		sp.WriteString("\n        # Create system users\n")
		for _, u := range df.Users {
			sp.WriteString(fmt.Sprintf("        self.create_user(\"%s\")\n", u))
		}
	}

	// Create directories (merge VOLUME + mkdir, dedup)
	allDirs := make(map[string]bool)
	var dirList []string
	for _, v := range p.volumePaths {
		if !allDirs[v.target] {
			allDirs[v.target] = true
			dirList = append(dirList, v.target)
		}
	}
	for _, d := range df.Directories {
		if !allDirs[d] {
			allDirs[d] = true
			dirList = append(dirList, d)
		}
	}
	if len(dirList) > 0 {
		sp.WriteString("\n        # Create directories\n")
		for _, d := range dirList {
			sp.WriteString(fmt.Sprintf("        self.create_dir(\"%s\")\n", d))
		}
	}

	// Chown — convert parsed chown commands to SDK self.chown() calls
	var chownCmds []RunCommand
	for _, rc := range df.RunCommands {
		if rc.Type == "chown" {
			chownCmds = append(chownCmds, rc)
		}
	}
	if len(chownCmds) > 0 {
		sp.WriteString("\n        # Set ownership\n")
		for _, rc := range chownCmds {
			args := shellSplit(rc.Original)
			// Parse: chown [-R] owner[:group] path [path...]
			recursive := false
			var owner string
			var paths []string
			for _, a := range args[1:] { // skip "chown"
				if a == "-R" || a == "--recursive" {
					recursive = true
				} else if owner == "" {
					owner = a
				} else {
					paths = append(paths, a)
				}
			}
			if owner != "" && len(paths) > 0 {
				for _, p := range paths {
					if recursive {
						sp.WriteString(fmt.Sprintf("        self.chown(\"%s\", \"%s\", recursive=True)\n", p, owner))
					} else {
						sp.WriteString(fmt.Sprintf("        self.chown(\"%s\", \"%s\")\n", p, owner))
					}
				}
			}
		}
	}

	// Downloads
	if len(df.Downloads) > 0 {
		sp.WriteString("\n        # Download files\n")
		for _, dl := range df.Downloads {
			sp.WriteString(fmt.Sprintf("        self.download(\"%s\", \"%s\")\n", dl.URL, dl.Dest))
		}
	}

	// COPY handling
	// Filter COPY instructions: skip multi-stage, ADD URLs, variable refs,
	// Docker init script patterns (COPY root/ /), and dedup
	var filteredCopy []CopyInstruction
	copyDedup := map[string]bool{}
	for _, ci := range df.CopyInstructions {
		if ci.FromStage != "" { // multi-stage build artifact
			continue
		}
		if ci.IsURL { // ADD with http URL — already handled as download or irrelevant
			continue
		}
		if strings.Contains(ci.Src, "$") { // unresolvable variable reference
			continue
		}
		// Skip Docker init script patterns: COPY root/ / is the LinuxServer
		// convention for deploying s6-overlay scripts — not useful in LXC
		src := strings.TrimSuffix(ci.Src, "/")
		if (src == "root" || src == ".") && (ci.Dest == "/" || ci.Dest == ".") {
			continue
		}
		key := ci.Src + " -> " + ci.Dest
		if copyDedup[key] {
			continue
		}
		copyDedup[key] = true
		filteredCopy = append(filteredCopy, ci)
	}
	if len(filteredCopy) > 0 {
		sp.WriteString("\n")
		if df.RepoURL != "" {
			sp.WriteString("        # Deploy config files from source repository\n")
			sp.WriteString(fmt.Sprintf("        self.run_command([\"git\", \"clone\", \"--depth\", \"1\",\n                          \"%s.git\", \"/tmp/_src\"])\n", df.RepoURL))
			for _, ci := range filteredCopy {
				src := strings.TrimSuffix(ci.Src, "/")
				dest := ci.Dest
				if dest == "/" || dest == "." {
					sp.WriteString(fmt.Sprintf("        self.run_command([\"cp\", \"-a\", \"/tmp/_src/%s/.\", \"/\"])\n", src))
				} else {
					sp.WriteString(fmt.Sprintf("        self.run_command([\"cp\", \"-a\", \"/tmp/_src/%s\", \"%s\"])\n", src, dest))
				}
			}
			sp.WriteString("        self.run_command([\"rm\", \"-rf\", \"/tmp/_src\"])\n")
			sp.WriteString("        # TODO: Review copied files — some may be Docker-specific\n")
			sp.WriteString("        # (s6-overlay init scripts, container health checks) and should be\n")
			sp.WriteString("        # adapted or removed for LXC.\n")
		} else {
			sp.WriteString("        # TODO: COPY instructions found but source repo URL unknown.\n")
			sp.WriteString("        # Manually deploy these files:\n")
			for _, ci := range filteredCopy {
				keyword := "COPY"
				if ci.IsAdd {
					keyword = "ADD"
				}
				sp.WriteString(fmt.Sprintf("        #   %s %s %s\n", keyword, ci.Src, ci.Dest))
			}
		}
	}

	// Symlinks
	if len(df.Symlinks) > 0 {
		sp.WriteString("\n        # Create symlinks\n")
		for _, sl := range df.Symlinks {
			sp.WriteString(fmt.Sprintf("        self.run_command([\"ln\", \"-sf\", \"%s\", \"%s\"])\n", sl.Target, sl.Link))
		}
	}

	// Emit all actionable RunCommands as run_command(); keep unknown as TODO
	var actionableCmds, todoCommands []RunCommand
	for _, rc := range df.RunCommands {
		if sdkHandledSet()[rc.Type] {
			continue
		}
		if rc.Type == "git_clone" || rc.Type == "unknown" {
			todoCommands = append(todoCommands, rc)
			continue
		}
		actionableCmds = append(actionableCmds, rc)
	}
	if len(actionableCmds) > 0 || len(todoCommands) > 0 {
		sp.WriteString("\n        # ── Auto-generated from Dockerfile ──────────────────────────\n")
		sp.WriteString("        # These commands were extracted from the Dockerfile and may\n")
		sp.WriteString("        # reference paths or files that don't exist in LXC.\n")
		sp.WriteString("        # Review each one — failures are logged as warnings, not errors.\n")
	}
	if len(actionableCmds) > 0 {
		for _, rc := range actionableCmds {
			if containsShellOperator(rc.Original) {
				sp.WriteString(fmt.Sprintf("        self.run_command([\"sh\", \"-c\", %q], check=False)\n", rc.Original))
			} else {
				args := shellSplit(rc.Original)
				sp.WriteString("        self.run_command(")
				writeArgList(&sp, args)
				sp.WriteString(", check=False)\n")
			}
		}
	}
	if len(todoCommands) > 0 {
		sp.WriteString("\n        # TODO: Review and adapt the following commands for LXC:\n")
		for _, rc := range todoCommands {
			sp.WriteString(fmt.Sprintf("        # %s\n", rc.Original))
		}
	}

	// Service management
	sp.WriteString("\n")
	impliedSvcs := inferImpliedServices(df.Packages, p.mainService, df.BaseImage)
	for _, svc := range impliedSvcs {
		sp.WriteString(fmt.Sprintf("        self.enable_service(\"%s\")\n", svc))
	}

	// Startup command priority: ExecCmd > EntrypointCmd > StartupCmd
	execStart := df.ExecCmd
	if execStart == "" {
		execStart = df.EntrypointCmd
	}
	if execStart == "" {
		execStart = df.StartupCmd
	}
	// Filter Docker init systems — not real service commands
	if isDockerInit(execStart) {
		execStart = ""
	}

	if execStart != "" {
		sp.WriteString(fmt.Sprintf("        self.create_service(\"%s\",\n                            exec_start=\"%s\")\n",
			p.mainService, strings.ReplaceAll(execStart, `"`, `\"`)))
	} else {
		sp.WriteString(fmt.Sprintf("        self.enable_service(\"%s\")\n", p.mainService))
	}

	sp.WriteString(fmt.Sprintf("        self.log.info(\"%s installation complete\")\n", p.name))
	sp.WriteString(fmt.Sprintf("\n\nrun(%s)\n", p.className))

	return sp.String()
}

// volumeNameFromPath derives a short volume name from a container path.
// e.g. "/config" → "config", "/usr/share/ollama/.ollama/models" → "models", "/" → "data"
func volumeNameFromPath(p string) string {
	p = strings.TrimRight(p, "/")
	if p == "" || p == "/" {
		return "data"
	}
	base := filepath.Base(p)
	// Strip leading dots
	base = strings.TrimLeft(base, ".")
	if base == "" {
		return "data"
	}
	return strings.ToLower(base)
}

// volumeLabelFromPath title-cases the basename of a path.
func volumeLabelFromPath(p string) string {
	name := volumeNameFromPath(p)
	if len(name) > 0 {
		return strings.ToUpper(name[:1]) + name[1:]
	}
	return "Data"
}

// writePkgInstall writes a self.pkg_install() call, wrapping to one package
// per line when there are more than 3 packages for readability.
func writePkgInstall(sp *strings.Builder, packages []string) {
	if len(packages) <= 3 {
		sp.WriteString("        self.pkg_install(")
		for i, pkg := range packages {
			if i > 0 {
				sp.WriteString(", ")
			}
			sp.WriteString(fmt.Sprintf("\"%s\"", pkg))
		}
		sp.WriteString(")\n")
		return
	}
	sp.WriteString("        self.pkg_install(\n")
	for i, pkg := range packages {
		sp.WriteString(fmt.Sprintf("            \"%s\"", pkg))
		if i < len(packages)-1 {
			sp.WriteString(",")
		}
		sp.WriteString("\n")
	}
	sp.WriteString("        )\n")
}
