package devmode

import (
	_ "embed"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed base_images.yml
var baseImagesYAML []byte

//go:embed command_rules.yml
var commandRulesYAML []byte

// baseImageRules maps OS family → matching rules loaded from base_images.yml.
type baseImageRules struct {
	Contains []string `yaml:"contains"`
	Prefix   []string `yaml:"prefix"`
}

// OSProfile centralizes all OS-specific knowledge for scaffold generation.
// Profiles are loaded from base_images.yml so adding a new OS requires
// only a YAML change — no Go switch statements.
type OSProfile struct {
	DisplayName string   `yaml:"display_name"` // e.g. "Debian/Ubuntu", "Alpine"
	OSTemplate  string   `yaml:"os_template"`  // e.g. "debian-12", "alpine-3.22"
	PkgManager  string   `yaml:"pkg_manager"`  // "apt" or "apk"
	ServiceInit string   `yaml:"service_init"` // "systemd" or "openrc"
	PipPrereqs  []string `yaml:"pip_prereqs"`  // ["python3", "python3-pip"] or ["python3", "py3-pip"]
}

// baseImagesConfig is the top-level structure of base_images.yml.
type baseImagesConfig struct {
	Debian          baseImageRules       `yaml:"debian"`
	Alpine          baseImageRules       `yaml:"alpine"`
	Profiles        map[string]OSProfile `yaml:"profiles"`
	ImpliedServices map[string]string    `yaml:"implied_services"`
	KnownServices   []string             `yaml:"known_services"`
}

// commandRulesConfig is the top-level structure of command_rules.yml.
type commandRulesConfig struct {
	Noise struct {
		Exact            []string `yaml:"exact"`
		Prefixes         []string `yaml:"prefixes"`
		Contains         []string `yaml:"contains"`
		SkipEchoMarkers  bool     `yaml:"skip_echo_markers"`
		SkipPrintfBuild  bool     `yaml:"skip_printf_build"`
	} `yaml:"noise"`
	CommandTypes []struct {
		Type   string `yaml:"type"`
		Prefix string `yaml:"prefix"`
	} `yaml:"command_types"`
	SDKHandled     []string `yaml:"sdk_handled"`
	ShellOperators []string `yaml:"shell_operators"`
}

var baseImageMap map[string]baseImageRules
var osProfiles map[string]OSProfile
var impliedServiceMap map[string]string
var knownServices []string
var cmdRules commandRulesConfig

func init() {
	var cfg baseImagesConfig
	if err := yaml.Unmarshal(baseImagesYAML, &cfg); err != nil {
		panic("devmode: failed to parse base_images.yml: " + err.Error())
	}
	baseImageMap = map[string]baseImageRules{
		"debian": cfg.Debian,
		"alpine": cfg.Alpine,
	}
	osProfiles = cfg.Profiles
	if osProfiles == nil {
		osProfiles = make(map[string]OSProfile)
	}
	impliedServiceMap = cfg.ImpliedServices
	if impliedServiceMap == nil {
		impliedServiceMap = make(map[string]string)
	}
	knownServices = cfg.KnownServices

	if err := yaml.Unmarshal(commandRulesYAML, &cmdRules); err != nil {
		panic("devmode: failed to parse command_rules.yml: " + err.Error())
	}
}

// ProfileFor returns the OS profile for a given BaseOS string.
// Falls back to the "unknown" profile (which defaults to Debian settings).
func ProfileFor(baseOS string) OSProfile {
	if p, ok := osProfiles[baseOS]; ok {
		return p
	}
	if p, ok := osProfiles["unknown"]; ok {
		return p
	}
	return OSProfile{
		DisplayName: "Unknown",
		OSTemplate:  "debian-12",
		PkgManager:  "apt",
		ServiceInit: "systemd",
		PipPrereqs:  []string{"python3", "python3-pip"},
	}
}

// PackageLayer tracks packages from a specific Dockerfile layer in the FROM chain.
type PackageLayer struct {
	Image    string   // short image name, e.g. "baseimage-alpine:3.21"
	Packages []string // deduplicated packages from this layer only
}

// CopyInstruction represents a COPY or ADD instruction from a Dockerfile.
type CopyInstruction struct {
	Src       string // e.g. "root/"
	Dest      string // e.g. "/"
	FromStage string // non-empty = multi-stage build artifact (skip for scaffold)
	IsAdd     bool   // ADD vs COPY
	IsURL     bool   // ADD with http URL
}

// RunCommand represents a categorized sub-command from a RUN instruction.
type RunCommand struct {
	Type     string // "mkdir", "useradd", "download", "symlink", "chmod", "chown", "sed", "git_clone", "skip", "unknown"
	Original string // raw command text
}

// DownloadAction represents a file download parsed from a RUN command.
type DownloadAction struct {
	URL  string
	Dest string
}

// SymlinkAction represents a symlink parsed from a RUN command.
type SymlinkAction struct {
	Target string
	Link   string
}

// DockerfileInfo holds data extracted from a Dockerfile.
type DockerfileInfo struct {
	BaseImage        string             // e.g. "ghcr.io/linuxserver/baseimage-ubuntu:noble"
	BaseOS           string             // "debian" | "alpine" | "unknown"
	AptKeys          []AptKey
	AptRepos         []AptRepo
	Packages         []string           // all packages merged and deduplicated
	PackageLayers    []PackageLayer     // packages grouped by FROM chain layer (parent-first)
	PipPackages      []string           // pip install packages
	Ports            []string           // from EXPOSE
	Volumes          []string           // from VOLUME
	EnvVars          []EnvVar           // from ENV
	ExecCmd          string             // startup command from s6 run script (if fetched)
	RepoURL          string             // GitHub repo URL, set during chain resolution
	CopyInstructions []CopyInstruction  // COPY/ADD instructions
	RunCommands      []RunCommand       // Categorized non-package RUN sub-commands
	Users            []string           // from useradd/adduser
	Directories      []string           // from mkdir -p (beyond VOLUME)
	Downloads        []DownloadAction   // from curl -o / wget -O
	Symlinks         []SymlinkAction    // from ln -s
	StartupCmd       string             // from CMD (joined if exec form)
	EntrypointCmd    string             // from ENTRYPOINT
}

// EnvVar represents an environment variable from a Dockerfile ENV instruction.
type EnvVar struct {
	Key     string
	Default string
}

// AptKey represents a GPG key added for an APT repository.
type AptKey struct {
	URL     string // https://...key.asc
	Keyring string // /usr/share/keyrings/name.asc
}

// AptRepo represents an APT repository source line.
type AptRepo struct {
	Line string // deb [signed-by=...] https://... suite component
	File string // /etc/apt/sources.list.d/name.list
}

// ParseDockerfile extracts structured info from Dockerfile content.
func ParseDockerfile(content string) *DockerfileInfo {
	info := &DockerfileInfo{BaseOS: "unknown"}

	lines := strings.Split(content, "\n")

	// Join continuation lines (backslash at end)
	var joined []string
	var buf strings.Builder
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t\r")
		if strings.HasSuffix(trimmed, "\\") {
			buf.WriteString(strings.TrimSuffix(trimmed, "\\"))
			buf.WriteString(" ")
			continue
		}
		buf.WriteString(trimmed)
		joined = append(joined, buf.String())
		buf.Reset()
	}
	if buf.Len() > 0 {
		joined = append(joined, buf.String())
	}

	// Track ARG values and stage aliases for FROM chain resolution
	argValues := map[string]string{}
	stageAliases := map[string]string{}

	// Track all FROM images in order (for scratch fallback)
	var fromImages []string

	// Track apk virtual groups: name -> list of packages in that group
	virtualGroups := map[string][]string{}
	// Track which virtual groups have been deleted
	deletedGroups := map[string]bool{}

	for _, line := range joined {
		stripped := strings.TrimSpace(line)

		// ARG
		if strings.HasPrefix(stripped, "ARG ") {
			parseArg(stripped, argValues)
			continue
		}

		// FROM — each new stage resets action fields (RunCommands, Users, etc.)
		// because earlier stages are build-time only (e.g. rootfs-builder).
		// Metadata (Ports, Volumes, EnvVars) and install data (Packages,
		// PipPackages, AptKeys, AptRepos) accumulate — Docker inherits
		// these across stages and they represent what's in the final image.
		if strings.HasPrefix(stripped, "FROM ") {
			if len(fromImages) > 0 {
				info.RunCommands = nil
				info.Users = nil
				info.Directories = nil
				info.Downloads = nil
				info.Symlinks = nil
				info.CopyInstructions = nil
				info.StartupCmd = ""
				info.EntrypointCmd = ""
			}
			parseFrom(stripped, info, argValues, stageAliases)
			fromImages = append(fromImages, info.BaseImage)
			continue
		}

		// EXPOSE
		if strings.HasPrefix(stripped, "EXPOSE ") {
			parseExpose(stripped, info)
			continue
		}

		// VOLUME
		if strings.HasPrefix(stripped, "VOLUME ") {
			parseVolume(stripped, info)
			continue
		}

		// ENV
		if strings.HasPrefix(stripped, "ENV ") {
			parseEnv(stripped, info)
			continue
		}

		// COPY / ADD
		if strings.HasPrefix(stripped, "COPY ") || strings.HasPrefix(stripped, "ADD ") {
			parseCopyAdd(stripped, info)
			continue
		}

		// CMD
		if strings.HasPrefix(stripped, "CMD ") {
			info.StartupCmd = parseExecForm(strings.TrimPrefix(stripped, "CMD "))
			continue
		}

		// ENTRYPOINT
		if strings.HasPrefix(stripped, "ENTRYPOINT ") {
			info.EntrypointCmd = parseExecForm(strings.TrimPrefix(stripped, "ENTRYPOINT "))
			continue
		}

		// RUN — split on && and ; (respecting quotes) and parse each command
		if strings.HasPrefix(stripped, "RUN ") {
			runBody := strings.TrimPrefix(stripped, "RUN ")
			cmds := splitRunCommands(runBody)
			for _, cmd := range cmds {
				cmd = strings.TrimSpace(cmd)
				if cmd == "" {
					continue
				}
				parseRunCommand(cmd, info)
				// Track apk virtual groups
				parseApkVirtual(cmd, virtualGroups, deletedGroups)
				// Track pip installs
				parsePipInstall(cmd, info)
				// Categorize non-package commands
				categorizeRunCommand(cmd, info)
			}
			continue
		}
	}

	// Resolve final base image through ARG + alias chain
	finalImage := resolveImageChain(info.BaseImage, argValues, stageAliases)
	info.BaseImage = finalImage
	info.BaseOS = detectBaseOS(strings.ToLower(finalImage))

	// If the final FROM is "scratch" (rootfs-builder pattern), fall back to the
	// previous FROM for base image/OS detection. The packages from all stages are
	// already collected — only the base detection needs fixing.
	if strings.ToLower(finalImage) == "scratch" && len(fromImages) >= 2 {
		prev := fromImages[len(fromImages)-2]
		prev = resolveImageChain(prev, argValues, stageAliases)
		info.BaseImage = prev
		info.BaseOS = detectBaseOS(strings.ToLower(prev))
	}

	// Remove packages belonging to deleted virtual groups
	if len(deletedGroups) > 0 {
		excludeSet := map[string]bool{}
		for group := range deletedGroups {
			for _, pkg := range virtualGroups[group] {
				excludeSet[pkg] = true
			}
		}
		if len(excludeSet) > 0 {
			var filtered []string
			for _, pkg := range info.Packages {
				if !excludeSet[pkg] {
					filtered = append(filtered, pkg)
				}
			}
			info.Packages = filtered
		}
	}

	// Filter out Docker-specific ENV vars that don't apply to LXC
	info.EnvVars = filterEnvVars(info.EnvVars)

	// Deduplicate packages, ports, and new fields
	info.Packages = dedup(info.Packages)
	info.PipPackages = dedup(info.PipPackages)
	info.Ports = dedup(info.Ports)
	info.Users = dedup(info.Users)
	info.Directories = dedup(info.Directories)

	return info
}

