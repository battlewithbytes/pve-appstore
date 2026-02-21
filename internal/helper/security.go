package helper

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// --- S1: CTID Ownership Verification ---

// validateCTID checks that a CTID is within range and belongs to a managed
// container (present in the installs or stacks table with active status).
func (s *Server) validateCTID(ctid int) error {
	if ctid < 100 || ctid > 999999999 {
		return fmt.Errorf("CTID %d out of valid range [100, 999999999]", ctid)
	}
	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM installs WHERE ctid=? AND status != 'uninstalled'",
		ctid,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("database error checking CTID %d: %w", ctid, err)
	}
	if count > 0 {
		return nil
	}
	// Check stacks table (may not exist yet)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM stacks WHERE ctid=?", ctid).Scan(&count)
	if count > 0 {
		return nil
	}
	// Also check running jobs — a container being installed has a CTID but
	// may not yet be in the installs table.
	count = 0
	_ = s.db.QueryRow(
		"SELECT COUNT(*) FROM jobs WHERE ctid=? AND status='running'",
		ctid,
	).Scan(&count)
	if count > 0 {
		return nil
	}
	return fmt.Errorf("CTID %d is not a managed container", ctid)
}

// --- S2: pct set Option Strict Allowlist ---

var (
	devOptionRe = regexp.MustCompile(`^-dev[0-9]{1,2}$`)
	mpOptionRe  = regexp.MustCompile(`^-mp[0-9]{1,2}$`)
)

// validatePctSetOption checks that a pct set option is in the strict allowlist.
// Only -dev[N] and -mp[N] are permitted.
func validatePctSetOption(option string) error {
	if devOptionRe.MatchString(option) {
		return nil
	}
	if mpOptionRe.MatchString(option) {
		return nil
	}
	return fmt.Errorf("pct set option %q is not allowed (only -devN and -mpN are permitted)", option)
}

// allowedDevicePatterns are the only device paths permitted for passthrough.
// Duplicated from engine/security.go to avoid importing engine (helper is independent).
var allowedDevicePatterns = []*regexp.Regexp{
	regexp.MustCompile(`^/dev/dri/(card|render)\d+$`),
	regexp.MustCompile(`^/dev/nvidia\d*$`),
	regexp.MustCompile(`^/dev/nvidia-uvm(-tools)?$`),
	regexp.MustCompile(`^/dev/nvidiactl$`),
	regexp.MustCompile(`^/dev/net/tun$`),
}

// validateDevicePath checks that a device path matches the allowed patterns.
func validateDevicePath(path string) error {
	for _, pat := range allowedDevicePatterns {
		if pat.MatchString(path) {
			return nil
		}
	}
	return fmt.Errorf("device path %q is not in the allowed list", path)
}

// safeDeviceGIDs are GIDs allowed for device passthrough.
var safeDeviceGIDs = map[int]bool{
	0:   true, // root
	44:  true, // video
	195: true, // nvidia (some distros)
}

var deviceModeRe = regexp.MustCompile(`^0[0-7]{3}$`)

// validateDevValue validates the value for a -devN pct set option.
// Format: /dev/path[,gid=N][,mode=NNNN]
func validateDevValue(value string) error {
	parts := strings.Split(value, ",")
	if len(parts) == 0 || parts[0] == "" {
		return fmt.Errorf("empty device value")
	}
	// First part is the device path
	if err := validateDevicePath(parts[0]); err != nil {
		return err
	}
	// Remaining parts are key=value options
	for _, kv := range parts[1:] {
		eqIdx := strings.IndexByte(kv, '=')
		if eqIdx < 0 {
			return fmt.Errorf("invalid device option %q (expected key=value)", kv)
		}
		key := kv[:eqIdx]
		val := kv[eqIdx+1:]
		switch key {
		case "gid":
			var gid int
			if _, err := fmt.Sscanf(val, "%d", &gid); err != nil {
				return fmt.Errorf("invalid GID %q", val)
			}
			if !safeDeviceGIDs[gid] {
				return fmt.Errorf("GID %d is not in the allowed list", gid)
			}
		case "mode":
			if !deviceModeRe.MatchString(val) {
				return fmt.Errorf("invalid device mode %q (must be octal like 0666)", val)
			}
		default:
			return fmt.Errorf("unknown device option %q", key)
		}
	}
	return nil
}

