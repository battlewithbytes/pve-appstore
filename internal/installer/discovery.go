package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DiscoveredResources holds everything we detect from the Proxmox host.
type DiscoveredResources struct {
	NodeName string
	Pools    []string
	Storages []StorageInfo
	Bridges  []string
	GPUs     []GPUInfo
}

type StorageInfo struct {
	ID      string
	Type    string
	Content string
}

type GPUInfo struct {
	Path string
	Type string // "intel", "amd", "nvidia"
	Name string
}

// Discover probes the local Proxmox host for resources.
func Discover() (*DiscoveredResources, error) {
	res := &DiscoveredResources{}

	// Node name
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("getting hostname: %w", err)
	}
	res.NodeName = hostname

	// Pools
	res.Pools = discoverPools()

	// Storages
	res.Storages = discoverStorages()

	// Bridges
	res.Bridges = discoverBridges()

	// GPUs
	res.GPUs = DiscoverGPUs()

	return res, nil
}

func discoverPools() []string {
	out, err := exec.Command("pvesh", "get", "/pools", "--output-format", "json").Output()
	if err != nil {
		return nil
	}

	var pools []struct {
		PoolID string `json:"poolid"`
	}
	if err := json.Unmarshal(out, &pools); err != nil {
		return nil
	}

	names := make([]string, 0, len(pools))
	for _, p := range pools {
		names = append(names, p.PoolID)
	}
	return names
}

func discoverStorages() []StorageInfo {
	out, err := exec.Command("pvesh", "get", "/storage", "--output-format", "json").Output()
	if err != nil {
		return nil
	}

	var storages []struct {
		Storage string `json:"storage"`
		Type    string `json:"type"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(out, &storages); err != nil {
		return nil
	}

	// Filter to storages that can hold rootdir content
	var result []StorageInfo
	for _, s := range storages {
		if strings.Contains(s.Content, "rootdir") || strings.Contains(s.Content, "images") {
			result = append(result, StorageInfo{
				ID:      s.Storage,
				Type:    s.Type,
				Content: s.Content,
			})
		}
	}
	return result
}

func discoverBridges() []string {
	out, err := exec.Command("ip", "-brief", "link", "show").Output()
	if err != nil {
		return nil
	}

	var bridges []string
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 1 && strings.HasPrefix(fields[0], "vmbr") {
			bridges = append(bridges, fields[0])
		}
	}
	return bridges
}

func DiscoverGPUs() []GPUInfo {
	var gpus []GPUInfo

	// Intel/AMD DRI render nodes
	matches, _ := filepath.Glob("/dev/dri/renderD*")
	for _, m := range matches {
		gpus = append(gpus, GPUInfo{
			Path: m,
			Type: "intel",
			Name: fmt.Sprintf("DRI render node (%s)", filepath.Base(m)),
		})
	}

	// Try to refine DRI device names via sysfs
	for i := range gpus {
		base := filepath.Base(gpus[i].Path) // e.g. renderD128
		vendor := readSysfs("/sys/class/drm/" + base + "/device/vendor")
		switch vendor {
		case "0x8086":
			gpus[i].Type = "intel"
			gpus[i].Name = "Intel iGPU (" + base + ")"
		case "0x1002":
			gpus[i].Type = "amd"
			gpus[i].Name = "AMD GPU (" + base + ")"
		}
	}

	// NVIDIA devices
	nvidiaMatches, _ := filepath.Glob("/dev/nvidia[0-9]*")
	for _, m := range nvidiaMatches {
		name := fmt.Sprintf("NVIDIA GPU (%s)", filepath.Base(m))
		// Try nvidia-smi for a friendly name
		if friendly := nvidiaSMIName(m); friendly != "" {
			name = friendly
		}
		gpus = append(gpus, GPUInfo{
			Path: m,
			Type: "nvidia",
			Name: name,
		})
	}

	return gpus
}

func readSysfs(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func nvidiaSMIName(devicePath string) string {
	out, err := exec.Command("nvidia-smi", "--query-gpu=name", "--format=csv,noheader,nounits").Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0])
	}
	return ""
}
