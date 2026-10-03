package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/pieroproietti/penguins-tailor/pkg/distro"
	"github.com/pieroproietti/penguins-tailor/pkg/provision"
	"github.com/spf13/cobra"
)

func applyCmd() *cobra.Command {
	var dryRun bool
	var family, initSystem, sysroot string
	cmd := &cobra.Command{
		Use:   "apply <recipe.yaml>",
		Short: "Apply an experimental operation-based recipe",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !dryRun && (family != "" || initSystem != "") {
				return fmt.Errorf("--family and --init are only allowed with --dry-run")
			}
			f, err := os.Open(args[0])
			if err != nil {
				return err
			}
			defer f.Close()
			recipe, err := provision.Load(f)
			if err != nil {
				return err
			}
			targetFamily := family
			if targetFamily == "" {
				targetFamily = distro.NewDistro().FamilyID
			}
			targetInit := initSystem
			if targetInit == "" {
				targetInit = "unknown"
				if info, err := os.Stat("/run/systemd/system"); err == nil && info.IsDir() {
					targetInit = "systemd"
				}
			}
			resolved, err := recipe.Resolve()
			if err != nil {
				return err
			}
			recipe = resolved
			if sysroot != "" {
				recipe.Sysroot = "" // The explicit CLI source overrides the recipe default.
			}
			plan, err := provision.Build(recipe, targetFamily, targetInit)
			if err != nil {
				return err
			}
			if sysroot != "" {
				if err := plan.AddSysroot(sysroot); err != nil {
					return err
				}
			}
			if err := plan.Describe(cmd.OutOrStdout()); err != nil {
				return err
			}
			if dryRun {
				return nil
			}
			if os.Geteuid() != 0 {
				return fmt.Errorf("applying a recipe requires root; inspect it with --dry-run first")
			}
			// Check tools before the first repository or filesystem mutation.
			tools := []string{"apt-get", "apt-cache", "dpkg-query"}
			if targetFamily == "archlinux" {
				tools = []string{"pacman"}
			}
			if targetFamily == "fedora" {
				tools = []string{"dnf", "rpm"}
			}
			if targetFamily == "opensuse" {
				tools = []string{"zypper", "rpm"}
			}
			if targetInit == "systemd" {
				tools = append(tools, "systemctl")
			}
			for _, step := range plan.Steps {
				if step.Sysroot != "" {
					tools = append(tools, "rsync")
					break
				}
			}
			for _, tool := range tools {
				if _, err := exec.LookPath(tool); err != nil {
					return err
				}
			}
			return plan.Execute(cmd.Context(), provision.HostRunner{Out: cmd.OutOrStdout(), Err: cmd.ErrOrStderr()}, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "Print the plan without executing commands or writing files")
	cmd.Flags().StringVar(&family, "family", "", "Preview a package family: debian, archlinux, fedora or opensuse (dry-run only)")
	cmd.Flags().StringVar(&initSystem, "init", "", "Preview an init system: systemd (dry-run only)")
	cmd.Flags().StringVar(&sysroot, "sysroot", "", "Copy the contents of a local sysroot directory to /, preserving archive metadata, ACLs and xattrs")
	return cmd
}
