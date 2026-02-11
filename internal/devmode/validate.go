package devmode

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

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
			if ref != "IP" && !inputKeys[ref] {
				result.addWarning("app.yml", 0,
					fmt.Sprintf("Output %q references unknown input key {{%s}}", out.Key, ref),
					"MANIFEST_OUTPUT_REF")
			}
		}
	}
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
