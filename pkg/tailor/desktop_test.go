package tailor

import (
	"errors"
	"fmt"
	"github.com/pieroproietti/penguins-tailor/pkg/distro"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func desktopFixture(t *testing.T, root, path, content string) {
	t.Helper()
	name := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestDesktopResolveProtocols(t *testing.T) {
	root := t.TempDir()
	s := &Suit{Desktop: "xfce", DisplayManager: "lightdm", SessionType: "wayland"}
	desktopFixture(t, root, "usr/share/xsessions/xfce.desktop", "[Desktop Entry]\n")
	if _, err := resolveDesktop(root, s); err == nil {
		t.Fatal("explicit Wayland silently fell back to X11")
	}
	desktopFixture(t, root, "usr/share/wayland-sessions/xfce-wayland.desktop", "[Desktop Entry]\n")
	plan, err := resolveDesktop(root, s)
	if err != nil || plan.session != "xfce-wayland" || plan.protocol != "wayland" {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	s.SessionType = "auto"
	if _, err := resolveDesktop(root, s); err == nil {
		t.Fatal("ambiguous auto selection accepted")
	}
}

func TestDesktopApplyPreservesConfigurationAndDoesNotRestart(t *testing.T) {
	root := t.TempDir()
	desktopFixture(t, root, "usr/share/xsessions/xfce.desktop", "[Desktop Entry]\n")
	if err := os.MkdirAll(filepath.Join(root, "run/systemd/system"), 0755); err != nil {
		t.Fatal(err)
	}
	desktopFixture(t, root, "etc/lightdm/lightdm.conf", "# existing\n[LightDM]\nlog-directory=/custom\n[Seat:*]\nuser-session=old\ngreeter-session=custom\n")
	var commands [][]string
	ds := desktopSystem{root, func(name string, args ...string) ([]byte, error) {
		commands = append(commands, append([]string{name}, args...))
		if args[0] == "show" {
			return []byte("loaded\n"), nil
		}
		return nil, nil
	}}
	s := &Suit{Desktop: "xfce", DisplayManager: "lightdm", SessionType: "x11"}
	if err := ds.apply(s); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "etc/lightdm/lightdm.conf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# existing", "log-directory=/custom", "greeter-session=custom", "user-session=xfce"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("missing %s in %s", want, data)
		}
	}
	want := [][]string{{"systemctl", "show", "lightdm.service", "--property=LoadState", "--value"}, {"systemctl", "enable", "--force", "lightdm.service"}}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands=%v", commands)
	}
	if err := ds.apply(s); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(filepath.Join(root, "etc/lightdm/lightdm.conf"))
	if string(again) != string(data) {
		t.Fatal("repeated application changes configuration")
	}
}

