package main

import (
	"fmt"
	"os"

	"github.com/battlewithbytes/pve-appstore/internal/installer"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(installCmd)
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Run the interactive TUI installer",
	Long:  "Discovers Proxmox resources, walks through configuration questions, and sets up the PVE App Store service.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Geteuid() != 0 {
			return fmt.Errorf("the installer must be run as root")
		}
		return installer.Run()
	},
}
