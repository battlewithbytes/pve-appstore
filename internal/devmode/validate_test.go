package devmode

import (
	"strings"
	"testing"
)

// runASTPermissions is a test helper: runs the AST analyzer on a script,
// then calls validatePermissionsFromAST with the given manifest.
func runASTPermissions(t *testing.T, script string, manifest []byte) *ValidationResult {
	t.Helper()
	result := &ValidationResult{Valid: true, Errors: []ValidationMsg{}, Warnings: []ValidationMsg{}}
	analysis, err := runASTAnalyzer(script)
	if err != nil {
		t.Fatalf("runASTAnalyzer failed: %v", err)
	}
	if analysis.Error != "" {
		t.Fatalf("AST analysis error: %s", analysis.Error)
	}
	validatePermissionsFromAST(result, analysis, manifest)
	return result
}

func TestValidatePermissions_MissingCommand(t *testing.T) {
	manifest := []byte(`
id: test-app
name: Test App
description: A test app
version: "1.0.0"
categories: [utilities]
lxc:
  ostemplate: debian-12
  defaults:
    cores: 1
    memory_mb: 512
    disk_gb: 4
provisioning:
  script: provision/install.py
permissions:
  packages: [curl]
  commands: [curl]
  services: [myapp]
  paths: [/etc/myapp/]
`)
	script := `#!/usr/bin/env python3
from appstore import BaseApp, run

class TestApp(BaseApp):
    def install(self):
        self.pkg_install("curl")
        self.run_command(["curl", "-fsSL", "https://example.com"])
        self.run_command(["gitlab-ctl", "reconfigure"])
        self.enable_service("myapp")

run(TestApp)
`
	result := runASTPermissions(t, script, manifest)

	// Should warn about gitlab-ctl not being in permissions.commands
	found := false
	for _, w := range result.Warnings {
		if w.Code == "PERM_MISSING_COMMAND" && strings.Contains(w.Message, "gitlab-ctl") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about gitlab-ctl not in permissions.commands, got warnings: %+v", result.Warnings)
	}

	// Should NOT warn about curl (it's allowed)
	for _, w := range result.Warnings {
		if w.Code == "PERM_MISSING_COMMAND" && strings.Contains(w.Message, `"curl"`) {
			t.Errorf("unexpected warning about curl: %+v", w)
		}
	}
}

func TestValidatePermissions_MissingPackage(t *testing.T) {
	manifest := []byte(`
id: test-app
name: Test
description: test
version: "1.0.0"
categories: [utilities]
lxc:
  ostemplate: debian-12
  defaults: {cores: 1, memory_mb: 512, disk_gb: 4}
provisioning:
  script: provision/install.py
permissions:
  packages: [curl]
`)
	script := `#!/usr/bin/env python3
from appstore import BaseApp, run

class TestApp(BaseApp):
    def install(self):
        self.pkg_install("curl", "wget", "git")

run(TestApp)
`
	result := runASTPermissions(t, script, manifest)

	// Should warn about wget and git but not curl
	warnPkgs := map[string]bool{}
	for _, w := range result.Warnings {
		if w.Code == "PERM_MISSING_PACKAGE" {
			for _, pkg := range []string{"wget", "git", "curl"} {
				if strings.Contains(w.Message, `"`+pkg+`"`) {
					warnPkgs[pkg] = true
				}
			}
		}
	}
	if !warnPkgs["wget"] {
		t.Error("expected warning about wget")
	}
	if !warnPkgs["git"] {
		t.Error("expected warning about git")
	}
	if warnPkgs["curl"] {
		t.Error("unexpected warning about curl")
	}
}

func TestValidatePermissions_MissingService(t *testing.T) {
	manifest := []byte(`
id: test-app
name: Test
description: test
version: "1.0.0"
categories: [utilities]
lxc:
  ostemplate: debian-12
  defaults: {cores: 1, memory_mb: 512, disk_gb: 4}
provisioning:
  script: provision/install.py
permissions:
  services: [nginx]
`)
	script := `#!/usr/bin/env python3
from appstore import BaseApp, run

class TestApp(BaseApp):
    def install(self):
        self.enable_service("nginx")
        self.restart_service("redis")

run(TestApp)
`
	result := runASTPermissions(t, script, manifest)

	found := false
	for _, w := range result.Warnings {
		if w.Code == "PERM_MISSING_SERVICE" && strings.Contains(w.Message, "redis") {
			found = true
		}
		if strings.Contains(w.Message, "nginx") {
			t.Errorf("unexpected warning about nginx: %+v", w)
		}
	}
	if !found {
		t.Error("expected warning about redis not in permissions.services")
	}
}

