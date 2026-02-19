// Package updater checks GitHub releases for new versions and applies binary updates.
package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ReleaseInfo describes a GitHub release.
type ReleaseInfo struct {
	Version     string `json:"version"`
	PublishedAt string `json:"published_at"`
	URL         string `json:"url"`
	DownloadURL string `json:"download_url"`
}

// UpdateStatus is the JSON response for update-check.
type UpdateStatus struct {
	Current   string       `json:"current"`
	Latest    string       `json:"latest"`
	Available bool         `json:"available"`
	Release   *ReleaseInfo `json:"release,omitempty"`
	CheckedAt string       `json:"checked_at"`
}

// Updater manages update checks with caching.
type Updater struct {
	mu     sync.Mutex
	cached *UpdateStatus
	ttl    time.Duration
}

// New creates an Updater with a 1-hour cache TTL.
func New() *Updater {
	return &Updater{ttl: 1 * time.Hour}
}

const (
	releasesURL = "https://api.github.com/repos/battlewithbytes/pve-appstore/releases/latest"
	UpdateScript = "/opt/pve-appstore/update.sh"
	BinaryPath   = "/opt/pve-appstore/pve-appstore"
	TempBinary   = "/var/lib/pve-appstore/pve-appstore.new"
)

// githubRelease is the subset of the GitHub API response we need.
type githubRelease struct {
	TagName     string        `json:"tag_name"`
	PublishedAt string        `json:"published_at"`
	HTMLURL     string        `json:"html_url"`
	Assets      []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// CheckLatestRelease checks GitHub for the latest release and compares with currentVersion.
// Results are cached for the configured TTL.
func (u *Updater) CheckLatestRelease(currentVersion string) (*UpdateStatus, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	// Return cached result if fresh
	if u.cached != nil {
		checkedAt, err := time.Parse(time.RFC3339, u.cached.CheckedAt)
		if err == nil && time.Since(checkedAt) < u.ttl {
			// Update current version in case of restart with new version
			result := *u.cached
			result.Current = currentVersion
			result.Available = isNewerVersion(result.Latest, currentVersion)
			return &result, nil
		}
	}

	rel, err := fetchLatestRelease()
	if err != nil {
		return nil, err
	}

	latestVersion := strings.TrimPrefix(rel.TagName, "v")
	suffix := archSuffix()
	var downloadURL string
	for _, a := range rel.Assets {
		if strings.Contains(a.Name, suffix) {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}

	status := &UpdateStatus{
		Current:   currentVersion,
		Latest:    latestVersion,
		Available: isNewerVersion(latestVersion, currentVersion),
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if status.Available && downloadURL != "" {
		status.Release = &ReleaseInfo{
			Version:     latestVersion,
			PublishedAt: rel.PublishedAt,
			URL:         rel.HTMLURL,
			DownloadURL: downloadURL,
		}
	}

	u.cached = status
	return status, nil
}

// InvalidateCache clears the cached update check.
func (u *Updater) InvalidateCache() {
	u.mu.Lock()
	u.cached = nil
	u.mu.Unlock()
}

// DownloadBinary downloads the release binary to destPath.
func DownloadBinary(downloadURL, destPath string) error {
	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("downloading binary: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}

	f, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		os.Remove(destPath)
		return fmt.Errorf("writing binary: %w", err)
	}

	return nil
}

// ApplyUpdateDirect replaces the binary and restarts the service (runs as root).
// Uses remove+rename to avoid "text file busy" on running binaries.
func ApplyUpdateDirect(newBinaryPath, targetPath string) error {
	// Backup current binary (copy, not rename, so target inode stays until removed)
	if err := copyFile(targetPath, targetPath+".bak"); err != nil {
		return fmt.Errorf("backing up current binary: %w", err)
	}

	// Remove the old binary first — Linux allows unlinking a running executable,
	// the kernel keeps the inode alive until the process exits.
	if err := os.Remove(targetPath); err != nil {
		return fmt.Errorf("removing old binary: %w", err)
	}

	// Rename new binary into place (atomic on same filesystem)
	if err := os.Rename(newBinaryPath, targetPath); err != nil {
		// Fallback: copy if rename fails (cross-device)
		if err2 := copyFile(newBinaryPath, targetPath); err2 != nil {
			return fmt.Errorf("installing new binary: %w", err2)
		}
		os.Remove(newBinaryPath)
	}
	os.Chmod(targetPath, 0755)

	// Restart service
	if err := exec.Command("systemctl", "restart", "pve-appstore").Run(); err != nil {
		return fmt.Errorf("restarting service: %w", err)
	}
	return nil
}

// ApplyUpdateSudo runs the update script via sudo (for web-triggered updates).
// Uses nsenter to escape the service's ProtectSystem=strict mount namespace,
// and setsid to detach so the process survives the service restart.
func ApplyUpdateSudo(newBinaryPath string) error {
	cmd := exec.Command("setsid",
		"sudo", "/usr/bin/nsenter", "--mount=/proc/1/ns/mnt", "--",
		UpdateScript, newBinaryPath,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting update script: %w", err)
	}
	// Don't wait — the script will restart our process
	return nil
}

// DeployUpdateScript writes the update.sh helper script.
func DeployUpdateScript() error {
	script := `#!/bin/bash
set -e
NEW="$1"
TARGET="/opt/pve-appstore/pve-appstore"
[ -f "$NEW" ] || { echo "new binary not found: $NEW"; exit 1; }
cp "$TARGET" "${TARGET}.bak" 2>/dev/null || true
rm -f "$TARGET"
mv "$NEW" "$TARGET"
chmod 0755 "$TARGET"
systemctl restart pve-appstore
`
	if err := os.WriteFile(UpdateScript, []byte(script), 0755); err != nil {
		return fmt.Errorf("writing update script: %w", err)
	}
	return nil
}

// ScriptExists checks if the update.sh helper is in place.
func ScriptExists() bool {
	_, err := os.Stat(UpdateScript)
	return err == nil
}

// fetchLatestRelease fetches the latest release from GitHub API.
func fetchLatestRelease() (*githubRelease, error) {
	req, err := http.NewRequest("GET", releasesURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "pve-appstore-updater")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GitHub API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decoding GitHub response: %w", err)
	}
	return &rel, nil
}

// archSuffix returns the binary name suffix for the current platform.
func archSuffix() string {
	return "linux-" + runtime.GOARCH
}

// isNewerVersion returns true if latest is strictly greater than current.
// Parses "major.minor.patch" semver (strips leading "v").
func isNewerVersion(latest, current string) bool {
	parse := func(s string) (int, int, int, bool) {
		s = strings.TrimPrefix(s, "v")
		parts := strings.SplitN(s, ".", 3)
		if len(parts) != 3 {
			return 0, 0, 0, false
		}
		major, e1 := strconv.Atoi(parts[0])
		minor, e2 := strconv.Atoi(parts[1])
		patchStr := strings.SplitN(parts[2], "-", 2)[0]
		patch, e3 := strconv.Atoi(patchStr)
		if e1 != nil || e2 != nil || e3 != nil {
			return 0, 0, 0, false
		}
		return major, minor, patch, true
	}

	lMaj, lMin, lPat, lok := parse(latest)
	cMaj, cMin, cPat, cok := parse(current)
	if !lok || !cok {
		return latest != current && latest > current
	}

	if lMaj != cMaj {
		return lMaj > cMaj
	}
	if lMin != cMin {
		return lMin > cMin
	}
	return lPat > cPat
}

// copyFile copies src to dst.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0755)
}
