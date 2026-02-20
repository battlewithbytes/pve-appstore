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

// filterAllowedDevices returns only the devices whose Path is in the allowed list.
// If allowed is empty, no devices are returned.
func filterAllowedDevices(devices []DevicePassthrough, allowed []string) []DevicePassthrough {
	if len(allowed) == 0 {
		return nil
	}
	set := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		set[a] = true
	}
	var out []DevicePassthrough
	for _, d := range devices {
		if set[d.Path] {
			out = append(out, d)
		}
	}
	return out
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

// GPUDriverStatus reports the state of GPU kernel drivers and userspace libraries on the host.
type GPUDriverStatus struct {
	NvidiaDriverLoaded bool   `json:"nvidia_driver_loaded"`
	NvidiaVersion      string `json:"nvidia_version,omitempty"`
	NvidiaLibsFound    bool   `json:"nvidia_libs_found"`
	IntelDriverLoaded  bool   `json:"intel_driver_loaded"`
	AmdDriverLoaded    bool   `json:"amd_driver_loaded"`
}

// DetectDriverStatus checks which GPU kernel modules are loaded and whether
// NVIDIA userspace libraries are present on the host.
func DetectDriverStatus() GPUDriverStatus {
	var s GPUDriverStatus

	// NVIDIA kernel module
	if _, err := os.Stat("/sys/module/nvidia"); err == nil {
		s.NvidiaDriverLoaded = true
		// Read driver version
		if data, err := os.ReadFile("/sys/module/nvidia/version"); err == nil {
			s.NvidiaVersion = strings.TrimSpace(string(data))
		}
	}

	// NVIDIA userspace libraries
	if _, err := resolveNvidiaLibPath(); err == nil {
		s.NvidiaLibsFound = true
	}

	// Intel: i915 or xe (newer Intel GPUs)
	if _, err := os.Stat("/sys/module/i915"); err == nil {
		s.IntelDriverLoaded = true
	} else if _, err := os.Stat("/sys/module/xe"); err == nil {
		s.IntelDriverLoaded = true
	}

	// AMD
	if _, err := os.Stat("/sys/module/amdgpu"); err == nil {
		s.AmdDriverLoaded = true
	}

	return s
}

// DevicesForGPUType returns the device passthrough config for a discovered GPU
// based on its type and device path.
func DevicesForGPUType(gpuType string, devicePath string) []DevicePassthrough {
	switch gpuType {
	case "intel", "amd":
		return []DevicePassthrough{{Path: devicePath, GID: 44, Mode: "0666"}}
	case "nvidia":
		devs := []DevicePassthrough{{Path: devicePath}}
		// Add shared NVIDIA control nodes (deduplicated by caller if multiple GPUs)
		if devicePath != "/dev/nvidiactl" {
			devs = append(devs, DevicePassthrough{Path: "/dev/nvidiactl"})
		}
		if devicePath != "/dev/nvidia-uvm" {
			devs = append(devs, DevicePassthrough{Path: "/dev/nvidia-uvm"})
		}
		return devs
	default:
		return []DevicePassthrough{{Path: devicePath}}
	}
}
