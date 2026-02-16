package devmode

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/battlewithbytes/pve-appstore/internal/catalog"
	"gopkg.in/yaml.v3"
)

// ValidationResult contains errors, warnings, and a deploy readiness checklist.
type ValidationResult struct {
	Valid     bool            `json:"valid"`
	Errors    []ValidationMsg `json:"errors"`
	Warnings  []ValidationMsg `json:"warnings"`
	Checklist []ChecklistItem `json:"checklist"`
}

// ValidationMsg is a single validation message.
type ValidationMsg struct {
	File    string `json:"file"`
	Line    int    `json:"line,omitempty"`
	Message string `json:"message"`
	Code    string `json:"code"`
}

// ChecklistItem is a deploy readiness check.
type ChecklistItem struct {
	Label  string `json:"label"`
	Passed bool   `json:"passed"`
}

// Validate runs all checks on a dev app directory.
func Validate(appDir string) *ValidationResult {
	result := &ValidationResult{
		Valid:    true,
		Errors:   []ValidationMsg{},
		Warnings: []ValidationMsg{},
	}

	manifestPath := filepath.Join(appDir, "app.yml")
	scriptPath := filepath.Join(appDir, "provision", "install.py")

	// --- Manifest validation ---
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		result.addError("app.yml", 0, "Manifest file not found", "MANIFEST_MISSING")
		result.Valid = false
	} else {
		validateManifest(result, manifestData, appDir)
	}

	// --- Script validation ---
	scriptData, err := os.ReadFile(scriptPath)
	if err != nil {
		result.addError("provision/install.py", 0, "Install script not found", "SCRIPT_MISSING")
		result.Valid = false
	} else {
		validateScript(result, string(scriptData), manifestData)
	}

	// --- Checklist ---
	buildChecklist(result, appDir, manifestData)

	return result
}

func validateManifest(result *ValidationResult, data []byte, appDir string) {
	var manifest catalog.AppManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		result.addError("app.yml", 0, fmt.Sprintf("YAML parse error: %v", err), "MANIFEST_PARSE_ERROR")
		result.Valid = false
		return
	}

	if err := manifest.Validate(); err != nil {
		result.addError("app.yml", 0, err.Error(), "MANIFEST_VALIDATION")
		result.Valid = false
	}

	// Additional dev mode checks
	if len(manifest.Maintainers) == 0 {
		result.addWarning("app.yml", 0, "No maintainers listed", "MANIFEST_NO_MAINTAINERS")
	}

	if manifest.Description == "" || strings.HasPrefix(manifest.Description, "TODO") {
		result.addWarning("app.yml", 0, "Description is empty or still contains TODO", "MANIFEST_TODO_DESC")
	}

	// Check version is semver-like
	if manifest.Version != "" && !regexp.MustCompile(`^\d+\.\d+\.\d+`).MatchString(manifest.Version) {
		result.addWarning("app.yml", 0, "Version should follow semver format (e.g. 1.0.0)", "MANIFEST_VERSION_FORMAT")
	}

	// Check icon
	iconPath := filepath.Join(appDir, "icon.png")
	if !isCustomIcon(iconPath) {
		result.addWarning("app.yml", 0, "No custom icon.png found — set one via the icon editor", "MANIFEST_NO_ICON")
	} else {
		if info, err := os.Stat(iconPath); err == nil {
			if info.Size() > 512*1024 {
				result.addWarning("icon.png", 0, "Icon file is larger than 512KB", "ICON_TOO_LARGE")
			}
		}
	}

	// Check readme
	if _, err := os.Stat(filepath.Join(appDir, "README.md")); os.IsNotExist(err) {
		result.addWarning("app.yml", 0, "No README.md file found (recommended)", "MANIFEST_NO_README")
	} else {
		if data, err := os.ReadFile(filepath.Join(appDir, "README.md")); err == nil {
			if len(strings.TrimSpace(string(data))) < 100 {
				result.addWarning("README.md", 0, "README is very short (< 100 chars)", "README_TOO_SHORT")
			}
		}
	}

	// Check that output template keys reference existing inputs
	inputKeys := make(map[string]bool)
	for _, inp := range manifest.Inputs {
		inputKeys[inp.Key] = true
	}
	for _, out := range manifest.Outputs {
		refs := extractTemplateVars(out.Value)
		for _, ref := range refs {
			if strings.EqualFold(ref, "ip") || inputKeys[ref] {
				continue
			}
			result.addWarning("app.yml", 0,
				fmt.Sprintf("Output %q references unknown input key {{%s}}", out.Key, ref),
				"MANIFEST_OUTPUT_REF")
		}
	}
}

