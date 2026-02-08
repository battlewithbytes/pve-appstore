package main

import (
	"fmt"
	"os"

	"github.com/battlewithbytes/pve-appstore/internal/ui"
	"github.com/battlewithbytes/pve-appstore/internal/version"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "pve-appstore",
	Short:   "PVE App Store — application store for Proxmox VE",
	Version: version.Version,
}

func init() {
	rootCmd.Long = ui.Green.Render("PVE App Store") + " " + ui.Cyan.Render(version.Version) + "\n" +
		ui.Dim.Render("An Unraid Apps-style application store for Proxmox VE that provisions self-contained LXC containers from a Git-based catalog.")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
