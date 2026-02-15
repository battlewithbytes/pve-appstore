package engine

import (
	"testing"

	"github.com/battlewithbytes/pve-appstore/internal/catalog"
)

// --- validateInputs ---

func TestValidateInputsRequiredMissing(t *testing.T) {
	manifest := &catalog.AppManifest{
		Inputs: []catalog.InputSpec{
			{Key: "password", Required: true, Type: "string"},
		},
	}
	err := validateInputs(manifest, map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing required input")
	}
}

func TestValidateInputsRequiredEmpty(t *testing.T) {
	manifest := &catalog.AppManifest{
		Inputs: []catalog.InputSpec{
			{Key: "password", Required: true, Type: "string"},
		},
	}
	err := validateInputs(manifest, map[string]string{"password": ""})
	if err == nil {
		t.Fatal("expected error for empty required input")
	}
}

func TestValidateInputsOptionalMissing(t *testing.T) {
	manifest := &catalog.AppManifest{
		Inputs: []catalog.InputSpec{
			{Key: "port", Required: false, Type: "number"},
		},
	}
	if err := validateInputs(manifest, map[string]string{}); err != nil {
		t.Fatalf("validateInputs = %v, want nil", err)
	}
}

func TestValidateInputsNumberMinMax(t *testing.T) {
	min, max := 1.0, 65535.0
	manifest := &catalog.AppManifest{
		Inputs: []catalog.InputSpec{
			{Key: "port", Type: "number", Validation: &catalog.InputValidation{Min: &min, Max: &max}},
		},
	}

	// Valid
	if err := validateInputs(manifest, map[string]string{"port": "8080"}); err != nil {
		t.Errorf("valid port: %v", err)
	}

	// Below min
	if err := validateInputs(manifest, map[string]string{"port": "0"}); err == nil {
		t.Error("expected error for port below min")
	}

	// Above max
	if err := validateInputs(manifest, map[string]string{"port": "99999"}); err == nil {
		t.Error("expected error for port above max")
	}

	// Not a number
	if err := validateInputs(manifest, map[string]string{"port": "abc"}); err == nil {
		t.Error("expected error for non-number")
	}
}

func TestValidateInputsStringMinMaxLen(t *testing.T) {
	minLen, maxLen := 3, 10
	manifest := &catalog.AppManifest{
		Inputs: []catalog.InputSpec{
			{Key: "name", Type: "string", Validation: &catalog.InputValidation{MinLength: &minLen, MaxLength: &maxLen}},
		},
	}

	// Valid
	if err := validateInputs(manifest, map[string]string{"name": "hello"}); err != nil {
		t.Errorf("valid name: %v", err)
	}

	// Too short
	if err := validateInputs(manifest, map[string]string{"name": "ab"}); err == nil {
		t.Error("expected error for too short")
	}

	// Too long
	if err := validateInputs(manifest, map[string]string{"name": "verylongname"}); err == nil {
		t.Error("expected error for too long")
	}
}

func TestValidateInputsRegex(t *testing.T) {
	manifest := &catalog.AppManifest{
		Inputs: []catalog.InputSpec{
			{Key: "email", Type: "string", Validation: &catalog.InputValidation{Regex: `^[^@]+@[^@]+$`}},
		},
	}

	if err := validateInputs(manifest, map[string]string{"email": "user@example.com"}); err != nil {
		t.Errorf("valid email: %v", err)
	}

	if err := validateInputs(manifest, map[string]string{"email": "notanemail"}); err == nil {
		t.Error("expected error for invalid email")
	}
}

func TestValidateInputsEnum(t *testing.T) {
	manifest := &catalog.AppManifest{
		Inputs: []catalog.InputSpec{
			{Key: "size", Type: "string", Validation: &catalog.InputValidation{Enum: []string{"small", "medium", "large"}}},
		},
	}

	if err := validateInputs(manifest, map[string]string{"size": "medium"}); err != nil {
		t.Errorf("valid enum: %v", err)
	}

	if err := validateInputs(manifest, map[string]string{"size": "xlarge"}); err == nil {
		t.Error("expected error for invalid enum value")
	}
}

func TestValidateInputsSecretType(t *testing.T) {
	minLen := 8
	manifest := &catalog.AppManifest{
		Inputs: []catalog.InputSpec{
			{Key: "api_key", Type: "secret", Validation: &catalog.InputValidation{MinLength: &minLen}},
		},
	}

	if err := validateInputs(manifest, map[string]string{"api_key": "longpassword"}); err != nil {
		t.Errorf("valid secret: %v", err)
	}

	if err := validateInputs(manifest, map[string]string{"api_key": "short"}); err == nil {
		t.Error("expected error for short secret")
	}
}

