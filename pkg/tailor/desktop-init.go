package tailor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func defaultInit(value string) string {
	if value == "" {
		return "auto"
	}
	return value
}

// Detect the running service manager, not merely an installed client binary.
// OpenRC can itself run over SysVinit, so check its runtime marker first.
func (ds desktopSystem) resolveInit(requested string) (string, error) {
	detected := ""
	if info, err := os.Stat(filepath.Join(ds.root, "run/systemd/system")); err == nil && info.IsDir() {
		detected = "systemd"
	} else if info, err := os.Stat(filepath.Join(ds.root, "run/openrc/softlevel")); err == nil && info.Mode().IsRegular() {
		detected = "openrc"
	} else if comm, err := os.ReadFile(filepath.Join(ds.root, "proc/1/comm")); err == nil && strings.TrimSpace(string(comm)) == "init" && ds.hasExecutable("usr/sbin/update-rc.d") {
		detected = "sysvinit"
	}
	if detected == "" {
		return "", fmt.Errorf("cannot detect a supported running init system; chroots and offline roots are not supported")
	}
	if requested != "" && requested != "auto" && requested != detected {
		return "", fmt.Errorf("costume requests init %q, but detected %q; Tailor does not replace the init system", requested, detected)
	}
	return detected, nil
}

func (ds desktopSystem) hasExecutable(path string) bool {
	info, err := os.Stat(filepath.Join(ds.root, path))
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0
}

func (ds desktopSystem) checkLoginService(init string) error {
	if init == "systemd" {
		output, err := ds.run("systemctl", "show", "lightdm.service", "--property=LoadState", "--value")
		if err != nil || strings.TrimSpace(string(output)) != "loaded" {
			return fmt.Errorf("lightdm.service is not available: %s", strings.TrimSpace(string(output)))
		}
		return nil
	}
	if !ds.hasExecutable("etc/init.d/lightdm") {
		return fmt.Errorf("%s requires an executable /etc/init.d/lightdm", init)
	}
	if init == "sysvinit" && !ds.hasExecutable("usr/sbin/update-rc.d") {
		return fmt.Errorf("SysVinit backend requires update-rc.d")
	}
	if init == "openrc" && !ds.hasExecutable("sbin/rc-update") && !ds.hasExecutable("usr/sbin/rc-update") {
		return fmt.Errorf("OpenRC backend requires rc-update")
	}
	return nil
}

func (ds desktopSystem) serviceCommand(name string, args ...string) error {
	output, err := ds.run(name, args...)
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, output)
	}
	return nil
}

func (ds desktopSystem) enableLogin(init string) error {
	switch init {
	case "systemd":
		// Replace the display-manager alias without stopping the active session.
		if err := ds.serviceCommand("systemctl", "enable", "--force", "lightdm.service"); err != nil {
			return err
		}
		output, err := ds.run("systemctl", "list-unit-files", "--no-legend", "slim.service", "gdm3.service", "gdm.service", "sddm.service")
		if err != nil {
			return fmt.Errorf("list competing display managers: %w: %s", err, output)
		}
		for _, line := range strings.Split(string(output), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			switch fields[0] {
			case "slim.service", "gdm3.service", "gdm.service", "sddm.service":
				if fields[1] == "enabled" || fields[1] == "enabled-runtime" {
					if err := ds.serviceCommand("systemctl", "disable", fields[0]); err != nil {
						return err
					}
				}
			}
		}
		return ds.serviceCommand("systemctl", "set-default", "graphical.target")
	case "sysvinit":
		if err := ds.serviceCommand("update-rc.d", "lightdm", "defaults"); err != nil {
			return err
		}
		if err := ds.serviceCommand("update-rc.d", "lightdm", "enable"); err != nil {
			return err
		}
		for _, other := range []string{"slim", "gdm3", "gdm", "sddm"} {
			if ds.hasExecutable("etc/init.d/" + other) {
				if err := ds.serviceCommand("update-rc.d", other, "disable"); err != nil {
					return err
				}
			}
		}
	case "openrc":
		if err := ds.serviceCommand("rc-update", "add", "lightdm", "default"); err != nil {
			return err
		}
		for _, other := range []string{"slim", "gdm3", "gdm", "sddm", "xdm", "display-manager"} {
			// Only remove existing default-runlevel entries; custom runlevels remain
			// the administrator's responsibility in this first implementation.
			if _, err := os.Lstat(filepath.Join(ds.root, "etc/runlevels/default", other)); err == nil {
				if err := ds.serviceCommand("rc-update", "del", other, "default"); err != nil {
					return err
				}
			}
		}
	default:
		return fmt.Errorf("unsupported init %q", init)
	}
	return nil
}
