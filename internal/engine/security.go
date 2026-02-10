package engine

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// --- Bind mount path validation ---

// denyListPaths are host paths that must never be bind-mounted into containers.
var denyListPaths = []string{
	"/etc",
	"/proc",
	"/sys",
	"/dev",
	"/root",
	"/boot",
	"/usr",
	"/bin",
	"/sbin",
	"/lib",
	"/lib64",
	"/var/lib/pve-appstore",
	"/etc/pve",
	"/etc/pve-appstore",
}

// ValidateBindMountPath checks that a host path is not in the deny list.
func ValidateBindMountPath(hostPath string) error {
	if hostPath == "" {
		return nil
	}
	cleaned := filepath.Clean(hostPath)
	if !filepath.IsAbs(cleaned) {
		return fmt.Errorf("bind mount path must be absolute: %q", hostPath)
	}
	for _, denied := range denyListPaths {
		if cleaned == denied || strings.HasPrefix(cleaned, denied+"/") {
			return fmt.Errorf("bind mount path %q is not allowed (restricted system path)", hostPath)
		}
	}
	return nil
}

// --- Device passthrough validation ---

// allowedDevicePatterns are the only device paths permitted for passthrough.
var allowedDevicePatterns = []*regexp.Regexp{
	regexp.MustCompile(`^/dev/dri/(card|render)\d+$`),
	regexp.MustCompile(`^/dev/nvidia\d*$`),
	regexp.MustCompile(`^/dev/nvidia-uvm(-tools)?$`),
	regexp.MustCompile(`^/dev/nvidiactl$`),
	regexp.MustCompile(`^/dev/net/tun$`),
}

// ValidateDevicePath checks that a device path matches the allowed patterns.
func ValidateDevicePath(path string) error {
	for _, pat := range allowedDevicePatterns {
		if pat.MatchString(path) {
			return nil
		}
	}
	return fmt.Errorf("device path %q is not in the allowed list (only GPU and TUN devices permitted)", path)
}

// ValidateDevices validates all device passthroughs.
func ValidateDevices(devices []DevicePassthrough) error {
	for _, dev := range devices {
		if err := ValidateDevicePath(dev.Path); err != nil {
			return err
		}
		// Validate mode field if present (must be octal like 0666)
		if dev.Mode != "" {
			if matched, _ := regexp.MatchString(`^0[0-7]{3}$`, dev.Mode); !matched {
				return fmt.Errorf("device mode %q is invalid (must be octal like 0666)", dev.Mode)
			}
		}
	}
	return nil
}

// --- Environment variable validation ---

var validEnvKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// reservedEnvKeys are environment variable names that must not be overridden.
var reservedEnvKeys = map[string]bool{
	"PATH":             true,
	"LD_PRELOAD":       true,
	"LD_LIBRARY_PATH":  true,
	"PYTHONPATH":       true,
	"PYTHONUNBUFFERED":  true,
	"HOME":             true,
	"USER":             true,
	"SHELL":            true,
	"TERM":             true,
}

// ValidateEnvVars checks env var keys for safety.
func ValidateEnvVars(envVars map[string]string) error {
	for k := range envVars {
		if !validEnvKeyRe.MatchString(k) {
			return fmt.Errorf("environment variable key %q is invalid (must match [A-Za-z_][A-Za-z0-9_]*)", k)
		}
		if reservedEnvKeys[strings.ToUpper(k)] {
			return fmt.Errorf("environment variable %q is reserved and cannot be overridden", k)
		}
	}
	return nil
}

// --- LXC ExtraConfig validation ---

// allowedLXCConfigPrefixes are the only LXC config keys permitted in ExtraConfig.
var allowedLXCConfigPrefixes = []string{
	"lxc.cgroup2.devices.allow",
	"lxc.cgroup.devices.allow",
	"lxc.mount.entry",
	"lxc.mount.auto",
	"lxc.environment",
}

// ValidateExtraConfig validates LXC extra config lines against an allowlist.
func ValidateExtraConfig(lines []string) error {
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Must not start with - (pct set flag injection)
		if strings.HasPrefix(line, "-") {
			return fmt.Errorf("invalid extra LXC config line: %q - must not start with '-'", line)
		}
		// Must contain = or :
		if !strings.Contains(line, "=") && !strings.Contains(line, ":") {
			return fmt.Errorf("invalid extra LXC config line: %q - must be key=value or key: value", line)
		}
		// Extract key (before = or :)
		key := line
		for i, c := range line {
			if c == '=' || c == ':' {
				key = strings.TrimSpace(line[:i])
				break
			}
		}
		key = strings.TrimSpace(key)
		allowed := false
		for _, prefix := range allowedLXCConfigPrefixes {
			if key == prefix || strings.HasPrefix(key, prefix+".") {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("LXC config key %q is not in the allowed list", key)
		}
	}
	return nil
}

// --- Input validation for Proxmox parameters ---

var (
	validHostnameRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?$`)
	validBridgeRe   = regexp.MustCompile(`^vmbr[0-9]+$`)
	validIPRe       = regexp.MustCompile(`^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}(/[0-9]{1,2})?$`)
	validTagsRe     = regexp.MustCompile(`^[a-zA-Z0-9\-_;]+$`)
)

// ValidateHostname checks hostname format.
func ValidateHostname(hostname string) error {
	if hostname == "" {
		return nil
	}
	if !validHostnameRe.MatchString(hostname) {
		return fmt.Errorf("invalid hostname %q (must be alphanumeric with hyphens, max 63 chars)", hostname)
	}
	return nil
}

// ValidateBridge checks bridge name format.
func ValidateBridge(bridge string) error {
	if bridge == "" {
		return nil
	}
	if !validBridgeRe.MatchString(bridge) {
		return fmt.Errorf("invalid bridge name %q (must match vmbr[0-9]+)", bridge)
	}
	return nil
}

// ValidateIPAddress checks IP address format.
func ValidateIPAddress(ip string) error {
	if ip == "" || ip == "dhcp" {
		return nil
	}
	if !validIPRe.MatchString(ip) {
		return fmt.Errorf("invalid IP address %q", ip)
	}
	return nil
}

// ValidateTags checks tag format.
func ValidateTags(tags string) error {
	if tags == "" {
		return nil
	}
	if !validTagsRe.MatchString(tags) {
		return fmt.Errorf("invalid tags %q (must be alphanumeric with hyphens/underscores/semicolons)", tags)
	}
	return nil
}
