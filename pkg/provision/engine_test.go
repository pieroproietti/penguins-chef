package provision

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type statusError int

func (e statusError) Error() string { return fmt.Sprintf("status %d", e) }
func (e statusError) ExitCode() int { return int(e) }

type fakeRunner struct {
	family         string
	events         []string
	installed      map[string]bool
	defaultTarget  string
	brokenTarget   bool
	hostname       string
	brokenHostname bool
	enabled        bool
	unavailable    string
	failRun        string
	brokenInstall  bool
	brokenService  bool
	displayManager string
	brokenAlias    bool
	queryErr       error
}

func (r *fakeRunner) Output(_ context.Context, args []string) (string, error) {
	r.events = append(r.events, strings.Join(args, " "))
	if r.queryErr != nil {
		return "", r.queryErr
	}
	last := args[len(args)-1]
	switch {
	case (args[0] == "hostnamectl" && args[1] == "hostname") || (args[0] == "hostname" && len(args) == 1):
		return r.hostname + "\n", nil
	case args[0] == "zypper":
		if last == r.unavailable {
			return "<stream><search-result/></stream>", nil
		}
		return `<stream><search-result><solvable-list><solvable kind="package" name="` + last + `" repository="repo-oss"/></solvable-list></search-result></stream>`, nil
	case args[0] == "apt-cache":
		if last == r.unavailable {
			return "  Candidate: (none)\n", nil
		}
		return "  Candidate: 1.0\n", nil
	case args[0] == "pacman" && args[1] == "-Si":
		if last == r.unavailable {
			return "", statusError(1)
		}
		return "Name : " + last, nil
	case args[0] == "dnf" && args[2] == "repoquery":
		if last == r.unavailable {
			return "No matching packages", nil
		}
		return "chef-package:" + last + "\n", nil
	case args[0] == "systemctl" && args[1] == "show":
		return r.displayManager + "\n", nil
	case args[0] == "systemctl" && args[1] == "get-default":
		return r.defaultTarget + "\n", nil
	case args[0] == "systemctl":
		if r.enabled {
			return "enabled\n", nil
		}
		return "disabled\n", statusError(1)
	default:
		if r.installed[last] {
			if r.family == "debian" {
				return "install ok installed", nil
			}
			return last + " 1.0", nil
		}
		return "", statusError(1)
	}
}

func (r *fakeRunner) Run(_ context.Context, args []string) error {
	event := strings.Join(args, " ")
	r.events = append(r.events, event)
	if r.failRun != "" && strings.Contains(event, r.failRun) {
		return errors.New("transaction failed")
	}
	if (args[0] == "hostnamectl" && args[1] == "set-hostname") || (args[0] == "hostname" && len(args) == 2) {
		if !r.brokenHostname {
			r.hostname = args[len(args)-1]
		}
		return nil
	}
	if args[0] == "systemctl" && args[1] == "set-default" {
		if !r.brokenTarget {
			r.defaultTarget = args[2]
		}
		return nil
	}
	if args[0] == "systemctl" {
		if !r.brokenService {
			r.enabled = true
			if !r.brokenAlias {
				for _, arg := range args {
					if strings.HasSuffix(arg, ".service") && isDisplayManagerService(arg) {
						r.displayManager = arg
					}
				}
			}
		}
		return nil
	}
	if (args[0] == "dnf" && args[1] == "install") || (args[0] == "zypper" && args[2] == "install") {
		for _, pkg := range args[3:] {
			if pkg == "--" {
				return errors.New("unknown argument -- for dnf install")
			}
			if !r.brokenInstall {
				r.installed[pkg] = true
			}
		}
		return nil
	}
	for i, arg := range args {
		if arg == "--" && !r.brokenInstall {
			for _, pkg := range args[i+1:] {
				r.installed[pkg] = true
			}
			break
		}
	}
	return nil
}

func fixture(t *testing.T, family string) (Plan, *fakeRunner, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "etc", "lightdm.conf")
	plan, err := Build(Recipe{Version: 1, Name: "test", Profiles: map[string]Profile{
		family: {Packages: []string{"lightdm", "greeter"}, Files: []File{{Path: path, Content: "[Seat:*]\n"}}, Services: []string{"lightdm.service"}},
	}}, family, "systemd")
	if err != nil {
		t.Fatal(err)
	}
	return plan, &fakeRunner{family: family, installed: map[string]bool{}}, path
}

func TestExecutionOrderAndResume(t *testing.T) {
	for _, family := range []string{"debian", "archlinux", "fedora", "opensuse"} {
		t.Run(family, func(t *testing.T) {
			p, r, path := fixture(t, family)
			if err := p.Execute(context.Background(), r, io.Discard); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != "[Seat:*]\n" {
				t.Fatalf("file: %q, %v", data, err)
			}
			prepare := "apt-get update --error-on=any"
			install := "apt-get install -y -- lightdm greeter"
			if family == "archlinux" {
				prepare = "pacman -Syu --noconfirm"
				install = "pacman -S --needed --noconfirm -- lightdm greeter"
			}
			if family == "fedora" {
				prepare = "dnf --refresh makecache"
				install = "dnf install -y lightdm greeter"
			}
			if family == "opensuse" {
				prepare = "zypper --non-interactive refresh"
				install = "zypper --non-interactive install lightdm greeter"
			}
			if r.events[0] != prepare {
				t.Fatalf("first operation: %v", r.events)
			}
			index := func(event string) int {
				for i, e := range r.events {
					if e == event {
						return i
					}
				}
				return -1
			}
			enable := "systemctl enable lightdm.service"
			if family == "opensuse" || family == "archlinux" {
				enable = "systemctl enable --force lightdm.service"
			}
			if index(install) <= 2 || index(enable) <= index(install) {
				t.Fatalf("wrong order: %v", r.events)
			}
			before, _ := os.Stat(path)
			r.events = nil
			if err := p.Execute(context.Background(), r, io.Discard); err != nil {
				t.Fatal(err)
			}
			after, _ := os.Stat(path)
			if !os.SameFile(before, after) {
				t.Fatal("unchanged configuration was rewritten")
			}
			for _, e := range r.events {
				if strings.Contains(e, " install ") || strings.Contains(e, " --needed ") || strings.Contains(e, " enable ") {
					t.Fatalf("repeated mutation: %s", e)
				}
			}
		})
	}
}

