package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "chef",
	Short: "penguins-chef: cook and configure your Linux distribution with recipes",
	Long: `penguins-chef is a lightweight tool to manage and apply system configurations,
desktop environments, and recipes to Linux distributions.`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(getCmd())
	rootCmd.AddCommand(applyCmd())
	rootCmd.AddCommand(versionCmd())
}
