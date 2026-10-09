package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/pieroproietti/penguins-chef/pkg/distro"
	"github.com/pieroproietti/penguins-chef/pkg/provision"
	"github.com/pieroproietti/penguins-chef/pkg/tui"
	"github.com/spf13/cobra"
)

var (
	runRecipePickerFn = tui.Run
	isTerminalFn      = func(f *os.File) bool {
		if f == nil {
			return false
		}
		stat, err := f.Stat()
		if err != nil {
			return false
		}
		return (stat.Mode() & os.ModeCharDevice) != 0
	}
)

func applyCmd() *cobra.Command {
	var dryRun bool
	var family, initSystem, sysroot string
	cmd := &cobra.Command{
		Use:   "apply [recipe.yaml]",
		Short: "Apply an experimental operation-based recipe",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !dryRun && (family != "" || initSystem != "") {
				return fmt.Errorf("--family and --init are only allowed with --dry-run")
			}
			var recipePath string
			if len(args) == 1 {
				recipePath = args[0]
			} else {
				if !isTerminalFn(os.Stdin) || !isTerminalFn(os.Stdout) {
					return fmt.Errorf("recipe argument required in non-interactive mode")
				}
				selected, err := runRecipePickerFn()
				if err != nil {
					return err
				}
				if selected == "" {
					return nil
				}
				recipePath = selected
			}
			f, err := os.Open(recipePath)
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
			} else if targetFamily == "devuan" {
				targetFamily = "debian"
			}
			targetInit := initSystem
			if targetInit == "" {
				if family == "devuan" {
					targetInit = "sysvinit"
				} else if dryRun && (family == "archlinux" || family == "fedora" || family == "opensuse") {
					targetInit = "systemd"
				} else {
					targetInit = detectInit()
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
			for _, step := range plan.Steps {
				if step.Hostname != "" {
					if targetInit == "systemd" {
						tools = append(tools, "hostnamectl")
					} else {
						tools = append(tools, "hostname")
					}
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
	cmd.Flags().StringVar(&initSystem, "init", "", "Preview an init system: systemd, sysvinit, openrc (optional preview override, dry-run only)")
	cmd.Flags().StringVar(&sysroot, "sysroot", "", "Copy the contents of a local sysroot directory to /, preserving archive metadata, ACLs and xattrs")
	return cmd
}

// detectInit checks whether systemd is booted (standard sd_booted check),
// falling back to sysvinit for non-systemd environments like Devuan.
func detectInit() string {
	if info, err := os.Stat("/run/systemd/system"); err == nil && info.IsDir() {
		return "systemd"
	}
	return "sysvinit"
}
