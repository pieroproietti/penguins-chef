package cmd

import (
	"github.com/pieroproietti/penguins-chef/pkg/chef"
	"github.com/spf13/cobra"
)

func getCmd() *cobra.Command {
	var repoURL string
	var branch string

	cmd := &cobra.Command{
		Use:   "get [url]",
		Short: "clone or pull chef repository in ~/.chef",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			url := repoURL
			if len(args) > 0 {
				url = args[0]
			}
			return chef.Get(url, branch)
		},
	}

	cmd.Flags().StringVarP(&repoURL, "url", "u", "", "URL of the chef repository")
	cmd.Flags().StringVarP(&branch, "branch", "b", "", "Branch of the chef repository")

	return cmd
}
