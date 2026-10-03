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
		return "tailor-package:" + last + "\n", nil
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
				r.displayManager = "lightdm.service"
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
		{"init", "debian", "sysv", Profile{Services: []string{"lightdm"}}},
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
	for _, example := range []string{"lightdm", "colibri"} {
		for _, family := range []string{"debian", "archlinux", "fedora", "opensuse"} {
			f, err := os.Open("../../examples/provision/" + example + ".yaml")
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
	for _, out := range []string{"", "No matching packages", "Warning: repository unavailable", "tailor-package:"} {
		if err := backend.validateAvailability(out); err == nil {
			t.Fatalf("accepted unavailable package: %q", out)
		}
	}
	if err := backend.validateAvailability("Warning: metadata refreshed\ntailor-package:lightdm\n"); err != nil {
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
	if _, err := Build(Recipe{Profiles: map[string]Profile{"fedora": {DefaultTarget: "graphical.target"}}}, "fedora", "sysv"); err == nil {
		t.Fatal("accepted unsupported init")
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
	for _, family := range []string{"opensuse", "archlinux"} {
		for _, enabled := range []bool{false, true} {
			for _, broken := range []bool{false, true} {
				p := Plan{Family: family, Init: "systemd", Steps: []Step{{ID: "service:lightdm.service", Service: "lightdm.service"}, {ID: "default-target", DefaultTarget: "graphical.target"}}}
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
				if r.displayManager != "lightdm.service" {
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

