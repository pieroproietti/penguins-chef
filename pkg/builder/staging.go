// Copyright 2026 Piero Proietti <piero.proietti@gmail.com>.
// All rights reserved.

package builder

import (
	"os"
	"path/filepath"

	"github.com/pieroproietti/penguins-chef/pkg/context"
)

func staging(ctx context.RuntimeContext) string {
	stageDir := ctx.StageDir
	buildDir := ctx.BaseBuildDir

	// Clean up previous stage
	os.RemoveAll(stageDir)

	dirs := []string{
		"usr/bin",
		"usr/share/man/man1",
		"usr/share/bash-completion/completions",
		"usr/share/zsh/vendor-completions",
		"usr/share/fish/vendor_completions.d",
	}
	for _, d := range dirs {
		os.MkdirAll(filepath.Join(stageDir, d), 0755)
	}

	// 1. Binary
	copyFile(filepath.Join(buildDir, "chef"), filepath.Join(stageDir, "usr/bin/chef"))

	// 2. Documentation (man pages)
	manFiles, _ := filepath.Glob(filepath.Join(buildDir, "docs/man/*.1"))
	for _, f := range manFiles {
		dest := filepath.Join(stageDir, "usr/share/man/man1", filepath.Base(f))
		copyFile(f, dest)
	}

	// 3. Completions
	src := filepath.Join(buildDir, "docs/completion/chef.bash")
	if _, err := os.Stat(src); err == nil {
		dest := filepath.Join(stageDir, "usr/share/bash-completion/completions/chef")
		copyFile(src, dest)
	}

	src = filepath.Join(buildDir, "docs/completion/chef.fish")
	if _, err := os.Stat(src); err == nil {
		dest := filepath.Join(stageDir, "usr/share/fish/vendor_completions.d/chef.fish")
		copyFile(src, dest)
	}

	src = filepath.Join(buildDir, "docs/completion/chef.zsh")
	if _, err := os.Stat(src); err == nil {
		dest := filepath.Join(stageDir, "usr/share/zsh/vendor-completions/_chef")
		copyFile(src, dest)
	}

	return stageDir
}