// validatePermissions cross-references SDK method calls in the script against
// the permissions declared in the manifest. Warns when the script uses a command,
// package, service, path, or URL that isn't allowed by the manifest.
func validatePermissions(result *ValidationResult, script string, manifestData []byte) {
	if manifestData == nil {
		return
	}
	var manifest catalog.AppManifest
	if err := yaml.Unmarshal(manifestData, &manifest); err != nil {
		return
	}
	perms := manifest.Permissions

	lines := strings.Split(script, "\n")

	// Build lookup sets
	pkgSet := toSet(perms.Packages)
	svcSet := toSet(perms.Services)
	cmdSet := toSet(perms.Commands)
	userSet := toSet(perms.Users)
	pipSet := toSet(perms.Pip)

	// Compiled regexes for SDK method calls
	var (
		// self.pkg_install("pkg1", "pkg2") or self.apt_install("pkg")
		rePkgInstall = regexp.MustCompile(`self\.(?:pkg_install|apt_install)\(([^)]+)\)`)
		// self.pip_install("pkg1", "pkg2")
		rePipInstall = regexp.MustCompile(`self\.pip_install\(([^)]+)\)`)
		// self.enable_service("svc"), self.restart_service("svc"), self.create_service("svc", ...)
		reService = regexp.MustCompile(`self\.(?:enable_service|restart_service|create_service)\(\s*"([^"]+)"`)
		// self.run_command(["cmd", "arg1", ...]) — extract first element
		reRunCmd = regexp.MustCompile(`self\.run_command\(\s*\[\s*"([^"]+)"`)
		// self.create_user("user")
		reCreateUser = regexp.MustCompile(`self\.create_user\(\s*"([^"]+)"`)
		// user="xxx" keyword argument (used within multi-line create_service calls)
		reUserKwarg = regexp.MustCompile(`\buser\s*=\s*"([^"]+)"`)
		// self.write_config("path", ...), self.create_dir("path"), self.chown("path", ...)
		rePath = regexp.MustCompile(`self\.(?:write_config|create_dir|chown|write_env_file|deploy_provision_file)\(\s*"([^"]+)"`)
		// self.download("url", ...), self.add_apt_key("url", ...), self.wait_for_http("url")
		reURL = regexp.MustCompile(`self\.(?:download|add_apt_key|wait_for_http)\(\s*"([^"]+)"`)
		// self.add_apt_repo("deb ...", "filename")
		reAptRepo = regexp.MustCompile(`self\.add_apt_repo\(\s*"([^"]+)"`)
		// Extract string literals from arg list: "pkg1", "pkg2"
		reStringLit = regexp.MustCompile(`"([^"]+)"`)
	)

	// First pass: collect users created via self.create_user()
	createdUsers := make(map[string]bool)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if m := reCreateUser.FindStringSubmatch(line); m != nil {
			createdUsers[m[1]] = true
		}
	}

	// Track multi-line create_service calls to find user= kwarg
	inCreateService := false
	createServiceStartLine := 0
	parenDepth := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		lineNo := i + 1

		// --- Track create_service call spans ---
		if strings.Contains(line, "self.create_service(") {
			inCreateService = true
			createServiceStartLine = lineNo
			parenDepth = strings.Count(line, "(") - strings.Count(line, ")")
		} else if inCreateService {
			parenDepth += strings.Count(line, "(") - strings.Count(line, ")")
			if parenDepth <= 0 {
				inCreateService = false
			}
		}

		// --- Service user (user= kwarg inside create_service) ---
		if inCreateService || strings.Contains(line, "self.create_service(") {
			if m := reUserKwarg.FindStringSubmatch(line); m != nil {
				user := m[1]
				reportLine := lineNo
				if createServiceStartLine > 0 {
					reportLine = createServiceStartLine
				}
				if user != "root" && !createdUsers[user] && !userSet[user] {
					result.addWarning("provision/install.py", reportLine,
						fmt.Sprintf("Service user %q not created via create_user() and not in permissions.users", user),
						"SCRIPT_SERVICE_USER_MISSING")
				}
			}
		}

		// --- Packages ---
		if m := rePkgInstall.FindStringSubmatch(line); m != nil {
			for _, sm := range reStringLit.FindAllStringSubmatch(m[1], -1) {
				pkg := sm[1]
				if !matchesAny(pkg, pkgSet, perms.Packages) {
					result.addWarning("provision/install.py", lineNo,
						fmt.Sprintf("Package %q not in permissions.packages", pkg),
						"PERM_MISSING_PACKAGE")
				}
			}
		}

		// --- Pip packages ---
		if m := rePipInstall.FindStringSubmatch(line); m != nil {
			// Split args and skip keyword arguments (e.g. venv="/path")
			args := strings.Split(m[1], ",")
			for _, arg := range args {
				arg = strings.TrimSpace(arg)
				if strings.Contains(arg, "=") {
					continue // keyword arg like venv="..."
				}
				if sm := reStringLit.FindStringSubmatch(arg); sm != nil {
					pkg := sm[1]
					if !matchesAny(pkg, pipSet, perms.Pip) {
						result.addWarning("provision/install.py", lineNo,
							fmt.Sprintf("Pip package %q not in permissions.pip", pkg),
							"PERM_MISSING_PIP")
					}
				}
			}
		}

		// --- Services ---
		if m := reService.FindStringSubmatch(line); m != nil {
			svc := m[1]
			if !svcSet[svc] {
				result.addWarning("provision/install.py", lineNo,
					fmt.Sprintf("Service %q not in permissions.services", svc),
					"PERM_MISSING_SERVICE")
			}
		}

		// --- Commands (run_command first arg) ---
		if m := reRunCmd.FindStringSubmatch(line); m != nil {
			cmd := m[1]
			// Skip common shell builtins that don't need explicit permission
			if cmd != "sh" && cmd != "bash" && !matchesAny(cmd, cmdSet, perms.Commands) {
				result.addWarning("provision/install.py", lineNo,
					fmt.Sprintf("Command %q not in permissions.commands", cmd),
					"PERM_MISSING_COMMAND")
			}
		}

		// --- Users ---
		if m := reCreateUser.FindStringSubmatch(line); m != nil {
			user := m[1]
			if !userSet[user] {
				result.addWarning("provision/install.py", lineNo,
					fmt.Sprintf("User %q not in permissions.users", user),
					"PERM_MISSING_USER")
			}
		}

		// --- Paths ---
		if m := rePath.FindStringSubmatch(line); m != nil {
			path := m[1]
			if !pathAllowed(path, perms.Paths) {
				result.addWarning("provision/install.py", lineNo,
					fmt.Sprintf("Path %q not covered by permissions.paths", path),
					"PERM_MISSING_PATH")
			}
		}

		// --- URLs ---
		if m := reURL.FindStringSubmatch(line); m != nil {
			url := m[1]
			if !matchesAnyGlob(url, perms.URLs) {
				result.addWarning("provision/install.py", lineNo,
					fmt.Sprintf("URL %q not covered by permissions.urls", url),
					"PERM_MISSING_URL")
			}
		}

		// --- APT repos ---
		if m := reAptRepo.FindStringSubmatch(line); m != nil {
			repoLine := m[1]
			if !aptRepoAllowed(repoLine, perms.AptRepos) {
				result.addWarning("provision/install.py", lineNo,
					fmt.Sprintf("APT repo not covered by permissions.apt_repos"),
					"PERM_MISSING_APT_REPO")
			}
		}
	}
}

