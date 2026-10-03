package provision

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type overlayRunner struct {
	HostRunner
	mutations int
}

func (r *overlayRunner) Run(ctx context.Context, args []string) error {
	r.mutations++
	return r.HostRunner.Run(ctx, args)
}

func TestSysrootCopiesBinaryHiddenFilesLinksAndModes(t *testing.T) {
	if _, err := exec.LookPath("rsync"); err != nil {
		t.Skip("rsync unavailable")
	}
	source, target := t.TempDir(), t.TempDir()
	if err := os.Chmod(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0755); err != nil {
		t.Fatal(err)
	}
	path := "etc/skel/.config/xfce4/panel.bin"
	if err := os.MkdirAll(filepath.Dir(filepath.Join(source, path)), 0755); err != nil {
		t.Fatal(err)
	}
	data := []byte{0, 1, 255, 42}
	if err := os.WriteFile(filepath.Join(source, path), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("panel.bin", filepath.Join(source, "etc/skel/.config/xfce4/panel-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "unrelated"), []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	r := &overlayRunner{HostRunner: HostRunner{Out: io.Discard, Err: io.Discard}}
	ctx := context.Background()
	if err := reconcileSysroot(ctx, r, source, target); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(target, path))
	if err != nil || string(got) != string(data) {
		t.Fatalf("binary: %v, %v", got, err)
	}
	info, err := os.Stat(filepath.Join(target, path))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("mode: %v, %v", info, err)
	}
	link, err := os.Readlink(filepath.Join(target, "etc/skel/.config/xfce4/panel-link"))
	if err != nil || link != "panel.bin" {
		t.Fatalf("link: %q, %v", link, err)
	}
	if _, err := os.Stat(filepath.Join(target, "unrelated")); err != nil {
		t.Fatal("unrelated file removed")
	}
	rootInfo, err := os.Stat(target)
	if err != nil || rootInfo.Mode().Perm() != 0755 {
		t.Fatal("destination root metadata changed")
	}
	if r.mutations != 1 {
		t.Fatalf("mutations: %d", r.mutations)
	}
	if err := reconcileSysroot(ctx, r, source, target); err != nil {
		t.Fatal(err)
	}
	if r.mutations != 1 {
		t.Fatal("unchanged sysroot recopied")
	}
	// A checksum must catch content changes even when size and mtime match.
	if err := os.WriteFile(filepath.Join(target, path), []byte{0, 2, 255, 42}, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(target, path), info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := reconcileSysroot(ctx, r, source, target); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(filepath.Join(target, path))
	if err != nil || string(got) != string(data) || r.mutations != 2 {
		t.Fatal("content drift not repaired")
	}
}

type brokenOverlayRunner struct{ failQuery, failCopy bool }

func (r brokenOverlayRunner) Output(context.Context, []string) (string, error) {
	if r.failQuery {
		return "", errors.New("query failed")
	}
	return ">f+++++++++\n", nil
}
func (r brokenOverlayRunner) Run(context.Context, []string) error {
	if r.failCopy {
		return errors.New("copy failed")
	}
	return nil
}

type ownershipRunner struct {
	queries  int
	commands [][]string
}

func (r *ownershipRunner) Output(_ context.Context, args []string) (string, error) {
	r.commands = append(r.commands, args)
	r.queries++
	if r.queries == 1 {
		return ">f+++++++++\n", nil
	}
	return "", nil
}
func (r *ownershipRunner) Run(_ context.Context, args []string) error {
	r.commands = append(r.commands, args)
	return nil
}

func TestSystemSysrootUsesRootOwnership(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "file"), []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}
	r := &ownershipRunner{}
	// The fake runner records commands without accessing the real system root.
	if err := reconcileSysroot(context.Background(), r, source, "/"); err != nil {
		t.Fatal(err)
	}
	if len(r.commands) != 3 {
		t.Fatal("missing copy or verification")
	}
	for _, args := range r.commands {
		cmdStr := strings.Join(args, " ")
		if !strings.Contains(cmdStr, "--chown=0:0") {
			t.Fatal("checkout ownership would be copied to system")
		}
		if !strings.Contains(cmdStr, "--chmod=D0755,Fgo-w") {
			t.Fatal("missing permissions normalization for system root")
		}
		if !strings.Contains(cmdStr, "--no-xattrs") {
			t.Fatal("user checkout xattrs would be copied to system root")
		}
	}
}

