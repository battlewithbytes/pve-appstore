package config

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
