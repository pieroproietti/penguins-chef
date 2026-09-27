package tailor

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func initExecutable(t *testing.T, root, path string) {
	t.Helper()
	desktopFixture(t, root, path, "#!/bin/sh\n")
	if err := os.Chmod(filepath.Join(root, path), 0755); err != nil {
		t.Fatal(err)
	}
}

func initFixture(t *testing.T, root, kind string) {
	t.Helper()
	switch kind {
	case "systemd":
		if err := os.MkdirAll(filepath.Join(root, "run/systemd/system"), 0755); err != nil {
			t.Fatal(err)
		}
	case "sysvinit":
		desktopFixture(t, root, "proc/1/comm", "init\n")
		initExecutable(t, root, "usr/sbin/update-rc.d")
	case "openrc":
		desktopFixture(t, root, "run/openrc/softlevel", "default\n")
		initExecutable(t, root, "sbin/rc-update")
	}
	initExecutable(t, root, "etc/init.d/lightdm")
}

func TestDesktopInitDetection(t *testing.T) {
	for _, kind := range []string{"systemd", "sysvinit", "openrc"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			initFixture(t, root, kind)
			ds := desktopSystem{root: root}
			for _, requested := range []string{"", "auto", kind} {
				got, err := ds.resolveInit(requested)
				if err != nil || got != kind {
					t.Fatalf("got=%s err=%v", got, err)
				}
			}
			if _, err := ds.resolveInit("wrong"); err == nil {
				t.Fatal("mismatch accepted")
			}
		})
	}
	// Installing systemctl/update-rc.d does not identify the running init.
	root := t.TempDir()
	initExecutable(t, root, "usr/sbin/update-rc.d")
	desktopFixture(t, root, "proc/1/comm", "python\n")
	if _, err := (desktopSystem{root: root}).resolveInit("auto"); err == nil {
		t.Fatal("unknown init accepted")
	}
	// OpenRC may run with a PID 1 named init and installed compatibility tools.
	initFixture(t, root, "openrc")
	desktopFixture(t, root, "proc/1/comm", "init\n")
	if got, err := (desktopSystem{root: root}).resolveInit("auto"); err != nil || got != "openrc" {
		t.Fatalf("got=%s err=%v", got, err)
	}
}

func TestDesktopInitActivation(t *testing.T) {
	cases := []struct {
		kind string
		want [][]string
	}{
		{"sysvinit", [][]string{{"update-rc.d", "lightdm", "defaults"}, {"update-rc.d", "lightdm", "enable"}, {"update-rc.d", "sddm", "disable"}}},
		{"openrc", [][]string{{"rc-update", "add", "lightdm", "default"}, {"rc-update", "del", "sddm", "default"}}},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			root := t.TempDir()
			initFixture(t, root, tc.kind)
			desktopFixture(t, root, "usr/share/xsessions/xfce.desktop", "[Desktop Entry]\n")
			initExecutable(t, root, "etc/init.d/sddm")
			desktopFixture(t, root, "etc/runlevels/default/sddm", "")
			var commands [][]string
			ds := desktopSystem{root, func(name string, args ...string) ([]byte, error) {
				commands = append(commands, append([]string{name}, args...))
				return nil, nil
			}}
			if err := ds.apply(&Suit{Desktop: "xfce", DisplayManager: "lightdm", SessionType: "x11", Init: "auto"}); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(commands, tc.want) {
				t.Fatalf("commands=%v", commands)
			}
		})
	}
}

func TestDesktopInitMismatchLeavesConfigurationUntouched(t *testing.T) {
	root := t.TempDir()
	initFixture(t, root, "sysvinit")
	desktopFixture(t, root, "usr/share/xsessions/xfce.desktop", "[Desktop Entry]\n")
	ds := desktopSystem{root, func(string, ...string) ([]byte, error) {
		t.Fatal("ran service command on mismatched init")
		return nil, nil
	}}
	err := ds.apply(&Suit{Desktop: "xfce", DisplayManager: "lightdm", SessionType: "x11", Init: "systemd"})
	if err == nil || !strings.Contains(err.Error(), "detected") {
		t.Fatalf("error=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "etc/lightdm/lightdm.conf")); !os.IsNotExist(err) {
		t.Fatalf("configuration changed: %v", err)
	}
}

func TestDesktopPackageSelectionPreservesAccessories(t *testing.T) {
	s := &Suit{Desktop: "xfce", DisplayManager: "lightdm", Accessories: []string{"desktop"}}
	addDesktopPackages(s)
	if len(s.Packages) != 0 {
		t.Fatalf("added packages to curated recipe: %v", s.Packages)
	}
	s = &Suit{Desktop: "xfce", DisplayManager: "lightdm"}
	addDesktopPackages(s)
	if !reflect.DeepEqual(s.Packages, []string{"xfce4", "lightdm", "lightdm-gtk-greeter"}) {
		t.Fatal(s.Packages)
	}
	if err := validateDesktop(&Suit{Init: "unsupported"}); err == nil {
		t.Fatal("unknown init accepted")
	}
}