func TestValidatePermissions_MissingPath(t *testing.T) {
	manifest := []byte(`
id: test-app
name: Test
description: test
version: "1.0.0"
categories: [utilities]
lxc:
  ostemplate: debian-12
  defaults: {cores: 1, memory_mb: 512, disk_gb: 4}
provisioning:
  script: provision/install.py
permissions:
  paths: [/etc/myapp/, /var/lib/myapp/]
`)
	script := `#!/usr/bin/env python3
from appstore import BaseApp, run

class TestApp(BaseApp):
    def install(self):
        self.write_config("/etc/myapp/config.json", "{}")
        self.create_dir("/var/lib/myapp/data")
        self.create_dir("/opt/secret")
        self.create_dir("/tmp/build")

run(TestApp)
`
	result := runASTPermissions(t, script, manifest)

	// /opt/secret should warn, /tmp/build should not (implicit), /etc/myapp/ and /var/lib/myapp/ should not
	warnPaths := map[string]bool{}
	for _, w := range result.Warnings {
		if w.Code == "PERM_MISSING_PATH" {
			warnPaths[w.Message] = true
		}
	}

	foundSecret := false
	for msg := range warnPaths {
		if strings.Contains(msg, "/opt/secret") {
			foundSecret = true
		}
		if strings.Contains(msg, "/tmp") {
			t.Error("unexpected warning about /tmp path")
		}
		if strings.Contains(msg, "/etc/myapp") {
			t.Error("unexpected warning about /etc/myapp path")
		}
	}
	if !foundSecret {
		t.Errorf("expected warning about /opt/secret, got: %+v", result.Warnings)
	}
}

func TestValidatePermissions_MissingUser(t *testing.T) {
	manifest := []byte(`
id: test-app
name: Test
description: test
version: "1.0.0"
categories: [utilities]
lxc:
  ostemplate: debian-12
  defaults: {cores: 1, memory_mb: 512, disk_gb: 4}
provisioning:
  script: provision/install.py
permissions:
  users: [appuser]
`)
	script := `#!/usr/bin/env python3
from appstore import BaseApp, run

class TestApp(BaseApp):
    def install(self):
        self.create_user("appuser")
        self.create_user("hackerman")

run(TestApp)
`
	result := runASTPermissions(t, script, manifest)

	found := false
	for _, w := range result.Warnings {
		if w.Code == "PERM_MISSING_USER" && strings.Contains(w.Message, "hackerman") {
			found = true
		}
		if strings.Contains(w.Message, "appuser") {
			t.Errorf("unexpected warning about appuser")
		}
	}
	if !found {
		t.Error("expected warning about hackerman not in permissions.users")
	}
}

func TestValidatePyflakes_UndefinedName(t *testing.T) {
	script := `from appstore import BaseApp, run

class TestApp(BaseApp):
    def install(self):
        x = undefined_var

run(TestApp)
`
	result := &ValidationResult{Valid: true, Errors: []ValidationMsg{}, Warnings: []ValidationMsg{}}
	validatePyflakes(result, script)

	if result.Valid {
		t.Error("expected Valid=false due to undefined name")
	}

	found := false
	for _, e := range result.Errors {
		if e.Code == "SCRIPT_UNDEFINED_NAME" && e.Line == 5 {
			found = true
			if !strings.Contains(e.Message, "undefined_var") {
				t.Errorf("expected message about undefined_var, got: %s", e.Message)
			}
		}
	}
	if !found {
		t.Errorf("expected SCRIPT_UNDEFINED_NAME error on line 5, got errors: %+v", result.Errors)
	}
}

func TestValidatePyflakes_CleanScript(t *testing.T) {
	script := `from appstore import BaseApp, run

class TestApp(BaseApp):
    def install(self):
        tz = self.inputs.string("timezone", "UTC")
        self.apt_install("nginx")
        self.write_config("/etc/nginx/conf.d/default.conf", tz)

run(TestApp)
`
	result := &ValidationResult{Valid: true, Errors: []ValidationMsg{}, Warnings: []ValidationMsg{}}
	validatePyflakes(result, script)

	for _, e := range result.Errors {
		if e.Code == "SCRIPT_UNDEFINED_NAME" {
			t.Errorf("unexpected undefined name error: %+v", e)
		}
	}
	if !result.Valid {
		t.Errorf("expected Valid=true for clean script, got errors: %+v", result.Errors)
	}
}

