package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/battlewithbytes/pve-appstore/internal/pct"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(shellCmd)
}

var shellCmd = &cobra.Command{
	Use:   "shell <ctid>",
	Short: "Open an interactive shell in a container",
	Long:  "Attaches to the specified container via pct enter, providing an interactive shell session.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctid, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid CTID %q: must be a number", args[0])
		}

		c := pct.SudoNsenterCmd("/usr/sbin/pct", "enter", strconv.Itoa(ctid))
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	},
}
