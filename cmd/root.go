package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "tailor",
	Short: "penguins-tailor: dress up your Linux distribution with costumes and configurations",
	Long: `penguins-tailor is a lightweight tool to manage and apply system configurations,
desktop environments, and themes ("costumes") to Linux distributions.`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(applyCmd())
	rootCmd.AddCommand(versionCmd())
}