func TestValidatePyflakes_UnusedImportWarning(t *testing.T) {
	script := `from appstore import BaseApp, run
import os

class TestApp(BaseApp):
    def install(self):
        self.apt_install("nginx")

run(TestApp)
`
	result := &ValidationResult{Valid: true, Errors: []ValidationMsg{}, Warnings: []ValidationMsg{}}
	validatePyflakes(result, script)

	// 'import os' unused should be a warning, not an error
	if !result.Valid {
		t.Error("unused import should be a warning, not make Valid=false")
	}

	found := false
	for _, w := range result.Warnings {
		if w.Code == "SCRIPT_LINT" && strings.Contains(w.Message, "os") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected SCRIPT_LINT warning about unused 'os' import, got warnings: %+v", result.Warnings)
	}
}

func TestValidatePermissions_ServiceUserNotCreated(t *testing.T) {
	manifest := []byte(`
id: test-app
name: Test
description: test
version: "1.0.0"
categories: [utilities]
lxc:
  ostemplate: debian-12
  defaults: {cores: 1, memory_mb: 512, disk_gb: 4}
provisioning:
  script: provision/install.py
permissions:
  services: [myapp]
  users: [appuser]
`)
	script := `#!/usr/bin/env python3
from appstore import BaseApp, run

class TestApp(BaseApp):
    def install(self):
        self.create_user("appuser")
        self.create_service("myapp",
            exec_start="/usr/bin/myapp",
            user="unknownuser",
        )

run(TestApp)
`
	result := runASTPermissions(t, script, manifest)

	found := false
	for _, w := range result.Warnings {
		if w.Code == "SCRIPT_SERVICE_USER_MISSING" && strings.Contains(w.Message, "unknownuser") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected warning about unknownuser not created, got: %+v", result.Warnings)
	}
}

func TestValidatePermissions_ServiceUserCreated(t *testing.T) {
	manifest := []byte(`
id: test-app
name: Test
description: test
version: "1.0.0"
categories: [utilities]
lxc:
  ostemplate: debian-12
  defaults: {cores: 1, memory_mb: 512, disk_gb: 4}
provisioning:
  script: provision/install.py
permissions:
  services: [myapp]
  users: [hauser]
`)
	script := `#!/usr/bin/env python3
from appstore import BaseApp, run

class TestApp(BaseApp):
    def install(self):
        self.create_user("hauser")
        self.create_service("myapp",
            exec_start="/usr/bin/myapp",
            user="hauser",
        )

run(TestApp)
`
	result := runASTPermissions(t, script, manifest)

	for _, w := range result.Warnings {
		if w.Code == "SCRIPT_SERVICE_USER_MISSING" {
			t.Errorf("unexpected service user warning: %+v", w)
		}
	}
}

func TestValidatePermissions_ServiceUserRoot(t *testing.T) {
	manifest := []byte(`
id: test-app
name: Test
description: test
version: "1.0.0"
categories: [utilities]
lxc:
  ostemplate: debian-12
  defaults: {cores: 1, memory_mb: 512, disk_gb: 4}
provisioning:
  script: provision/install.py
permissions:
  services: [myapp]
`)
	script := `#!/usr/bin/env python3
from appstore import BaseApp, run

class TestApp(BaseApp):
    def install(self):
        self.create_service("myapp",
            exec_start="/usr/bin/myapp",
            user="root",
        )

run(TestApp)
`
	result := runASTPermissions(t, script, manifest)

	for _, w := range result.Warnings {
		if w.Code == "SCRIPT_SERVICE_USER_MISSING" {
			t.Errorf("root should not trigger service user warning: %+v", w)
		}
	}
}

