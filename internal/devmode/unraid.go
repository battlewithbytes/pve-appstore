package devmode

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// UnraidContainer represents the XML structure of an Unraid template.
type UnraidContainer struct {
	XMLName     xml.Name       `xml:"Container"`
	Name        string         `xml:"Name"`
	Repository  string         `xml:"Repository"`
	Registry    string         `xml:"Registry"`
	Network     string         `xml:"Network"`
	Privileged  string         `xml:"Privileged"`
	Overview    string         `xml:"Overview"`
	Description string         `xml:"Description"`
	Category    string         `xml:"Category"`
	WebUI       string         `xml:"WebUI"`
	Icon        string         `xml:"Icon"`
	Project     string         `xml:"Project"`
	GitHub      string         `xml:"GitHub"`
	ReadMe      string         `xml:"ReadMe"`
	Shell       string         `xml:"Shell"`
	Configs     []UnraidConfig `xml:"Config"`
}

// UnraidConfig represents a Config element in an Unraid template.
type UnraidConfig struct {
	Name     string `xml:"Name,attr"`
	Target   string `xml:"Target,attr"`
	Default  string `xml:"Default,attr"`
	Mode     string `xml:"Mode,attr"`
	Display  string `xml:"Display,attr"`
	Type     string `xml:"Type,attr"`
	Required string `xml:"Required,attr"`
	Mask     string `xml:"Mask,attr"`
	Value    string `xml:",chardata"`
	Desc     string `xml:"Description,attr"`
}

// ParseUnraidXML parses an Unraid XML template.
func ParseUnraidXML(data []byte) (*UnraidContainer, error) {
	var c UnraidContainer
	if err := xml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing Unraid XML: %w", err)
	}
	if c.Name == "" {
		return nil, fmt.Errorf("Unraid XML: missing Name element")
	}
	return &c, nil
}

