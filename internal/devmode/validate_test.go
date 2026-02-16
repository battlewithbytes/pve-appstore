package devmode

import (
	"strings"
	"testing"
)

func TestValidatePermissions_MissingCommand(t *testing.T) {
	manifest := `
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
`
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
	result := &ValidationResult{Valid: true, Errors: []ValidationMsg{}, Warnings: []ValidationMsg{}}
	validatePermissions(result, script, []byte(manifest))

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
		if w.Code == "PERM_MISSING_COMMAND" && strings.Contains(w.Message, "curl") {
			t.Errorf("unexpected warning about curl: %+v", w)
		}
	}
}

func TestValidatePermissions_MissingPackage(t *testing.T) {
	manifest := `
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
`
	script := `#!/usr/bin/env python3
from appstore import BaseApp, run

class TestApp(BaseApp):
    def install(self):
        self.pkg_install("curl", "wget", "git")

run(TestApp)
`
	result := &ValidationResult{Valid: true, Errors: []ValidationMsg{}, Warnings: []ValidationMsg{}}
	validatePermissions(result, script, []byte(manifest))

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
	manifest := `
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
`
	script := `#!/usr/bin/env python3
from appstore import BaseApp, run

class TestApp(BaseApp):
    def install(self):
        self.enable_service("nginx")
        self.restart_service("redis")

run(TestApp)
`
	result := &ValidationResult{Valid: true, Errors: []ValidationMsg{}, Warnings: []ValidationMsg{}}
	validatePermissions(result, script, []byte(manifest))

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
	manifest := `
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
`
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
	result := &ValidationResult{Valid: true, Errors: []ValidationMsg{}, Warnings: []ValidationMsg{}}
	validatePermissions(result, script, []byte(manifest))

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
	manifest := `
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
`
	script := `#!/usr/bin/env python3
from appstore import BaseApp, run

class TestApp(BaseApp):
    def install(self):
        self.create_user("appuser")
        self.create_user("hackerman")

run(TestApp)
`
	result := &ValidationResult{Valid: true, Errors: []ValidationMsg{}, Warnings: []ValidationMsg{}}
	validatePermissions(result, script, []byte(manifest))

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
	manifest := `
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
`
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
	result := &ValidationResult{Valid: true, Errors: []ValidationMsg{}, Warnings: []ValidationMsg{}}
	validatePermissions(result, script, []byte(manifest))

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
	manifest := `
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
`
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
	result := &ValidationResult{Valid: true, Errors: []ValidationMsg{}, Warnings: []ValidationMsg{}}
	validatePermissions(result, script, []byte(manifest))

	for _, w := range result.Warnings {
		if w.Code == "SCRIPT_SERVICE_USER_MISSING" {
			t.Errorf("unexpected service user warning: %+v", w)
		}
	}
}

func TestValidatePermissions_ServiceUserRoot(t *testing.T) {
	manifest := `
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
`
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
	result := &ValidationResult{Valid: true, Errors: []ValidationMsg{}, Warnings: []ValidationMsg{}}
	validatePermissions(result, script, []byte(manifest))

	for _, w := range result.Warnings {
		if w.Code == "SCRIPT_SERVICE_USER_MISSING" {
			t.Errorf("root should not trigger service user warning: %+v", w)
		}
	}
}

func TestValidatePermissions_AllAllowed(t *testing.T) {
	manifest := `
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
`
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
	result := &ValidationResult{Valid: true, Errors: []ValidationMsg{}, Warnings: []ValidationMsg{}}
	validatePermissions(result, script, []byte(manifest))

	// No permission warnings expected
	for _, w := range result.Warnings {
		if strings.HasPrefix(w.Code, "PERM_") {
			t.Errorf("unexpected permission warning: %+v", w)
		}
	}
}