// validateMpValue validates the value for a -mpN pct set option.
// Format: /host/path,mp=/container/path[,ro=1]
func (s *Server) validateMpValue(value string) error {
	parts := strings.Split(value, ",")
	if len(parts) < 2 {
		return fmt.Errorf("mount point value must have at least host_path,mp=/container/path")
	}
	hostPath := parts[0]
	var containerPath string
	for _, kv := range parts[1:] {
		eqIdx := strings.IndexByte(kv, '=')
		if eqIdx < 0 {
			return fmt.Errorf("invalid mount option %q", kv)
		}
		key := kv[:eqIdx]
		val := kv[eqIdx+1:]
		switch key {
		case "mp":
			containerPath = val
		case "ro":
			if val != "0" && val != "1" {
				return fmt.Errorf("invalid ro value %q (must be 0 or 1)", val)
			}
		default:
			return fmt.Errorf("unknown mount option %q", key)
		}
	}
	if containerPath == "" {
		return fmt.Errorf("mount point must specify mp=/container/path")
	}
	if !filepath.IsAbs(containerPath) {
		return fmt.Errorf("container path must be absolute")
	}
	// Validate host path
	return s.validateStoragePath(hostPath)
}

// --- S3: Path Validation with Symlink Resolution ---

// denyListPaths are host paths that must never be targets of fs operations.
// Duplicated from engine/security.go for independence.
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

// validatePath validates a path for filesystem operations. It:
// 1. Cleans the path (normalize, remove ..)
// 2. Resolves symlinks on the parent directory
// 3. Checks the resolved path is under an allowed storage root
// 4. Checks against the deny list
func (s *Server) validatePath(path string) error {
	if path == "" {
		return fmt.Errorf("empty path")
	}
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		return fmt.Errorf("path must be absolute: %q", path)
	}

	// Check deny list on the cleaned path first (fast reject)
	if err := checkDenyList(cleaned); err != nil {
		return err
	}

	// Resolve symlinks on the parent directory
	parent := filepath.Dir(cleaned)
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		// Parent may not exist yet (e.g. mkdir -p) — validate the
		// nearest existing ancestor instead
		resolvedParent, err = resolveNearestAncestor(parent)
		if err != nil {
			return fmt.Errorf("cannot resolve path %q: %w", path, err)
		}
	}
	resolvedPath := filepath.Join(resolvedParent, filepath.Base(cleaned))

	// Check deny list on resolved path
	if err := checkDenyList(resolvedPath); err != nil {
		return err
	}

	// Check that resolved path is under an allowed storage root
	return s.validateStoragePath(resolvedPath)
}

// validateStoragePath checks that a path is under an allowed storage root.
func (s *Server) validateStoragePath(path string) error {
	cfg := s.getConfig()

	// Build allowed roots from config storages
	// Note: we resolve storage paths dynamically from the config
	allowedRoots := []string{
		"/var/lib/pve-appstore/tmp",
	}

	// Add well-known storage type paths
	// These cover common Proxmox storage configurations
	commonStorageRoots := []string{
		"/mnt/",   // common for manual mounts, NFS, CIFS
		"/tank/",  // common ZFS pool
		"/data/",  // common generic data dir
		"/rpool/", // common root ZFS pool
	}
	allowedRoots = append(allowedRoots, commonStorageRoots...)

	// Add browsable storages from config (the main service resolves these
	// via the Proxmox API; here we use a static list from config)
	_ = cfg // storages are string IDs, not paths — we need runtime resolution
	// For now, allow paths under common storage mount patterns.
	// The actual storage path validation happens via the isUnderRoot check below.

	// Also allow /usr/lib/nvidia paths (for GPU library bind mounts)
	if strings.HasPrefix(path, "/usr/lib/nvidia") ||
		strings.HasPrefix(path, "/usr/lib/x86_64-linux-gnu/nvidia") {
		return nil
	}

	cleaned := filepath.Clean(path)
	for _, root := range allowedRoots {
		if cleaned == root || strings.HasPrefix(cleaned, root) {
			return nil
		}
	}

	return fmt.Errorf("path %q is not under an allowed storage root", path)
}

// validateRmPath validates a path for removal. In addition to standard path
// validation, it ensures the target is not a storage root itself.
func (s *Server) validateRmPath(path string) error {
	if err := s.validatePath(path); err != nil {
		return err
	}

	// Use Lstat to detect symlinks at the leaf
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // removing non-existent path is a no-op
		}
		return fmt.Errorf("cannot stat %q: %w", path, err)
	}

	// If the target is a symlink, resolve and re-validate
	if info.Mode()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return fmt.Errorf("cannot resolve symlink %q: %w", path, err)
		}
		if err := checkDenyList(resolved); err != nil {
			return fmt.Errorf("symlink target: %w", err)
		}
	}

	return nil
}