func TestFailuresStopDependentOperations(t *testing.T) {
	for _, family := range []string{"debian", "archlinux", "fedora", "opensuse"} {
		for _, failure := range []string{"prepare", "unavailable", "install", "postcondition", "query", "service"} {
			t.Run(family+"/"+failure, func(t *testing.T) {
				p, r, path := fixture(t, family)
				switch failure {
				case "prepare":
					r.failRun = "apt-get update"
					if family == "archlinux" {
						r.failRun = "-Syu"
					}
					if family == "fedora" {
						r.failRun = "makecache"
					}
					if family == "opensuse" {
						r.failRun = "refresh"
					}
				case "unavailable":
					r.unavailable = "greeter"
				case "install":
					r.failRun = " -- "
					if family == "fedora" {
						r.failRun = "dnf install "
					}
					if family == "opensuse" {
						r.failRun = "zypper --non-interactive install "
					}
				case "postcondition":
					r.brokenInstall = true
				case "query":
					r.queryErr = errors.New("cannot execute query")
				case "service":
					r.brokenService = true
				}
				if err := p.Execute(context.Background(), r, io.Discard); err == nil {
					t.Fatal("expected failure")
				}
				if failure != "service" {
					if _, err := os.Stat(path); !os.IsNotExist(err) {
						t.Fatalf("configuration applied after failure: %v", err)
					}
					for _, e := range r.events {
						if strings.HasPrefix(e, "systemctl") {
							t.Fatalf("init executed after failure: %v", r.events)
						}
					}
				}
				if failure == "unavailable" {
					for _, e := range r.events {
						if strings.Contains(e, " -- ") {
							t.Fatal("installed before all availability checks")
						}
					}
				}
			})
		}
	}
}