// ConvertUnraidToScaffold generates an app ID, app.yml, and install.py from an Unraid template.
// If df is non-nil, the output uses real SDK v2 calls derived from the parsed Dockerfile.
func ConvertUnraidToScaffold(c *UnraidContainer, df *DockerfileInfo) (id, manifest, script string) {
	id = toKebabCase(c.Name)

	if df != nil && len(df.Packages) > 0 {
		return convertWithDockerfile(c, df, id)
	}

	description := c.Overview
	if description == "" {
		description = c.Description
	}
	if description == "" {
		description = "Imported from Unraid: " + c.Name
	}
	// Strip HTML tags and markdown-style links like Name(url)
	description = StripHTML(description)
	description = stripMarkdownLinks(description)
	// Escape quotes for YAML
	description = strings.ReplaceAll(description, `"`, `\"`)
	if len(description) > 200 {
		description = description[:200] + "..."
	}

	// Collect ports, paths, variables
	type portInfo struct {
		name, target, defaultVal, mode string
	}
	type pathInfo struct {
		name, target, defaultPath, mode string
	}
	type varInfo struct {
		key, name, defaultVal, desc string
		required, mask              bool
	}

	var ports []portInfo
	var paths []pathInfo
	var vars []varInfo
	// Track port targets so we can skip duplicate variables that just configure the same port
	portTargets := map[string]bool{}

	// First pass: collect port targets
	for _, cfg := range c.Configs {
		if strings.ToLower(cfg.Type) == "port" {
			portTargets[cfg.Target] = true
		}
	}

	// Deduplicate ports by target (e.g. tcp/udp on same port)
	seenPortTargets := map[string]bool{}

	for _, cfg := range c.Configs {
		switch strings.ToLower(cfg.Type) {
		case "port":
			dv := cfg.Default
			if dv == "" {
				dv = cfg.Value
			}
			if dv == "" {
				dv = cfg.Target
			}
			// Deduplicate by target (e.g. 6881 tcp + 6881 udp → one input)
			if seenPortTargets[cfg.Target] {
				continue
			}
			seenPortTargets[cfg.Target] = true
			ports = append(ports, portInfo{
				name: cfg.Name, target: cfg.Target, defaultVal: dv, mode: cfg.Mode,
			})
		case "path":
			dp := cfg.Default
			if dp == "" {
				dp = cfg.Value
			}
			paths = append(paths, pathInfo{
				name: cfg.Name, target: cfg.Target, defaultPath: dp, mode: cfg.Mode,
			})
		case "variable":
			key := toSnakeCase(cfg.Target)
			if key == "" {
				key = toSnakeCase(cfg.Name)
			}
			// Skip Docker-specific variables
			if key == "puid" || key == "pgid" || key == "umask" || key == "tz" || key == "docker_mods" {
				continue
			}
			dv := cfg.Default
			if dv == "" {
				dv = cfg.Value
			}
			// Skip variables that just configure an existing port (e.g. WEBUI_PORT=8080 when port 8080 already exists)
			if portTargets[dv] || portTargets[cfg.Target] {
				continue
			}
			desc := cfg.Desc
			if desc == "" {
				desc = cfg.Name
			}
			vars = append(vars, varInfo{
				key: key, name: cfg.Name, defaultVal: dv, desc: desc,
				required: strings.ToLower(cfg.Required) == "true",
				mask:     strings.ToLower(cfg.Mask) == "true",
			})
		}
	}

	// Build manifest
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`id: %s
name: "%s"
description: "%s"
version: "0.1.0"
categories:
  - utilities
tags:
  - unraid-import`, id, c.Name, description))
	sb.WriteString("\nmaintainers:\n  - \"Your Name\"\n")
	sb.WriteString("icon: \"\"  # Paste icon URL or use icon editor in header\n")

	// Add source info as comments
	sb.WriteString(fmt.Sprintf("\n# Imported from Unraid template for %s\n", c.Name))
	sb.WriteString(fmt.Sprintf("# Original Docker image: %s\n", c.Repository))
	if c.Project != "" {
		sb.WriteString(fmt.Sprintf("# Project homepage: %s\n", c.Project))
	}
	sb.WriteString("# This is a SCAFFOLD — you must implement the provisioning logic.\n")

	sb.WriteString(`
lxc:
  ostemplate: "local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst"
  defaults:
    unprivileged: true
    cores: 2
    memory_mb: 1024
    disk_gb: 8
    onboot: true
`)

	// Build port input keys for use in outputs
	type portInput struct {
		key  string
		port portInfo
	}
	var portInputs []portInput

	// Inputs from variables + ports
	if len(vars) > 0 || len(ports) > 0 {
		sb.WriteString("\ninputs:\n")

		// Add port inputs so users can configure them
		for _, p := range ports {
			// Generate clean key: use name if descriptive, otherwise "port_NNNN"
			key := toSnakeCase(p.name)
			if key == "" || key == "port" {
				key = "port_" + p.target
			}
			// Ensure "port" prefix for clarity if the name doesn't imply it
			if !strings.Contains(key, "port") {
				key = key + "_port"
			}
			portInputs = append(portInputs, portInput{key: key, port: p})
			sb.WriteString(fmt.Sprintf(`  - key: %s
    label: "%s"
    type: number
    default: %s
    required: true
    validation:
      min: 1
      max: 65535
    help: "Port %s (%s)"
`, key, p.name, p.defaultVal, p.target, p.mode))
		}

		for _, v := range vars {
			inputType := "string"
			if v.mask {
				inputType = "secret"
			}
			reqStr := "false"
			if v.required {
				reqStr = "true"
			}
			sb.WriteString(fmt.Sprintf(`  - key: %s
    label: "%s"
    type: %s
    default: "%s"
    required: %s
    help: "%s"
`, v.key, v.name, inputType, v.defaultVal, reqStr, strings.ReplaceAll(v.desc, `"`, `\"`)))
		}
	}

	sb.WriteString(`
provisioning:
  script: provision/install.py
  timeout_sec: 600

`)

	// Comments about Docker paths
	if len(paths) > 0 {
		sb.WriteString("# Docker volume mappings (implement as directories in install.py):\n")
		for _, p := range paths {
			sb.WriteString(fmt.Sprintf("#   %s → %s (%s, %s)\n", p.target, p.defaultPath, p.name, p.mode))
		}
		sb.WriteString("\n")
	}

	// Outputs — use WebUI port if available, reference the input key
	webUIKey := ""
	webUIDefault := ""
	for _, pi := range portInputs {
		if strings.Contains(strings.ToLower(pi.port.name), "webui") || strings.Contains(strings.ToLower(pi.port.name), "web") {
			webUIKey = pi.key
			webUIDefault = pi.port.defaultVal
			break
		}
	}
	if webUIKey == "" && len(portInputs) > 0 {
		webUIKey = portInputs[0].key
		webUIDefault = portInputs[0].port.defaultVal
	}

	if webUIKey != "" {
		sb.WriteString(fmt.Sprintf(`outputs:
  - key: url
    label: "Web UI"
    value: "http://{{IP}}:{{%s}}"
  - key: webui_port
    label: "Web UI Port"
    value: "{{%s}}"
`, webUIKey, webUIKey))
		_ = webUIDefault // used for script comments
	} else {
		sb.WriteString(`outputs:
  - key: url
    label: "Access URL"
    value: "http://{{IP}}/"
`)
	}

	manifest = sb.String()

	// Build script
	className := toPascalCase(id)

	var scriptParts []string
	scriptParts = append(scriptParts, fmt.Sprintf(`#!/usr/bin/env python3
"""
Provisioning script for %s.
Imported from Unraid template — original Docker image: %s
%s

This scaffold converts the Docker template to native LXC provisioning.
Replace the TODOs below with actual package installs and configuration.
"""
from appstore import BaseApp, run


class %s(BaseApp):
    def install(self):`, c.Name, c.Repository, c.Project, className))

	// Add input reads
	if len(portInputs) > 0 || len(vars) > 0 {
		scriptParts = append(scriptParts, "        # Read inputs")
		for _, pi := range portInputs {
			varName := toSnakeCase(pi.key)
			scriptParts = append(scriptParts, fmt.Sprintf(`        %s = self.inputs.integer("%s", %s)`, varName, pi.key, pi.port.defaultVal))
		}
		for _, v := range vars {
			if v.mask {
				scriptParts = append(scriptParts, fmt.Sprintf(`        %s = self.inputs.secret("%s")`, v.key, v.key))
			} else {
				scriptParts = append(scriptParts, fmt.Sprintf(`        %s = self.inputs.string("%s", "%s")`, v.key, v.key, v.defaultVal))
			}
		}
		scriptParts = append(scriptParts, "")
	}

	// Add install steps
	scriptParts = append(scriptParts, `        # Step 1: Install packages
        # TODO: Replace with the actual packages needed
        # self.apt_install(["package1", "package2"])`)

	if len(paths) > 0 {
		scriptParts = append(scriptParts, "\n        # Step 2: Create data directories")
		for _, p := range paths {
			scriptParts = append(scriptParts, fmt.Sprintf(`        self.create_dir("%s")  # %s`, p.target, p.name))
		}
	}

	scriptParts = append(scriptParts, `
        # Step 3: Configure the application
        # TODO: Write config files, set up users, etc.
        # self.write_config("/etc/app/config.conf", config_content)

        # Step 4: Create and enable systemd service
        # TODO: Create a service unit for the application
        # self.enable_service("app-name")

        self.log.info("Installation complete — configure the application manually")`)

	scriptParts = append(scriptParts, fmt.Sprintf(`

run(%s)
`, className))

	script = strings.Join(scriptParts, "\n")

	return id, manifest, script
}

