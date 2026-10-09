package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyDryRunDoesNotExecuteOrWrite(t *testing.T) {
	// No executables are available: preview must not even query the host.
	t.Setenv("PATH", t.TempDir())
	for _, family := range []string{"debian", "archlinux", "fedora", "opensuse"} {
		t.Run(family, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "never-created")
			sysroot := filepath.Join(dir, "sysroot")
			if err := os.Mkdir(sysroot, 0755); err != nil {
				t.Fatal(err)
			}
			recipe := filepath.Join(dir, "recipe.yaml")
			text := "version: 1\nname: test\nsysroot: missing-default\nprofiles:\n  " + family + ":\n    packages: [lightdm]\n    files:\n      - path: " + target + "\n        content: test\n    services: [lightdm.service]\n"
			if err := os.WriteFile(recipe, []byte(text), 0644); err != nil {
				t.Fatal(err)
			}
			cmd := applyCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetArgs([]string{recipe, "--dry-run", "--family", family, "--sysroot", sysroot})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), "[init]") || !strings.Contains(out.String(), "sysroot:copy") {
				t.Fatalf("incomplete plan: %s", out.String())
			}
			if _, err := os.Stat(target); !os.IsNotExist(err) {
				t.Fatal("dry-run wrote configuration")
			}
		})
	}
}

func TestApplyRejectsTargetOverrideDuringExecution(t *testing.T) {
	cmd := applyCmd()
	cmd.SetArgs([]string{"nonexistent.yaml", "--family", "archlinux"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "only allowed with --dry-run") {
		t.Fatalf("override accepted: %v", err)
	}
}

func TestApplyDryRunDebianAlternativeInit(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for _, initName := range []string{"sysv", "sysvinit", "openrc", "unknown"} {
		t.Run(initName, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "never-created")
			recipe := filepath.Join(dir, "recipe.yaml")
			text := "version: 1\nname: test\nprofiles:\n  debian:\n    packages: [lightdm]\n    files:\n      - path: " + target + "\n        content: test\n    services: [lightdm.service]\n"
			if err := os.WriteFile(recipe, []byte(text), 0644); err != nil {
				t.Fatal(err)
			}
			cmd := applyCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetArgs([]string{recipe, "--dry-run", "--family", "debian", "--init", initName})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("apply failed for debian with init %s: %v", initName, err)
			}
			if strings.Contains(out.String(), "[init]") {
				t.Fatalf("unexpected [init] phase in plan for debian with init %s: %s", initName, out.String())
			}
			if !strings.Contains(out.String(), "[packages]") || !strings.Contains(out.String(), "packages:install") {
				t.Fatalf("missing packages:install in plan for debian with init %s: %s", initName, out.String())
			}
		})
	}
}

func TestApplyDryRunCrossFamilyWithoutInit(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for _, family := range []string{"archlinux", "fedora", "opensuse"} {
		t.Run(family, func(t *testing.T) {
			dir := t.TempDir()
			recipe := filepath.Join(dir, "recipe.yaml")
			text := "version: 1\nname: test\nprofiles:\n  " + family + ":\n    packages: [lightdm]\n    services: [lightdm.service]\n"
			if err := os.WriteFile(recipe, []byte(text), 0644); err != nil {
				t.Fatal(err)
			}
			cmd := applyCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			// Notice: absolutely no --init parameter passed
			cmd.SetArgs([]string{recipe, "--dry-run", "--family", family})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("cross-family dry-run failed for %s without --init: %v", family, err)
			}
			if !strings.Contains(out.String(), "[init]") || !strings.Contains(out.String(), "service:lightdm.service") {
				t.Fatalf("expected init phase with service in plan for %s: %s", family, out.String())
			}
		})
	}
}

func TestApplyDryRunDevuanDefaultsToSysvinit(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	dir := t.TempDir()
	recipe := filepath.Join(dir, "recipe.yaml")
	text := "version: 1\nname: test\nprofiles:\n  debian:\n    packages: [lightdm]\n    services: [lightdm.service]\n"
	if err := os.WriteFile(recipe, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := applyCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	// Testing --family devuan without any --init parameter
	cmd.SetArgs([]string{recipe, "--dry-run", "--family", "devuan"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("dry-run failed for devuan: %v", err)
	}
	if !strings.Contains(out.String(), "packages: debian, init: sysvinit") {
		t.Fatalf("expected debian with sysvinit for devuan dry-run, got: %s", out.String())
	}
	if strings.Contains(out.String(), "[init]") {
		t.Fatalf("unexpected [init] phase for devuan: %s", out.String())
	}
}

func TestApplyWithoutArgsNonInteractive(t *testing.T) {
	origIsTerm := isTerminalFn
	defer func() { isTerminalFn = origIsTerm }()
	isTerminalFn = func(f *os.File) bool { return false }

	cmd := applyCmd()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "recipe argument required in non-interactive mode") {
		t.Fatalf("expected non-interactive error, got: %v", err)
	}
}

func TestApplyWithoutArgsInteractiveCancelled(t *testing.T) {
	origIsTerm := isTerminalFn
	origPicker := runRecipePickerFn
	defer func() {
		isTerminalFn = origIsTerm
		runRecipePickerFn = origPicker
	}()

	isTerminalFn = func(f *os.File) bool { return true }
	runRecipePickerFn = func(dirs ...string) (string, error) {
		return "", nil // simulated cancellation
	}

	cmd := applyCmd()
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected nil error on cancelled picker, got: %v", err)
	}
}

func TestApplyWithoutArgsInteractiveSelected(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	origIsTerm := isTerminalFn
	origPicker := runRecipePickerFn
	defer func() {
		isTerminalFn = origIsTerm
		runRecipePickerFn = origPicker
	}()

	dir := t.TempDir()
	recipe := filepath.Join(dir, "recipe.yaml")
	text := "version: 1\nname: interactive-test\ndescription: \"Interactive test recipe\"\nprofiles:\n  debian:\n    packages: [curl]\n"
	if err := os.WriteFile(recipe, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}

	isTerminalFn = func(f *os.File) bool { return true }
	runRecipePickerFn = func(dirs ...string) (string, error) {
		return recipe, nil
	}

	cmd := applyCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--dry-run", "--family", "debian"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("apply with picker failed: %v", err)
	}
	if !strings.Contains(out.String(), "interactive-test") || !strings.Contains(out.String(), "packages:install") {
		t.Fatalf("unexpected plan output: %s", out.String())
	}
}

