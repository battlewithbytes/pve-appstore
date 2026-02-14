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

// ValidateStack runs all checks on a dev stack directory.
func ValidateStack(stackDir string) *ValidationResult {
	result := &ValidationResult{
		Valid:    true,
		Errors:   []ValidationMsg{},
		Warnings: []ValidationMsg{},
	}

	manifestPath := filepath.Join(stackDir, "stack.yml")

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		result.addError("stack.yml", 0, "Stack manifest file not found", "STACK_MANIFEST_MISSING")
		result.Valid = false
		result.Checklist = buildStackChecklist(result, stackDir, nil)
		return result
	}

	var sm catalog.StackManifest
	if err := yaml.Unmarshal(data, &sm); err != nil {
		result.addError("stack.yml", 0, fmt.Sprintf("YAML parse error: %v", err), "STACK_MANIFEST_PARSE_ERROR")
		result.Valid = false
		result.Checklist = buildStackChecklist(result, stackDir, nil)
		return result
	}

	if err := catalog.ValidateStackManifest(&sm); err != nil {
		result.addError("stack.yml", 0, err.Error(), "STACK_MANIFEST_VALIDATION")
		result.Valid = false
	}

	// Additional dev mode checks
	if sm.Description == "" || strings.HasPrefix(sm.Description, "TODO") {
		result.addWarning("stack.yml", 0, "Description is empty or contains TODO", "STACK_TODO_DESC")
	}

	if sm.Version != "" && !regexp.MustCompile(`^\d+\.\d+\.\d+`).MatchString(sm.Version) {
		result.addWarning("stack.yml", 0, "Version should follow semver format (e.g. 1.0.0)", "STACK_VERSION_FORMAT")
	}

	// Check for duplicate app IDs
	seen := make(map[string]bool)
	for _, app := range sm.Apps {
		if seen[app.AppID] {
			result.addWarning("stack.yml", 0,
				fmt.Sprintf("Duplicate app_id %q in stack", app.AppID),
				"STACK_DUPLICATE_APP")
		}
		seen[app.AppID] = true
	}

	// Check icon
	iconPath := filepath.Join(stackDir, "icon.png")
	if !isCustomIcon(iconPath) {
		result.addWarning("stack.yml", 0, "No custom icon.png found", "STACK_NO_ICON")
	}

	// Check readme
	if _, err := os.Stat(filepath.Join(stackDir, "README.md")); os.IsNotExist(err) {
		result.addWarning("stack.yml", 0, "No README.md file found (recommended)", "STACK_NO_README")
	}

	result.Checklist = buildStackChecklist(result, stackDir, &sm)

	return result
}

func buildStackChecklist(result *ValidationResult, stackDir string, sm *catalog.StackManifest) []ChecklistItem {
	manifestValid := true
	for _, e := range result.Errors {
		if strings.HasPrefix(e.Code, "STACK_MANIFEST") {
			manifestValid = false
			break
		}
	}

	iconCustom := isCustomIcon(filepath.Join(stackDir, "icon.png"))
	_, readmeExists := os.Stat(filepath.Join(stackDir, "README.md"))

	hasApps := false
	hasVersion := false
	if sm != nil {
		hasApps = len(sm.Apps) > 0
		hasVersion = sm.Version != "" && sm.Version != "0.0.0"
	}

	return []ChecklistItem{
		{Label: "Manifest validates", Passed: manifestValid},
		{Label: "At least one app", Passed: hasApps},
		{Label: "Icon present", Passed: iconCustom},
		{Label: "README present", Passed: readmeExists == nil},
		{Label: "Version set", Passed: hasVersion},
	}
}