func TestDesktopMissingServiceDoesNotWrite(t *testing.T) {
	root := t.TempDir()
	desktopFixture(t, root, "usr/share/xsessions/xfce.desktop", "[Desktop Entry]\n")
	if err := os.MkdirAll(filepath.Join(root, "run/systemd/system"), 0755); err != nil {
		t.Fatal(err)
	}
	ds := desktopSystem{root, func(string, ...string) ([]byte, error) { return []byte("not-found"), nil }}
	if err := ds.apply(&Suit{Desktop: "xfce", DisplayManager: "lightdm", SessionType: "x11"}); err == nil {
		t.Fatal("missing service accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "etc/lightdm/lightdm.conf")); !os.IsNotExist(err) {
		t.Fatalf("unexpected configuration write: %v", err)
	}
}

func TestDesktopValidationAndPackages(t *testing.T) {
	for _, s := range []Suit{{Desktop: "unknown", DisplayManager: "lightdm"}, {Desktop: "xfce", DisplayManager: "sddm"}, {Desktop: "xfce", DisplayManager: "lightdm", SessionType: "invalid"}, {SessionType: "wayland"}} {
		if err := validateDesktop(&s); err == nil {
			t.Fatalf("invalid choice accepted: %+v", s)
		}
	}
	if err := validateDesktop(&Suit{}); err != nil {
		t.Fatal(err)
	}
	s := &Suit{Desktop: "xfce", DisplayManager: "lightdm", Packages: []string{"xfce4", "custom"}}
	addDesktopPackages(s)
	addDesktopPackages(s)
	if !reflect.DeepEqual(s.Packages, []string{"xfce4", "custom"}) {
		t.Fatal(s.Packages)
	}
	path := filepath.Join(t.TempDir(), "index.yaml")
	if err := os.WriteFile(path, []byte("desktop: xfce\ndisplay_manager: lightdm\nsession_type: x11\n"), 0644); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadSuit(path)
	if err != nil || loaded.Desktop != "xfce" || loaded.SessionType != "x11" {
		t.Fatalf("suit=%+v err=%v", loaded, err)
	}
}

func TestDesktopEnableFailureIsReported(t *testing.T) {
	root := t.TempDir()
	desktopFixture(t, root, "usr/share/xsessions/xfce.desktop", "[Desktop Entry]\n")
	if err := os.MkdirAll(filepath.Join(root, "run/systemd/system"), 0755); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("enable failed")
	ds := desktopSystem{root, func(_ string, args ...string) ([]byte, error) {
		if args[0] == "show" {
			return []byte("loaded"), nil
		}
		return nil, failure
	}}
	if err := ds.apply(&Suit{Desktop: "xfce", DisplayManager: "lightdm", SessionType: "x11"}); !errors.Is(err, failure) {
		t.Fatalf("error=%v", err)
	}
}

func TestDesktopWearOrdering(t *testing.T) {
	for _, family := range []string{"debian", "archlinux"} {
		for _, missing := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/missing=%v", family, missing), func(t *testing.T) {
				root := t.TempDir()
				desktopFixture(t, root, "v2/costumes/desktop/index.yaml", "name: desktop\ndesktop: xfce\ndisplay_manager: lightdm\nsession_type: x11\n")
				withAccessory := family == "debian" && !missing
				if withAccessory {
					desktopFixture(t, root, "v2/costumes/desktop/index.yaml", "name: desktop\ndesktop: xfce\ndisplay_manager: lightdm\nsession_type: x11\naccessories: [session-provider]\n")
					desktopFixture(t, root, "v2/accessories/session-provider/index.yaml", "name: session-provider\npackages: [xfce4-session]\n")
				}
				oldDistro, oldPM := newWearDistro, newWearPackageManager
				oldRoot, oldV2 := getWearWardrobeRoot, getWearWardrobeV2Dir
				oldPrompt, oldSuit := promptWearConfirm, applyWearSuit
				oldPrepare, oldApply := prepareWearDesktop, applyWearDesktop
				oldSysroot, oldSkel := applyWearSysroot, copyWearSkelToUser
				t.Cleanup(func() {
					newWearDistro, newWearPackageManager = oldDistro, oldPM
					getWearWardrobeRoot, getWearWardrobeV2Dir = oldRoot, oldV2
					promptWearConfirm, applyWearSuit = oldPrompt, oldSuit
					prepareWearDesktop, applyWearDesktop = oldPrepare, oldApply
					applyWearSysroot, copyWearSkelToUser = oldSysroot, oldSkel
				})
				newWearDistro = func() *distro.Distro { return &distro.Distro{DistroID: family, FamilyID: family} }
				newWearPackageManager = func(string) (PackageManager, error) {
					if family == "debian" {
						return &orderedPackageManager{}, nil
					}
					return nil, errors.New("unsupported")
				}
				getWearWardrobeRoot = func() (string, error) { return root, nil }
				getWearWardrobeV2Dir = func() (string, error) { return filepath.Join(root, "v2"), nil }
				promptWearConfirm = func(string) bool { return true }
				var events []string
				applyWearSuit = func(_ string, s *Suit, _ bool, accessory bool, _ PackageManager) (PackageInstallResult, error) {
					if withAccessory {
						if accessory {
							events = append(events, "accessory-packages")
						}
						return PackageInstallResult{}, nil
					}
					if !slices.Contains(s.Packages, "xfce4") || !slices.Contains(s.Packages, "lightdm") {
						t.Fatal(s.Packages)
					}
					return PackageInstallResult{}, nil
				}
				missingErr := errors.New("session missing")
				prepareWearDesktop = func(*Suit, bool) error {
					events = append(events, "check")
					if missing {
						return missingErr
					}
					return nil
				}
				applyWearSysroot = func(string, string, bool, bool) { events = append(events, "sysroot") }
				applyWearDesktop = func(*Suit, bool) error { events = append(events, "desktop"); return nil }
				copyWearSkelToUser = func(bool) {}
				err := Wear("desktop", true, "", true)
				expected := []string{"check", "sysroot", "desktop"}
				if withAccessory {
					expected = append([]string{"accessory-packages"}, expected...)
				}
				if missing {
					if !errors.Is(err, missingErr) || !reflect.DeepEqual(events, []string{"check"}) {
						t.Fatalf("events=%v err=%v", events, err)
					}
				} else if err != nil || !reflect.DeepEqual(events, expected) {
					t.Fatalf("events=%v err=%v", events, err)
				}
			})
		}
	}
}
