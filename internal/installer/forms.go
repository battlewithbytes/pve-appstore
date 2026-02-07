package installer

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/battlewithbytes/pve-appstore/internal/config"
)

// BuildForm constructs the full TUI form from discovered resources.
func BuildForm(res *DiscoveredResources, answers *InstallerAnswers) *huh.Form {
	// Set defaults
	answers.CoresStr = fmt.Sprintf("%d", config.DefaultCores)
	answers.MemoryMBStr = fmt.Sprintf("%d", config.DefaultMemoryMB)
	answers.DiskGBStr = fmt.Sprintf("%d", config.DefaultDiskGB)
	answers.BindAddress = config.DefaultBindAddress
	answers.PortStr = fmt.Sprintf("%d", config.DefaultPort)
	answers.UnprivilegedOnly = true
	answers.AuthMode = config.AuthModePassword
	answers.AutoCreateToken = true
	answers.CatalogURL = config.DefaultCatalogURL
	answers.Branch = config.DefaultCatalogBranch
	answers.Refresh = config.RefreshDaily
	answers.GPUEnabled = len(res.GPUs) > 0
	answers.GPUPolicy = config.GPUPolicyAllow

	groups := []*huh.Group{
		welcomeGroup(res),
		poolSelectGroup(res, answers),
		newPoolGroup(answers),
		placementGroup(res, answers),
		resourcesGroup(answers),
		securityGroup(answers),
		serviceGroup(answers),
		authModeGroup(answers),
		passwordGroup(answers),
		proxmoxAutoGroup(answers),
		proxmoxManualGroup(answers),
		catalogGroup(answers),
	}

	// GPU groups (only if GPUs detected)
	if len(res.GPUs) > 0 {
		groups = append(groups,
			gpuEnableGroup(answers),
			gpuPolicyGroup(answers),
			gpuDevicesGroup(res, answers),
		)
	}

	groups = append(groups, confirmGroup(answers))

	return huh.NewForm(groups...).WithTheme(huh.ThemeCatppuccin())
}

func welcomeGroup(res *DiscoveredResources) *huh.Group {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Node:      %s\n", res.NodeName))
	sb.WriteString(fmt.Sprintf("Pools:     %d found\n", len(res.Pools)))
	sb.WriteString(fmt.Sprintf("Storages:  %d compatible\n", len(res.Storages)))
	sb.WriteString(fmt.Sprintf("Bridges:   %s\n", strings.Join(res.Bridges, ", ")))
	sb.WriteString(fmt.Sprintf("GPUs:      %d detected", len(res.GPUs)))
	for _, g := range res.GPUs {
		sb.WriteString(fmt.Sprintf("\n  - %s [%s]", g.Name, g.Type))
	}

	return huh.NewGroup(
		huh.NewNote().
			Title("PVE App Store Installer").
			Description("Welcome! Here's what we detected on this host:\n\n"+sb.String()+"\n\nLet's configure your App Store."),
	)
}

func poolSelectGroup(res *DiscoveredResources, answers *InstallerAnswers) *huh.Group {
	poolOpts := []huh.Option[string]{
		huh.NewOption("Create new pool (appstore)", "__new__"),
	}
	for _, p := range res.Pools {
		poolOpts = append(poolOpts, huh.NewOption(p, p))
	}

	answers.NewPool = "appstore"

	return huh.NewGroup(
		huh.NewSelect[string]().
			Title("Proxmox Pool").
			Description("All managed containers will be confined to this pool.").
			Options(poolOpts...).
			Value(&answers.PoolChoice),
	)
}

func newPoolGroup(answers *InstallerAnswers) *huh.Group {
	return huh.NewGroup(
		huh.NewInput().
			Title("New pool name").
			Value(&answers.NewPool).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("pool name cannot be empty")
				}
				return nil
			}),
	).WithHideFunc(func() bool { return answers.PoolChoice != "__new__" })
}

