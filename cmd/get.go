package cmd

import (
	"github.com/pieroproietti/penguins-tailor/pkg/tailor"
	"github.com/spf13/cobra"
)

func getCmd() *cobra.Command {
	var repoURL string
	var branch string

	cmd := &cobra.Command{
		Use:   "get [url]",
		Short: "clone or pull atelier repository in ./wardrobe",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			url := repoURL
			if len(args) > 0 {
				url = args[0]
			}
			return tailor.Get(url, branch)
		},
	}

	cmd.Flags().StringVarP(&repoURL, "url", "u", "", "URL of the atelier repository")
	cmd.Flags().StringVarP(&branch, "branch", "b", "", "Branch of the atelier repository")

	return cmd
}