// toSet builds a lookup map from a string slice.
func toSet(items []string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, item := range items {
		m[item] = true
	}
	return m
}

// matchesAny checks exact set membership first, then falls back to fnmatch-style
// glob matching against the raw patterns (for wildcard entries like "lib*").
func matchesAny(value string, set map[string]bool, patterns []string) bool {
	if set[value] {
		return true
	}
	for _, p := range patterns {
		if matched, _ := filepath.Match(p, value); matched {
			return true
		}
	}
	return false
}

// matchesAnyGlob checks if a URL matches any allowed pattern using glob matching.
func matchesAnyGlob(url string, patterns []string) bool {
	for _, p := range patterns {
		if matched, _ := filepath.Match(p, url); matched {
			return true
		}
		// Also try prefix match for wildcard patterns like "https://example.com/*"
		prefix := strings.TrimSuffix(p, "*")
		if prefix != p && strings.HasPrefix(url, prefix) {
			return true
		}
	}
	return false
}

// pathAllowed checks if a path is covered by the allowed paths list.
// Matches if the path equals an allowed path, or starts with an allowed directory prefix.
// Implicitly allows /tmp and /opt/venv (matching SDK behavior).
func pathAllowed(path string, allowed []string) bool {
	// Implicit paths (always allowed by SDK)
	if strings.HasPrefix(path, "/tmp") || strings.HasPrefix(path, "/opt/venv") {
		return true
	}
	for _, a := range allowed {
		a = strings.TrimRight(a, "/")
		if path == a || strings.HasPrefix(path, a+"/") || strings.HasPrefix(path, a) {
			return true
		}
	}
	return false
}

