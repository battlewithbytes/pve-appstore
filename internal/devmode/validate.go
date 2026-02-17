package devmode

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/battlewithbytes/pve-appstore/internal/catalog"
	sdk "github.com/battlewithbytes/pve-appstore/sdk"
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

// --- AST analysis types ---

type astAnalysis struct {
	Imports          []string        `json:"imports"`
	ClassName        string          `json:"class_name"`
	HasInstallMethod bool            `json:"has_install_method"`
	HasRunCall       bool            `json:"has_run_call"`
	InputKeys        []inputKeyRef   `json:"input_keys"`
	MethodCalls      []methodCall    `json:"method_calls"`
	UnsafePatterns   []unsafePattern `json:"unsafe_patterns"`
	DefinedMethods   []string        `json:"defined_methods"`
	Error            string          `json:"error"`
}

type inputKeyRef struct {
	Key  string `json:"key"`
	Line int    `json:"line"`
	Type string `json:"type"`
}

type methodCall struct {
	Method string         `json:"method"`
	Line   int            `json:"line"`
	Args   []any          `json:"args"`
	Kwargs map[string]any `json:"kwargs"`
}

type unsafePattern struct {
	Line    int    `json:"line"`
	Pattern string `json:"pattern"`
}

// stringArg returns the string value at position pos in call.Args.
// Returns "", false for missing, non-string, or "<dynamic>" values.
func stringArg(call methodCall, pos int) (string, bool) {
	if pos >= len(call.Args) {
		return "", false
	}
	s, ok := call.Args[pos].(string)
	if !ok || s == "<dynamic>" {
		return "", false
	}
	return s, true
}

