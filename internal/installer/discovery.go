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
	Type string // "intel", "amd", "nvidia", "aspeed", "unknown"
	Name string
}

// pciVendors maps PCI vendor IDs to GPU type and label prefix.
var pciVendors = map[string]struct{ typ, label string }{
	"0x8086": {"intel", "Intel"},
	"0x1002": {"amd", "AMD"},
	"0x10de": {"nvidia", "NVIDIA"},
	"0x1a03": {"aspeed", "ASPEED"},
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

	// GPUs (nil = use pvesh, runs as root in TUI installer)
	res.GPUs = DiscoverGPUs(nil)

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

// PCIGPUInfo holds GPU-class PCI device info for use by DiscoverGPUs.
// Can be populated from the Proxmox REST API or from pvesh (TUI installer).
type PCIGPUInfo struct {
	ID         string // PCI address e.g. "0000:61:00.0"
	Vendor     string // vendor hex ID e.g. "0x10de"
	VendorName string // e.g. "NVIDIA Corporation"
	DeviceName string // e.g. "GA104 [GeForce RTX 3060 Ti]"
}

// pciDevice is the JSON shape returned by pvesh get /nodes/{node}/hardware/pci.
type pciDevice struct {
	ID         string `json:"id"`
	Class      string `json:"class"`
	Vendor     string `json:"vendor"`
	VendorName string `json:"vendor_name"`
	Device     string `json:"device"`
	DeviceName string `json:"device_name"`
}

// queryPCIGPUsPvesh queries the Proxmox PCI API via pvesh (requires root).
// Used by the TUI installer. Returns nil if pvesh is unavailable.
func queryPCIGPUsPvesh() []PCIGPUInfo {
	hostname, err := os.Hostname()
	if err != nil {
		return nil
	}
	out, err := exec.Command("pvesh", "get",
		fmt.Sprintf("/nodes/%s/hardware/pci", hostname),
		"--output-format", "json").Output()
	if err != nil {
		return nil
	}
	var devices []pciDevice
	if err := json.Unmarshal(out, &devices); err != nil {
		return nil
	}
	var gpus []PCIGPUInfo
	for _, d := range devices {
		// 0x0300xx = VGA controller, 0x0302xx = 3D controller
		if len(d.Class) >= 6 {
			prefix := d.Class[:6]
			if prefix == "0x0300" || prefix == "0x0302" {
				gpus = append(gpus, PCIGPUInfo{
					ID:         d.ID,
					Vendor:     d.Vendor,
					VendorName: d.VendorName,
					DeviceName: d.DeviceName,
				})
			}
		}
	}
	return gpus
}

// buildPCIGPUMap converts a slice of PCIGPUInfo into a map keyed by PCI address.
func buildPCIGPUMap(devices []PCIGPUInfo) map[string]PCIGPUInfo {
	if len(devices) == 0 {
		return nil
	}
	m := make(map[string]PCIGPUInfo, len(devices))
	for _, d := range devices {
		m[d.ID] = d
	}
	return m
}

// pciAddrFromSysfs resolves a sysfs device path to a PCI address (e.g. "0000:61:00.0").
func pciAddrFromSysfs(sysfsDevicePath string) string {
	link, err := os.Readlink(sysfsDevicePath)
	if err != nil {
		return ""
	}
	return filepath.Base(link)
}

// DiscoverGPUs detects GPU device nodes and enriches them with PCI device info.
// If pciDevices is non-nil, it is used directly (server context, pre-fetched via Proxmox REST API).
// If pciDevices is nil, falls back to pvesh (TUI installer, runs as root).
func DiscoverGPUs(pciDevices []PCIGPUInfo) []GPUInfo {
	var gpus []GPUInfo

	// Build PCI address → device info map
	if pciDevices == nil {
		pciDevices = queryPCIGPUsPvesh()
	}
	pciGPUs := buildPCIGPUMap(pciDevices)

	// DRI render nodes
	matches, _ := filepath.Glob("/dev/dri/renderD*")
	for _, m := range matches {
		base := filepath.Base(m)
		typ := "unknown"
		name := fmt.Sprintf("DRI render node (%s)", base)

		pciAddr := pciAddrFromSysfs("/sys/class/drm/" + base + "/device")
		if dev, ok := pciGPUs[pciAddr]; ok {
			if v, ok := pciVendors[dev.Vendor]; ok {
				typ = v.typ
			}
			name = strings.TrimSpace(dev.VendorName+" "+dev.DeviceName) + " (" + base + ")"
		} else {
			// Fallback: sysfs vendor ID only
			vendor := readSysfs("/sys/class/drm/" + base + "/device/vendor")
			if v, ok := pciVendors[vendor]; ok {
				typ = v.typ
				name = v.label + " GPU (" + base + ")"
			}
		}

		gpus = append(gpus, GPUInfo{Path: m, Type: typ, Name: name})
	}

	// NVIDIA device nodes (/dev/nvidia0, etc.)
	nvidiaMatches, _ := filepath.Glob("/dev/nvidia[0-9]*")
	nvidiaAddrs := nvidiaPCIAddrs()
	for _, m := range nvidiaMatches {
		base := filepath.Base(m)
		name := fmt.Sprintf("NVIDIA GPU (%s)", base)

		// nvidia0 → first PCI addr, nvidia1 → second, etc.
		idx := 0
		fmt.Sscanf(base, "nvidia%d", &idx)
		if idx < len(nvidiaAddrs) {
			if dev, ok := pciGPUs[nvidiaAddrs[idx]]; ok {
				name = strings.TrimSpace(dev.VendorName+" "+dev.DeviceName) + " (" + base + ")"
			}
		}

		gpus = append(gpus, GPUInfo{Path: m, Type: "nvidia", Name: name})
	}

	return gpus
}

// nvidiaPCIAddrs returns PCI addresses bound to the nvidia driver, sorted.
func nvidiaPCIAddrs() []string {
	entries, err := filepath.Glob("/sys/bus/pci/drivers/nvidia/0000:*")
	if err != nil || len(entries) == 0 {
		return nil
	}
	var addrs []string
	for _, e := range entries {
		addrs = append(addrs, filepath.Base(e))
	}
	return addrs
}

func readSysfs(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
