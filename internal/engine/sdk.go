package engine

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/battlewithbytes/pve-appstore/internal/catalog"
	appstoresdk "github.com/battlewithbytes/pve-appstore/sdk"
)

const (
	// sdkTargetDir is where the Python SDK is installed inside containers.
	sdkTargetDir = "/opt/appstore/sdk"
	// provisionDir is where app scripts and assets live inside containers.
	provisionTargetDir = "/opt/appstore/provision"
	// inputsPath is where the JSON inputs file is written inside containers.
	inputsPath = "/opt/appstore/inputs.json"
	// permissionsPath is where the JSON permissions file is written inside containers.
	permissionsPath = "/opt/appstore/permissions.json"
	// hostTmpDir is where temp files are written on the host before pushing
	// into containers. We use /var/lib/pve-appstore instead of /tmp because
	// the service runs with PrivateTmp=yes — nsenter escapes the mount
	// namespace so pct push sees the real filesystem, not the private /tmp.
	hostTmpDir = "/var/lib/pve-appstore/tmp"
)

// ensurePython verifies python3 is available in the container.
// If missing (e.g., non-Debian base image), it attempts to install it.
func ensurePython(ctid int, cm ContainerManager) error {
	result, err := cm.Exec(ctid, []string{"which", "python3"})
	if err == nil && result.ExitCode == 0 {
		return nil // python3 already available
	}

	// Try to install python3
	result, err = cm.Exec(ctid, []string{"apt-get", "update"})
	if err != nil {
		return fmt.Errorf("apt-get update for python3 install: %w", err)
	}
	result, err = cm.Exec(ctid, []string{"apt-get", "install", "-y", "python3"})
	if err != nil {
		return fmt.Errorf("installing python3: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("apt-get install python3 exited with %d: %s", result.ExitCode, result.Output)
	}
	return nil
}

// pushSDK extracts the embedded Python SDK into the container.
func pushSDK(ctid int, cm ContainerManager) error {
	// Create the SDK target directory
	cm.Exec(ctid, []string{"mkdir", "-p", sdkTargetDir + "/appstore"})

	// Ensure host tmp dir exists (visible to nsenter/pct push)
	os.MkdirAll(hostTmpDir, 0750)

	return fs.WalkDir(appstoresdk.PythonFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		data, err := appstoresdk.PythonFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading embedded %s: %w", path, err)
		}

		// path is like "python/appstore/base.py"
		// Strip "python/" prefix to get "appstore/base.py"
		rel := strings.TrimPrefix(path, "python/")
		dst := sdkTargetDir + "/" + rel

		// Write to temp file on host, then push into container
		tmpFile, err := os.CreateTemp(hostTmpDir, "sdk-*")
		if err != nil {
			return fmt.Errorf("creating temp file: %w", err)
		}
		tmpPath := tmpFile.Name()
		defer os.Remove(tmpPath)

		if _, err := tmpFile.Write(data); err != nil {
			tmpFile.Close()
			return fmt.Errorf("writing temp file: %w", err)
		}
		tmpFile.Close()

		if err := cm.Push(ctid, tmpPath, dst, "0644"); err != nil {
			return fmt.Errorf("pushing SDK file %s: %w", rel, err)
		}

		return nil
	})
}

// pushInputsJSON writes app inputs as a JSON file inside the container.
func pushInputsJSON(ctid int, cm ContainerManager, inputs map[string]string) error {
	os.MkdirAll(hostTmpDir, 0750)
	data, err := json.Marshal(inputs)
	if err != nil {
		return fmt.Errorf("marshaling inputs: %w", err)
	}

	tmpFile, err := os.CreateTemp(hostTmpDir, "inputs-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return err
	}
	tmpFile.Close()

	return cm.Push(ctid, tmpPath, inputsPath, "0644")
}

// pushPermissionsJSON writes the app's permission allowlist as a JSON file inside the container.
func pushPermissionsJSON(ctid int, cm ContainerManager, perms catalog.PermissionsSpec) error {
	os.MkdirAll(hostTmpDir, 0750)
	// Convert to the JSON structure the Python SDK expects
	permData := map[string][]string{
		"packages":          perms.Packages,
		"pip":               perms.Pip,
		"urls":              perms.URLs,
		"paths":             perms.Paths,
		"services":          perms.Services,
		"users":             perms.Users,
		"commands":          perms.Commands,
		"installer_scripts": perms.InstallerScripts,
		"apt_repos":         perms.AptRepos,
	}
	// Ensure nil slices become empty arrays in JSON
	for k, v := range permData {
		if v == nil {
			permData[k] = []string{}
		}
	}

	data, err := json.Marshal(permData)
	if err != nil {
		return fmt.Errorf("marshaling permissions: %w", err)
	}

	tmpFile, err := os.CreateTemp(hostTmpDir, "perms-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return err
	}
	tmpFile.Close()

	return cm.Push(ctid, tmpPath, permissionsPath, "0644")
}

// buildProvisionCommand builds the command to run a Python provisioning script
// via the SDK runner.
func buildProvisionCommand(scriptName, action string) []string {
	scriptPath := provisionTargetDir + "/" + filepath.Base(scriptName)
	return []string{
		"env", "PYTHONPATH=" + sdkTargetDir,
		"python3", "-m", "appstore.runner",
		inputsPath,
		permissionsPath,
		action,
		scriptPath,
	}
}