func TestValidatePermissions_AllAllowed(t *testing.T) {
	manifest := []byte(`
id: test-app
name: Test
description: test
version: "1.0.0"
categories: [utilities]
lxc:
  ostemplate: debian-12
  defaults: {cores: 1, memory_mb: 512, disk_gb: 4}
provisioning:
  script: provision/install.py
permissions:
  packages: [curl, nginx]
  commands: [gitlab-ctl]
  services: [nginx]
  paths: [/etc/nginx/]
  urls: ["https://example.com/*"]
  users: [www-data]
`)
	script := `#!/usr/bin/env python3
from appstore import BaseApp, run

class TestApp(BaseApp):
    def install(self):
        self.pkg_install("curl", "nginx")
        self.run_command(["gitlab-ctl", "reconfigure"])
        self.enable_service("nginx")
        self.write_config("/etc/nginx/nginx.conf", "server {}")
        self.download("https://example.com/file.tar.gz", "/tmp/file.tar.gz")
        self.create_user("www-data")

run(TestApp)
`
	result := runASTPermissions(t, script, manifest)

	// No permission warnings expected
	for _, w := range result.Warnings {
		if strings.HasPrefix(w.Code, "PERM_") {
			t.Errorf("unexpected permission warning: %+v", w)
		}
	}
}

// --- New AST-specific tests ---

func TestASTAnalyzer_MultiLineCall(t *testing.T) {
	script := `from appstore import BaseApp, run

class TestApp(BaseApp):
    def install(self):
        self.pkg_install(
            "python3",
            "python3-venv",
            "libffi-dev",
        )

run(TestApp)
`
	analysis, err := runASTAnalyzer(script)
	if err != nil {
		t.Fatalf("runASTAnalyzer failed: %v", err)
	}

	// All three packages should be found despite multi-line call
	found := 0
	for _, call := range analysis.MethodCalls {
		if call.Method == "pkg_install" {
			args := allStringArgs(call)
			for _, a := range args {
				if a == "python3" || a == "python3-venv" || a == "libffi-dev" {
					found++
				}
			}
		}
	}
	if found != 3 {
		t.Errorf("expected 3 package args from multi-line call, got %d", found)
	}
}

func TestASTAnalyzer_FStringSkipped(t *testing.T) {
	manifest := []byte(`
id: test-app
name: Test
description: test
version: "1.0.0"
categories: [utilities]
lxc:
  ostemplate: debian-12
  defaults: {cores: 1, memory_mb: 512, disk_gb: 4}
provisioning:
  script: provision/install.py
permissions:
  paths: [/etc/myapp/]
`)
	script := `#!/usr/bin/env python3
from appstore import BaseApp, run

class TestApp(BaseApp):
    def install(self):
        name = "myapp"
        self.create_dir(f"/etc/{name}/config")

run(TestApp)
`
	result := runASTPermissions(t, script, manifest)

	// f-string path should be <dynamic> and thus skipped — no path warning
	for _, w := range result.Warnings {
		if w.Code == "PERM_MISSING_PATH" {
			t.Errorf("unexpected path warning for f-string path: %+v", w)
		}
	}
}

func TestASTAnalyzer_VariableSkipped(t *testing.T) {
	manifest := []byte(`
id: test-app
name: Test
description: test
version: "1.0.0"
categories: [utilities]
lxc:
  ostemplate: debian-12
  defaults: {cores: 1, memory_mb: 512, disk_gb: 4}
provisioning:
  script: provision/install.py
permissions:
  paths: [/opt/app/]
`)
	script := `#!/usr/bin/env python3
from appstore import BaseApp, run

class TestApp(BaseApp):
    def install(self):
        config_path = self.inputs.string("config_path", "/opt/app/config")
        self.create_dir(config_path)

run(TestApp)
`
	result := runASTPermissions(t, script, manifest)

	// Variable path should be <dynamic> and thus skipped — no path warning
	for _, w := range result.Warnings {
		if w.Code == "PERM_MISSING_PATH" {
			t.Errorf("unexpected path warning for variable path: %+v", w)
		}
	}
}

func TestASTAnalyzer_CommentIgnored(t *testing.T) {
	manifest := []byte(`
id: test-app
name: Test
description: test
version: "1.0.0"
categories: [utilities]
lxc:
  ostemplate: debian-12
  defaults: {cores: 1, memory_mb: 512, disk_gb: 4}
provisioning:
  script: provision/install.py
permissions: {}
`)
	script := `#!/usr/bin/env python3
from appstore import BaseApp, run

class TestApp(BaseApp):
    def install(self):
        # self.pkg_install("secret-package")
        pass

run(TestApp)
`
	result := runASTPermissions(t, script, manifest)

	// Commented-out code should not trigger warnings
	for _, w := range result.Warnings {
		if w.Code == "PERM_MISSING_PACKAGE" {
			t.Errorf("unexpected package warning from commented code: %+v", w)
		}
	}
}