// parseArg extracts ARG name=value pairs.
func parseArg(line string, argValues map[string]string) {
	rest := strings.TrimPrefix(line, "ARG ")
	rest = strings.TrimSpace(rest)
	parts := strings.SplitN(rest, "=", 2)
	name := strings.TrimSpace(parts[0])
	if len(parts) == 2 {
		argValues[name] = strings.Trim(strings.TrimSpace(parts[1]), `"'`)
	}
	// ARG without default: don't store (unresolvable)
}

// parseEnv extracts ENV KEY=value or ENV KEY value pairs.
func parseEnv(line string, info *DockerfileInfo) {
	rest := strings.TrimPrefix(line, "ENV ")
	rest = strings.TrimSpace(rest)

	// Try KEY=VALUE form (can have multiple on one line)
	if strings.Contains(rest, "=") {
		// Split carefully: KEY=VALUE KEY2=VALUE2 or KEY="value with spaces"
		parts := splitEnvKeyValues(rest)
		for _, kv := range parts {
			eq := strings.IndexByte(kv, '=')
			if eq < 0 {
				continue
			}
			key := strings.TrimSpace(kv[:eq])
			val := strings.Trim(strings.TrimSpace(kv[eq+1:]), `"'`)
			if key != "" {
				info.EnvVars = append(info.EnvVars, EnvVar{Key: key, Default: val})
			}
		}
		return
	}

	// Legacy form: ENV KEY value
	fields := strings.Fields(rest)
	if len(fields) >= 2 {
		info.EnvVars = append(info.EnvVars, EnvVar{Key: fields[0], Default: strings.Join(fields[1:], " ")})
	} else if len(fields) == 1 {
		info.EnvVars = append(info.EnvVars, EnvVar{Key: fields[0], Default: ""})
	}
}