func TestValidateInputsNoValidation(t *testing.T) {
	manifest := &catalog.AppManifest{
		Inputs: []catalog.InputSpec{
			{Key: "notes", Type: "string"},
		},
	}
	if err := validateInputs(manifest, map[string]string{"notes": "anything goes"}); err != nil {
		t.Errorf("no validation: %v", err)
	}
}

// --- mergeEnvVars ---

func TestMergeEnvVarsBothEmpty(t *testing.T) {
	result := mergeEnvVars(nil, nil)
	if len(result) != 0 {
		t.Errorf("mergeEnvVars(nil, nil) = %v, want empty", result)
	}
}

func TestMergeEnvVarsJobOverridesManifest(t *testing.T) {
	manifest := map[string]string{"KEY": "manifest", "ONLY_M": "m"}
	job := map[string]string{"KEY": "job", "ONLY_J": "j"}
	result := mergeEnvVars(manifest, job)

	if result["KEY"] != "job" {
		t.Errorf("KEY = %q, want %q (job should override)", result["KEY"], "job")
	}
	if result["ONLY_M"] != "m" {
		t.Errorf("ONLY_M = %q, want %q", result["ONLY_M"], "m")
	}
	if result["ONLY_J"] != "j" {
		t.Errorf("ONLY_J = %q, want %q", result["ONLY_J"], "j")
	}
}

func TestMergeEnvVarsNoOverlap(t *testing.T) {
	manifest := map[string]string{"A": "1"}
	job := map[string]string{"B": "2"}
	result := mergeEnvVars(manifest, job)
	if len(result) != 2 || result["A"] != "1" || result["B"] != "2" {
		t.Errorf("mergeEnvVars = %v, want {A:1, B:2}", result)
	}
}

// --- parseAppLog ---

func TestParseAppLogFound(t *testing.T) {
	entry, ok := parseAppLog(`{"level":"error","msg":"something failed"}`)
	if !ok {
		t.Fatal("parseAppLog returned !ok")
	}
	if entry.Level != "error" {
		t.Errorf("level = %q, want %q", entry.Level, "error")
	}
	if entry.Msg != "something failed" {
		t.Errorf("msg = %q, want %q", entry.Msg, "something failed")
	}
}

func TestParseAppLogMissing(t *testing.T) {
	entry, ok := parseAppLog(`{"level":"info"}`)
	if !ok {
		t.Fatal("parseAppLog returned !ok")
	}
	if entry.Msg != "" {
		t.Errorf("msg = %q, want empty", entry.Msg)
	}
}

func TestParseAppLogInvalid(t *testing.T) {
	_, ok := parseAppLog("")
	if ok {
		t.Error("parseAppLog should return !ok for empty string")
	}
}

func TestParseAppLogUnescapesUnicode(t *testing.T) {
	entry, ok := parseAppLog(`{"level":"info","msg":"color \u001b[37mwhite\u001b[0m done"}`)
	if !ok {
		t.Fatal("parseAppLog returned !ok")
	}
	// \u001b should be unescaped to the actual ESC byte (0x1B)
	want := "color \x1b[37mwhite\x1b[0m done"
	if entry.Msg != want {
		t.Errorf("msg = %q, want %q", entry.Msg, want)
	}
}

func TestParseAppLogEscapedQuotes(t *testing.T) {
	entry, ok := parseAppLog(`{"level":"info","msg":"said \"hello\" world"}`)
	if !ok {
		t.Fatal("parseAppLog returned !ok")
	}
	want := `said "hello" world`
	if entry.Msg != want {
		t.Errorf("msg = %q, want %q", entry.Msg, want)
	}
}

// --- buildTags ---

func TestBuildTagsBothPresent(t *testing.T) {
	if got := buildTags("appstore;managed", "gpu"); got != "appstore;managed;gpu" {
		t.Errorf("buildTags = %q, want %q", got, "appstore;managed;gpu")
	}
}

func TestBuildTagsEmptyExtra(t *testing.T) {
	if got := buildTags("appstore;managed", ""); got != "appstore;managed" {
		t.Errorf("buildTags = %q, want %q", got, "appstore;managed")
	}
}

func TestBuildTagsEmptyBase(t *testing.T) {
	if got := buildTags("", "extra"); got != ";extra" {
		t.Errorf("buildTags = %q, want %q", got, ";extra")
	}
}