func TestASTAnalyzer_PipInstallVenvKwarg(t *testing.T) {
	manifest := []byte(`
id: test-app
name: Test
description: test
version: "1.0.0"
categories: [utilities]
lxc:
  ostemplate: debian-12
  defaults: {cores: 1, memory_mb: 512, disk_gb: 4}
provisioning:
  script: provision/install.py
permissions:
  pip: [flask]
`)
	script := `#!/usr/bin/env python3
from appstore import BaseApp, run

class TestApp(BaseApp):
    def install(self):
        self.pip_install("flask", venv="/opt/app/venv")

run(TestApp)
`
	result := runASTPermissions(t, script, manifest)

	// venv kwarg should not be treated as a pip package
	for _, w := range result.Warnings {
		if w.Code == "PERM_MISSING_PIP" && strings.Contains(w.Message, "/opt/app/venv") {
			t.Errorf("venv kwarg incorrectly treated as pip package: %+v", w)
		}
	}
}

func TestASTAnalyzer_PipVersionSpecifier(t *testing.T) {
	manifest := []byte(`
id: test-app
name: Test
description: test
version: "1.0.0"
categories: [utilities]
lxc:
  ostemplate: debian-12
  defaults: {cores: 1, memory_mb: 512, disk_gb: 4}
provisioning:
  script: provision/install.py
permissions:
  pip: [josepy, homeassistant]
`)
	script := `#!/usr/bin/env python3
from appstore import BaseApp, run

class TestApp(BaseApp):
    def install(self):
        self.pip_install("josepy<2")
        self.pip_install("homeassistant[all]>=2024.1.0")

run(TestApp)
`
	result := runASTPermissions(t, script, manifest)

	// Version specifiers and extras should be stripped before matching
	for _, w := range result.Warnings {
		if w.Code == "PERM_MISSING_PIP" {
			t.Errorf("version-pinned pip package incorrectly flagged: %+v", w)
		}
	}
}

func TestASTAnalyzer_RunInstallerScript(t *testing.T) {
	manifest := []byte(`
id: test-app
name: Test
description: test
version: "1.0.0"
categories: [utilities]
lxc:
  ostemplate: debian-12
  defaults: {cores: 1, memory_mb: 512, disk_gb: 4}
provisioning:
  script: provision/install.py
permissions:
  urls: ["https://allowed.com/*"]
`)
	script := `#!/usr/bin/env python3
from appstore import BaseApp, run

class TestApp(BaseApp):
    def install(self):
        self.run_installer_script("https://notallowed.com/install.sh")

run(TestApp)
`
	result := runASTPermissions(t, script, manifest)

	found := false
	for _, w := range result.Warnings {
		if w.Code == "PERM_MISSING_URL" && strings.Contains(w.Message, "notallowed.com") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected URL warning for run_installer_script, got: %+v", result.Warnings)
	}
}

func TestASTAnalyzer_StructuralChecks(t *testing.T) {
	script := `from appstore import BaseApp, run

class MyApp(BaseApp):
    def install(self):
        self.apt_install("nginx")

run(MyApp)
`
	analysis, err := runASTAnalyzer(script)
	if err != nil {
		t.Fatalf("runASTAnalyzer failed: %v", err)
	}

	if analysis.ClassName != "MyApp" {
		t.Errorf("expected ClassName=MyApp, got %s", analysis.ClassName)
	}
	if !analysis.HasInstallMethod {
		t.Error("expected HasInstallMethod=true")
	}
	if !analysis.HasRunCall {
		t.Error("expected HasRunCall=true")
	}

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
		t.Error("expected BaseApp in imports")
	}
	if !hasRun {
		t.Error("expected run in imports")
	}
}

func TestASTAnalyzer_SyntaxError(t *testing.T) {
	script := `from appstore import BaseApp, run

class MyApp(BaseApp)
    def install(self):
        pass
`
	analysis, err := runASTAnalyzer(script)
	if err != nil {
		t.Fatalf("runASTAnalyzer failed: %v", err)
	}

	if analysis.Error == "" {
		t.Error("expected syntax error from analyzer")
	}
}