// stringKwarg returns the string value for key in call.Kwargs.
func stringKwarg(call methodCall, key string) (string, bool) {
	v, ok := call.Kwargs[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	if !ok || s == "<dynamic>" {
		return "", false
	}
	return s, true
}

// allStringArgs returns all string positional args, skipping <dynamic> values.
func allStringArgs(call methodCall) []string {
	var result []string
	for _, a := range call.Args {
		s, ok := a.(string)
		if ok && s != "<dynamic>" {
			result = append(result, s)
		}
	}
	return result
}

// firstListElement returns the first string element if arg at pos is a list.
func firstListElement(call methodCall, pos int) (string, bool) {
	if pos >= len(call.Args) {
		return "", false
	}
	list, ok := call.Args[pos].([]any)
	if !ok || len(list) == 0 {
		return "", false
	}
	s, ok := list[0].(string)
	if !ok || s == "<dynamic>" {
		return "", false
	}
	return s, true
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

	// Check that manifest ID matches directory name
	dirName := filepath.Base(appDir)
	if manifest.ID != "" && manifest.ID != dirName {
		result.addWarning("app.yml", 0,
			fmt.Sprintf("Manifest id %q does not match directory name %q — exports and APIs use the directory name. Use Rename App to fix.", manifest.ID, dirName),
			"MANIFEST_ID_MISMATCH")
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

	// Note require_static_ip flag
	if manifest.LXC.Defaults.RequireStaticIP {
		result.addWarning("app.yml", 0, "require_static_ip is set — users must provide a static IP at install time", "MANIFEST_REQUIRE_STATIC_IP")
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

// runASTAnalyzer runs the Python AST analyzer on the given script and returns
// structured analysis data.
func runASTAnalyzer(script string) (*astAnalysis, error) {
	// Write script to temp file
	scriptFile, err := os.CreateTemp("", "validate-script-*.py")
	if err != nil {
		return nil, fmt.Errorf("creating temp script: %w", err)
	}
	defer os.Remove(scriptFile.Name())

	if _, err := scriptFile.WriteString(script); err != nil {
		scriptFile.Close()
		return nil, fmt.Errorf("writing temp script: %w", err)
	}
	scriptFile.Close()

	// Extract analyze_script.py from embedded FS
	analyzerSrc, err := fs.ReadFile(sdk.PythonFS, "python/appstore/analyze_script.py")
	if err != nil {
		return nil, fmt.Errorf("reading embedded analyzer: %w", err)
	}

	analyzerFile, err := os.CreateTemp("", "analyze-script-*.py")
	if err != nil {
		return nil, fmt.Errorf("creating temp analyzer: %w", err)
	}
	defer os.Remove(analyzerFile.Name())

	if _, err := analyzerFile.Write(analyzerSrc); err != nil {
		analyzerFile.Close()
		return nil, fmt.Errorf("writing temp analyzer: %w", err)
	}
	analyzerFile.Close()

	// Run analyzer with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "python3", analyzerFile.Name(), scriptFile.Name())
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("running analyzer: %w", err)
	}

	var analysis astAnalysis
	if err := json.Unmarshal(output, &analysis); err != nil {
		return nil, fmt.Errorf("parsing analyzer output: %w", err)
	}
	return &analysis, nil
}

func validateScript(result *ValidationResult, script string, manifestData []byte) {
	// Run AST analysis
	analysis, err := runASTAnalyzer(script)
	if err != nil {
		// Fall back: add a warning but don't block validation
		result.addWarning("provision/install.py", 0,
			fmt.Sprintf("AST analysis failed: %v", err), "SCRIPT_ANALYSIS_FAILED")
		// Still run pyflakes
		validatePyflakes(result, script)
		return
	}

	// If the analyzer reported a syntax error, that's a hard error
	if analysis.Error != "" {
		result.addError("provision/install.py", 0, analysis.Error, "SCRIPT_SYNTAX_ERROR")
		result.Valid = false
		return
	}

	// --- Structural checks from AST ---

	// Check imports
	hasBaseApp := false
	hasRun := false
	for _, imp := range analysis.Imports {
		if imp == "BaseApp" {
			hasBaseApp = true
		}
		if imp == "run" {
			hasRun = true
		}
	}
	if !hasBaseApp {
		result.addError("provision/install.py", 0, "Script must import BaseApp from appstore", "SCRIPT_NO_BASEAPP")
		result.Valid = false
	}
	if !hasRun {
		result.addError("provision/install.py", 0, "Script must import run from appstore", "SCRIPT_NO_RUN")
		result.Valid = false
	}

	// Check class
	if analysis.ClassName == "" {
		result.addError("provision/install.py", 0, "Script must define a class extending BaseApp", "SCRIPT_NO_CLASS")
		result.Valid = false
	}

	// Check install method
	if !analysis.HasInstallMethod {
		result.addError("provision/install.py", 0, "Script must define an install() method", "SCRIPT_NO_INSTALL")
		result.Valid = false
	}

	// Check run() call
	if analysis.ClassName != "" && !analysis.HasRunCall {
		result.addError("provision/install.py", 0,
			fmt.Sprintf("Script must call run(%s) at module level", analysis.ClassName),
			"SCRIPT_NO_RUN_CALL")
		result.Valid = false
	}

	// --- Unsafe patterns ---
	for _, p := range analysis.UnsafePatterns {
		if p.Pattern == "os.system" {
			result.addWarning("provision/install.py", p.Line,
				"Use self.run_command() or self.run_shell() instead of os.system()", "SCRIPT_OS_SYSTEM")
		} else {
			result.addWarning("provision/install.py", p.Line,
				"Use self.run_command() or self.run_shell() instead of subprocess directly", "SCRIPT_SUBPROCESS")
		}
	}

	// --- Input key cross-reference ---
	if manifestData != nil {
		var manifest struct {
			Inputs []struct{ Key string `yaml:"key"` } `yaml:"inputs"`
		}
		yaml.Unmarshal(manifestData, &manifest)
		manifestKeys := make(map[string]bool)
		for _, inp := range manifest.Inputs {
			manifestKeys[inp.Key] = true
		}
		for _, ik := range analysis.InputKeys {
			if !manifestKeys[ik.Key] {
				result.addWarning("provision/install.py", ik.Line,
					fmt.Sprintf("Script uses self.inputs[\"%s\"] but no input with key %q exists in app.yml — add it under inputs: in the manifest", ik.Key, ik.Key),
					"SCRIPT_UNKNOWN_INPUT")
			}
		}
	}

	// --- Unknown self.<method>() calls ---
	validateUnknownMethods(result, analysis)

	// Cross-reference SDK calls against manifest permissions
	validatePermissionsFromAST(result, analysis, manifestData)

	// Run pyflakes for Python static analysis (undefined names, etc.)
	validatePyflakes(result, script)
}

// validatePermissionsFromAST cross-references AST-extracted method calls against
// the permissions declared in the manifest.
func validatePermissionsFromAST(result *ValidationResult, analysis *astAnalysis, manifestData []byte) {
	if manifestData == nil {
		return
	}
	var manifest catalog.AppManifest
	if err := yaml.Unmarshal(manifestData, &manifest); err != nil {
		return
	}
	perms := manifest.Permissions

	// Build lookup sets
	pkgSet := toSet(perms.Packages)
	svcSet := toSet(perms.Services)
	cmdSet := toSet(perms.Commands)
	userSet := toSet(perms.Users)
	pipBasePatterns := make([]string, len(perms.Pip))
	for i, p := range perms.Pip {
		pipBasePatterns[i] = pipBaseName(p)
	}
	pipSet := toSet(pipBasePatterns)

	// Collect users created via create_user for service user cross-ref
	createdUsers := make(map[string]bool)
	for _, call := range analysis.MethodCalls {
		if call.Method == "create_user" {
			if user, ok := stringArg(call, 0); ok {
				createdUsers[user] = true
			}
		}
	}

	for _, call := range analysis.MethodCalls {
		switch call.Method {
		case "pkg_install", "apt_install":
			for _, pkg := range allStringArgs(call) {
				if !matchesAny(pkg, pkgSet, perms.Packages) {
					result.addWarning("provision/install.py", call.Line,
						fmt.Sprintf("Package %q not in permissions.packages", pkg),
						"PERM_MISSING_PACKAGE")
				}
			}

		case "pip_install":
			// Positional args only (kwargs like venv= are not packages)
			// Normalize: strip version specifiers so "josepy<2" matches "josepy"
			for _, pkg := range allStringArgs(call) {
				base := pipBaseName(pkg)
				if !matchesAny(base, pipSet, pipBasePatterns) {
					result.addWarning("provision/install.py", call.Line,
						fmt.Sprintf("Pip package %q not in permissions.pip", base),
						"PERM_MISSING_PIP")
				}
			}

		case "create_service":
			// arg[0] → service name
			if svc, ok := stringArg(call, 0); ok {
				if !svcSet[svc] {
					result.addWarning("provision/install.py", call.Line,
						fmt.Sprintf("Service %q not in permissions.services", svc),
						"PERM_MISSING_SERVICE")
				}
			}
			// kwarg user → check created or in perms
			if user, ok := stringKwarg(call, "user"); ok {
				if user != "root" && !createdUsers[user] && !userSet[user] {
					result.addWarning("provision/install.py", call.Line,
						fmt.Sprintf("Service user %q not created via create_user() and not in permissions.users", user),
						"SCRIPT_SERVICE_USER_MISSING")
				}
			}

		case "enable_service", "restart_service":
			if svc, ok := stringArg(call, 0); ok {
				if !svcSet[svc] {
					result.addWarning("provision/install.py", call.Line,
						fmt.Sprintf("Service %q not in permissions.services", svc),
						"PERM_MISSING_SERVICE")
				}
			}

		case "create_user":
			if user, ok := stringArg(call, 0); ok {
				if !userSet[user] {
					result.addWarning("provision/install.py", call.Line,
						fmt.Sprintf("User %q not in permissions.users", user),
						"PERM_MISSING_USER")
				}
			}

		case "run_command":
			// arg[0] can be a list ["git", "clone", ...] or a string "git clone ..."
			// (SDK accepts both via shlex.split)
			if cmd, ok := firstListElement(call, 0); ok {
				if cmd != "sh" && cmd != "bash" && !matchesAny(cmd, cmdSet, perms.Commands) {
					result.addWarning("provision/install.py", call.Line,
						fmt.Sprintf("Command %q not in permissions.commands", cmd),
						"PERM_MISSING_COMMAND")
				}
			} else if cmdStr, ok := stringArg(call, 0); ok {
				// String form: extract first word as the command binary
				cmd := cmdStr
				if idx := strings.Index(cmdStr, " "); idx > 0 {
					cmd = cmdStr[:idx]
				}
				if cmd != "sh" && cmd != "bash" && !matchesAny(cmd, cmdSet, perms.Commands) {
					result.addWarning("provision/install.py", call.Line,
						fmt.Sprintf("Command %q not in permissions.commands", cmd),
						"PERM_MISSING_COMMAND")
				}
			}

		case "run_shell":
			// arg[0] is a shell command string; extract first word as the command binary
			if cmdStr, ok := stringArg(call, 0); ok {
				cmd := cmdStr
				if idx := strings.Index(cmdStr, " "); idx > 0 {
					cmd = cmdStr[:idx]
				}
				if cmd != "sh" && cmd != "bash" && !matchesAny(cmd, cmdSet, perms.Commands) {
					result.addWarning("provision/install.py", call.Line,
						fmt.Sprintf("Command %q not in permissions.commands", cmd),
						"PERM_MISSING_COMMAND")
				}
			}

		case "write_config", "create_dir", "chown", "write_env_file":
			if path, ok := stringArg(call, 0); ok {
				if !pathAllowed(path, perms.Paths) {
					result.addWarning("provision/install.py", call.Line,
						fmt.Sprintf("Path %q not covered by permissions.paths", path),
						"PERM_MISSING_PATH")
				}
			}

		case "deploy_provision_file":
			// arg[1] is the destination path
			if path, ok := stringArg(call, 1); ok {
				if !pathAllowed(path, perms.Paths) {
					result.addWarning("provision/install.py", call.Line,
						fmt.Sprintf("Path %q not covered by permissions.paths", path),
						"PERM_MISSING_PATH")
				}
			}

		case "download":
			// arg[0] → URL, arg[1] → path
			if url, ok := stringArg(call, 0); ok {
				if !matchesAnyGlob(url, perms.URLs) {
					result.addWarning("provision/install.py", call.Line,
						fmt.Sprintf("URL %q not covered by permissions.urls", url),
						"PERM_MISSING_URL")
				}
			}
			if path, ok := stringArg(call, 1); ok {
				if !pathAllowed(path, perms.Paths) {
					result.addWarning("provision/install.py", call.Line,
						fmt.Sprintf("Path %q not covered by permissions.paths", path),
						"PERM_MISSING_PATH")
				}
			}

		case "add_apt_key":
			// arg[0] → URL, arg[1] → path
			if url, ok := stringArg(call, 0); ok {
				if !matchesAnyGlob(url, perms.URLs) {
					result.addWarning("provision/install.py", call.Line,
						fmt.Sprintf("URL %q not covered by permissions.urls", url),
						"PERM_MISSING_URL")
				}
			}
			if path, ok := stringArg(call, 1); ok {
				if !pathAllowed(path, perms.Paths) {
					result.addWarning("provision/install.py", call.Line,
						fmt.Sprintf("Path %q not covered by permissions.paths", path),
						"PERM_MISSING_PATH")
				}
			}

		case "wait_for_http":
			if url, ok := stringArg(call, 0); ok {
				if !matchesAnyGlob(url, perms.URLs) {
					result.addWarning("provision/install.py", call.Line,
						fmt.Sprintf("URL %q not covered by permissions.urls", url),
						"PERM_MISSING_URL")
				}
			}

		case "run_installer_script":
			if url, ok := stringArg(call, 0); ok {
				if !matchesAnyGlob(url, perms.URLs) {
					result.addWarning("provision/install.py", call.Line,
						fmt.Sprintf("URL %q not covered by permissions.urls", url),
						"PERM_MISSING_URL")
				}
			}

		case "add_apt_repo":
			if repoLine, ok := stringArg(call, 0); ok {
				if !aptRepoAllowed(repoLine, perms.AptRepos) {
					repoURL := extractRepoURL(repoLine)
					if repoURL == "" {
						repoURL = repoLine
					}
					result.addWarning("provision/install.py", call.Line,
						fmt.Sprintf("APT repo URL %q not covered by permissions.apt_repos", repoURL),
						"PERM_MISSING_APT_REPO")
				}
			}

		case "add_apt_repository":
			// arg[0] → URL (also apt_repos), kwarg key_url → URL
			if url, ok := stringArg(call, 0); ok {
				if !matchesAnyGlob(url, perms.URLs) {
					result.addWarning("provision/install.py", call.Line,
						fmt.Sprintf("URL %q not covered by permissions.urls", url),
						"PERM_MISSING_URL")
				}
				if !aptRepoAllowed(url, perms.AptRepos) {
					repoURL := extractRepoURL(url)
					if repoURL == "" {
						repoURL = url
					}
					result.addWarning("provision/install.py", call.Line,
						fmt.Sprintf("APT repo URL %q not covered by permissions.apt_repos", repoURL),
						"PERM_MISSING_APT_REPO")
				}
			}
			if keyURL, ok := stringKwarg(call, "key_url"); ok {
				if !matchesAnyGlob(keyURL, perms.URLs) {
					result.addWarning("provision/install.py", call.Line,
						fmt.Sprintf("URL %q not covered by permissions.urls", keyURL),
						"PERM_MISSING_URL")
				}
			}

		case "pull_oci_binary":
			// arg[1] → destination path
			if path, ok := stringArg(call, 1); ok {
				if !pathAllowed(path, perms.Paths) {
					result.addWarning("provision/install.py", call.Line,
						fmt.Sprintf("Path %q not covered by permissions.paths", path),
						"PERM_MISSING_PATH")
				}
			}
		}
	}
}

// knownSDKMethods is extracted from the embedded SDK's base.py at init time.
// It contains all public method names defined on AppBase (i.e. `def method(self`
// where method does not start with '_').
var knownSDKMethods map[string]bool

func init() {
	knownSDKMethods = extractSDKMethods()
}

// extractSDKMethods parses the embedded base.py to find all public instance methods.
func extractSDKMethods() map[string]bool {
	data, err := sdk.PythonFS.ReadFile("python/appstore/base.py")
	if err != nil {
		return map[string]bool{}
	}
	methods := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "def ") {
			continue
		}
		// Extract method name from "def method_name(self..."
		rest := trimmed[4:]
		idx := strings.Index(rest, "(")
		if idx <= 0 {
			continue
		}
		name := rest[:idx]
		// Skip private/dunder methods
		if strings.HasPrefix(name, "_") {
			continue
		}
		// Verify it takes self as first parameter
		params := rest[idx+1:]
		if strings.HasPrefix(params, "self") {
			methods[name] = true
		}
	}
	return methods
}

// validateUnknownMethods flags any self.<method>() call that doesn't match
// a known SDK method or a method defined in the user's own class.
func validateUnknownMethods(result *ValidationResult, analysis *astAnalysis) {
	userMethods := make(map[string]bool, len(analysis.DefinedMethods))
	for _, m := range analysis.DefinedMethods {
		userMethods[m] = true
	}
	for _, call := range analysis.MethodCalls {
		if !knownSDKMethods[call.Method] && !userMethods[call.Method] {
			result.addWarning("provision/install.py", call.Line,
				fmt.Sprintf("Unknown SDK method self.%s() — see SDK reference for valid methods", call.Method),
				"SCRIPT_UNKNOWN_METHOD")
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

// pipBaseName strips version specifiers and extras from a pip package string.
// e.g. "josepy<2" → "josepy", "homeassistant[all]>=2024.1" → "homeassistant"
func pipBaseName(pkg string) string {
	for i, ch := range pkg {
		if ch == '[' || ch == '<' || ch == '>' || ch == '=' || ch == '!' || ch == '~' || ch == ';' || ch == '@' {
			return strings.TrimSpace(pkg[:i])
		}
	}
	return pkg
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
// extractRepoURL pulls the URL from a deb line or bare URL string.
// "deb [signed-by=...] https://example.com/repo stable main" → "https://example.com/repo"
// "https://example.com/repo" → "https://example.com/repo"
func extractRepoURL(s string) string {
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		// Bare URL — take first token only (handles "https://x.com/repo public main" too)
		return strings.TrimRight(strings.Fields(s)[0], "/")
	}
	for _, token := range strings.Fields(s) {
		if strings.HasPrefix(token, "http://") || strings.HasPrefix(token, "https://") {
			return strings.TrimRight(token, "/")
		}
	}
	return ""
}

func aptRepoAllowed(repoLine string, allowed []string) bool {
	repoURL := extractRepoURL(repoLine)
	if repoURL == "" {
		return false
	}
	for _, a := range allowed {
		// Extract URL from allowed entry too (may be a full deb line or bare URL)
		allowedURL := extractRepoURL(a)
		if allowedURL == "" {
			continue
		}
		if repoURL == allowedURL || strings.HasPrefix(repoURL, allowedURL+"/") {
			return true
		}
		if matched, _ := filepath.Match(allowedURL, repoURL); matched {
			return true
		}
	}
	return false
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
