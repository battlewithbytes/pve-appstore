package config

import (
	"fmt"
	"strings"
)

const (
	// Filesystem paths
	DefaultConfigPath = "/etc/pve-appstore/config.yml"
	DefaultDataDir    = "/var/lib/pve-appstore"
	DefaultLogDir     = "/var/log/pve-appstore"
	DefaultInstallDir = "/opt/pve-appstore"

	// Service defaults
	DefaultBindAddress = "0.0.0.0"
	DefaultPort        = 8088

	// Container defaults
	DefaultCores    = 2
	DefaultMemoryMB = 2048
	DefaultDiskGB   = 8

	// System user
	ServiceUser  = "appstore"
	ServiceGroup = "appstore"

	// Catalog defaults
	DefaultCatalogURL    = "https://github.com/battlewithbytes/pve-appstore-catalog.git"
	DefaultCatalogBranch = "main"

	// Auth modes
	AuthModeNone     = "none"
	AuthModePassword = "password"

	// GPU policies
	GPUPolicyNone      = "none"
	GPUPolicyAllow     = "allow"
	GPUPolicyAllowlist = "allowlist"

	// Catalog refresh schedules
	RefreshDaily  = "daily"
	RefreshWeekly = "weekly"
	RefreshManual = "manual"
)

// ParseGitHubRepo extracts owner and repo from a GitHub URL.
// Supports "https://github.com/owner/repo.git" and "https://github.com/owner/repo".
func ParseGitHubRepo(repoURL string) (owner, repo string, err error) {
	u := strings.TrimSuffix(repoURL, ".git")

	// Remove scheme prefix
	for _, prefix := range []string{"https://github.com/", "http://github.com/"} {
		if strings.HasPrefix(u, prefix) {
			u = strings.TrimPrefix(u, prefix)
			parts := strings.SplitN(u, "/", 3)
			if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
				return "", "", fmt.Errorf("invalid GitHub URL: %s", repoURL)
			}
			return parts[0], parts[1], nil
		}
	}

	return "", "", fmt.Errorf("not a GitHub URL: %s", repoURL)
}