func TestASTAnalyzer_UnsafePatterns(t *testing.T) {
	script := `import os
import subprocess
from appstore import BaseApp, run

class TestApp(BaseApp):
    def install(self):
        os.system("rm -rf /")
        subprocess.call(["ls"])

run(TestApp)
`
	analysis, err := runASTAnalyzer(script)
	if err != nil {
		t.Fatalf("runASTAnalyzer failed: %v", err)
	}

	foundOsSystem := false
	foundSubprocess := false
	for _, p := range analysis.UnsafePatterns {
		if p.Pattern == "os.system" {
			foundOsSystem = true
		}
		if p.Pattern == "subprocess.call" {
			foundSubprocess = true
		}
	}
	if !foundOsSystem {
		t.Error("expected os.system in unsafe patterns")
	}
	if !foundSubprocess {
		t.Error("expected subprocess.call in unsafe patterns")
	}
}

func TestASTAnalyzer_InputKeys(t *testing.T) {
	script := `from appstore import BaseApp, run

class TestApp(BaseApp):
    def install(self):
        tz = self.inputs.string("timezone", "UTC")
        port = self.inputs.integer("port", 8080)
        debug = self.inputs.boolean("debug", False)

run(TestApp)
`
	analysis, err := runASTAnalyzer(script)
	if err != nil {
		t.Fatalf("runASTAnalyzer failed: %v", err)
	}

	keys := map[string]string{}
	for _, ik := range analysis.InputKeys {
		keys[ik.Key] = ik.Type
	}
	if keys["timezone"] != "string" {
		t.Error("expected input key 'timezone' with type 'string'")
	}
	if keys["port"] != "integer" {
		t.Error("expected input key 'port' with type 'integer'")
	}
	if keys["debug"] != "boolean" {
		t.Error("expected input key 'debug' with type 'boolean'")
	}
}

func TestASTAnalyzer_AddAptRepository(t *testing.T) {
	manifest := []byte(`
id: test-app
name: Test
description: test
version: "1.0.0"
categories: [utilities]
lxc:
  ostemplate: debian-12
  defaults: {cores: 1, memory_mb: 512, disk_gb: 4}
provisioning:
  script: provision/install.py
permissions:
  urls: ["https://packages.example.com/*"]
  apt_repos: ["https://packages.example.com/repo"]
`)
	script := `#!/usr/bin/env python3
from appstore import BaseApp, run

class TestApp(BaseApp):
    def install(self):
        self.add_apt_repository(
            "https://packages.example.com/repo",
            key_url="https://packages.example.com/gpgkey",
            name="example",
        )

run(TestApp)
`
	result := runASTPermissions(t, script, manifest)

	// All URLs and apt_repos are covered — no warnings expected
	for _, w := range result.Warnings {
		if strings.HasPrefix(w.Code, "PERM_") {
			t.Errorf("unexpected permission warning: %+v", w)
		}
	}
}

func TestASTAnalyzer_AptRepoDebLineMatchesBareURL(t *testing.T) {
	// Manifest has a full deb line, script uses bare URL — should still match
	manifest := []byte(`
id: plex
name: Plex
description: Plex Media Server
version: "1.0.0"
categories: [media]
lxc:
  ostemplate: debian-12
  defaults: {cores: 2, memory_mb: 2048, disk_gb: 8}
provisioning:
  script: provision/install.py
permissions:
  urls: ["https://downloads.plex.tv/*"]
  apt_repos: ["deb [signed-by=/usr/share/keyrings/plex-archive-keyring.gpg] https://downloads.plex.tv/repo/deb public main"]
`)
	script := `#!/usr/bin/env python3
from appstore import BaseApp, run

class PlexApp(BaseApp):
    def install(self):
        self.add_apt_repository(
            "https://downloads.plex.tv/repo/deb",
            key_url="https://downloads.plex.tv/PlexSign.key",
            name="plexmediaserver",
        )

run(PlexApp)
`
	result := runASTPermissions(t, script, manifest)

	// Bare URL from script should match the URL in the full deb line
	for _, w := range result.Warnings {
		if w.Code == "PERM_MISSING_APT_REPO" {
			t.Errorf("bare URL should match full deb line in apt_repos: %+v", w)
		}
	}
}

