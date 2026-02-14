package engine

import (
	"testing"
)

// --- ValidateBindMountPath ---

func TestValidateBindMountPathValid(t *testing.T) {
	paths := []string{"/mnt/data", "/home/user/media", "/opt/apps", "/srv/share"}
	for _, p := range paths {
		if err := ValidateBindMountPath(p); err != nil {
			t.Errorf("ValidateBindMountPath(%q) = %v, want nil", p, err)
		}
	}
}

func TestValidateBindMountPathEmpty(t *testing.T) {
	if err := ValidateBindMountPath(""); err != nil {
		t.Errorf("ValidateBindMountPath(\"\") = %v, want nil", err)
	}
}

func TestValidateBindMountPathDenied(t *testing.T) {
	denied := []string{"/etc", "/proc", "/dev", "/root", "/boot", "/sys", "/usr", "/bin", "/sbin", "/lib", "/lib64"}
	for _, p := range denied {
		if err := ValidateBindMountPath(p); err == nil {
			t.Errorf("ValidateBindMountPath(%q) = nil, want error", p)
		}
	}
}

func TestValidateBindMountPathNestedDenied(t *testing.T) {
	nested := []string{"/etc/nginx", "/proc/1/status", "/dev/sda", "/root/.ssh"}
	for _, p := range nested {
		if err := ValidateBindMountPath(p); err == nil {
			t.Errorf("ValidateBindMountPath(%q) = nil, want error (nested deny)", p)
		}
	}
}

func TestValidateBindMountPathRelative(t *testing.T) {
	if err := ValidateBindMountPath("relative/path"); err == nil {
		t.Error("ValidateBindMountPath(\"relative/path\") = nil, want error")
	}
}

// --- ValidateDevicePath ---

func TestValidateDevicePathValid(t *testing.T) {
	valid := []string{
		"/dev/dri/render128",
		"/dev/dri/card0",
		"/dev/nvidia0",
		"/dev/nvidia1",
		"/dev/nvidia",
		"/dev/nvidiactl",
		"/dev/nvidia-uvm",
		"/dev/nvidia-uvm-tools",
		"/dev/net/tun",
	}
	for _, p := range valid {
		if err := ValidateDevicePath(p); err != nil {
			t.Errorf("ValidateDevicePath(%q) = %v, want nil", p, err)
		}
	}
}

func TestValidateDevicePathInvalid(t *testing.T) {
	invalid := []string{
		"/dev/sda",
		"/dev/sda1",
		"/dev/tty0",
		"/dev/mem",
		"",
		"/dev/dri/",
	}
	for _, p := range invalid {
		if err := ValidateDevicePath(p); err == nil {
			t.Errorf("ValidateDevicePath(%q) = nil, want error", p)
		}
	}
}

// --- ValidateDevices ---

func TestValidateDevicesValid(t *testing.T) {
	devices := []DevicePassthrough{
		{Path: "/dev/dri/render128", Mode: "0666"},
		{Path: "/dev/nvidia0"},
	}
	if err := ValidateDevices(devices); err != nil {
		t.Errorf("ValidateDevices = %v, want nil", err)
	}
}

func TestValidateDevicesInvalidPath(t *testing.T) {
	devices := []DevicePassthrough{{Path: "/dev/sda"}}
	if err := ValidateDevices(devices); err == nil {
		t.Error("ValidateDevices with /dev/sda = nil, want error")
	}
}

func TestValidateDevicesInvalidMode(t *testing.T) {
	devices := []DevicePassthrough{{Path: "/dev/dri/renderD128", Mode: "777"}}
	if err := ValidateDevices(devices); err == nil {
		t.Error("ValidateDevices with bad mode = nil, want error")
	}
}

func TestValidateDevicesEmpty(t *testing.T) {
	if err := ValidateDevices(nil); err != nil {
		t.Errorf("ValidateDevices(nil) = %v, want nil", err)
	}
}

// --- ValidateEnvVars ---

func TestValidateEnvVarsValid(t *testing.T) {
	vars := map[string]string{
		"MY_VAR":     "hello",
		"DB_HOST":    "localhost",
		"_INTERNAL":  "val",
		"APP_PORT_1": "8080",
	}
	if err := ValidateEnvVars(vars); err != nil {
		t.Errorf("ValidateEnvVars = %v, want nil", err)
	}
}

func TestValidateEnvVarsReserved(t *testing.T) {
	reserved := []string{"PATH", "LD_PRELOAD", "LD_LIBRARY_PATH", "HOME", "USER", "SHELL", "TERM"}
	for _, k := range reserved {
		vars := map[string]string{k: "value"}
		if err := ValidateEnvVars(vars); err == nil {
			t.Errorf("ValidateEnvVars(%q) = nil, want error (reserved)", k)
		}
	}
}

func TestValidateEnvVarsInvalidKey(t *testing.T) {
	invalid := []string{"1BAD", "has space", "has-dash", "a.b"}
	for _, k := range invalid {
		vars := map[string]string{k: "value"}
		if err := ValidateEnvVars(vars); err == nil {
			t.Errorf("ValidateEnvVars(%q) = nil, want error (invalid key)", k)
		}
	}
}

