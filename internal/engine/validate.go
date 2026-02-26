package engine

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

// hostnameRe validates RFC-952/RFC-1123 hostnames: alphanumeric with hyphens, max 63 chars.
var hostnameRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?$`)

// bridgeRe validates Proxmox bridge names (vmbr0, vmbr1, ...).
var bridgeRe = regexp.MustCompile(`^vmbr[0-9]+$`)

// ipRe validates IPv4 addresses, optionally with CIDR prefix length.
var ipRe = regexp.MustCompile(`^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}(/[0-9]{1,2})?$`)

// macRe validates MAC addresses in colon-separated hex format.
var macRe = regexp.MustCompile(`^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$`)

// envKeyRe validates environment variable key names.
var envKeyValidateRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// validateCommonInputs validates fields shared between single-app and stack installs:
// hostname format, bridge name format, IP address format, MAC address format (unicast check),
// environment variable key safety, and device passthrough paths.
func validateCommonInputs(hostname, bridge, ip, mac string, envVars map[string]string, devices []DevicePassthrough) error {
	if hostname != "" && !hostnameRe.MatchString(hostname) {
		return fmt.Errorf("invalid hostname %q (must be alphanumeric with hyphens, max 63 chars)", hostname)
	}

	if bridge != "" && !bridgeRe.MatchString(bridge) {
		return fmt.Errorf("invalid bridge name %q (must match vmbr[0-9]+)", bridge)
	}

	if ip != "" && ip != "dhcp" {
		if !ipRe.MatchString(ip) {
			return fmt.Errorf("invalid IP address %q", ip)
		}
		// Additional semantic validation: each octet must be 0-255
		parts := strings.Split(strings.Split(ip, "/")[0], ".")
		for _, p := range parts {
			var octet int
			fmt.Sscanf(p, "%d", &octet)
			if octet > 255 {
				return fmt.Errorf("invalid IP address %q", ip)
			}
		}
		if strings.Contains(ip, "/") {
			cidr := strings.Split(ip, "/")[1]
			var prefix int
			fmt.Sscanf(cidr, "%d", &prefix)
			if prefix > 32 {
				return fmt.Errorf("invalid IP address %q", ip)
			}
		}
	}

	if mac != "" {
		upper := strings.ToUpper(mac)
		if !macRe.MatchString(upper) {
			return fmt.Errorf("invalid MAC address %q (expected XX:XX:XX:XX:XX:XX)", mac)
		}
		// Unicast check: LSB of first byte must be 0
		hwAddr, err := net.ParseMAC(upper)
		if err != nil {
			return fmt.Errorf("invalid MAC address %q: %v", mac, err)
		}
		if hwAddr[0]&0x01 != 0 {
			return fmt.Errorf("invalid MAC address %q: must be a unicast address (first octet %02X has multicast bit set)", mac, hwAddr[0])
		}
	}

	for k := range envVars {
		if !envKeyValidateRe.MatchString(k) {
			return fmt.Errorf("environment variable key %q is invalid (must match [A-Za-z_][A-Za-z0-9_]*)", k)
		}
		if reservedEnvKeys[strings.ToUpper(k)] {
			return fmt.Errorf("environment variable %q is reserved and cannot be overridden", k)
		}
	}

	if err := ValidateDevices(devices); err != nil {
		return err
	}

	return nil
}
