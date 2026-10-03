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