func TestInterruptedExecutionResumesFromSystemState(t *testing.T) {
	p, r, path := fixture(t, "debian")
	r.failRun = "systemctl enable"
	if err := p.Execute(context.Background(), r, io.Discard); err == nil {
		t.Fatal("expected service failure")
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	r.failRun = ""
	r.events = nil
	if err := p.Execute(context.Background(), r, io.Discard); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(path)
	if !os.SameFile(before, after) {
		t.Fatal("configuration rewritten on resume")
	}
	for _, e := range r.events {
		if strings.Contains(e, " install ") {
			t.Fatal("packages reinstalled on resume")
		}
	}
}

func TestManagedFileRejectsSymlinksBeforeMutation(t *testing.T) {
	p, r, path := fixture(t, "debian")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "original")
	if err := os.WriteFile(target, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if err := p.Execute(context.Background(), r, io.Discard); err == nil {
		t.Fatal("symlink accepted")
	}
	if len(r.events) != 0 {
		t.Fatalf("mutated before preflight: %v", r.events)
	}
	data, _ := os.ReadFile(target)
	if string(data) != "original" {
		t.Fatal("symlink target modified")
	}
}

func TestFileReconcilesContentAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := reconcileFile(File{Path: path, Content: "new"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	info, _ := os.Stat(path)
	if string(data) != "new" || info.Mode().Perm() != 0644 {
		t.Fatalf("wrong content/mode: %q %v", data, info.Mode())
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := reconcileFile(File{Path: path, Content: "new"}); err != nil {
		t.Fatal(err)
	}
	info, _ = os.Stat(path)
	if info.Mode().Perm() != 0644 {
		t.Fatal("permissions not reconciled")
	}
}

func TestBuildRejectsInvalidProfiles(t *testing.T) {
	cases := []struct {
		name, family, init string
		profile            Profile
	}{
		{"backend", "alpine", "systemd", Profile{}},
		{"argument", "debian", "systemd", Profile{Packages: []string{"--purge"}}},
		{"relative", "debian", "systemd", Profile{Files: []File{{Path: "etc/config"}}}},
		{"duplicate", "debian", "systemd", Profile{Files: []File{{Path: "/etc/config"}, {Path: "/etc/config"}}}},
		{"parent-file", "debian", "systemd", Profile{Files: []File{{Path: "/etc/config"}, {Path: "/etc/config/child"}}}},
		{"dnf-repository", "fedora", "systemd", Profile{Repositories: []File{{Path: "/etc/yum.repos.d/test.repo"}}}},
		{"repository", "archlinux", "systemd", Profile{Repositories: []File{{Path: "/etc/pacman.conf"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Build(Recipe{Name: "test", Profiles: map[string]Profile{tc.family: tc.profile}}, tc.family, tc.init)
			if err == nil {
				t.Fatal("invalid profile accepted")
			}
		})
	}
}

func TestDebianAPTTrustsPackageServiceManagementOnNonSystemdInit(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "etc", "lightdm.conf")
	recipe := Recipe{
		Version: 1,
		Name:    "test-decouple",
		Profiles: map[string]Profile{
			"debian": {
				Packages:      []string{"lightdm", "lightdm-gtk-greeter"},
				Files:         []File{{Path: filePath, Content: "[Seat:*]\n"}},
				Services:      []string{"lightdm.service"},
				DefaultTarget: "graphical.target",
			},
		},
	}

	for _, initName := range []string{"sysv", "sysvinit", "openrc", "unknown", "none"} {
		t.Run("non-systemd-"+initName, func(t *testing.T) {
			plan, err := Build(recipe, "debian", initName)
			if err != nil {
				t.Fatalf("Build failed for debian with init %s: %v", initName, err)
			}
			// Verify that no service or default-target steps were added (delegated cleanly to APT)
			for _, step := range plan.Steps {
				if step.Service != "" {
					t.Fatalf("unexpected service step %s in plan for init %s", step.ID, initName)
				}
				if step.DefaultTarget != "" {
					t.Fatalf("unexpected default-target step %s in plan for init %s", step.ID, initName)
				}
			}
			// Verify package installation step is present
			hasPkgInstall := false
			for _, step := range plan.Steps {
				if step.ID == "packages:install" {
					hasPkgInstall = true
					expected := []string{"lightdm", "lightdm-gtk-greeter"}
					if !reflect.DeepEqual(step.Packages, expected) {
						t.Fatalf("packages = %v, want %v", step.Packages, expected)
					}
				}
			}
			if !hasPkgInstall {
				t.Fatal("packages:install step missing from plan")
			}
			// Verify execution does not attempt to invoke systemctl or fail
			runner := &fakeRunner{family: "debian", installed: map[string]bool{}}
			var out strings.Builder
			if err := plan.Execute(context.Background(), runner, &out); err != nil {
				t.Fatalf("Execute failed for debian with init %s: %v", initName, err)
			}
			for _, ev := range runner.events {
				if strings.Contains(ev, "systemctl") {
					t.Fatalf("systemctl called unexpectedly for init %s: %s", initName, ev)
				}
			}
		})
	}

	// Verify that with systemd, explicit service and target steps ARE included and managed
	t.Run("systemd", func(t *testing.T) {
		plan, err := Build(recipe, "debian", "systemd")
		if err != nil {
			t.Fatalf("Build failed for debian with systemd: %v", err)
		}
		hasService := false
		hasTarget := false
		for _, step := range plan.Steps {
			if step.ID == "service:lightdm.service" {
				hasService = true
			}
			if step.ID == "default-target:graphical.target" {
				hasTarget = true
			}
		}
		if !hasService || !hasTarget {
			t.Fatalf("expected service and default-target steps with systemd, got steps: %v", plan.Steps)
		}
		runner := &fakeRunner{family: "debian", installed: map[string]bool{}}
		var out strings.Builder
		if err := plan.Execute(context.Background(), runner, &out); err != nil {
			t.Fatalf("Execute failed for debian with systemd: %v", err)
		}
		systemctlCalled := false
		for _, ev := range runner.events {
			if strings.Contains(ev, "systemctl") {
				systemctlCalled = true
				break
			}
		}
		if !systemctlCalled {
			t.Fatal("expected systemctl to be called for systemd init")
		}
	})

	// Verify that non-systemd init is tolerated across all distributions without blocking
	for _, fam := range []string{"fedora", "archlinux", "opensuse"} {
		t.Run("tolerant-"+fam, func(t *testing.T) {
			famRecipe := Recipe{
				Version: 1,
				Name:    "test",
				Profiles: map[string]Profile{
					fam: {
						Services: []string{"lightdm.service"},
					},
				},
			}
			plan, err := Build(famRecipe, fam, "sysv")
			if err != nil {
				t.Fatalf("expected Build to succeed for %s with sysv, got %v", fam, err)
			}
			for _, step := range plan.Steps {
				if step.Service != "" {
					t.Fatalf("unexpected service step for %s with sysv: %s", fam, step.ID)
				}
			}
		})
	}
}

func TestBaseRecipeConditionalSystemdPackages(t *testing.T) {
	f, err := os.Open("../../recipes/base/base.yaml")
	if err != nil {
		t.Fatalf("failed to open base.yaml: %v", err)
	}
	defer f.Close()

	recipe, err := Load(f)
	if err != nil {
		t.Fatalf("failed to load base.yaml: %v", err)
	}

	// 1. With systemd on debian, systemd-timesyncd is included in packages and services
	planSystemd, err := Build(recipe, "debian", "systemd")
	if err != nil {
		t.Fatalf("Build failed for base debian with systemd: %v", err)
	}
	hasTimesyncdPkg := false
	hasTimesyncdSvc := false
	for _, step := range planSystemd.Steps {
		if step.ID == "packages:install" {
			for _, pkg := range step.Packages {
				if pkg == "systemd-timesyncd" {
					hasTimesyncdPkg = true
				}
			}
		}
		if step.ID == "service:systemd-timesyncd.service" {
			hasTimesyncdSvc = true
		}
	}
	if !hasTimesyncdPkg {
		t.Fatal("expected systemd-timesyncd in packages for debian with systemd")
	}
	if !hasTimesyncdSvc {
		t.Fatal("expected systemd-timesyncd.service in services for debian with systemd")
	}

	// 2. With non-systemd (e.g. sysv, sysvinit, openrc) on debian, systemd-timesyncd is excluded
	for _, initName := range []string{"sysv", "sysvinit", "openrc", "unknown"} {
		t.Run("non-systemd-"+initName, func(t *testing.T) {
			planNonSystemd, err := Build(recipe, "debian", initName)
			if err != nil {
				t.Fatalf("Build failed for base debian with init %s: %v", initName, err)
			}
			for _, step := range planNonSystemd.Steps {
				if step.ID == "packages:install" {
					for _, pkg := range step.Packages {
						if pkg == "systemd-timesyncd" {
							t.Fatalf("unexpected systemd-timesyncd in packages for debian with init %s", initName)
						}
					}
				}
				if step.Service != "" {
					t.Fatalf("unexpected service step %s for debian with init %s", step.ID, initName)
				}
			}
		})
	}
}

func TestDebianExcludesSystemdPackagesOnNonSystemdInit(t *testing.T) {
	// Even if a recipe explicitly declares systemd-timesyncd in generic debian packages,
	// building for non-systemd debian excludes it to prevent blocking availability checks on Devuan.
	r := Recipe{
		Version: 1,
		Name:    "legacy-recipe",
		Profiles: map[string]Profile{
			"debian": {
				Packages: []string{"curl", "systemd-timesyncd", "systemd-resolved", "git"},
			},
		},
	}

	planSysv, err := Build(r, "debian", "sysv")
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	for _, step := range planSysv.Steps {
		if step.ID == "packages:install" {
			expected := []string{"curl", "git"}
			if !reflect.DeepEqual(step.Packages, expected) {
				t.Fatalf("packages = %v, want %v", step.Packages, expected)
			}
		}
		if strings.HasPrefix(step.ID, "available:systemd-") {
			t.Fatalf("unexpected availability check: %s", step.ID)
		}
	}

	// On systemd, all packages are preserved
	planSystemd, err := Build(r, "debian", "systemd")
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	for _, step := range planSystemd.Steps {
		if step.ID == "packages:install" {
			expected := []string{"curl", "systemd-timesyncd", "systemd-resolved", "git"}
			if !reflect.DeepEqual(step.Packages, expected) {
				t.Fatalf("packages = %v, want %v", step.Packages, expected)
			}
		}
	}
}

func TestStrictSchema(t *testing.T) {
	for _, input := range []string{
		"version: 1\nname: test\nprofiles:\n  debian:\n    package: [lightdm]\n",
		"version: 2\nname: test\nprofiles: {debian: {}}\n",
		"version: 1\nname: test\nprofiles: {debian: {}}\n---\nname: second\n",
	} {
		if _, err := Load(strings.NewReader(input)); err == nil {
			t.Fatalf("invalid schema accepted: %s", input)
		}
	}
	for _, recipePath := range []string{"../../recipes/dm/lightdm.yaml", "../../recipes/costumes/colibri/colibri.yaml"} {
		for _, family := range []string{"debian", "archlinux", "fedora", "opensuse"} {
			f, err := os.Open(recipePath)
			if err != nil {
				t.Fatal(err)
			}
			recipe, err := Load(f)
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			p, err := Build(recipe, family, "systemd")
			if err != nil {
				t.Fatal(err)
			}
			before := append([]Step(nil), p.Steps...)
			if err := p.Describe(io.Discard); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, p.Steps) {
				t.Fatal("description mutated plan")
			}
		}
	}
}

func TestDNFAvailabilityRejectsEmptyAndDiagnosticOutput(t *testing.T) {
	backend := dnfBackend{}
	for _, out := range []string{"", "No matching packages", "Warning: repository unavailable", "chef-package:"} {
		if err := backend.validateAvailability(out); err == nil {
			t.Fatalf("accepted unavailable package: %q", out)
		}
	}
	if err := backend.validateAvailability("Warning: metadata refreshed\nchef-package:lightdm\n"); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultTargetReconciliation(t *testing.T) {
	for _, failure := range []string{"", "query", "set", "verify", "service"} {
		t.Run(failure, func(t *testing.T) {
			p, err := Build(Recipe{Profiles: map[string]Profile{"fedora": {Services: []string{"lightdm.service"}, DefaultTarget: "graphical.target"}}}, "fedora", "systemd")
			if err != nil {
				t.Fatal(err)
			}
			r := &fakeRunner{defaultTarget: "multi-user.target"}
			switch failure {
			case "query":
				r.queryErr = errors.New("query failed")
			case "set":
				r.failRun = "set-default"
			case "verify":
				r.brokenTarget = true
			case "service":
				r.brokenService = true
			}
			err = p.Execute(context.Background(), r, io.Discard)
			if failure != "" {
				if err == nil {
					t.Fatal("expected failure")
				}
				if failure == "service" && r.defaultTarget != "multi-user.target" {
					t.Fatal("target changed after service failure")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if r.defaultTarget != "graphical.target" {
				t.Fatal("default target unchanged")
			}
			r.events = nil
			if err := p.Execute(context.Background(), r, io.Discard); err != nil {
				t.Fatal(err)
			}
			for _, event := range r.events {
				if strings.Contains(event, "set-default") {
					t.Fatal("repeated target mutation")
				}
			}
		})
	}
}

func TestDefaultTargetValidation(t *testing.T) {
	for _, target := range []string{"--bad.target", "graphical.service", "../graphical.target"} {
		if _, err := Build(Recipe{Profiles: map[string]Profile{"fedora": {DefaultTarget: target}}}, "fedora", "systemd"); err == nil {
			t.Fatalf("accepted %q", target)
		}
	}
	plan, err := Build(Recipe{Profiles: map[string]Profile{"fedora": {DefaultTarget: "graphical.target"}}}, "fedora", "sysv")
	if err != nil {
		t.Fatalf("unexpected Build failure with sysv: %v", err)
	}
	for _, step := range plan.Steps {
		if step.DefaultTarget != "" {
			t.Fatalf("unexpected default-target step with sysv: %s", step.ID)
		}
	}
}

func TestZypperAvailabilityRequiresRepositoryPackage(t *testing.T) {
	backend := zypperBackend{}
	for _, out := range []string{"", "No matching packages", "<stream><search-result/></stream>", `<stream><search-result><solvable-list><solvable kind="package" name="lightdm" repository="(System Packages)"/></solvable-list></search-result></stream>`, `<stream><search-result><solvable-list><solvable kind="package" name="lightdm" repository="@System"/></solvable-list></search-result></stream>`, `<stream><search-result><solvable-list><solvable kind="pattern" name="lightdm" repository="repo-oss"/></solvable-list></search-result></stream>`} {
		if err := backend.validateAvailability(out); err == nil {
			t.Fatalf("accepted %q", out)
		}
	}
	if err := backend.validateAvailability(`<?xml version="1.0"?><stream><message type="info">Loading repositories</message><search-result><solvable-list><solvable kind="package" name="lightdm" repository="repo-oss"/></solvable-list></search-result></stream>`); err != nil {
		t.Fatal(err)
	}
	if err := backend.validateRepository(File{Path: "/etc/zypp/repos.d/test.repo"}); err == nil {
		t.Fatal("accepted unsupported repository mutation")
	}
}

func TestDisplayManagerAliasReconciliation(t *testing.T) {
	for _, service := range []string{"lightdm.service", "sddm.service"} {
		for _, family := range []string{"opensuse", "archlinux"} {
			for _, enabled := range []bool{false, true} {
				for _, broken := range []bool{false, true} {
					p := Plan{Family: family, Init: "systemd", Steps: []Step{{ID: "service:" + service, Service: service}, {ID: "default-target", DefaultTarget: "graphical.target"}}}
					r := &fakeRunner{enabled: enabled, displayManager: "display-manager-legacy.service", brokenAlias: broken, defaultTarget: "multi-user.target"}
					err := p.Execute(context.Background(), r, io.Discard)
					if broken {
						if err == nil {
							t.Fatal("accepted incorrect alias")
						}
						if r.defaultTarget != "multi-user.target" {
							t.Fatal("changed target after failed alias verification")
						}
						continue
					}
					if err != nil {
						t.Fatal(err)
					}
					if r.displayManager != service {
						t.Fatal("legacy alias retained")
					}
					r.events = nil
					if err := p.Execute(context.Background(), r, io.Discard); err != nil {
						t.Fatal(err)
					}
					for _, event := range r.events {
						if strings.Contains(event, " enable ") {
							t.Fatal("repeated enable")
						}
					}
				}
			}
		}
	}
}

func TestRecipeIncludeResolution(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "base.yaml")
	baseContent := `version: 1
name: base
profiles:
  debian:
    packages: [sudo, curl]
    files:
      - path: /etc/test.conf
        content: base-content
    services: [systemd-timesyncd.service]
`
	if err := os.WriteFile(basePath, []byte(baseContent), 0644); err != nil {
		t.Fatal(err)
	}

	childPath := filepath.Join(dir, "child.yaml")
	childContent := `version: 1
name: child
include:
  - base.yaml
profiles:
  debian:
    packages: [curl, git]
    files:
      - path: /etc/test.conf
        content: child-override
      - path: /etc/extra.conf
        content: extra
    default_target: graphical.target
    services: [lightdm.service]
`
	if err := os.WriteFile(childPath, []byte(childContent), 0644); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(childPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	recipe, err := Load(f)
	if err != nil {
		t.Fatal(err)
	}

	plan, err := Build(recipe, "debian", "systemd")
	if err != nil {
		t.Fatal(err)
	}

	// Verify packages: sudo, curl, git (curl deduplicated)
	pkgsStep := false
	for _, s := range plan.Steps {
		if s.ID == "packages:install" {
			pkgsStep = true
			expected := []string{"sudo", "curl", "git"}
			if !reflect.DeepEqual(s.Packages, expected) {
				t.Fatalf("packages = %v, want %v", s.Packages, expected)
			}
		}
	}
	if !pkgsStep {
		t.Fatal("packages:install step missing")
	}

	// Verify file override
	overrideFound := false
	for _, s := range plan.Steps {
		if s.File != nil && s.File.Path == "/etc/test.conf" {
			if s.File.Content != "child-override" {
				t.Fatalf("file content = %q, want %q", s.File.Content, "child-override")
			}
			overrideFound = true
		}
	}
	if !overrideFound {
		t.Fatal("overridden file /etc/test.conf not found in plan")
	}

	// Verify services: timesyncd and lightdm
	services := []string{}
	for _, s := range plan.Steps {
		if s.Service != "" {
			services = append(services, s.Service)
		}
	}
	expectedServices := []string{"systemd-timesyncd.service", "lightdm.service"}
	if !reflect.DeepEqual(services, expectedServices) {
		t.Fatalf("services = %v, want %v", services, expectedServices)
	}
}

func TestRecipeIncludeCircularDetection(t *testing.T) {
	dir := t.TempDir()
	aPath := filepath.Join(dir, "a.yaml")
	bPath := filepath.Join(dir, "b.yaml")

	aContent := fmt.Sprintf("version: 1\nname: a\ninclude:\n  - %s\nprofiles:\n  debian: {}\n", bPath)
	bContent := fmt.Sprintf("version: 1\nname: b\ninclude:\n  - %s\nprofiles:\n  debian: {}\n", aPath)

	if err := os.WriteFile(aPath, []byte(aContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bPath, []byte(bContent), 0644); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(aPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	recipe, err := Load(f)
	if err != nil {
		t.Fatal(err)
	}

	_, err = Build(recipe, "debian", "systemd")
	if err == nil || !strings.Contains(err.Error(), "circular include detected") {
		t.Fatalf("expected circular include error, got: %v", err)
	}
}

func TestHostnameReconciliation(t *testing.T) {
	ctx := context.Background()

	// 1. Hostname change needed: queries, sets, verifies.
	r := &fakeRunner{hostname: "oldhost"}
	p := Plan{Steps: []Step{{ID: "hostname:newhost", Phase: "configuration", Hostname: "newhost"}}}
	if err := p.Execute(ctx, r, io.Discard); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.hostname != "newhost" {
		t.Fatalf("hostname was not updated, got %s", r.hostname)
	}
	expectedEvents := []string{
		"hostnamectl hostname",
		"hostnamectl set-hostname newhost",
		"hostnamectl hostname",
	}
	if !reflect.DeepEqual(r.events, expectedEvents) {
		t.Fatalf("events = %v, want %v", r.events, expectedEvents)
	}

	// 2. Idempotent: hostname already matches, no mutation needed.
	r = &fakeRunner{hostname: "newhost"}
	if err := p.Execute(ctx, r, io.Discard); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.events) != 1 || r.events[0] != "hostnamectl hostname" {
		t.Fatalf("unexpected events on already-matching hostname: %v", r.events)
	}

	// 3. Execution failure during set-hostname.
	r = &fakeRunner{hostname: "oldhost", failRun: "set-hostname"}
	if err := p.Execute(ctx, r, io.Discard); err == nil {
		t.Fatal("expected failure when set-hostname fails")
	}

	// 4. Verification failure: hostnamectl output does not match target.
	r = &fakeRunner{hostname: "oldhost", brokenHostname: true}
	if err := p.Execute(ctx, r, io.Discard); err == nil || !strings.Contains(err.Error(), "hostname is not newhost") {
		t.Fatalf("expected verification failure, got: %v", err)
	}
}

func TestUpdateHostsContent(t *testing.T) {
	// Debian style: replace oldhost and oldhost.localdomain on 127.0.1.1
	debianHosts := "127.0.0.1 localhost\n127.0.1.1 debian debian.localdomain\n::1 localhost ip6-localhost ip6-loopback\n"
	updatedDebian := updateHostsContent(debianHosts, "debian", "colibri")
	expectedDebian := "127.0.0.1 localhost\n127.0.1.1 colibri colibri.localdomain\n::1 localhost ip6-localhost ip6-loopback\n"
	if updatedDebian != expectedDebian {
		t.Fatalf("debian hosts mismatch:\ngot:\n%s\nwant:\n%s", updatedDebian, expectedDebian)
	}

	// Fedora style: no oldhost in file, append to 127.0.0.1
	fedoraHosts := "127.0.0.1 localhost localhost.localdomain\n::1 localhost\n"
	updatedFedora := updateHostsContent(fedoraHosts, "fedora", "colibri")
	expectedFedora := "127.0.0.1 localhost localhost.localdomain colibri\n::1 localhost\n"
	if updatedFedora != expectedFedora {
		t.Fatalf("fedora hosts mismatch:\ngot:\n%s\nwant:\n%s", updatedFedora, expectedFedora)
	}

	// Idempotency: newhost already present
	idempotent := updateHostsContent(expectedFedora, "fedora", "colibri")
	if idempotent != expectedFedora {
		t.Fatalf("idempotency failed, content changed:\n%s", idempotent)
	}

	// Never replace localhost even if oldHost is passed as "localhost"
	localhostInput := "127.0.0.1 localhost\n::1 localhost\n"
	updatedLocalhost := updateHostsContent(localhostInput, "localhost", "colibri")
	if strings.Contains(updatedLocalhost, "127.0.0.1 colibri\n") {
		t.Fatal("localhost was wrongly overwritten")
	}
	if !strings.Contains(updatedLocalhost, "127.0.0.1 localhost colibri") {
		t.Fatalf("colibri was not appended to localhost line: %s", updatedLocalhost)
	}

	// Empty input generates standard hosts
	emptyResult := updateHostsContent("", "", "colibri")
	expectedEmpty := "127.0.0.1 localhost colibri\n::1 localhost\n"
	if emptyResult != expectedEmpty {
		t.Fatalf("empty content result mismatch:\ngot:\n%s\nwant:\n%s", emptyResult, expectedEmpty)
	}
}

func TestReconcileHostsFile(t *testing.T) {
	dir := t.TempDir()
	hostsPath := filepath.Join(dir, "hosts")
	initial := "127.0.0.1 localhost\n127.0.1.1 oldbox\n"
	if err := os.WriteFile(hostsPath, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	if err := reconcileHostsFile(hostsPath, "oldbox", "colibri"); err != nil {
		t.Fatalf("reconcileHostsFile failed: %v", err)
	}

	data, err := os.ReadFile(hostsPath)
	if err != nil {
		t.Fatal(err)
	}
	expected := "127.0.0.1 localhost\n127.0.1.1 colibri\n"
	if string(data) != expected {
		t.Fatalf("got:\n%s\nwant:\n%s", string(data), expected)
	}

	info, err := os.Stat(hostsPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0644 {
		t.Fatalf("permissions = %v, want 0644", info.Mode().Perm())
	}

	// Idempotent second run
	if err := reconcileHostsFile(hostsPath, "colibri", "colibri"); err != nil {
		t.Fatalf("second run failed: %v", err)
	}
	data2, err := os.ReadFile(hostsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data2) != expected {
		t.Fatalf("content changed on second run: %s", string(data2))
	}
}

func TestCostumeHostnameInBuild(t *testing.T) {
	// Explicit Hostname set
	r1 := Recipe{
		Version:  1,
		Name:     "custom",
		Hostname: "myhost",
		Profiles: map[string]Profile{"fedora": {}},
	}
	p1, err := Build(r1, "fedora", "systemd")
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	var foundHostname bool
	for _, s := range p1.Steps {
		if s.ID == "hostname:myhost" && s.Hostname == "myhost" && s.Phase == "configuration" {
			foundHostname = true
			break
		}
	}
	if !foundHostname {
		t.Fatal("step hostname:myhost missing from plan")
	}

	// Costume directory detection when Hostname is empty
	r2 := Recipe{
		Version:  1,
		Name:     "colibri",
		baseDir:  "/home/user/recipes/costumes/colibri",
		Profiles: map[string]Profile{"fedora": {}},
	}
	p2, err := Build(r2, "fedora", "systemd")
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	foundHostname = false
	for _, s := range p2.Steps {
		if s.ID == "hostname:colibri" && s.Hostname == "colibri" && s.Phase == "configuration" {
			foundHostname = true
			break
		}
	}
	if !foundHostname {
		t.Fatal("step hostname:colibri missing from costume plan")
	}

	// Non-costume recipe (desktop) without explicit hostname has NO hostname step
	r3 := Recipe{
		Version:  1,
		Name:     "xfce4",
		baseDir:  "/home/user/recipes/de",
		Profiles: map[string]Profile{"fedora": {}},
	}
	p3, err := Build(r3, "fedora", "systemd")
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	for _, s := range p3.Steps {
		if strings.HasPrefix(s.ID, "hostname:") || s.Hostname != "" {
			t.Fatalf("non-costume recipe unexpectedly configured hostname: %v", s)
		}
	}

	// Invalid hostname validation
	invalidNames := []string{"-leadingdash", "trailingdash-", "has_underscore", "toolong" + strings.Repeat("x", 60)}
	for _, inv := range invalidNames {
		rInv := Recipe{
			Version:  1,
			Name:     "test",
			Hostname: inv,
			Profiles: map[string]Profile{"fedora": {}},
		}
		if _, err := Build(rInv, "fedora", "systemd"); err == nil {
			t.Fatalf("Build accepted invalid hostname %q", inv)
		}
	}

	// Hostname step is preserved with alternative init
	pSysv, err := Build(r1, "fedora", "sysv")
	if err != nil {
		t.Fatalf("Build failed with sysv: %v", err)
	}
	hasHostStep := false
	for _, step := range pSysv.Steps {
		if step.Hostname == "myhost" {
			hasHostStep = true
		}
	}
	if !hasHostStep {
		t.Fatal("expected hostname step in plan with sysv")
	}
}

func TestColibriRecipeHasHostname(t *testing.T) {
	path := "../../recipes/costumes/colibri/colibri.yaml"
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open colibri.yaml: %v", err)
	}
	defer f.Close()

	recipe, err := Load(f)
	if err != nil {
		t.Fatalf("failed to load colibri.yaml: %v", err)
	}
	if recipe.Hostname != "colibri" {
		t.Fatalf("recipe.Hostname = %q, want %q", recipe.Hostname, "colibri")
	}

	for _, family := range []string{"debian", "archlinux", "fedora", "opensuse"} {
		plan, err := Build(recipe, family, "systemd")
		if err != nil {
			t.Fatalf("Build for %s failed: %v", family, err)
		}
		var found bool
		for _, s := range plan.Steps {
			if s.ID == "hostname:colibri" && s.Hostname == "colibri" && s.Phase == "configuration" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("step hostname:colibri missing in plan for family %s", family)
		}
	}
}

func TestDuckRecipeHasHostnameAndBuilds(t *testing.T) {
	path := "../../recipes/costumes/duck/duck.yaml"
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open duck.yaml: %v", err)
	}
	defer f.Close()

	recipe, err := Load(f)
	if err != nil {
		t.Fatalf("failed to load duck.yaml: %v", err)
	}
	if recipe.Hostname != "duck" {
		t.Fatalf("recipe.Hostname = %q, want %q", recipe.Hostname, "duck")
	}

	for _, family := range []string{"debian", "archlinux", "fedora", "opensuse"} {
		plan, err := Build(recipe, family, "systemd")
		if err != nil {
			t.Fatalf("Build for %s failed: %v", family, err)
		}
		var found bool
		for _, s := range plan.Steps {
			if s.ID == "hostname:duck" && s.Hostname == "duck" && s.Phase == "configuration" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("step hostname:duck missing in plan for family %s", family)
		}
	}
}

func TestEagleRecipeHasHostnameAndBuilds(t *testing.T) {
	path := "../../recipes/costumes/eagle/eagle.yaml"
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open eagle.yaml: %v", err)
	}
	defer f.Close()

	recipe, err := Load(f)
	if err != nil {
		t.Fatalf("failed to load eagle.yaml: %v", err)
	}
	if recipe.Hostname != "eagle" {
		t.Fatalf("recipe.Hostname = %q, want %q", recipe.Hostname, "eagle")
	}

	for _, family := range []string{"debian", "archlinux", "fedora", "opensuse"} {
		plan, err := Build(recipe, family, "systemd")
		if err != nil {
			t.Fatalf("Build for %s failed: %v", family, err)
		}
		var found bool
		for _, s := range plan.Steps {
			if s.ID == "hostname:eagle" && s.Hostname == "eagle" && s.Phase == "configuration" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("step hostname:eagle missing in plan for family %s", family)
		}
	}
}

func TestMateDesktopRecipeBuilds(t *testing.T) {
	path := "../../recipes/de/mate.yaml"
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open mate.yaml: %v", err)
	}
	defer f.Close()

	recipe, err := Load(f)
	if err != nil {
		t.Fatalf("failed to load mate.yaml: %v", err)
	}
	if recipe.Name != "mate-desktop" {
		t.Fatalf("recipe.Name = %q, want %q", recipe.Name, "mate-desktop")
	}

	for _, family := range []string{"debian", "archlinux", "fedora", "opensuse"} {
		plan, err := Build(recipe, family, "systemd")
		if err != nil {
			t.Fatalf("Build for %s failed: %v", family, err)
		}
		var foundPackages bool
		for _, s := range plan.Steps {
			if s.ID == "packages:install" {
				foundPackages = true
				break
			}
		}
		if !foundPackages {
			t.Fatalf("step packages:install missing in plan for family %s", family)
		}
	}
}

func TestSparrowRecipeHasHostnameAndBuilds(t *testing.T) {
	path := "../../recipes/costumes/sparrow/sparrow.yaml"
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open sparrow.yaml: %v", err)
	}
	defer f.Close()

	recipe, err := Load(f)
	if err != nil {
		t.Fatalf("failed to load sparrow.yaml: %v", err)
	}
	if recipe.Hostname != "sparrow" {
		t.Fatalf("recipe.Hostname = %q, want %q", recipe.Hostname, "sparrow")
	}

	for _, family := range []string{"debian", "archlinux", "fedora", "opensuse"} {
		plan, err := Build(recipe, family, "systemd")
		if err != nil {
			t.Fatalf("Build for %s failed: %v", family, err)
		}
		var found bool
		for _, s := range plan.Steps {
			if s.ID == "hostname:sparrow" && s.Hostname == "sparrow" && s.Phase == "configuration" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("step hostname:sparrow missing in plan for family %s", family)
		}
	}
}

func TestLxqtDesktopRecipeBuilds(t *testing.T) {
	path := "../../recipes/de/lxqt.yaml"
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open lxqt.yaml: %v", err)
	}
	defer f.Close()

	recipe, err := Load(f)
	if err != nil {
		t.Fatalf("failed to load lxqt.yaml: %v", err)
	}
	if recipe.Name != "lxqt-desktop" {
		t.Fatalf("recipe.Name = %q, want %q", recipe.Name, "lxqt-desktop")
	}

	for _, family := range []string{"debian", "archlinux", "fedora", "opensuse"} {
		plan, err := Build(recipe, family, "systemd")
		if err != nil {
			t.Fatalf("Build for %s failed: %v", family, err)
		}
		var foundPackages bool
		for _, s := range plan.Steps {
			if s.ID == "packages:install" {
				foundPackages = true
				break
			}
		}
		if !foundPackages {
			t.Fatalf("step packages:install missing in plan for family %s", family)
		}
	}
}

func TestDisplayManagerRecipesBuild(t *testing.T) {
	for _, dm := range []struct {
		path    string
		service string
	}{
		{"../../recipes/dm/lightdm.yaml", "service:lightdm.service"},
		{"../../recipes/dm/sddm.yaml", "service:sddm.service"},
	} {
		f, err := os.Open(dm.path)
		if err != nil {
			t.Fatalf("failed to open %s: %v", dm.path, err)
		}
		recipe, err := Load(f)
		f.Close()
		if err != nil {
			t.Fatalf("failed to load %s: %v", dm.path, err)
		}
		for _, family := range []string{"debian", "archlinux", "fedora", "opensuse"} {
			plan, err := Build(recipe, family, "systemd")
			if err != nil {
				t.Fatalf("Build for %s failed: %v", family, err)
			}
			var foundDM, foundTarget bool
			for _, s := range plan.Steps {
				if s.ID == dm.service {
					foundDM = true
				}
				if s.ID == "default-target:graphical.target" {
					foundTarget = true
				}
			}
			if !foundDM {
				t.Fatalf("step %s missing in plan for %s", dm.service, dm.path)
			}
			if !foundTarget {
				t.Fatalf("step default-target missing in plan for %s", dm.path)
			}
		}
	}
}

func TestSwallowRecipeHasHostnameAndBuilds(t *testing.T) {
	path := "../../recipes/costumes/swallow/swallow.yaml"
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open swallow.yaml: %v", err)
	}
	defer f.Close()

	recipe, err := Load(f)
	if err != nil {
		t.Fatalf("failed to load swallow.yaml: %v", err)
	}
	if recipe.Hostname != "swallow" {
		t.Fatalf("recipe.Hostname = %q, want %q", recipe.Hostname, "swallow")
	}

	for _, family := range []string{"debian", "archlinux", "fedora", "opensuse"} {
		plan, err := Build(recipe, family, "systemd")
		if err != nil {
			t.Fatalf("Build for %s failed: %v", family, err)
		}
		var found bool
		for _, s := range plan.Steps {
			if s.ID == "hostname:swallow" && s.Hostname == "swallow" && s.Phase == "configuration" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("step hostname:swallow missing in plan for family %s", family)
		}
	}
}

func TestAppRecipesBuild(t *testing.T) {
	for _, appPath := range []string{
		"../../recipes/graphics/gimp.yaml",
		"../../recipes/office/libreoffice.yaml",
		"../../recipes/multimedia/vlc.yaml",
	} {
		f, err := os.Open(appPath)
		if err != nil {
			t.Fatalf("failed to open %s: %v", appPath, err)
		}
		recipe, err := Load(f)
		f.Close()
		if err != nil {
			t.Fatalf("failed to load %s: %v", appPath, err)
		}
		for _, family := range []string{"debian", "archlinux", "fedora", "opensuse"} {
			plan, err := Build(recipe, family, "systemd")
			if err != nil {
				t.Fatalf("Build for %s (%s) failed: %v", appPath, family, err)
			}
			if len(plan.Steps) == 0 {
				t.Fatalf("expected plan steps for %s (%s)", appPath, family)
			}
		}
	}
}
