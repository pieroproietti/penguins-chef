package provision

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AddSysroot inserts the overlay after configuration and before init operations.
// The explicit local path keeps costume assets separate from the recipe repo.
func (p *Plan) AddSysroot(source string) error {
	source, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	if err := validateSysroot(source); err != nil {
		return err
	}
	for _, step := range p.Steps {
		if step.Sysroot != "" {
			return fmt.Errorf("sysroot already configured")
		}
	}
	i := len(p.Steps)
	for n, step := range p.Steps {
		if step.Phase == "init" {
			i = n
			break
		}
	}
	step := Step{ID: "sysroot:copy", Phase: "configuration", Sysroot: source}
	p.Steps = append(p.Steps, Step{})
	copy(p.Steps[i+1:], p.Steps[i:])
	p.Steps[i] = step
	return nil
}

func validateSysroot(source string) error {
	if source == "/" {
		return fmt.Errorf("sysroot must not be the system root")
	}
	info, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("sysroot: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("sysroot must be a directory: %s", source)
	}
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("unsupported sysroot file type: %s", path)
		}
		return nil
	})
}

// Rsync's checksum comparison detects same-size changes with unchanged mtimes.
// Dry-run itemization also verifies permissions, ownership, links, ACLs and xattrs.
// Enumerating top-level entries avoids copying sysroot's own metadata onto /.
// --delete is deliberately absent.
func reconcileSysroot(ctx context.Context, r Runner, source, target string) error {
	if err := validateSysroot(source); err != nil {
		return err
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	var paths []string
	for _, entry := range entries {
		paths = append(paths, filepath.Join(source, entry.Name()))
	}
	paths = append(paths, strings.TrimRight(target, "/")+"/")
	check := append([]string{"rsync", "-aAXc", "--dry-run", "--itemize-changes", "--out-format=%i", "--"}, paths...)
	out, err := r.Output(ctx, check)
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) == "" {
		return nil
	}
	if err := r.Run(ctx, append([]string{"rsync", "-aAXc", "--"}, paths...)); err != nil {
		return err
	}
	out, err = r.Output(ctx, check)
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "" {
		return fmt.Errorf("sysroot verification failed: destination still differs")
	}
	return nil
}
