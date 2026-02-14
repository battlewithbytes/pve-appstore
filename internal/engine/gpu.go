package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// nvidiaContainerLibPath is where NVIDIA libraries are mounted inside containers.
	nvidiaContainerLibPath = "/usr/lib/nvidia"

	// nvidiaLdconfPath is the ldconfig config file created inside containers.
	nvidiaLdconfPath = "/etc/ld.so.conf.d/nvidia.conf"
)

// Well-known host paths for NVIDIA libraries (Debian/Proxmox).
var nvidiaHostLibDirs = []string{
	"/usr/lib/x86_64-linux-gnu/nvidia/current",  // Debian NVIDIA driver package (curated)
	"/usr/lib/aarch64-linux-gnu/nvidia/current", // ARM64 equivalent
}

// Fallback: discover libraries by glob in standard lib directories.
var nvidiaFallbackLibDirs = []string{
	"/usr/lib/x86_64-linux-gnu",
	"/usr/lib/aarch64-linux-gnu",
}

var nvidiaLibGlobs = []string{
	"libnvidia-*.so*",
	"libcuda*.so*",
	"libnvcuvid*.so*",
	"libnvoptix*.so*",
	"libvdpau_nvidia*.so*",
	"libEGL_nvidia*.so*",
	"libGLX_nvidia*.so*",
	"libGLESv*_nvidia*.so*",
}

// hasNvidiaDevices returns true if any device in the list is an NVIDIA device node.
func hasNvidiaDevices(devices []DevicePassthrough) bool {
	for _, d := range devices {
		if strings.Contains(d.Path, "nvidia") {
			return true
		}
	}
	return false
}

// deviceNodeExists checks if a specific device node exists on the host.
func deviceNodeExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// validateGPUDevices checks that all requested device nodes exist on the host.
// Returns the subset of devices whose nodes are present, and any errors for missing required devices.
func validateGPUDevices(devices []DevicePassthrough) (available []DevicePassthrough, missing []string) {
	for _, d := range devices {
		if deviceNodeExists(d.Path) {
			available = append(available, d)
		} else {
			missing = append(missing, d.Path)
		}
	}
	return
}

// resolveNvidiaLibPath returns the host path to bind-mount for NVIDIA libraries.
// It prefers the curated nvidia/current directory, falling back to a staging directory.
// Returns empty string if no NVIDIA libraries are found.
func resolveNvidiaLibPath() (string, error) {
	// Check for curated nvidia/current directory (Debian NVIDIA packages)
	for _, dir := range nvidiaHostLibDirs {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			// Verify it actually has libraries
			entries, _ := os.ReadDir(dir)
			hasLibs := false
			for _, e := range entries {
				if strings.HasSuffix(e.Name(), ".so") || strings.Contains(e.Name(), ".so.") {
					hasLibs = true
					break
				}
			}
			if hasLibs {
				return dir, nil
			}
		}
	}

	// Fallback: create a staging directory with symlinks to discovered libraries
	return prepareNvidiaLibStaging()
}

// prepareNvidiaLibStaging discovers NVIDIA libraries on the host and creates
// a staging directory with symlinks suitable for bind-mounting into containers.
func prepareNvidiaLibStaging() (string, error) {
	stagingDir := filepath.Join(hostTmpDir, "nvidia-libs")

	var libs []string
	for _, dir := range nvidiaFallbackLibDirs {
		for _, pattern := range nvidiaLibGlobs {
			matches, _ := filepath.Glob(filepath.Join(dir, pattern))
			libs = append(libs, matches...)
		}
	}

	if len(libs) == 0 {
		return "", fmt.Errorf("no NVIDIA libraries found on host")
	}

	// Create/clean staging directory
	if err := os.MkdirAll(stagingDir, 0755); err != nil {
		return "", fmt.Errorf("creating nvidia staging dir: %w", err)
	}
	entries, _ := os.ReadDir(stagingDir)
	for _, e := range entries {
		os.Remove(filepath.Join(stagingDir, e.Name()))
	}

	// Create symlinks to actual library files
	for _, lib := range libs {
		name := filepath.Base(lib)
		link := filepath.Join(stagingDir, name)
		// Resolve to real path (follow symlink chain)
		real, err := filepath.EvalSymlinks(lib)
		if err != nil {
			continue
		}
		os.Symlink(real, link)
	}

	return stagingDir, nil
}