// aptRepoAllowed checks if a repo line is covered by the allowed apt repos.
// Extracts the URL from the deb line and matches against allowed entries.
func aptRepoAllowed(repoLine string, allowed []string) bool {
	// Extract URL from deb line
	var repoURL string
	for _, token := range strings.Fields(repoLine) {
		if strings.HasPrefix(token, "http://") || strings.HasPrefix(token, "https://") {
			repoURL = strings.TrimRight(token, "/")
			break
		}
	}
	if repoURL == "" {
		return false
	}
	for _, a := range allowed {
		a = strings.TrimRight(a, "/")
		if repoURL == a || strings.HasPrefix(repoURL, a+"/") {
			return true
		}
		if matched, _ := filepath.Match(a, repoURL); matched {
			return true
		}
	}
	return false
}

func validateScript(result *ValidationResult, script string, manifestData []byte) {
	lines := strings.Split(script, "\n")

	// Check imports
	hasBaseAppImport := false
	hasRunImport := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "from appstore import") {
			if strings.Contains(trimmed, "BaseApp") {
				hasBaseAppImport = true
			}
			if strings.Contains(trimmed, "run") {
				hasRunImport = true
			}
		}
	}

	if !hasBaseAppImport {
		result.addError("provision/install.py", 0, "Script must import BaseApp from appstore", "SCRIPT_NO_BASEAPP")
		result.Valid = false
	}
	if !hasRunImport {
		result.addError("provision/install.py", 0, "Script must import run from appstore", "SCRIPT_NO_RUN")
		result.Valid = false
	}

	// Check for class extending BaseApp
	hasClass := false
	className := ""
	for i, line := range lines {
		if match := regexp.MustCompile(`class\s+(\w+)\s*\(\s*BaseApp\s*\)`).FindStringSubmatch(line); match != nil {
			hasClass = true
			className = match[1]
			_ = i
			break
		}
	}

	if !hasClass {
		result.addError("provision/install.py", 0, "Script must define a class extending BaseApp", "SCRIPT_NO_CLASS")
		result.Valid = false
	}

	// Check for install method
	hasInstall := false
	for _, line := range lines {
		if strings.Contains(line, "def install(self") {
			hasInstall = true
			break
		}
	}

	if !hasInstall {
		result.addError("provision/install.py", 0, "Script must define an install() method", "SCRIPT_NO_INSTALL")
		result.Valid = false
	}

	// Check for run() call
	hasRun := false
	if className != "" {
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == fmt.Sprintf("run(%s)", className) {
				hasRun = true
				break
			}
		}
	}

	if className != "" && !hasRun {
		result.addError("provision/install.py", 0,
			fmt.Sprintf("Script must call run(%s) at module level", className),
			"SCRIPT_NO_RUN_CALL")
		result.Valid = false
	}

	// Check for unsafe patterns
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.Contains(trimmed, "os.system(") {
			result.addWarning("provision/install.py", i+1,
				"Use self.run_command() instead of os.system()", "SCRIPT_OS_SYSTEM")
		}
		if strings.Contains(trimmed, "subprocess.call(") || strings.Contains(trimmed, "subprocess.run(") {
			result.addWarning("provision/install.py", i+1,
				"Use self.run_command() instead of subprocess directly", "SCRIPT_SUBPROCESS")
		}
	}

	// Check that input keys used in script match manifest inputs
	if manifestData != nil {
		var manifest struct {
			Inputs []struct{ Key string `yaml:"key"` } `yaml:"inputs"`
		}
		yaml.Unmarshal(manifestData, &manifest)
		manifestKeys := make(map[string]bool)
		for _, inp := range manifest.Inputs {
			manifestKeys[inp.Key] = true
		}

		for i, line := range lines {
			for _, match := range regexp.MustCompile(`self\.inputs\.\w+\("(\w+)"`).FindAllStringSubmatch(line, -1) {
				key := match[1]
				if !manifestKeys[key] {
					result.addWarning("provision/install.py", i+1,
						fmt.Sprintf("Script reads input %q which is not defined in manifest", key),
						"SCRIPT_UNKNOWN_INPUT")
				}
			}
		}
	}

	// Cross-reference SDK calls against manifest permissions
	validatePermissions(result, script, manifestData)

	// Run pyflakes for Python static analysis (undefined names, etc.)
	validatePyflakes(result, script)
}

