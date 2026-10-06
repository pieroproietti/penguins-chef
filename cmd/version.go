package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var Version = "0.1.0"

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print chef version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("chef %s - A \"chef\" to cook and configure your system recipes\n", Version)
		},
	}
}
