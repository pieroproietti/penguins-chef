package distro

import (
	"github.com/pieroproietti/penguins-chef/pkg/provision"
	"os"
	"testing"
)

func TestIdentityUsesCodenameAndReleaseFallback(t *testing.T) {
	tests := []struct {
		name string
		d    Distro
		want string
	}{
		{"codename", Distro{DistroID: "Debian", CodenameID: "Bookworm", ReleaseID: "12"}, "debian-bookworm"},
		{"release fallback", Distro{DistroID: "Arch Linux", ReleaseID: "rolling release"}, "arch-linux-rolling-release"},
		{"distribution only", Distro{DistroID: "Alpine"}, "alpine"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.Identity(); got != tt.want {
				t.Fatalf("Identity() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOpenSUSEFamilyDetection(t *testing.T) {
	for _, id := range []string{"opensuse", "opensuse-tumbleweed", "opensuse-slowroll", "opensuse-leap"} {
		d := distroFromRelease(map[string]string{"ID": id})
		if d.FamilyID != "opensuse" {
			t.Fatalf("%s: %s", id, d.FamilyID)
		}
	}
	if d := distroFromRelease(map[string]string{"ID": "derivative", "ID_LIKE": "opensuse suse"}); d.FamilyID != "opensuse" {
		t.Fatal("ID_LIKE detection failed")
	}
}

func TestManjaroSelectsArchDesktopProfiles(t *testing.T) {
	for _, like := range []string{"", "arch"} {
		d := distroFromRelease(map[string]string{"ID": "manjaro", "ID_LIKE": like, "VERSION_ID": "26.0"})
		if d.FamilyID != "archlinux" || d.DistroID != "manjaro" {
			t.Fatalf("unexpected identity: %+v", d)
		}
		for _, path := range []string{"../../recipes/dm/lightdm.yaml", "../../recipes/costumes/colibri/colibri.yaml"} {
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			recipe, err := provision.Load(f)
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			plan, err := provision.Build(recipe, d.FamilyID, "systemd")
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Steps) == 0 || plan.Steps[0].Command[0] != "pacman" {
				t.Fatal("Manjaro did not select pacman")
			}
		}
	}
}

func TestDevuanFamilyDetection(t *testing.T) {
	for _, like := range []string{"", "debian"} {
		d := distroFromRelease(map[string]string{"ID": "devuan", "ID_LIKE": like, "VERSION_CODENAME": "daedalus"})
		if d.FamilyID != "debian" || d.DistroLike != "Debian" {
			t.Fatalf("unexpected devuan identity with ID_LIKE=%q: %+v", like, d)
		}
	}
}
