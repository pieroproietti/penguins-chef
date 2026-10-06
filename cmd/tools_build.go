// Copyright 2026 Piero Proietti <piero.proietti@gmail.com>.
// All rights reserved.

package cmd

import (
	"os"

	"github.com/pieroproietti/penguins-chef/pkg/builder"
	"github.com/pieroproietti/penguins-chef/pkg/distro"
	"github.com/pieroproietti/penguins-chef/pkg/utils"
	"github.com/spf13/cobra"
)

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Compile binaries and generate native distribution packages (.deb, PKGBUILD, .rpm, .apk)",
	Long: `The 'build' command is the integrated packaging tool for penguins-chef.
It packages chef into native distribution formats like .deb (Debian/Ubuntu), PKGBUILD (Arch Linux), .rpm (Fedora/openSUSE), or APK (Alpine).`,
	Example: `  # Generate native packages
  chef tools build`,
	Run: func(cmd *cobra.Command, args []string) {
		if os.Geteuid() == 0 {
			utils.Fatal("Execution aborted. Do NOT run 'chef tools build' with sudo!")
			utils.LogNormal("Compilation must be run as a normal user to avoid " +
				"creating root-owned files and packages in your workspace.")
			os.Exit(1)
		}

		myDistro := distro.NewDistro()
		builder.HandleBuild(myDistro)
	},
}

func init() {
	toolsCmd.AddCommand(buildCmd)
}