// validatePushSrc validates the source path for pct push. Must be under
// /var/lib/pve-appstore/tmp/ or a catalog/dev-app path, and a regular file.
func validatePushSrc(src string) error {
	cleaned := filepath.Clean(src)
	if !filepath.IsAbs(cleaned) {
		return fmt.Errorf("push src must be absolute: %q", src)
	}

	// Resolve symlinks
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		return fmt.Errorf("cannot resolve push src %q: %w", src, err)
	}

	// Must be under allowed source directories
	allowedPrefixes := []string{
		"/var/lib/pve-appstore/tmp/",
		"/var/lib/pve-appstore/catalog/",
		"/var/lib/pve-appstore/dev-apps/",
	}
	allowed := false
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(resolved, prefix) {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("push src %q is not under an allowed directory", src)
	}

	// Must be a regular file
	info, err := os.Lstat(resolved)
	if err != nil {
		return fmt.Errorf("push src %q: %w", src, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("push src %q is not a regular file", src)
	}

	return nil
}

// checkDenyList checks if a path is in or under a denied path.
func checkDenyList(path string) error {
	for _, denied := range denyListPaths {
		if path == denied || strings.HasPrefix(path, denied+"/") {
			return fmt.Errorf("path %q is a restricted system path", path)
		}
	}
	return nil
}

// resolveNearestAncestor walks up the directory tree to find the nearest
// existing ancestor and resolves its symlinks.
func resolveNearestAncestor(path string) (string, error) {
	current := filepath.Clean(path)
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			return resolved, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "/", nil // reached root
		}
		current = parent
	}
}

// --- S4: LXC Config Line Validation ---

// allowedLXCKeys and their value validation rules.
var allowedLXCConfKeys = map[string]func(string) error{
	"lxc.cgroup2.devices.allow": validateCgroupDevicesAllow,
	"lxc.cgroup.devices.allow":  validateCgroupDevicesAllow,
	"lxc.mount.entry":           nil, // validated separately
	"lxc.mount.auto":            validateMountAuto,
	"lxc.environment":           validateEnvironment,
	"lxc.cgroup2.cpuset.cpus":   validateCpusetCpus,
}

// rejectedLXCKeys are keys that must NEVER be allowed.
var rejectedLXCKeys = map[string]bool{
	"lxc.apparmor.profile": true,
	"lxc.seccomp.profile":  true,
	"lxc.cap.drop":         true,
	"lxc.cap.keep":         true,
	"lxc.rootfs":           true,
	"lxc.idmap":            true,
	"lxc.init.cmd":         true,
}

// validateConfLine validates a single LXC config line (key and value).
func (s *Server) validateConfLine(line string) error {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	if strings.HasPrefix(line, "#") {
		return nil // comments are ok
	}
	if strings.HasPrefix(line, "-") {
		return fmt.Errorf("config line must not start with '-': %q", line)
	}

	// Parse key = value (or key: value)
	var key, value string
	for i, c := range line {
		if c == '=' || c == ':' {
			key = strings.TrimSpace(line[:i])
			value = strings.TrimSpace(line[i+1:])
			break
		}
	}
	if key == "" {
		return fmt.Errorf("invalid config line (no key): %q", line)
	}

	// Check rejected keys
	if rejectedLXCKeys[key] {
		return fmt.Errorf("LXC config key %q is explicitly forbidden", key)
	}

	// Check allowed keys
	validator, ok := allowedLXCConfKeys[key]
	if !ok {
		return fmt.Errorf("LXC config key %q is not in the allowed list", key)
	}

	// Validate value if a validator is registered
	if validator != nil {
		if err := validator(value); err != nil {
			return fmt.Errorf("LXC config %q value invalid: %w", key, err)
		}
	}

	// Special case: lxc.mount.entry needs path validation
	if key == "lxc.mount.entry" {
		return s.validateMountEntry(value)
	}

	return nil
}

var cgroupDevicesAllowRe = regexp.MustCompile(`^[cb] [0-9]+:[0-9*]+ [rwm]+$`)

func validateCgroupDevicesAllow(value string) error {
	value = strings.TrimSpace(value)
	if value == "a" || strings.HasPrefix(value, "a ") {
		return fmt.Errorf("'allow all' device access (a) is not permitted")
	}
	if !cgroupDevicesAllowRe.MatchString(value) {
		return fmt.Errorf("must be '[cb] major:minor perms' (e.g. 'c 195:* rwm')")
	}
	return nil
}

var mountAutoAllowed = map[string]bool{
	"proc:rw":     true,
	"proc:mixed":  true,
	"proc:ro":     true,
	"sys:ro":      true,
	"sys:mixed":   true,
	"cgroup:ro":   true,
	"cgroup:rw":   true,
	"cgroup:mixed": true,
}