func TestASTAnalyzer_AptRepoWarningShowsURL(t *testing.T) {
	// Verify that the warning message includes the actual URL
	manifest := []byte(`
id: test-app
name: Test
description: test
version: "1.0.0"
categories: [utilities]
lxc:
  ostemplate: debian-12
  defaults: {cores: 1, memory_mb: 512, disk_gb: 4}
provisioning:
  script: provision/install.py
permissions:
  urls: ["https://allowed.example.com/*"]
  apt_repos: ["https://allowed.example.com/repo"]
`)
	script := `#!/usr/bin/env python3
from appstore import BaseApp, run

class TestApp(BaseApp):
    def install(self):
        self.add_apt_repository(
            "https://notallowed.example.com/repo",
            key_url="https://allowed.example.com/key.gpg",
            name="test",
        )

run(TestApp)
`
	result := runASTPermissions(t, script, manifest)

	found := false
	for _, w := range result.Warnings {
		if w.Code == "PERM_MISSING_APT_REPO" {
			if strings.Contains(w.Message, "https://notallowed.example.com/repo") {
				found = true
			} else {
				t.Errorf("apt_repo warning should include the URL, got: %s", w.Message)
			}
		}
	}
	if !found {
		t.Errorf("expected PERM_MISSING_APT_REPO warning with URL, got: %+v", result.Warnings)
	}
}

// --- Direct unit tests for validation helpers ---

func TestExtractRepoURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://downloads.plex.tv/repo/deb", "https://downloads.plex.tv/repo/deb"},
		{"https://downloads.plex.tv/repo/deb/", "https://downloads.plex.tv/repo/deb"},
		{"deb [signed-by=/usr/share/keyrings/plex.gpg] https://downloads.plex.tv/repo/deb public main", "https://downloads.plex.tv/repo/deb"},
		{"deb http://example.com/repo stable main", "http://example.com/repo"},
		{"https://example.com/repo public main", "https://example.com/repo"},
		{"no-url-here", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := extractRepoURL(tt.input)
		if got != tt.want {
			t.Errorf("extractRepoURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestAptRepoAllowed(t *testing.T) {
	tests := []struct {
		name     string
		repoLine string
		allowed  []string
		want     bool
	}{
		{
			name:     "bare URL matches bare URL",
			repoLine: "https://downloads.plex.tv/repo/deb",
			allowed:  []string{"https://downloads.plex.tv/repo/deb"},
			want:     true,
		},
		{
			name:     "bare URL matches full deb line",
			repoLine: "https://downloads.plex.tv/repo/deb",
			allowed:  []string{"deb [signed-by=/usr/share/keyrings/plex.gpg] https://downloads.plex.tv/repo/deb public main"},
			want:     true,
		},
		{
			name:     "full deb line matches bare URL",
			repoLine: "deb [signed-by=/usr/share/keyrings/plex.gpg] https://downloads.plex.tv/repo/deb public main",
			allowed:  []string{"https://downloads.plex.tv/repo/deb"},
			want:     true,
		},
		{
			name:     "full deb line matches full deb line",
			repoLine: "deb [signed-by=/usr/share/keyrings/plex.gpg] https://downloads.plex.tv/repo/deb public main",
			allowed:  []string{"deb [signed-by=/usr/share/keyrings/plex.gpg] https://downloads.plex.tv/repo/deb public main"},
			want:     true,
		},
		{
			name:     "URL prefix match",
			repoLine: "https://downloads.plex.tv/repo/deb/extra",
			allowed:  []string{"https://downloads.plex.tv/repo/deb"},
			want:     true,
		},
		{
			name:     "no match",
			repoLine: "https://evil.com/repo",
			allowed:  []string{"https://downloads.plex.tv/repo/deb"},
			want:     false,
		},
		{
			name:     "empty allowed list",
			repoLine: "https://example.com/repo",
			allowed:  []string{},
			want:     false,
		},
		{
			name:     "no URL in input",
			repoLine: "not-a-url",
			allowed:  []string{"https://example.com"},
			want:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := aptRepoAllowed(tt.repoLine, tt.allowed)
			if got != tt.want {
				t.Errorf("aptRepoAllowed(%q, %v) = %v, want %v", tt.repoLine, tt.allowed, got, tt.want)
			}
		})
	}
}

func TestPipBaseName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"josepy", "josepy"},
		{"josepy<2", "josepy"},
		{"homeassistant[all]>=2024.1.0", "homeassistant"},
		{"flask==2.3.0", "flask"},
		{"requests~=2.28", "requests"},
		{"pkg!=1.0", "pkg"},
		{"certbot;python_version>='3'", "certbot"},
		{"  spaces  ", "  spaces  "},
	}
	for _, tt := range tests {
		got := pipBaseName(tt.input)
		if got != tt.want {
			t.Errorf("pipBaseName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