// convertWithDockerfile generates a manifest and script using both Unraid XML and Dockerfile data.
func convertWithDockerfile(c *UnraidContainer, df *DockerfileInfo, id string) (string, string, string) {
	description := c.Overview
	if description == "" {
		description = c.Description
	}
	if description == "" {
		description = "Converted from Unraid: " + c.Name
	}
	description = StripHTML(description)
	description = stripMarkdownLinks(description)
	description = strings.ReplaceAll(description, `"`, `\"`)
	if len(description) > 200 {
		description = description[:200] + "..."
	}

	// Ensure pip prerequisites are in the package list
	ensurePipPrereqs(df)

	// Determine OS template from profile
	profile := ProfileFor(df.BaseOS)
	osTemplate := profile.OSTemplate

	// Determine unprivileged from XML Privileged field (inverted)
	unprivileged := true
	if strings.ToLower(c.Privileged) == "true" {
		unprivileged = false
	}

	// Collect ports, paths, variables from XML (same as scaffold path)
	type portInfo struct {
		name, target, defaultVal, mode string
	}
	type pathInfo struct {
		name, target, defaultPath, mode string
	}
	type varInfo struct {
		key, name, defaultVal, desc string
		required, mask              bool
	}

	var ports []portInfo
	var paths []pathInfo
	var vars []varInfo
	portTargets := map[string]bool{}
	for _, cfg := range c.Configs {
		if strings.ToLower(cfg.Type) == "port" {
			portTargets[cfg.Target] = true
		}
	}
	seenPortTargets := map[string]bool{}
	for _, cfg := range c.Configs {
		switch strings.ToLower(cfg.Type) {
		case "port":
			dv := cfg.Default
			if dv == "" {
				dv = cfg.Value
			}
			if dv == "" {
				dv = cfg.Target
			}
			if seenPortTargets[cfg.Target] {
				continue
			}
			seenPortTargets[cfg.Target] = true
			ports = append(ports, portInfo{name: cfg.Name, target: cfg.Target, defaultVal: dv, mode: cfg.Mode})
		case "path":
			dp := cfg.Default
			if dp == "" {
				dp = cfg.Value
			}
			paths = append(paths, pathInfo{name: cfg.Name, target: cfg.Target, defaultPath: dp, mode: cfg.Mode})
		case "variable":
			key := toSnakeCase(cfg.Target)
			if key == "" {
				key = toSnakeCase(cfg.Name)
			}
			if key == "puid" || key == "pgid" || key == "umask" || key == "tz" || key == "docker_mods" {
				continue
			}
			dv := cfg.Default
			if dv == "" {
				dv = cfg.Value
			}
			if portTargets[dv] || portTargets[cfg.Target] {
				continue
			}
			desc := cfg.Desc
			if desc == "" {
				desc = cfg.Name
			}
			vars = append(vars, varInfo{
				key: key, name: cfg.Name, defaultVal: dv, desc: desc,
				required: strings.ToLower(cfg.Required) == "true",
				mask:     strings.ToLower(cfg.Mask) == "true",
			})
		}
	}

	// Merge Dockerfile volumes with XML paths
	allVolumes := make(map[string]bool)
	for _, p := range paths {
		allVolumes[p.target] = true
	}
	for _, v := range df.Volumes {
		if !allVolumes[v] {
			allVolumes[v] = true
			paths = append(paths, pathInfo{name: "Data", target: v, defaultPath: v, mode: "rw"})
		}
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
  - unraid-import`, id, c.Name, description))
	sb.WriteString("\nmaintainers:\n  - \"Your Name\"\n")
	sb.WriteString("icon: \"\"  # Paste icon URL or use icon editor in header\n")

	if c.Project != "" {
		sb.WriteString(fmt.Sprintf("\n# Project homepage: %s\n", c.Project))
	}

	sb.WriteString(fmt.Sprintf(`
lxc:
  ostemplate: "%s"
  defaults:
    unprivileged: %v
    cores: 2
    memory_mb: 1024
    disk_gb: 8
    onboot: true
`, osTemplate, unprivileged))

	// Inputs (same as scaffold path)
	type portInput struct {
		key  string
		port portInfo
	}
	var portInputs []portInput

	// Merge Dockerfile ENV vars with XML variables (XML takes precedence)
	xmlVarKeys := make(map[string]bool)
	for _, v := range vars {
		xmlVarKeys[strings.ToUpper(v.key)] = true
	}
	var envOnlyVars []EnvVar
	if df != nil {
		for _, ev := range df.EnvVars {
			if !xmlVarKeys[ev.Key] && !xmlVarKeys[strings.ToUpper(toSnakeCase(ev.Key))] {
				envOnlyVars = append(envOnlyVars, ev)
			}
		}
	}

	if len(vars) > 0 || len(ports) > 0 || len(envOnlyVars) > 0 {
		sb.WriteString("\ninputs:\n")
		for _, p := range ports {
			key := toSnakeCase(p.name)
			if key == "" || key == "port" {
				key = "port_" + p.target
			}
			if !strings.Contains(key, "port") {
				key = key + "_port"
			}
			portInputs = append(portInputs, portInput{key: key, port: p})
			sb.WriteString(fmt.Sprintf(`  - key: %s
    label: "%s"
    type: number
    default: %s
    required: true
    validation:
      min: 1
      max: 65535
    help: "Port %s (%s)"
`, key, p.name, p.defaultVal, p.target, p.mode))
		}
		for _, v := range vars {
			inputType := "string"
			if v.mask {
				inputType = "secret"
			}
			reqStr := "false"
			if v.required {
				reqStr = "true"
			}
			sb.WriteString(fmt.Sprintf(`  - key: %s
    label: "%s"
    type: %s
    default: "%s"
    required: %s
    help: "%s"
`, v.key, v.name, inputType, v.defaultVal, reqStr, strings.ReplaceAll(v.desc, `"`, `\"`)))
		}
		// ENV vars from Dockerfile not covered by XML variables
		for _, ev := range envOnlyVars {
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

	sb.WriteString(`
provisioning:
  script: provision/install.py
  timeout_sec: 600

`)

	// Permissions section
	sb.WriteString("permissions:\n")
	if len(df.PipPackages) > 0 {
		sb.WriteString("  packages:\n")
		for _, pkg := range df.Packages {
			sb.WriteString(fmt.Sprintf("    - %s\n", pkg))
		}
		sb.WriteString("  pip:\n")
		for _, pkg := range df.PipPackages {
			sb.WriteString(fmt.Sprintf("    - %s\n", pkg))
		}
	} else if len(df.Packages) > 0 {
		sb.WriteString("  packages:\n")
		for _, pkg := range df.Packages {
			sb.WriteString(fmt.Sprintf("    - %s\n", pkg))
		}
	}
	// URLs from APT keys/repos + downloads + repo URL
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
	// Paths
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
	// Services (main + implied from module packages and base image name)
	sb.WriteString("  services:\n")
	for _, svc := range inferImpliedServices(df.Packages, mainService, df.BaseImage) {
		sb.WriteString(fmt.Sprintf("    - %s\n", svc))
	}
	sb.WriteString(fmt.Sprintf("    - %s\n", mainService))

	sb.WriteString("\n")

	// Outputs
	webUIKey := ""
	for _, pi := range portInputs {
		if strings.Contains(strings.ToLower(pi.port.name), "webui") || strings.Contains(strings.ToLower(pi.port.name), "web") {
			webUIKey = pi.key
			break
		}
	}
	if webUIKey == "" && len(portInputs) > 0 {
		webUIKey = portInputs[0].key
	}
	if webUIKey != "" {
		sb.WriteString(fmt.Sprintf(`outputs:
  - key: url
    label: "Web UI"
    value: "http://{{IP}}:{{%s}}"
  - key: webui_port
    label: "Web UI Port"
    value: "{{%s}}"
`, webUIKey, webUIKey))
	} else {
		sb.WriteString(`outputs:
  - key: url
    label: "Access URL"
    value: "http://{{IP}}/"
`)
	}

	manifest := sb.String()

	// Convert portInputs and vars to shared types
	var scriptPortInputs []portInputInfo
	for _, pi := range portInputs {
		scriptPortInputs = append(scriptPortInputs, portInputInfo{
			key: pi.key, port: pi.port.target, defaultVal: pi.port.defaultVal,
		})
	}
	var secretVars []struct{ key, name string }
	var stringVars []struct{ key, name, defaultVal string }
	for _, v := range vars {
		if v.mask {
			secretVars = append(secretVars, struct{ key, name string }{v.key, v.name})
		} else {
			stringVars = append(stringVars, struct{ key, name, defaultVal string }{v.key, v.name, v.defaultVal})
		}
	}
	var volumePaths []volumePathInfo
	for _, p := range paths {
		volumePaths = append(volumePaths, volumePathInfo{name: p.name, target: p.target})
	}

	script := buildInstallScript(buildScriptParams{
		name:        c.Name,
		className:   toPascalCase(id),
		docstring:   fmt.Sprintf("Provisioning script for %s.\nConverted from Unraid template — original Docker image: %s\nGenerated with Dockerfile analysis.", c.Name, c.Repository),
		df:          df,
		portInputs:  scriptPortInputs,
		envInputs:   envOnlyVars,
		secretVars:  secretVars,
		stringVars:  stringVars,
		volumePaths: volumePaths,
		mainService: mainService,
	})

	return id, manifest, script
}