func placementGroup(res *DiscoveredResources, answers *InstallerAnswers) *huh.Group {
	storageOpts := make([]huh.Option[string], 0, len(res.Storages))
	for _, s := range res.Storages {
		label := fmt.Sprintf("%s (%s)", s.ID, s.Type)
		storageOpts = append(storageOpts, huh.NewOption(label, s.ID))
	}
	if len(storageOpts) == 0 {
		storageOpts = append(storageOpts, huh.NewOption("local-lvm", "local-lvm"))
	}

	bridgeOpts := make([]huh.Option[string], 0, len(res.Bridges))
	for _, b := range res.Bridges {
		bridgeOpts = append(bridgeOpts, huh.NewOption(b, b))
	}
	if len(bridgeOpts) == 0 {
		bridgeOpts = append(bridgeOpts, huh.NewOption("vmbr0", "vmbr0"))
	}

	return huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title("Allowed Storages").
			Description("Select one or more storages for container root filesystems.").
			Options(storageOpts...).
			Value(&answers.Storages),
		huh.NewMultiSelect[string]().
			Title("Allowed Network Bridges").
			Description("Select one or more network bridges for containers.").
			Options(bridgeOpts...).
			Value(&answers.Bridges),
	)
}

func resourcesGroup(answers *InstallerAnswers) *huh.Group {
	return huh.NewGroup(
		huh.NewNote().
			Title("Default Resource Limits").
			Description("These defaults apply to new containers. Apps can override within these bounds."),
		huh.NewInput().
			Title("CPU Cores").
			Value(&answers.CoresStr).
			Validate(ValidatePositiveInt),
		huh.NewInput().
			Title("Memory (MB)").
			Value(&answers.MemoryMBStr).
			Validate(ValidateMemory),
		huh.NewInput().
			Title("Disk (GB)").
			Value(&answers.DiskGBStr).
			Validate(ValidatePositiveInt),
	)
}

func securityGroup(answers *InstallerAnswers) *huh.Group {
	return huh.NewGroup(
		huh.NewConfirm().
			Title("Block privileged containers?").
			Description("Recommended: Yes. If an app manifest requests a privileged container,\n"+
				"the install will be refused. Individual app features (nesting, keyctl,\n"+
				"fuse) are controlled per-app via the catalog manifest, not here.").
			Value(&answers.UnprivilegedOnly),
	)
}

func serviceGroup(answers *InstallerAnswers) *huh.Group {
	return huh.NewGroup(
		huh.NewInput().
			Title("Bind Address").
			Description("IP address to listen on. Use 0.0.0.0 for all interfaces.").
			Value(&answers.BindAddress).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("bind address cannot be empty")
				}
				return nil
			}),
		huh.NewInput().
			Title("Port").
			Value(&answers.PortStr).
			Validate(ValidatePort),
	)
}

func authModeGroup(answers *InstallerAnswers) *huh.Group {
	return huh.NewGroup(
		huh.NewSelect[string]().
			Title("Authentication Mode").
			Options(
				huh.NewOption("Password (recommended)", config.AuthModePassword),
				huh.NewOption("None", config.AuthModeNone),
			).
			Value(&answers.AuthMode),
	)
}

func passwordGroup(answers *InstallerAnswers) *huh.Group {
	return huh.NewGroup(
		huh.NewInput().
			Title("Password").
			EchoMode(huh.EchoModePassword).
			Value(&answers.Password).
			Validate(func(s string) error {
				if len(s) < 8 {
					return fmt.Errorf("password must be at least 8 characters")
				}
				return nil
			}),
		huh.NewInput().
			Title("Confirm Password").
			EchoMode(huh.EchoModePassword).
			Value(&answers.PasswordConfirm).
			Validate(func(s string) error {
				if s != answers.Password {
					return fmt.Errorf("passwords do not match")
				}
				return nil
			}),
	).WithHideFunc(func() bool { return answers.AuthMode != config.AuthModePassword })
}

