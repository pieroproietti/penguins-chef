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
	family        string
	events        []string
	installed     map[string]bool
	defaultTarget string
	brokenTarget  bool
	enabled       bool
	unavailable   string
	failRun       string
	brokenInstall bool
	brokenService bool
	queryErr      error
}

func (r *fakeRunner) Output(_ context.Context, args []string) (string, error) {
	r.events = append(r.events, strings.Join(args, " "))
	if r.queryErr != nil {
		return "", r.queryErr
	}
	last := args[len(args)-1]
	switch {
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
		}
		return nil
	}
	if args[0] == "dnf" && args[1] == "install" {
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
	for _, family := range []string{"debian", "archlinux", "fedora"} {
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
			if index(install) <= 2 || index("systemctl enable lightdm.service") <= index(install) {
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
	for _, family := range []string{"debian", "archlinux", "fedora"} {
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
				case "unavailable":
					r.unavailable = "greeter"
				case "install":
					r.failRun = " -- "
					if family == "fedora" {
						r.failRun = "dnf install "
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
		for _, family := range []string{"debian", "archlinux", "fedora"} {
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
