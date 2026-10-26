package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pieroproietti/penguins-chef/pkg/tui"
)

func TestCookCmdFlags(t *testing.T) {
	cmd := cookCmd()

	pullFlag := cmd.Flags().Lookup("pull")
	if pullFlag == nil {
		t.Fatal("expected --pull flag to exist")
	}
	if pullFlag.Shorthand != "p" {
		t.Errorf("expected shorthand 'p' for --pull, got %q", pullFlag.Shorthand)
	}

	urlFlag := cmd.Flags().Lookup("url")
	if urlFlag == nil {
		t.Fatal("expected --url flag to exist")
	}
	if urlFlag.Shorthand != "u" {
		t.Errorf("expected shorthand 'u' for --url, got %q", urlFlag.Shorthand)
	}

	branchFlag := cmd.Flags().Lookup("branch")
	if branchFlag == nil {
		t.Fatal("expected --branch flag to exist")
	}
	if branchFlag.Shorthand != "b" {
		t.Errorf("expected shorthand 'b' for --branch, got %q", branchFlag.Shorthand)
	}
}

func TestCookDryRunDoesNotExecuteOrWrite(t *testing.T) {
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
			cmd := cookCmd()
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

func TestCookRejectsTargetOverrideDuringExecution(t *testing.T) {
	cmd := cookCmd()
	cmd.SetArgs([]string{"nonexistent.yaml", "--family", "archlinux"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "only allowed with --dry-run") {
		t.Fatalf("override accepted: %v", err)
	}
}

func TestCookDryRunDebianAlternativeInit(t *testing.T) {
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
			cmd := cookCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetArgs([]string{recipe, "--dry-run", "--family", "debian", "--init", initName})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("cook failed for debian with init %s: %v", initName, err)
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

func TestCookDryRunCrossFamilyWithoutInit(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for _, family := range []string{"archlinux", "fedora", "opensuse"} {
		t.Run(family, func(t *testing.T) {
			dir := t.TempDir()
			recipe := filepath.Join(dir, "recipe.yaml")
			text := "version: 1\nname: test\nprofiles:\n  " + family + ":\n    packages: [lightdm]\n    services: [lightdm.service]\n"
			if err := os.WriteFile(recipe, []byte(text), 0644); err != nil {
				t.Fatal(err)
			}
			cmd := cookCmd()
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

func TestCookDryRunDevuanDefaultsToSysvinit(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	dir := t.TempDir()
	recipe := filepath.Join(dir, "recipe.yaml")
	text := "version: 1\nname: test\nprofiles:\n  debian:\n    packages: [lightdm]\n    services: [lightdm.service]\n"
	if err := os.WriteFile(recipe, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := cookCmd()
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

func TestCookDryRunOpenKylinDefaultsToSystemd(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	dir := t.TempDir()
	recipe := filepath.Join(dir, "recipe.yaml")
	text := "version: 1\nname: test\nprofiles:\n  debian:\n    packages: [lightdm]\n    services: [lightdm.service]\n"
	if err := os.WriteFile(recipe, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := cookCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{recipe, "--dry-run", "--family", "openkylin"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("dry-run failed for openkylin: %v", err)
	}
	if !strings.Contains(out.String(), "packages: debian, init: systemd") {
		t.Fatalf("expected debian with systemd for openkylin dry-run, got: %s", out.String())
	}
	if !strings.Contains(out.String(), "[init]") {
		t.Fatalf("expected [init] phase for openkylin: %s", out.String())
	}
}

func TestCookWithoutArgsNonInteractive(t *testing.T) {
	origIsTerm := isTerminalFn
	defer func() { isTerminalFn = origIsTerm }()
	isTerminalFn = func(f *os.File) bool { return false }

	cmd := cookCmd()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "recipe argument required in non-interactive mode") {
		t.Fatalf("expected non-interactive error, got: %v", err)
	}
}

func TestCookWithoutArgsInteractiveCancelled(t *testing.T) {
	origIsTerm := isTerminalFn
	origPicker := runRecipePickerFn
	origFind := findRecipesFn
	defer func() {
		isTerminalFn = origIsTerm
		runRecipePickerFn = origPicker
		findRecipesFn = origFind
	}()

	isTerminalFn = func(f *os.File) bool { return true }
	findRecipesFn = func(dirs ...string) ([]tui.RecipeItem, error) {
		return []tui.RecipeItem{{Name: "dummy"}}, nil
	}
	runRecipePickerFn = func(dirs ...string) (string, error) {
		return "", nil // simulated cancellation
	}

	cmd := cookCmd()
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected nil error on cancelled picker, got: %v", err)
	}
}

func TestCookWithoutArgsInteractiveSelected(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	origIsTerm := isTerminalFn
	origPicker := runRecipePickerFn
	origFind := findRecipesFn
	defer func() {
		isTerminalFn = origIsTerm
		runRecipePickerFn = origPicker
		findRecipesFn = origFind
	}()

	dir := t.TempDir()
	recipe := filepath.Join(dir, "recipe.yaml")
	text := "version: 1\nname: interactive-test\ndescription: \"Interactive test recipe\"\nprofiles:\n  debian:\n    packages: [curl]\n"
	if err := os.WriteFile(recipe, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}

	isTerminalFn = func(f *os.File) bool { return true }
	findRecipesFn = func(dirs ...string) ([]tui.RecipeItem, error) {
		return []tui.RecipeItem{{Name: "interactive-test", Path: recipe}}, nil
	}
	runRecipePickerFn = func(dirs ...string) (string, error) {
		return recipe, nil
	}

	cmd := cookCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--dry-run", "--family", "debian"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cook with picker failed: %v", err)
	}
	if !strings.Contains(out.String(), "interactive-test") || !strings.Contains(out.String(), "packages:install") {
		t.Fatalf("unexpected plan output: %s", out.String())
	}
}

func TestCookAutoDownloadsWhenRecipesMissing(t *testing.T) {
	origIsTerm := isTerminalFn
	origPicker := runRecipePickerFn
	origFind := findRecipesFn
	origGet := chefGetFn
	defer func() {
		isTerminalFn = origIsTerm
		runRecipePickerFn = origPicker
		findRecipesFn = origFind
		chefGetFn = origGet
	}()

	getCalled := false
	isTerminalFn = func(f *os.File) bool { return true }
	findRecipesFn = func(dirs ...string) ([]tui.RecipeItem, error) {
		return nil, nil // No recipes found initially
	}
	chefGetFn = func(repoURL, branch string) error {
		getCalled = true
		return nil
	}
	runRecipePickerFn = func(dirs ...string) (string, error) {
		return "", nil
	}

	cmd := cookCmd()
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if !getCalled {
		t.Fatal("expected chef.Get to be called automatically when no recipes are found")
	}
}

func TestCookPullFlagTriggersDownload(t *testing.T) {
	origIsTerm := isTerminalFn
	origGet := chefGetFn
	defer func() {
		isTerminalFn = origIsTerm
		chefGetFn = origGet
	}()

	isTerminalFn = func(f *os.File) bool { return false }
	var gotURL, gotBranch string
	getCalled := false
	chefGetFn = func(repoURL, branch string) error {
		getCalled = true
		gotURL = repoURL
		gotBranch = branch
		return nil
	}

	cmd := cookCmd()
	cmd.SetArgs([]string{"--pull", "--url", "https://example.com/chef.git", "--branch", "devel"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected nil error on non-interactive --pull, got: %v", err)
	}
	if !getCalled {
		t.Fatal("expected chef.Get to be called when --pull is passed")
	}
	if gotURL != "https://example.com/chef.git" || gotBranch != "devel" {
		t.Fatalf("unexpected url/branch passed to chef.Get: %q / %q", gotURL, gotBranch)
	}
}