func proxmoxAutoGroup(answers *InstallerAnswers) *huh.Group {
	return huh.NewGroup(
		huh.NewConfirm().
			Title("Auto-create Proxmox API token?").
			Description("Creates a dedicated 'appstore@pve' user with least-privilege permissions.").
			Value(&answers.AutoCreateToken),
	)
}

func proxmoxManualGroup(answers *InstallerAnswers) *huh.Group {
	return huh.NewGroup(
		huh.NewInput().
			Title("API Token ID").
			Description("Format: user@realm!tokenname").
			Value(&answers.TokenID).
			Validate(func(s string) error {
				if !strings.Contains(s, "!") {
					return fmt.Errorf("token ID must contain '!' (e.g., appstore@pve!appstore)")
				}
				return nil
			}),
		huh.NewInput().
			Title("API Token Secret").
			EchoMode(huh.EchoModePassword).
			Value(&answers.TokenSecret).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("token secret cannot be empty")
				}
				return nil
			}),
	).WithHideFunc(func() bool { return answers.AutoCreateToken })
}

func catalogGroup(answers *InstallerAnswers) *huh.Group {
	return huh.NewGroup(
		huh.NewInput().
			Title("Catalog Repository URL").
			Value(&answers.CatalogURL),
		huh.NewInput().
			Title("Branch").
			Value(&answers.Branch),
		huh.NewSelect[string]().
			Title("Auto-refresh Schedule").
			Options(
				huh.NewOption("Daily", config.RefreshDaily),
				huh.NewOption("Weekly", config.RefreshWeekly),
				huh.NewOption("Manual only", config.RefreshManual),
			).
			Value(&answers.Refresh),
	)
}

func gpuEnableGroup(answers *InstallerAnswers) *huh.Group {
	return huh.NewGroup(
		huh.NewConfirm().
			Title("Enable GPU support?").
			Description("Allow apps to request GPU access for hardware acceleration.").
			Value(&answers.GPUEnabled),
	)
}

func gpuPolicyGroup(answers *InstallerAnswers) *huh.Group {
	return huh.NewGroup(
		huh.NewSelect[string]().
			Title("Default GPU policy").
			Options(
				huh.NewOption("Allow (opt-in per app)", config.GPUPolicyAllow),
				huh.NewOption("Allowlist (select devices)", config.GPUPolicyAllowlist),
				huh.NewOption("None (disabled)", config.GPUPolicyNone),
			).
			Value(&answers.GPUPolicy),
	).WithHideFunc(func() bool { return !answers.GPUEnabled })
}

func gpuDevicesGroup(res *DiscoveredResources, answers *InstallerAnswers) *huh.Group {
	deviceOpts := make([]huh.Option[string], 0, len(res.GPUs))
	for _, g := range res.GPUs {
		label := fmt.Sprintf("%s [%s]", g.Name, g.Type)
		deviceOpts = append(deviceOpts, huh.NewOption(label, g.Path))
	}

	return huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title("Allowed GPU devices").
			Options(deviceOpts...).
			Value(&answers.GPUDevices),
	).WithHideFunc(func() bool {
		return !answers.GPUEnabled || answers.GPUPolicy != config.GPUPolicyAllowlist
	})
}

func confirmGroup(answers *InstallerAnswers) *huh.Group {
	return huh.NewGroup(
		huh.NewNote().
			Title("Ready to Install").
			Description("The installer will now:\n"+
				"  1. Create Proxmox pool and API token (if selected)\n"+
				"  2. Create 'appstore' system user\n"+
				"  3. Write configuration to /etc/pve-appstore/config.yml\n"+
				"  4. Install sudoers and systemd unit\n"+
				"  5. Start the pve-appstore service\n"),
		huh.NewConfirm().
			Title("Proceed with installation?").
			Value(&answers.Confirmed),
	)
}