func buildChecklist(result *ValidationResult, appDir string, manifestData []byte) {
	// Parse manifest if we can
	var manifest catalog.AppManifest
	manifestValid := false
	if manifestData != nil {
		if err := yaml.Unmarshal(manifestData, &manifest); err == nil {
			if err := manifest.Validate(); err == nil {
				manifestValid = true
			}
		}
	}

	// Script structure
	scriptOK := true
	for _, e := range result.Errors {
		if strings.HasPrefix(e.Code, "SCRIPT_") {
			scriptOK = false
			break
		}
	}

	iconCustom := isCustomIcon(filepath.Join(appDir, "icon.png"))
	_, readmeExists := os.Stat(filepath.Join(appDir, "README.md"))

	result.Checklist = []ChecklistItem{
		{Label: "Manifest validates", Passed: manifestValid},
		{Label: "Script has correct structure", Passed: scriptOK},
		{Label: "Icon present", Passed: iconCustom},
		{Label: "README present", Passed: readmeExists == nil},
		{Label: "Maintainers listed", Passed: len(manifest.Maintainers) > 0},
		{Label: "Version set", Passed: manifest.Version != "" && manifest.Version != "0.0.0"},
	}
}

func extractTemplateVars(s string) []string {
	re := regexp.MustCompile(`\{\{(\w+)\}\}`)
	matches := re.FindAllStringSubmatch(s, -1)
	var vars []string
	for _, m := range matches {
		vars = append(vars, m[1])
	}
	return vars
}

// isCustomIcon returns true if icon.png exists and is NOT the default placeholder.
func isCustomIcon(iconPath string) bool {
	data, err := os.ReadFile(iconPath)
	if err != nil {
		return false
	}
	// Compare against the embedded default icon
	if len(data) == len(defaultIconPNG) {
		match := true
		for i := range data {
			if data[i] != defaultIconPNG[i] {
				match = false
				break
			}
		}
		if match {
			return false // it's the default placeholder
		}
	}
	return true
}

func (r *ValidationResult) addError(file string, line int, message, code string) {
	r.Errors = append(r.Errors, ValidationMsg{File: file, Line: line, Message: message, Code: code})
}

func (r *ValidationResult) addWarning(file string, line int, message, code string) {
	r.Warnings = append(r.Warnings, ValidationMsg{File: file, Line: line, Message: message, Code: code})
}

// validatePyflakes runs pyflakes on the script to catch Python errors like
// undefined names, unused imports, and syntax errors.
func validatePyflakes(result *ValidationResult, script string) {
	// Write script to a temp file
	tmpFile, err := os.CreateTemp("", "validate-*.py")
	if err != nil {
		return
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(script); err != nil {
		tmpFile.Close()
		return
	}
	tmpFile.Close()

	// Run pyflakes with a timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "python3", "-m", "pyflakes", tmpFile.Name())
	output, _ := cmd.CombinedOutput()
	// pyflakes exits 1 when issues found — that's expected

	if len(output) == 0 {
		return
	}

	// Parse output: /tmp/validate-123.py:10:5: undefined name 'broker'
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}

		// Strip the temp filename prefix to find the line:col:message part
		// Format: <filename>:<line>:<col>: <message>
		prefix := tmpFile.Name() + ":"
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		rest := line[len(prefix):]

		// Parse line number
		parts := strings.SplitN(rest, ":", 3)
		if len(parts) < 3 {
			continue
		}
		lineNo, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		msg := strings.TrimSpace(parts[2])

		// Classify: "undefined name" is an error, everything else a warning
		if strings.HasPrefix(msg, "undefined name") {
			result.addError("provision/install.py", lineNo, msg, "SCRIPT_UNDEFINED_NAME")
			result.Valid = false
		} else {
			result.addWarning("provision/install.py", lineNo, msg, "SCRIPT_LINT")
		}
	}
}
