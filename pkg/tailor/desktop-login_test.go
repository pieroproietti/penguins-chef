package tailor

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopLoginOptions(t *testing.T) {
	for _, kind := range []string{"systemd", "sysvinit", "openrc"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			initFixture(t, root, kind)
			desktopFixture(t, root, "usr/share/xsessions/xfce.desktop", "[Desktop Entry]\n")
			desktopFixture(t, root, "usr/share/xgreeters/lightdm-gtk-greeter.desktop", "[Desktop Entry]\n")
			desktopFixture(t, root, "usr/share/backgrounds/test.jpg", "image")
			desktopFixture(t, root, "etc/lightdm/lightdm-gtk-greeter.conf", "[greeter]\ntheme-name=existing\nbackground=old\n")
			desktopFixture(t, root, "etc/lightdm/lightdm.conf", "[Seat:*]\nautologin-user=old\nautologin-guest=true\n")
			enabled := false
			s := &Suit{Desktop: "xfce", DisplayManager: "lightdm", SessionType: "x11", Autologin: &enabled, LoginBackground: "/usr/share/backgrounds/test.jpg"}
			ds := desktopSystem{root, func(_ string, args ...string) ([]byte, error) {
				if args[0] == "show" {
					return []byte("loaded"), nil
				}
				return nil, nil
			}}
			if err := ds.apply(s); err != nil {
				t.Fatal(err)
			}
			lightdm, _ := os.ReadFile(filepath.Join(root, "etc/lightdm/lightdm.conf"))
			for _, want := range []string{"autologin-user=\n", "autologin-guest=false", "greeter-session=lightdm-gtk-greeter", "user-session=xfce"} {
				if !strings.Contains(string(lightdm), want) {
					t.Fatalf("missing %q in %s", want, lightdm)
				}
			}
			greeter, _ := os.ReadFile(filepath.Join(root, "etc/lightdm/lightdm-gtk-greeter.conf"))
			if !strings.Contains(string(greeter), "theme-name=existing") || !strings.Contains(string(greeter), "background=/usr/share/backgrounds/test.jpg") {
				t.Fatal(string(greeter))
			}
			if err := ds.apply(s); err != nil {
				t.Fatal(err)
			}
			again, _ := os.ReadFile(filepath.Join(root, "etc/lightdm/lightdm-gtk-greeter.conf"))
			if string(again) != string(greeter) {
				t.Fatal("non-idempotent greeter update")
			}
			// Omitted options preserve prior values.
			s.Autologin, s.LoginBackground = nil, ""
			if err := ds.apply(s); err != nil {
				t.Fatal(err)
			}
			again, _ = os.ReadFile(filepath.Join(root, "etc/lightdm/lightdm-gtk-greeter.conf"))
			if string(again) != string(greeter) {
				t.Fatal("omitted background changed settings")
			}
		})
	}
}

func TestDesktopAutologin(t *testing.T) {
	account := firstHumanUser()
	if account == nil {
		t.Skip("no non-root account on host")
	}
	t.Setenv("SUDO_USER", "")
	t.Setenv("DOAS_USER", account.Username)
	root := t.TempDir()
	initFixture(t, root, "sysvinit")
	desktopFixture(t, root, "usr/share/xsessions/xfce.desktop", "")
	desktopFixture(t, root, "etc/debian_version", "test")
	enabled := true
	ds := desktopSystem{root, func(string, ...string) ([]byte, error) { return nil, nil }}
	s := &Suit{Desktop: "xfce", DisplayManager: "lightdm", SessionType: "x11", Autologin: &enabled}
	if err := ds.apply(s); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(root, "etc/lightdm/lightdm.conf"))
	for _, want := range []string{"autologin-user=" + account.Username, "autologin-session=xfce", "autologin-user-timeout=0"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("missing %q in %s", want, data)
		}
	}
	selector, err := os.ReadFile(filepath.Join(root, "etc/X11/default-display-manager"))
	if err != nil || string(selector) != "/usr/sbin/lightdm\n" {
		t.Fatalf("selector=%s err=%v", selector, err)
	}
}

func TestAutologinRejectsRootAndInvalidNames(t *testing.T) {
	for _, name := range []string{"", "root", "bad\nuser"} {
		_, err := validateAutologinUser(name, func(string) (*user.User, error) { return &user.User{Uid: "0", Username: name}, nil })
		if err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
}

func TestMissingBackgroundDoesNotWrite(t *testing.T) {
	root := t.TempDir()
	initFixture(t, root, "sysvinit")
	desktopFixture(t, root, "usr/share/xsessions/xfce.desktop", "")
	desktopFixture(t, root, "usr/share/xgreeters/lightdm-gtk-greeter.desktop", "")
	ds := desktopSystem{root, func(string, ...string) ([]byte, error) { t.Fatal("unexpected service mutation"); return nil, nil }}
	s := &Suit{Desktop: "xfce", DisplayManager: "lightdm", SessionType: "x11", LoginBackground: "/missing.jpg"}
	if err := ds.apply(s); err == nil {
		t.Fatal("missing background accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "etc/lightdm/lightdm.conf")); !os.IsNotExist(err) {
		t.Fatal("wrote configuration")
	}
}

func TestLoginYAMLValidation(t *testing.T) {
	for _, extra := range []string{"login_background: relative.jpg", "login_background: \"/image\\ninvalid\"", "autologin: maybe"} {
		root := t.TempDir()
		desktopFixture(t, root, "index.yaml", "desktop: xfce\ndisplay_manager: lightdm\n"+extra+"\n")
		if _, err := loadSuit(filepath.Join(root, "index.yaml")); err == nil {
			t.Fatalf("accepted %s", extra)
		}
	}
	root := t.TempDir()
	desktopFixture(t, root, "index.yaml", "desktop: xfce\ndisplay_manager: lightdm\nautologin: false\nlogin_background: /image.jpg\n")
	s, err := loadSuit(filepath.Join(root, "index.yaml"))
	if err != nil || s.Autologin == nil || *s.Autologin || s.LoginBackground != "/image.jpg" {
		t.Fatalf("suit=%+v err=%v", s, err)
	}
	if err := validateDesktop(&Suit{Autologin: s.Autologin}); err == nil {
		t.Fatal("login options without desktop accepted")
	}
}

func TestSystemdDisablesCompetitorWithoutStoppingIt(t *testing.T) {
	var commands []string
	ds := desktopSystem{root: t.TempDir(), run: func(name string, args ...string) ([]byte, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		if args[0] == "list-unit-files" {
			return []byte("slim.service enabled enabled\ngdm.service masked enabled\n"), nil
		}
		return nil, nil
	}}
	if err := ds.enableLogin("systemd"); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(commands, "\n")
	if !strings.Contains(joined, "systemctl disable slim.service") || !strings.Contains(joined, "systemctl set-default graphical.target") {
		t.Fatal(joined)
	}
	if strings.Contains(joined, "stop") || strings.Contains(joined, "restart") || strings.Contains(joined, "disable gdm") {
		t.Fatal(joined)
	}
}