// splitEnvKeyValues splits "KEY1=val1 KEY2=val2" respecting quoted values.
func splitEnvKeyValues(s string) []string {
	var parts []string
	var current strings.Builder
	inQuote := byte(0)

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inQuote != 0 {
			current.WriteByte(ch)
			if ch == inQuote {
				inQuote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			inQuote = ch
			current.WriteByte(ch)
			continue
		}
		if ch == ' ' && current.Len() > 0 {
			// Check if next non-space looks like a KEY= (uppercase letter)
			rest := strings.TrimSpace(s[i+1:])
			if len(rest) > 0 && rest[0] >= 'A' && rest[0] <= 'Z' && strings.Contains(rest, "=") {
				parts = append(parts, current.String())
				current.Reset()
				continue
			}
		}
		current.WriteByte(ch)
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

// dockerSpecificEnvVars are ENV vars that are Docker-specific and should not
// be converted to LXC container inputs.
var dockerSpecificEnvVars = map[string]bool{
	// UID/GID (linuxserver convention)
	"PUID": true, "PGID": true, "UMASK": true, "UMASK_SET": true,
	// Timezone
	"TZ": true,
	// Docker/s6 internals
	"DOCKER_MODS": true, "S6_OVERLAY_VERSION": true,
	"S6_CMD_WAIT_FOR_SERVICES_MAXTIME": true, "S6_VERBOSITY": true,
	"S6_BEHAVIOUR_IF_STAGE2_FAILS": true, "S6_STAGE2_HOOK": true,
	// System paths (not configurable)
	"HOME": true, "PATH": true, "LANG": true, "LC_ALL": true,
	"LANGUAGE": true, "DEBIAN_FRONTEND": true, "TERM": true,
	// Python/pip internals
	"PYTHONDONTWRITEBYTECODE": true, "PYTHONUNBUFFERED": true,
	"PIP_NO_CACHE_DIR": true, "PIP_DISABLE_PIP_VERSION_CHECK": true,
	// Go/Node build vars
	"GOPATH": true, "GOROOT": true, "NODE_ENV": true,
	// XDG dirs
	"XDG_DATA_HOME": true, "XDG_CONFIG_HOME": true, "XDG_CACHE_HOME": true,
}

// filterEnvVars removes Docker-specific env vars and deduplicates.
func filterEnvVars(vars []EnvVar) []EnvVar {
	seen := map[string]bool{}
	var out []EnvVar
	for _, v := range vars {
		if dockerSpecificEnvVars[v.Key] {
			continue
		}
		// Skip vars that reference other variables (not user-configurable)
		if strings.Contains(v.Default, "$") {
			continue
		}
		if seen[v.Key] {
			continue
		}
		seen[v.Key] = true
		out = append(out, v)
	}
	return out
}

func parseFrom(line string, info *DockerfileInfo, argValues map[string]string, stageAliases map[string]string) {
	// FROM [--platform=...] image:tag [AS name]
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return
	}

	// Skip "FROM" keyword, then skip --platform=... flags
	idx := 1
	for idx < len(parts) && strings.HasPrefix(parts[idx], "--") {
		idx++
	}
	if idx >= len(parts) {
		return
	}

	imageRef := parts[idx]
	info.BaseImage = imageRef
	img := strings.ToLower(imageRef)
	info.BaseOS = detectBaseOS(img)

	// Check for "AS stagename"
	for i := idx + 1; i < len(parts)-1; i++ {
		if strings.EqualFold(parts[i], "AS") {
			stageName := parts[i+1]
			stageAliases[stageName] = imageRef
			break
		}
	}
}

// expandVariables replaces ${VAR} and $VAR with their ARG values.
func expandVariables(s string, argValues map[string]string) string {
	result := s
	for name, val := range argValues {
		result = strings.ReplaceAll(result, "${"+name+"}", val)
		result = strings.ReplaceAll(result, "$"+name, val)
	}
	return result
}

// resolveImageChain expands variables and follows alias chains to find the concrete base image.
func resolveImageChain(ref string, argValues map[string]string, stageAliases map[string]string) string {
	current := ref
	for i := 0; i < 10; i++ { // max depth to prevent cycles
		// Expand variables
		expanded := expandVariables(current, argValues)
		if expanded == current {
			// No variable expansion happened; try alias resolution
			if base, ok := stageAliases[current]; ok && base != current {
				current = base
				continue
			}
			break
		}
		current = expanded
		// After expansion, check if result is a stage alias
		if base, ok := stageAliases[current]; ok && base != current {
			current = base
			continue
		}
		// If no alias match, we're done
		break
	}
	return current
}

// detectBaseOS looks up the image name against base_images.yml rules.
func detectBaseOS(img string) string {
	for osFamily, rules := range baseImageMap {
		for _, substr := range rules.Contains {
			if strings.Contains(img, substr) {
				return osFamily
			}
		}
		for _, pfx := range rules.Prefix {
			if strings.HasPrefix(img, pfx) {
				return osFamily
			}
		}
	}
	return "unknown"
}

func parseExpose(line string, info *DockerfileInfo) {
	// EXPOSE 8080 8443/tcp
	fields := strings.Fields(line)
	for _, f := range fields[1:] {
		port := strings.Split(f, "/")[0]
		port = strings.TrimSpace(port)
		if port != "" && !strings.HasPrefix(port, "$") {
			info.Ports = append(info.Ports, port)
		}
	}
}

func parseVolume(line string, info *DockerfileInfo) {
	rest := strings.TrimPrefix(line, "VOLUME ")
	rest = strings.TrimSpace(rest)

	// JSON form: VOLUME ["/data", "/config"]
	if strings.HasPrefix(rest, "[") {
		rest = strings.Trim(rest, "[]")
		for _, part := range strings.Split(rest, ",") {
			v := strings.Trim(strings.TrimSpace(part), `"'`)
			if v != "" {
				info.Volumes = append(info.Volumes, v)
			}
		}
		return
	}
	// Space-separated form: VOLUME /data /config
	for _, part := range strings.Fields(rest) {
		v := strings.Trim(part, `"'`)
		if v != "" {
			info.Volumes = append(info.Volumes, v)
		}
	}
}

// Regex patterns for APT key/repo extraction
var (
	// curl ... URL | tee/gpg DEST
	reAptKey = regexp.MustCompile(`curl\s+[^|]*?(https?://\S+\.(?:asc|gpg\.key|gpg|key))\s*[|>]\s*(?:tee|gpg\s+--dearmor\s*(?:-o|>)\s*|tee\s+)(/\S+)`)
	// Alternate: curl ... URL | gpg --dearmor | tee DEST
	reAptKey2 = regexp.MustCompile(`curl\s+[^|]*?(https?://\S+\.(?:asc|gpg\.key|gpg|key))\s*\|\s*gpg\s+--dearmor\s*\|\s*tee\s+(/\S+)`)
	// wget -qO- URL | gpg --dearmor > DEST
	reAptKey3 = regexp.MustCompile(`(?:wget|curl)\s+[^|]*?(https?://\S+\.(?:asc|gpg\.key|gpg|key))\s*\|\s*gpg\s+--dearmor\s*>\s*(/\S+)`)

	// echo "deb ..." > /etc/apt/sources.list.d/...
	reAptRepo = regexp.MustCompile(`echo\s+["'](deb\s+.+?)["']\s*(?:>|[|]\s*tee)\s*(/etc/apt/sources\.list\.d/\S+)`)

	// apt-get install / apt install -y packages
	reAptInstall = regexp.MustCompile(`apt(?:-get)?\s+install\s+(?:-[a-zA-Z]+\s+)*(.+)`)

	// apk add packages (skip --virtual=NAME and --no-cache flags)
	reApkAdd = regexp.MustCompile(`apk\s+add\s+(?:--[a-zA-Z-]+=?\S*\s+)*(.+)`)

	// apk add --virtual=NAME — captures the virtual group name
	reApkVirtual = regexp.MustCompile(`apk\s+add\s+[^&]*--virtual[=\s]+(\S+)`)

	// apk del --purge NAME — captures deleted group names
	reApkDel = regexp.MustCompile(`apk\s+del\s+(?:--[a-zA-Z-]+\s+)*(.+)`)

	// pip/pip3 install packages
	rePipInstall = regexp.MustCompile(`pip3?\s+install\s+(?:-[a-zA-Z]+\s+)*(.+)`)

	// Strip version qualifiers from packages
	reVersionQualifier = regexp.MustCompile(`[=<>]+\S*`)
)

func parseRunCommand(cmd string, info *DockerfileInfo) {
	// APT key extraction — try multiple patterns
	if m := reAptKey2.FindStringSubmatch(cmd); len(m) == 3 {
		info.AptKeys = append(info.AptKeys, AptKey{URL: m[1], Keyring: m[2]})
	} else if m := reAptKey3.FindStringSubmatch(cmd); len(m) == 3 {
		info.AptKeys = append(info.AptKeys, AptKey{URL: m[1], Keyring: m[2]})
	} else if m := reAptKey.FindStringSubmatch(cmd); len(m) == 3 {
		info.AptKeys = append(info.AptKeys, AptKey{URL: m[1], Keyring: m[2]})
	}

	// APT repo extraction
	if m := reAptRepo.FindStringSubmatch(cmd); len(m) == 3 {
		info.AptRepos = append(info.AptRepos, AptRepo{Line: m[1], File: m[2]})
	}

	// Package extraction
	if m := reAptInstall.FindStringSubmatch(cmd); len(m) == 2 {
		pkgs := extractPackages(m[1])
		info.Packages = append(info.Packages, pkgs...)
	}
	if m := reApkAdd.FindStringSubmatch(cmd); len(m) == 2 {
		pkgs := extractPackages(m[1])
		info.Packages = append(info.Packages, pkgs...)
	}
}

// parseApkVirtual tracks apk virtual group additions and deletions.
func parseApkVirtual(cmd string, virtualGroups map[string][]string, deletedGroups map[string]bool) {
	// Detect `apk add --virtual=NAME pkg1 pkg2 ...`
	if m := reApkVirtual.FindStringSubmatch(cmd); len(m) == 2 {
		groupName := m[1]
		// Extract the packages from the same command
		if m2 := reApkAdd.FindStringSubmatch(cmd); len(m2) == 2 {
			pkgs := extractPackages(m2[1])
			virtualGroups[groupName] = pkgs
		}
	}

	// Detect `apk del [--purge] NAME`
	if m := reApkDel.FindStringSubmatch(cmd); len(m) == 2 {
		for _, name := range strings.Fields(m[1]) {
			if strings.HasPrefix(name, "-") {
				continue
			}
			deletedGroups[name] = true
		}
	}
}

// parsePipInstall extracts packages from pip install commands.
func parsePipInstall(cmd string, info *DockerfileInfo) {
	if m := rePipInstall.FindStringSubmatch(cmd); len(m) == 2 {
		pkgs := extractPipPackages(m[1])
		info.PipPackages = append(info.PipPackages, pkgs...)
	}
}

func extractPipPackages(raw string) []string {
	var pkgs []string
	skipNext := false
	for _, tok := range strings.Fields(raw) {
		if skipNext {
			skipNext = false
			continue
		}
		if strings.HasPrefix(tok, "-") {
			// Flags that take a file/value argument — skip the next token too
			if tok == "-r" || tok == "-c" || tok == "-f" || tok == "-i" || tok == "-e" ||
				tok == "--requirement" || tok == "--constraint" || tok == "--find-links" ||
				tok == "--index-url" || tok == "--extra-index-url" || tok == "--target" {
				skipNext = true
			}
			continue
		}
		if tok == "&&" || tok == "||" || tok == ";" || tok == "\\" || tok == "|" {
			break
		}
		if strings.HasPrefix(tok, "$") || strings.HasPrefix(tok, "http://") || strings.HasPrefix(tok, "https://") {
			continue
		}
		// Skip file references (e.g. requirements.txt, ./setup.py)
		if strings.HasSuffix(tok, ".txt") || strings.HasSuffix(tok, ".cfg") ||
			strings.HasPrefix(tok, ".") || strings.HasPrefix(tok, "/") {
			continue
		}
		// Strip version qualifiers (==, >=, etc.)
		pkg := reVersionQualifier.ReplaceAllString(tok, "")
		pkg = strings.TrimSpace(pkg)
		if pkg != "" {
			pkgs = append(pkgs, pkg)
		}
	}
	return pkgs
}

// splitRunCommands splits a RUN body on && and ; while respecting quotes and subshells.
func splitRunCommands(body string) []string {
	var parts []string
	var current strings.Builder
	inQuote := byte(0)
	parenDepth := 0

	for i := 0; i < len(body); i++ {
		ch := body[i]

		// Track quoting
		if inQuote != 0 {
			current.WriteByte(ch)
			if ch == inQuote {
				inQuote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			inQuote = ch
			current.WriteByte(ch)
			continue
		}

		// Track subshells
		if ch == '(' {
			parenDepth++
			current.WriteByte(ch)
			continue
		}
		if ch == ')' {
			if parenDepth > 0 {
				parenDepth--
			}
			current.WriteByte(ch)
			continue
		}

		// Only split at top level
		if parenDepth > 0 {
			current.WriteByte(ch)
			continue
		}

		// Split on &&
		if ch == '&' && i+1 < len(body) && body[i+1] == '&' {
			parts = append(parts, current.String())
			current.Reset()
			i++ // skip second &
			continue
		}

		// Split on ;
		if ch == ';' {
			parts = append(parts, current.String())
			current.Reset()
			continue
		}

		current.WriteByte(ch)
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

// parseCopyAdd parses COPY and ADD instructions.
func parseCopyAdd(line string, info *DockerfileInfo) {
	isAdd := strings.HasPrefix(line, "ADD ")
	rest := line[4:] // skip "ADD " or "COPY"
	if !isAdd {
		rest = line[5:] // skip "COPY "
	}
	rest = strings.TrimSpace(rest)

	// Parse flags: --from=stage, --chown=..., --chmod=..., --link, etc.
	var fromStage string
	for strings.HasPrefix(rest, "--") {
		spaceIdx := strings.IndexByte(rest, ' ')
		if spaceIdx < 0 {
			return // malformed
		}
		flag := rest[:spaceIdx]
		rest = strings.TrimSpace(rest[spaceIdx+1:])

		if strings.HasPrefix(flag, "--from=") {
			fromStage = strings.TrimPrefix(flag, "--from=")
		}
		// Ignore --chown, --chmod, --link, etc.
	}

	// Parse JSON form: COPY ["src", "dest"]
	if strings.HasPrefix(rest, "[") {
		rest = strings.Trim(rest, "[]")
		var items []string
		for _, part := range strings.Split(rest, ",") {
			items = append(items, strings.Trim(strings.TrimSpace(part), `"'`))
		}
		if len(items) >= 2 {
			dest := items[len(items)-1]
			for _, src := range items[:len(items)-1] {
				isURL := strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://")
				info.CopyInstructions = append(info.CopyInstructions, CopyInstruction{
					Src: src, Dest: dest, FromStage: fromStage, IsAdd: isAdd, IsURL: isURL,
				})
			}
		}
		return
	}

	// Space-separated form: last token is dest
	fields := strings.Fields(rest)
	if len(fields) < 2 {
		return
	}
	dest := fields[len(fields)-1]
	for _, src := range fields[:len(fields)-1] {
		isURL := strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://")
		info.CopyInstructions = append(info.CopyInstructions, CopyInstruction{
			Src: src, Dest: dest, FromStage: fromStage, IsAdd: isAdd, IsURL: isURL,
		})
	}
}

// parseExecForm parses CMD/ENTRYPOINT arguments.
// Handles exec form: ["cmd", "arg1"] and shell form: cmd arg1
func parseExecForm(rest string) string {
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, "[") {
		// Exec form: ["cmd", "arg1", "arg2"]
		rest = strings.Trim(rest, "[]")
		var parts []string
		for _, part := range strings.Split(rest, ",") {
			p := strings.Trim(strings.TrimSpace(part), `"'`)
			if p != "" {
				parts = append(parts, p)
			}
		}
		return strings.Join(parts, " ")
	}
	// Shell form: use as-is
	return rest
}

// Regex patterns for RUN command categorization
var (
	reMkdir    = regexp.MustCompile(`^mkdir\s+(?:-[a-zA-Z]+\s+)*(.+)`)
	reUseradd  = regexp.MustCompile(`^(?:useradd|adduser)\s+(.+)`)
	reCurlO    = regexp.MustCompile(`curl\s+[^|]*?(?:-o|--output)\s+(\S+)\s+.*?(https?://\S+)`)
	reCurlOAlt = regexp.MustCompile(`curl\s+[^|]*?(https?://\S+)\s+[^|]*?(?:-o|--output)\s+(\S+)`)
	reWgetO    = regexp.MustCompile(`wget\s+[^|]*?(?:-O|--output-document[=\s])\s*(\S+)\s+.*?(https?://\S+)`)
	reWgetOAlt = regexp.MustCompile(`wget\s+[^|]*?(https?://\S+)\s+[^|]*?(?:-O|--output-document[=\s])\s*(\S+)`)
	reLnS      = regexp.MustCompile(`ln\s+(?:-[a-zA-Z]+\s+)*(\S+)\s+(\S+)`)
	reChmod    = regexp.MustCompile(`^chmod\s+(.+)`)
	reChown    = regexp.MustCompile(`^chown\s+(.+)`)
	reSed      = regexp.MustCompile(`^sed\s+`)
	reGitClone = regexp.MustCompile(`git\s+clone\s+(.+)`)
)

// isShellNoise returns true for Docker layer optimization commands that should be skipped.
// Rules are loaded from command_rules.yml.
func isShellNoise(cmd string) bool {
	trimmed := strings.TrimSpace(cmd)
	if trimmed == "" {
		return true
	}

	// Exact matches
	for _, exact := range cmdRules.Noise.Exact {
		if trimmed == exact {
			return true
		}
	}

	// Prefix matches
	for _, pfx := range cmdRules.Noise.Prefixes {
		if strings.HasPrefix(trimmed, pfx) {
			return true
		}
	}

	// Contains matches (lowercased)
	lower := strings.ToLower(trimmed)
	for _, pat := range cmdRules.Noise.Contains {
		if strings.Contains(lower, pat) {
			return true
		}
	}

	// Echo section markers (echo "****...")
	if cmdRules.Noise.SkipEchoMarkers {
		if strings.HasPrefix(trimmed, "echo ") && strings.Contains(trimmed, "****") {
			return true
		}
	}

	// Printf build version info
	if cmdRules.Noise.SkipPrintfBuild {
		if strings.HasPrefix(trimmed, "printf ") && strings.Contains(lower, "build") {
			return true
		}
	}

	return false
}

// isAlreadyHandled returns true if the command is already handled by existing
// package/key/repo parsing and should not be re-categorized.
func isAlreadyHandled(cmd string) bool {
	trimmed := strings.TrimSpace(cmd)
	lower := strings.ToLower(trimmed)

	// apt-get/apt install
	if strings.Contains(lower, "apt-get install") || strings.Contains(lower, "apt install") {
		return true
	}
	// apk add
	if strings.Contains(lower, "apk add") {
		return true
	}
	// apk del
	if strings.Contains(lower, "apk del") {
		return true
	}
	// pip install
	if rePipInstall.MatchString(trimmed) {
		return true
	}
	// APT key patterns
	if reAptKey.MatchString(trimmed) || reAptKey2.MatchString(trimmed) || reAptKey3.MatchString(trimmed) {
		return true
	}
	// APT repo patterns
	if reAptRepo.MatchString(trimmed) {
		return true
	}
	return false
}

// categorizeRunCommand categorizes a RUN sub-command and populates the
// corresponding fields on DockerfileInfo.
func categorizeRunCommand(cmd string, info *DockerfileInfo) {
	trimmed := strings.TrimSpace(cmd)
	if trimmed == "" {
		return
	}

	// Skip shell noise
	if isShellNoise(trimmed) {
		return
	}

	// Skip commands already handled by package/key/repo parsing
	if isAlreadyHandled(trimmed) {
		return
	}

	// mkdir -p /path
	if m := reMkdir.FindStringSubmatch(trimmed); len(m) == 2 {
		for _, dir := range strings.Fields(m[1]) {
			if strings.HasPrefix(dir, "-") {
				continue
			}
			dir = strings.Trim(dir, `"'`)
			if dir != "" && strings.HasPrefix(dir, "/") {
				info.Directories = append(info.Directories, dir)
			}
		}
		info.RunCommands = append(info.RunCommands, RunCommand{Type: "mkdir", Original: trimmed})
		return
	}

	// useradd / adduser
	if m := reUseradd.FindStringSubmatch(trimmed); len(m) == 2 {
		// Extract username: last non-flag argument
		args := strings.Fields(m[1])
		for i := len(args) - 1; i >= 0; i-- {
			if !strings.HasPrefix(args[i], "-") {
				info.Users = append(info.Users, args[i])
				break
			}
		}
		info.RunCommands = append(info.RunCommands, RunCommand{Type: "useradd", Original: trimmed})
		return
	}

	// curl -o dest url or curl url -o dest
	if m := reCurlO.FindStringSubmatch(trimmed); len(m) == 3 {
		info.Downloads = append(info.Downloads, DownloadAction{URL: stripQuotes(m[2]), Dest: stripQuotes(m[1])})
		info.RunCommands = append(info.RunCommands, RunCommand{Type: "download", Original: trimmed})
		return
	}
	if m := reCurlOAlt.FindStringSubmatch(trimmed); len(m) == 3 {
		info.Downloads = append(info.Downloads, DownloadAction{URL: stripQuotes(m[1]), Dest: stripQuotes(m[2])})
		info.RunCommands = append(info.RunCommands, RunCommand{Type: "download", Original: trimmed})
		return
	}

	// wget -O dest url or wget url -O dest
	if m := reWgetO.FindStringSubmatch(trimmed); len(m) == 3 {
		info.Downloads = append(info.Downloads, DownloadAction{URL: stripQuotes(m[2]), Dest: stripQuotes(m[1])})
		info.RunCommands = append(info.RunCommands, RunCommand{Type: "download", Original: trimmed})
		return
	}
	if m := reWgetOAlt.FindStringSubmatch(trimmed); len(m) == 3 {
		info.Downloads = append(info.Downloads, DownloadAction{URL: stripQuotes(m[1]), Dest: stripQuotes(m[2])})
		info.RunCommands = append(info.RunCommands, RunCommand{Type: "download", Original: trimmed})
		return
	}

	// ln -s target link
	if strings.HasPrefix(trimmed, "ln ") {
		if m := reLnS.FindStringSubmatch(trimmed); len(m) == 3 {
			info.Symlinks = append(info.Symlinks, SymlinkAction{Target: m[1], Link: m[2]})
			info.RunCommands = append(info.RunCommands, RunCommand{Type: "symlink", Original: trimmed})
			return
		}
	}

	// chmod
	if reChmod.MatchString(trimmed) {
		info.RunCommands = append(info.RunCommands, RunCommand{Type: "chmod", Original: trimmed})
		return
	}

	// chown
	if reChown.MatchString(trimmed) {
		info.RunCommands = append(info.RunCommands, RunCommand{Type: "chown", Original: trimmed})
		return
	}

	// rm of relative paths only is Docker build-time cleanup
	// (e.g. "rm composer-setup.php" — file was created in same RUN block, never exists in LXC)
	if strings.HasPrefix(trimmed, "rm ") {
		allRelative := true
		for _, arg := range strings.Fields(trimmed)[1:] {
			if strings.HasPrefix(arg, "-") {
				continue
			}
			if strings.HasPrefix(arg, "/") {
				allRelative = false
				break
			}
		}
		if allRelative {
			return // skip — build artifact cleanup
		}
	}

	// YAML-driven command type detection (prefix-based)
	for _, rule := range cmdRules.CommandTypes {
		if strings.HasPrefix(trimmed, rule.Prefix) {
			info.RunCommands = append(info.RunCommands, RunCommand{Type: rule.Type, Original: trimmed})
			return
		}
	}

	// git clone (regex-based, stays in code)
	if reGitClone.MatchString(trimmed) {
		info.RunCommands = append(info.RunCommands, RunCommand{Type: "git_clone", Original: trimmed})
		return
	}

	// Unknown — capture for TODO comments
	info.RunCommands = append(info.RunCommands, RunCommand{Type: "unknown", Original: trimmed})
}

func extractPackages(raw string) []string {
	var pkgs []string
	for _, tok := range strings.Fields(raw) {
		// Skip flags
		if strings.HasPrefix(tok, "-") {
			continue
		}
		// Skip shell operators and continuations
		if tok == "&&" || tok == "||" || tok == ";" || tok == "\\" || tok == "|" {
			break // rest is another command
		}
		// Stop at inline comments
		if strings.HasPrefix(tok, "#") {
			break
		}
		// Skip variable references
		if strings.HasPrefix(tok, "$") || strings.HasPrefix(tok, "${") {
			continue
		}
		// Skip local .deb/.apk files (apt install ./file.deb)
		if strings.HasSuffix(tok, ".deb") || strings.HasSuffix(tok, ".apk") ||
			strings.HasPrefix(tok, "./") || strings.HasPrefix(tok, "/") {
			continue
		}
		// Skip shell keywords that leak through from complex RUN blocks
		if tok == "if" || tok == "then" || tok == "else" || tok == "fi" ||
			tok == "echo" || tok == "do" || tok == "done" || tok == "for" ||
			tok == "while" || tok == "case" || tok == "esac" {
			break
		}
		// Strip surrounding quotes (Dockerfiles often quote package names)
		tok = strings.Trim(tok, `"'`)
		if tok == "" {
			continue
		}
		// Re-check for variable references after unquoting (e.g. "$PKG")
		if strings.HasPrefix(tok, "$") || strings.HasPrefix(tok, "${") {
			continue
		}
		// Strip version qualifiers
		pkg := reVersionQualifier.ReplaceAllString(tok, "")
		pkg = strings.TrimSpace(pkg)
		if pkg != "" {
			pkgs = append(pkgs, pkg)
		}
	}
	return pkgs
}

// InferDockerfileURL derives a raw GitHub Dockerfile URL from Repository and GitHub fields.
func InferDockerfileURL(repository, githubURL string) (url string, branch string) {
	// From explicit GitHub URL
	if githubURL != "" {
		clean := strings.TrimSuffix(githubURL, "/")
		// Strip fragment (#application-setup, etc.)
		if idx := strings.Index(clean, "#"); idx > 0 {
			clean = clean[:idx]
		}
		// Extract owner/repo from https://github.com/owner/repo
		if strings.HasPrefix(clean, "https://github.com/") {
			path := strings.TrimPrefix(clean, "https://github.com/")
			// path = "owner/repo" or "owner/repo/tree/branch"
			parts := strings.SplitN(path, "/", 4)
			if len(parts) >= 2 {
				owner := parts[0]
				repo := parts[1]
				return "https://raw.githubusercontent.com/" + owner + "/" + repo + "/{branch}/Dockerfile", "master"
			}
		}
	}

	// From Repository field — try linuxserver convention
	repo := strings.ToLower(repository)
	if strings.Contains(repo, "linuxserver/") {
		// lscr.io/linuxserver/resilio-sync or ghcr.io/linuxserver/resilio-sync
		parts := strings.Split(repo, "/")
		name := parts[len(parts)-1]
		// Strip tag
		if idx := strings.Index(name, ":"); idx > 0 {
			name = name[:idx]
		}
		return "https://raw.githubusercontent.com/linuxserver/docker-" + name + "/{branch}/Dockerfile", "master"
	}

	return "", ""
}

// InferS6RunURL derives the s6 run script URL for a linuxserver-style image.
func InferS6RunURL(dockerfileBaseURL, appName string) string {
	if dockerfileBaseURL == "" {
		return ""
	}
	// Base is e.g. https://raw.githubusercontent.com/linuxserver/docker-resilio-sync/{branch}
	base := strings.TrimSuffix(dockerfileBaseURL, "/Dockerfile")
	kebab := toKebabCase(appName)

	// linuxserver convention: root/etc/s6-overlay/s6-rc.d/svc-{name}/run
	return base + "/root/etc/s6-overlay/s6-rc.d/svc-" + kebab + "/run"
}

// ParseS6RunScript extracts the exec command from an s6 run script.
func ParseS6RunScript(content string) string {
	lines := strings.Split(content, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// s6 scripts often use: exec s6-notifyoncheck ... or exec command
		if strings.HasPrefix(line, "exec ") {
			cmd := strings.TrimPrefix(line, "exec ")
			// Strip s6 wrappers like s6-notifyoncheck -d -n 300 -w 1000 -c "..."
			if strings.HasPrefix(cmd, "s6-") {
				// Find the actual command after s6 flags
				// Look for the last quoted string or the last recognizable command
				if idx := strings.LastIndex(cmd, "-- "); idx >= 0 {
					return strings.TrimSpace(cmd[idx+3:])
				}
				// Or take everything after the s6-setuidgid or similar
				if idx := strings.Index(cmd, "/"); idx >= 0 {
					// Find path-like command
					rest := cmd[idx:]
					return strings.TrimSpace(rest)
				}
			}
			return strings.TrimSpace(cmd)
		}
		// Non-exec last line is the command itself
		if !strings.HasPrefix(line, "cd ") && !strings.HasPrefix(line, "umask") &&
			!strings.HasPrefix(line, "chown") && !strings.HasPrefix(line, "if ") {
			return line
		}
	}
	return ""
}

// stripQuotes removes surrounding or trailing quote characters from a string.
func stripQuotes(s string) string {
	return strings.Trim(s, `"'`)
}

func dedup(items []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			out = append(out, item)
		}
	}
	return out
}