func TestValidateEnvVarsEmpty(t *testing.T) {
	if err := ValidateEnvVars(nil); err != nil {
		t.Errorf("ValidateEnvVars(nil) = %v, want nil", err)
	}
}

// --- ValidateExtraConfig ---

func TestValidateExtraConfigValid(t *testing.T) {
	lines := []string{
		"lxc.mount.entry = tmpfs /tmp tmpfs defaults 0 0",
		"lxc.mount.auto = proc:rw",
		"lxc.cgroup2.devices.allow = c 195:* rwm",
		"lxc.environment = NVIDIA_VISIBLE_DEVICES=all",
	}
	if err := ValidateExtraConfig(lines); err != nil {
		t.Errorf("ValidateExtraConfig = %v, want nil", err)
	}
}

func TestValidateExtraConfigDisallowed(t *testing.T) {
	lines := []string{"lxc.rootfs.path = /bad"}
	if err := ValidateExtraConfig(lines); err == nil {
		t.Error("ValidateExtraConfig with disallowed prefix = nil, want error")
	}
}

func TestValidateExtraConfigDashInjection(t *testing.T) {
	lines := []string{"-delete mp0"}
	if err := ValidateExtraConfig(lines); err == nil {
		t.Error("ValidateExtraConfig with dash prefix = nil, want error")
	}
}

func TestValidateExtraConfigEmptyLine(t *testing.T) {
	lines := []string{"", "  ", "lxc.mount.entry = foo bar"}
	if err := ValidateExtraConfig(lines); err != nil {
		t.Errorf("ValidateExtraConfig with empty lines = %v, want nil", err)
	}
}

func TestValidateExtraConfigNoDelimiter(t *testing.T) {
	lines := []string{"lxc.mount.entry no delimiter"}
	if err := ValidateExtraConfig(lines); err == nil {
		t.Error("ValidateExtraConfig without = or : = nil, want error")
	}
}

// --- ValidateHostname ---

func TestValidateHostnameValid(t *testing.T) {
	valid := []string{"myhost", "web-01", "a", "a1b2c3", ""}
	for _, h := range valid {
		if err := ValidateHostname(h); err != nil {
			t.Errorf("ValidateHostname(%q) = %v, want nil", h, err)
		}
	}
}

func TestValidateHostnameInvalid(t *testing.T) {
	invalid := []string{
		"-starts-with-dash",
		"has space",
		"has.dot",
		"has_underscore",
		"a!b",
	}
	for _, h := range invalid {
		if err := ValidateHostname(h); err == nil {
			t.Errorf("ValidateHostname(%q) = nil, want error", h)
		}
	}
}

func TestValidateHostnameTooLong(t *testing.T) {
	long := ""
	for i := 0; i < 64; i++ {
		long += "a"
	}
	if err := ValidateHostname(long); err == nil {
		t.Errorf("ValidateHostname(64 chars) = nil, want error")
	}
}

// --- ValidateBridge ---

func TestValidateBridgeValid(t *testing.T) {
	valid := []string{"vmbr0", "vmbr1", "vmbr99", ""}
	for _, b := range valid {
		if err := ValidateBridge(b); err != nil {
			t.Errorf("ValidateBridge(%q) = %v, want nil", b, err)
		}
	}
}

func TestValidateBridgeInvalid(t *testing.T) {
	invalid := []string{"eth0", "br0", "vmbr", "bridge0"}
	for _, b := range invalid {
		if err := ValidateBridge(b); err == nil {
			t.Errorf("ValidateBridge(%q) = nil, want error", b)
		}
	}
}

// --- ValidateIPAddress ---

func TestValidateIPAddressValid(t *testing.T) {
	valid := []string{"192.168.1.100", "10.0.0.1/24", "dhcp", ""}
	for _, ip := range valid {
		if err := ValidateIPAddress(ip); err != nil {
			t.Errorf("ValidateIPAddress(%q) = %v, want nil", ip, err)
		}
	}
}

func TestValidateIPAddressInvalid(t *testing.T) {
	// The regex only checks format (1-3 digits per octet), not range
	invalid := []string{"not-an-ip", "abc", "1234.1.1.1"}
	for _, ip := range invalid {
		if err := ValidateIPAddress(ip); err == nil {
			t.Errorf("ValidateIPAddress(%q) = nil, want error", ip)
		}
	}
}

// --- ValidateTags ---

func TestValidateTagsValid(t *testing.T) {
	valid := []string{"appstore;managed", "tag1;tag2;tag3", "simple", "with-dash_and_under", ""}
	for _, tags := range valid {
		if err := ValidateTags(tags); err != nil {
			t.Errorf("ValidateTags(%q) = %v, want nil", tags, err)
		}
	}
}

func TestValidateTagsInvalid(t *testing.T) {
	invalid := []string{"has space", "has!bang", "has@at"}
	for _, tags := range invalid {
		if err := ValidateTags(tags); err == nil {
			t.Errorf("ValidateTags(%q) = nil, want error", tags)
		}
	}
}