func TestSysrootFailuresAndPreflight(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "file"), []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, r := range []brokenOverlayRunner{{failQuery: true}, {failCopy: true}, {}} {
		if err := reconcileSysroot(context.Background(), r, source, t.TempDir()); err == nil {
			t.Fatal("expected failure")
		}
	}
	p := Plan{Steps: []Step{{ID: "prepare", Command: []string{"prepare"}}, {ID: "sysroot", Sysroot: filepath.Join(source, "missing")}}}
	r := &fakeRunner{}
	if err := p.Execute(context.Background(), r, io.Discard); err == nil || len(r.events) != 0 {
		t.Fatal("missing sysroot not rejected before mutations")
	}
	if err := validateSysroot("/"); err == nil {
		t.Fatal("accepted host root")
	}
	if err := validateSysroot(filepath.Join(source, "file")); err == nil {
		t.Fatal("accepted file")
	}
}

func TestSysrootInsertedBeforeInit(t *testing.T) {
	p := Plan{Steps: []Step{{Phase: "packages"}, {Phase: "configuration"}, {Phase: "init"}}}
	if err := p.AddSysroot(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if p.Steps[2].Sysroot == "" || p.Steps[3].Phase != "init" {
		t.Fatal("wrong overlay order")
	}
	if err := p.AddSysroot(t.TempDir()); err == nil {
		t.Fatal("duplicate overlay accepted")
	}
	var out strings.Builder
	if err := p.Describe(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "sysroot:copy") {
		t.Fatal("overlay missing from preview")
	}
}

func TestRecipeSysrootResolvedRelativeToRecipe(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "recipe.yaml")
	if err := os.WriteFile(path, []byte("version: 1\nname: test\nsysroot: assets\nprofiles: {fedora: {}}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	recipe, err := Load(f)
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	plan, err := Build(recipe, "fedora", "systemd")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Sysroot != filepath.Join(dir, "assets") {
		t.Fatal("sysroot resolved against working directory")
	}
	if err := os.Remove(filepath.Join(dir, "assets")); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(recipe, "fedora", "systemd"); err == nil {
		t.Fatal("accepted missing recipe sysroot")
	}
}

func TestPublicColibriSysrootContainsOnlyReviewedAssets(t *testing.T) {
	root := "../../examples/provision/colibri/sysroot"
	allowed := map[string]bool{
		"etc/modules-load.d/uinput.conf": true,
		"etc/skel/.bashrc":               true, "etc/skel/.bash_logout": true, "etc/skel/.profile": true,
		"etc/skel/.config/xfce4/terminal/accels.scm":                true,
		"etc/skel/.config/xfce4/terminal/terminalrc":                true,
		"usr/share/backgrounds/colibri/3794764350_2839ca0b26_b.jpg": true,
		"usr/share/backgrounds/colibri/credits.md":                  true,
	}
	for _, name := range []string{"xsettings", "thunar", "xfce4-desktop", "xfwm4", "xfce4-terminal", "xfce4-keyboard-shortcuts", "xfce4-session", "xfce4-notifyd", "keyboards", "xfce4-panel"} {
		allowed["etc/skel/.config/xfce4/xfconf/xfce-perchannel-xml/"+name+".xml"] = true
	}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if !allowed[filepath.ToSlash(rel)] {
			t.Errorf("unreviewed public sysroot asset: %s", rel)
		}
		delete(allowed, filepath.ToSlash(rel))
		if strings.HasSuffix(path, ".jpg") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, private := range []string{"/home/", "/root/", `name="recent"`, `name="known-legacy-items"`, "colibri-wallpapers/"} {
			if strings.Contains(string(data), private) {
				t.Errorf("private or stale state in %s: %s", rel, private)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(allowed) != 0 {
		t.Fatalf("missing public assets: %v", allowed)
	}
}