func validateMountAuto(value string) error {
	value = strings.TrimSpace(value)
	entries := strings.Fields(value)
	for _, entry := range entries {
		if !mountAutoAllowed[entry] {
			return fmt.Errorf("mount.auto value %q is not allowed", entry)
		}
	}
	return nil
}

var envKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validateEnvironment(value string) error {
	// Format: KEY=VALUE
	eqIdx := strings.IndexByte(value, '=')
	if eqIdx < 0 {
		return fmt.Errorf("must be KEY=VALUE format")
	}
	key := value[:eqIdx]
	val := value[eqIdx+1:]
	if !envKeyRe.MatchString(key) {
		return fmt.Errorf("invalid environment key %q", key)
	}
	if len(val) > 4096 {
		return fmt.Errorf("environment value exceeds 4096 bytes")
	}
	return nil
}

var cpusetRe = regexp.MustCompile(`^[0-9, -]+$`)

func validateCpusetCpus(value string) error {
	value = strings.TrimSpace(value)
	if !cpusetRe.MatchString(value) {
		return fmt.Errorf("must contain only digits, commas, and hyphens")
	}
	return nil
}

// validateMountEntry validates an lxc.mount.entry value.
// Source path must be under allowed storage roots or known-safe GPU paths.
func (s *Server) validateMountEntry(value string) error {
	fields := strings.Fields(value)
	if len(fields) < 2 {
		return fmt.Errorf("mount entry must have at least source and destination")
	}
	src := fields[0]
	// Skip validation for special filesystems
	if src == "none" || src == "proc" || src == "sysfs" || src == "tmpfs" || src == "cgroup" {
		return nil
	}
	// Validate source path
	if filepath.IsAbs(src) {
		// Allow known-safe GPU paths
		if strings.HasPrefix(src, "/dev/dri/") ||
			strings.HasPrefix(src, "/dev/nvidia") ||
			strings.HasPrefix(src, "/usr/lib/nvidia") ||
			strings.HasPrefix(src, "/usr/lib/x86_64-linux-gnu/nvidia") {
			return nil
		}
		// Validate against storage roots
		return s.validateStoragePath(src)
	}
	return nil
}

// --- S5: Chown UID/GID Allowlist ---

// allowedChownPairs are the only UID:GID combinations permitted for chown.
var allowedChownPairs = map[[2]int]bool{
	{100000, 100000}: true, // standard unprivileged container root mapping
}

func validateChownOwnership(uid, gid int) error {
	if !allowedChownPairs[[2]int{uid, gid}] {
		return fmt.Errorf("chown to %d:%d is not allowed (only unprivileged container mappings permitted)", uid, gid)
	}
	return nil
}

// --- S6: Terminal Shell Allowlist ---

var allowedShells = map[string]bool{
	"/bin/bash": true,
	"/bin/sh":   true,
	"/bin/ash":  true,
	"/bin/zsh":  true,
}

func validateShell(shell string) error {
	if !allowedShells[shell] {
		return fmt.Errorf("shell %q is not allowed (only bash, sh, ash, zsh are permitted)", shell)
	}
	// No arguments, spaces, or semicolons allowed in shell path
	if strings.ContainsAny(shell, " \t;|&") {
		return fmt.Errorf("shell path contains invalid characters")
	}
	return nil
}

// --- S7: Command validation ---

func validateCommand(command []string) error {
	if len(command) == 0 {
		return fmt.Errorf("empty command")
	}
	if len(command) > 1000 {
		return fmt.Errorf("too many arguments (%d, max 1000)", len(command))
	}
	for _, arg := range command {
		if strings.ContainsRune(arg, 0) {
			return fmt.Errorf("command argument contains null byte")
		}
	}
	return nil
}

// --- S10: Update path validation ---

const hardcodedUpdateBinaryPath = "/var/lib/pve-appstore/pve-appstore.new"

func validateUpdatePath() error {
	info, err := os.Lstat(hardcodedUpdateBinaryPath)
	if err != nil {
		return fmt.Errorf("update binary not found at %s: %w", hardcodedUpdateBinaryPath, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("update binary at %s is not a regular file", hardcodedUpdateBinaryPath)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("update binary at %s is a symlink", hardcodedUpdateBinaryPath)
	}
	return nil
}

// --- Perms validation ---

var permsRe = regexp.MustCompile(`^0?[0-7]{3}$`)

func validatePerms(perms string) error {
	if perms == "" {
		return nil
	}
	if !permsRe.MatchString(perms) {
		return fmt.Errorf("invalid permissions %q (must be octal like 0755)", perms)
	}
	return nil
}
