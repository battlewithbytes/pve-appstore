package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/battlewithbytes/pve-appstore/internal/config"
	"github.com/battlewithbytes/pve-appstore/internal/ui"
)

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configAddStorageCmd)
	configCmd.AddCommand(configRemoveStorageCmd)
	configCmd.AddCommand(configAddBridgeCmd)
	configCmd.AddCommand(configRemoveBridgeCmd)
	rootCmd.AddCommand(configCmd)
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View and modify PVE App Store configuration",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Display the current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(config.DefaultConfigPath)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		fmt.Println(ui.Cyan.Render("Node:     ") + ui.White.Render(cfg.NodeName))
		fmt.Println(ui.Cyan.Render("Pool:     ") + ui.White.Render(cfg.Pool))
		fmt.Println(ui.Cyan.Render("Storages: ") + ui.White.Render(strings.Join(cfg.Storages, ", ")))
		fmt.Println(ui.Cyan.Render("Bridges:  ") + ui.White.Render(strings.Join(cfg.Bridges, ", ")))
		fmt.Println()
		fmt.Println(ui.Cyan.Render("Defaults:"))
		fmt.Println(ui.Dim.Render("  Cores:     ") + ui.White.Render(fmt.Sprintf("%d", cfg.Defaults.Cores)))
		fmt.Println(ui.Dim.Render("  Memory:    ") + ui.White.Render(fmt.Sprintf("%d MB", cfg.Defaults.MemoryMB)))
		fmt.Println(ui.Dim.Render("  Disk:      ") + ui.White.Render(fmt.Sprintf("%d GB", cfg.Defaults.DiskGB)))
		fmt.Println()
		fmt.Println(ui.Cyan.Render("Service:"))
		fmt.Println(ui.Dim.Render("  Bind:      ") + ui.White.Render(fmt.Sprintf("%s:%d", cfg.Service.BindAddress, cfg.Service.Port)))
		fmt.Println(ui.Dim.Render("  Auth:      ") + ui.White.Render(cfg.Auth.Mode))
		fmt.Println()
		fmt.Println(ui.Cyan.Render("Catalog:"))
		fmt.Println(ui.Dim.Render("  URL:       ") + ui.White.Render(cfg.Catalog.URL))
		fmt.Println(ui.Dim.Render("  Branch:    ") + ui.White.Render(cfg.Catalog.Branch))
		fmt.Println(ui.Dim.Render("  Refresh:   ") + ui.White.Render(cfg.Catalog.Refresh))
		fmt.Println()
		fmt.Println(ui.Cyan.Render("GPU:"))
		fmt.Println(ui.Dim.Render("  Enabled:   ") + ui.White.Render(fmt.Sprintf("%v", cfg.GPU.Enabled)))
		fmt.Println(ui.Dim.Render("  Policy:    ") + ui.White.Render(cfg.GPU.Policy))
		fmt.Println()
		fmt.Println(ui.Dim.Render("Config file: " + config.DefaultConfigPath))

		return nil
	},
}

var configAddStorageCmd = &cobra.Command{
	Use:   "add-storage <name>",
	Short: "Add a storage to the allowed list",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Geteuid() != 0 {
			return fmt.Errorf("must be run as root")
		}

		name := args[0]
		cfg, err := config.Load(config.DefaultConfigPath)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		for _, s := range cfg.Storages {
			if s == name {
				return fmt.Errorf("storage %q is already configured", name)
			}
		}

		cfg.Storages = append(cfg.Storages, name)

		if err := cfg.Validate(); err != nil {
			return fmt.Errorf("invalid config: %w", err)
		}
		if err := saveAndRestore(cfg); err != nil {
			return err
		}

		// Grant Proxmox ACL for the new storage
		exec.Command("pveum", "acl", "modify", "/storage/"+name,
			"--roles", "AppStoreRole", "--users", "appstore@pve").Run()

		restartService()
		fmt.Println(ui.Green.Render("✓") + " Added storage " + ui.White.Render(name))
		return nil
	},
}

var configRemoveStorageCmd = &cobra.Command{
	Use:   "remove-storage <name>",
	Short: "Remove a storage from the allowed list",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Geteuid() != 0 {
			return fmt.Errorf("must be run as root")
		}

		name := args[0]
		cfg, err := config.Load(config.DefaultConfigPath)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		found := false
		filtered := cfg.Storages[:0]
		for _, s := range cfg.Storages {
			if s == name {
				found = true
			} else {
				filtered = append(filtered, s)
			}
		}
		if !found {
			return fmt.Errorf("storage %q is not configured", name)
		}

		cfg.Storages = filtered

		if err := cfg.Validate(); err != nil {
			return fmt.Errorf("invalid config (at least one storage required): %w", err)
		}
		if err := saveAndRestore(cfg); err != nil {
			return err
		}

		// Revoke Proxmox ACL for the removed storage
		exec.Command("pveum", "acl", "delete", "/storage/"+name,
			"--roles", "AppStoreRole", "--users", "appstore@pve").Run()

		restartService()
		fmt.Println(ui.Green.Render("✓") + " Removed storage " + ui.White.Render(name))
		return nil
	},
}

var configAddBridgeCmd = &cobra.Command{
	Use:   "add-bridge <name>",
	Short: "Add a bridge to the allowed list",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Geteuid() != 0 {
			return fmt.Errorf("must be run as root")
		}

		name := args[0]
		cfg, err := config.Load(config.DefaultConfigPath)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		for _, b := range cfg.Bridges {
			if b == name {
				return fmt.Errorf("bridge %q is already configured", name)
			}
		}

		cfg.Bridges = append(cfg.Bridges, name)

		if err := cfg.Validate(); err != nil {
			return fmt.Errorf("invalid config: %w", err)
		}
		if err := saveAndRestore(cfg); err != nil {
			return err
		}

		restartService()
		fmt.Println(ui.Green.Render("✓") + " Added bridge " + ui.White.Render(name))
		return nil
	},
}

var configRemoveBridgeCmd = &cobra.Command{
	Use:   "remove-bridge <name>",
	Short: "Remove a bridge from the allowed list",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Geteuid() != 0 {
			return fmt.Errorf("must be run as root")
		}

		name := args[0]
		cfg, err := config.Load(config.DefaultConfigPath)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		found := false
		filtered := cfg.Bridges[:0]
		for _, b := range cfg.Bridges {
			if b == name {
				found = true
			} else {
				filtered = append(filtered, b)
			}
		}
		if !found {
			return fmt.Errorf("bridge %q is not configured", name)
		}

		cfg.Bridges = filtered

		if err := cfg.Validate(); err != nil {
			return fmt.Errorf("invalid config (at least one bridge required): %w", err)
		}
		if err := saveAndRestore(cfg); err != nil {
			return err
		}

		restartService()
		fmt.Println(ui.Green.Render("✓") + " Removed bridge " + ui.White.Render(name))
		return nil
	},
}

// saveAndRestore saves the config and restores root:appstore 0640 ownership.
func saveAndRestore(cfg *config.Config) error {
	if err := cfg.Save(config.DefaultConfigPath); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	exec.Command("chown", "root:"+config.ServiceGroup, config.DefaultConfigPath).Run()
	exec.Command("chmod", "0640", config.DefaultConfigPath).Run()
	return nil
}

// restartService restarts the pve-appstore systemd service.
func restartService() {
	if err := exec.Command("systemctl", "restart", "pve-appstore.service").Run(); err != nil {
		fmt.Println(ui.Red.Render("✗") + " Service restart failed: " + err.Error())
		fmt.Println(ui.Dim.Render("  Run: systemctl restart pve-appstore"))
	}
}
