package devmode

import (
	"regexp"
	"strings"
)

// DockerfileInfo holds data extracted from a Dockerfile.
type DockerfileInfo struct {
	BaseImage   string   // e.g. "ghcr.io/linuxserver/baseimage-ubuntu:noble"
	BaseOS      string   // "debian" | "alpine" | "unknown"
	AptKeys     []AptKey
	AptRepos    []AptRepo
	Packages    []string // apt-get install or apk add packages (version stripped)
	PipPackages []string // pip install packages
	Ports       []string // from EXPOSE
	Volumes     []string // from VOLUME
	ExecCmd     string   // startup command from s6 run script (if fetched)
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

	// Track apk virtual groups: name -> list of packages in that group
	virtualGroups := map[string][]string{}
	// Track which virtual groups have been deleted
	deletedGroups := map[string]bool{}

	for _, line := range joined {
		stripped := strings.TrimSpace(line)

		// FROM
		if strings.HasPrefix(stripped, "FROM ") {
			parseFrom(stripped, info)
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

		// RUN — split on && and parse each command
		if strings.HasPrefix(stripped, "RUN ") {
			runBody := strings.TrimPrefix(stripped, "RUN ")
			cmds := strings.Split(runBody, "&&")
			for _, cmd := range cmds {
				cmd = strings.TrimSpace(cmd)
				parseRunCommand(cmd, info)
				// Track apk virtual groups
				parseApkVirtual(cmd, virtualGroups, deletedGroups)
				// Track pip installs
				parsePipInstall(cmd, info)
			}
			continue
		}
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

	// Deduplicate packages and ports
	info.Packages = dedup(info.Packages)
	info.PipPackages = dedup(info.PipPackages)
	info.Ports = dedup(info.Ports)

	return info
}

func parseFrom(line string, info *DockerfileInfo) {
	// FROM image:tag [AS name]
	parts := strings.Fields(line)
	if len(parts) >= 2 {
		info.BaseImage = parts[1]
		img := strings.ToLower(parts[1])
		switch {
		case strings.Contains(img, "alpine"):
			info.BaseOS = "alpine"
		case strings.Contains(img, "debian"),
			strings.Contains(img, "ubuntu"),
			strings.Contains(img, "jammy"),
			strings.Contains(img, "noble"),
			strings.Contains(img, "focal"),
			strings.Contains(img, "bookworm"),
			strings.Contains(img, "bullseye"):
			info.BaseOS = "debian"
		}
	}
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

	// apt-get install -y packages
	reAptInstall = regexp.MustCompile(`apt-get\s+install\s+(?:-[a-zA-Z]+\s+)*(.+)`)

	// apk add packages (skip --virtual=NAME and --no-cache flags)
	reApkAdd = regexp.MustCompile(`apk\s+add\s+(?:--[a-zA-Z-]+=?\S*\s+)*(.+)`)

	// apk add --virtual=NAME — captures the virtual group name
	reApkVirtual = regexp.MustCompile(`apk\s+add\s+[^&]*--virtual[=\s]+(\S+)`)

	// apk del --purge NAME — captures deleted group names
	reApkDel = regexp.MustCompile(`apk\s+del\s+(?:--[a-zA-Z-]+\s+)*(.+)`)

	// pip install packages
	rePipInstall = regexp.MustCompile(`pip\s+install\s+(?:-[a-zA-Z]+\s+)*(.+)`)

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
	for _, tok := range strings.Fields(raw) {
		if strings.HasPrefix(tok, "-") {
			continue
		}
		if tok == "&&" || tok == "||" || tok == ";" || tok == "\\" || tok == "|" {
			break
		}
		if strings.HasPrefix(tok, "$") || strings.HasPrefix(tok, "http://") || strings.HasPrefix(tok, "https://") {
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
		// Skip variable references
		if strings.HasPrefix(tok, "$") {
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
