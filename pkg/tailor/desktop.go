package tailor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pieroproietti/penguins-tailor/pkg/utils"
)

// This first backend targets LightDM with supported service managers. Keep policy independent of
// package installation so unsupported distribution families can use it too.
var desktopPackages = map[string]string{
	"gnome": "gnome-core", "plasma": "kde-plasma-desktop",
	"xfce": "xfce4", "cinnamon": "cinnamon-desktop-environment",
	"mate": "mate-desktop-environment", "lxqt": "lxqt", "budgie": "budgie-desktop",
}
var desktopSessions = map[string][]string{
	"gnome": {"gnome", "gnome-xorg"}, "plasma": {"plasma", "plasmax11", "plasmawayland"},
	"xfce": {"xfce", "xfce-wayland"}, "cinnamon": {"cinnamon", "cinnamon-wayland"},
	"mate": {"mate"}, "lxqt": {"lxqt", "lxqt-wayland"}, "budgie": {"budgie-desktop", "budgie"},
}

type desktopPlan struct {
	session  string
	protocol string
	init     string
}

func validateDesktop(s *Suit) error {
	switch s.Init {
	case "", "auto", "systemd", "sysvinit", "openrc":
	default:
		return fmt.Errorf("invalid init %q: use auto, systemd, sysvinit or openrc", s.Init)
	}
	if s.Desktop == "" && s.DisplayManager == "" && s.SessionType == "" {
		return nil
	}
	if _, ok := desktopPackages[s.Desktop]; !ok {
		return fmt.Errorf("unsupported desktop %q (supported: gnome, plasma, xfce, cinnamon, mate, lxqt, budgie)", s.Desktop)
	}
	if s.DisplayManager != "lightdm" {
		return fmt.Errorf("experimental desktop configuration requires display_manager: lightdm; %q is not implemented", s.DisplayManager)
	}
	switch s.SessionType {
	case "", "auto", "x11", "wayland":
	default:
		return fmt.Errorf("invalid session_type %q: use auto, x11 or wayland", s.SessionType)
	}
	return nil
}

func addDesktopPackages(s *Suit) {
	if s.Desktop == "" {
		return
	}
	// Existing recipes own their package selection, including packages supplied
	// by accessories. Only a minimal declarative recipe gets inferred packages.
	if len(s.Packages) > 0 || len(s.PackagesNoRecommends) > 0 || len(s.PackagesInteractive) > 0 || len(s.Accessories) > 0 {
		return
	}
	for _, pkg := range []string{desktopPackages[s.Desktop], "lightdm", "lightdm-gtk-greeter"} {
		found := false
		for _, existing := range s.Packages {
			if existing == pkg {
				found = true
				break
			}
		}
		if !found {
			s.Packages = append(s.Packages, pkg)
		}
	}
}

// Resolve actual installed session files, never silently change an explicit
// protocol. Ambiguous auto selections require an explicit protocol.
func resolveDesktop(root string, s *Suit) (desktopPlan, error) {
	var matches []desktopPlan
	for _, protocol := range []string{"x11", "wayland"} {
		if s.SessionType != "" && s.SessionType != "auto" && s.SessionType != protocol {
			continue
		}
		dir := "xsessions"
		if protocol == "wayland" {
			dir = "wayland-sessions"
		}
		for _, name := range desktopSessions[s.Desktop] {
			path := filepath.Join(root, "usr/share", dir, name+".desktop")
			if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
				matches = append(matches, desktopPlan{session: name, protocol: protocol})
			}
		}
	}
	if len(matches) == 0 {
		return desktopPlan{}, fmt.Errorf("no installed %s session for session_type %q; install the desktop/session packages first", s.Desktop, s.SessionType)
	}
	if len(matches) > 1 {
		return desktopPlan{}, fmt.Errorf("multiple %s sessions match session_type %q; select an unambiguous x11 or wayland session", s.Desktop, s.SessionType)
	}
	return matches[0], nil
}

// The injectable root and command runner allow tests to exercise application
// without touching the host login manager.
type desktopSystem struct {
	root string
	run  func(string, ...string) ([]byte, error)
}

func hostDesktopSystem() desktopSystem {
	return desktopSystem{"/", func(name string, args ...string) ([]byte, error) { return exec.Command(name, args...).CombinedOutput() }}
}
func (ds desktopSystem) check(s *Suit) (desktopPlan, error) {
	plan, err := resolveDesktop(ds.root, s)
	if err != nil {
		return plan, err
	}
	plan.init, err = ds.resolveInit(s.Init)
	if err != nil {
		return plan, err
	}
	if err := ds.checkLoginService(plan.init); err != nil {
		return plan, err
	}
	// LightDM uses only a session basename. An identical basename in both
	// directories cannot reliably express the requested protocol.
	other := "wayland-sessions"
	if plan.protocol == "wayland" {
		other = "xsessions"
	}
	if _, err := os.Stat(filepath.Join(ds.root, "usr/share", other, plan.session+".desktop")); err == nil {
		return plan, fmt.Errorf("LightDM session %q exists for both X11 and Wayland; cannot select the protocol unambiguously", plan.session)
	}
	return plan, nil
}

func prepareDesktop(s *Suit, dryRun bool) error {
	if s.Desktop == "" {
		return nil
	}
	if dryRun {
		utils.LogNormal("[DRY-RUN] Desktop: %s; login: %s; session: %s; init: %s (availability checked during actual wear)", s.Desktop, s.DisplayManager, s.SessionType, defaultInit(s.Init))
		return nil
	}
	_, err := hostDesktopSystem().check(s)
	return err
}

func applyDesktop(s *Suit, dryRun bool) error {
	if s.Desktop == "" {
		return nil
	}
	if dryRun {
		utils.LogNormal("[DRY-RUN] Would configure LightDM after sysroot and select it for the next boot; no service restart")
		return nil
	}
	return hostDesktopSystem().apply(s)
}

func (ds desktopSystem) apply(s *Suit) error {
	plan, err := ds.check(s)
	if err != nil {
		return err
	}
	// lightdm.conf overrides conf.d, so update its default seat in place.
	path := filepath.Join(ds.root, "etc/lightdm/lightdm.conf")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	data = setDesktopINI(data, "Seat:*", "user-session", plan.session)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	// Keep Debian's display-manager selector consistent with the systemd alias.
	if _, err := os.Stat(filepath.Join(ds.root, "etc/debian_version")); err == nil {
		if err := os.WriteFile(filepath.Join(ds.root, "etc/X11/default-display-manager"), []byte("/usr/sbin/lightdm\n"), 0644); err != nil {
			return err
		}
	}
	if err := ds.enableLogin(plan.init); err != nil {
		return err
	}
	utils.LogNormal("Selected LightDM with %s (%s) using %s for the next graphical boot. Existing per-user session preferences may override the default.", plan.session, plan.protocol, plan.init)
	return nil
}

// Preserve unrelated settings/comments, replacing all occurrences of the key
// in the requested section so a later duplicate cannot undo the selection.
func setDesktopINI(data []byte, section, key, value string) []byte {
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	var out []string
	active, found := false, false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			active = trimmed == "["+section+"]"
			out = append(out, line)
			if active {
				out = append(out, key+"="+value)
				found = true
			}
			continue
		}
		if active {
			if k, _, ok := strings.Cut(trimmed, "="); ok && strings.TrimSpace(k) == key {
				continue
			}
		}
		out = append(out, line)
	}
	if !found {
		out = append(out, "["+section+"]", key+"="+value)
	}
	return []byte(strings.Join(out, "\n") + "\n")
}
