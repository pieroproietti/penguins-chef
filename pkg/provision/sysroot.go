package provision

import (
	"context"
	"fmt"
	"os"
	"os/user"
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
	options := []string{"rsync", "-aAXc"}
	if target == "/" {
		// Git does not store ownership, directory permissions or system SELinux labels:
		// a user's checkout must not chown /etc or /usr to that user, nor make them
		// group-writable (0775), nor copy local user xattrs (user_home_t) onto system files.
		options = append(options, "--chown=0:0", "--chmod=D0755,Fgo-w", "--no-xattrs")
	}
	check := append(append([]string{}, options...), "--dry-run", "--itemize-changes", "--out-format=%i", "--")
	check = append(check, paths...)
	out, err := r.Output(ctx, check)
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "" {
		copyCommand := append(append([]string{}, options...), "--")
		if err := r.Run(ctx, append(copyCommand, paths...)); err != nil {
			return err
		}
		out, err = r.Output(ctx, check)
		if err != nil {
			return err
		}
		if strings.TrimSpace(out) != "" {
			return fmt.Errorf("sysroot verification failed: destination still differs")
		}
	}
	if target == "/" {
		if err := replicateSkelToCurrentUser(ctx, r, source); err != nil {
			return err
		}
	}
	return nil
}

// replicateSkelToCurrentUser copies the skeleton directory (etc/skel) from the
// sysroot overlay into the current non-root user's home directory.
func replicateSkelToCurrentUser(ctx context.Context, r Runner, source string) error {
	skelDir := filepath.Join(source, "etc", "skel")
	info, err := os.Stat(skelDir)
	if err != nil || !info.IsDir() {
		return nil
	}
	u, err := currentNonRootUser()
	if err != nil || u == nil {
		return nil
	}
	if info, err := os.Stat(u.HomeDir); err != nil || !info.IsDir() {
		return nil
	}
	skelEntries, err := os.ReadDir(skelDir)
	if err != nil || len(skelEntries) == 0 {
		return nil
	}
	var paths []string
	for _, entry := range skelEntries {
		paths = append(paths, filepath.Join(skelDir, entry.Name()))
	}
	paths = append(paths, strings.TrimRight(u.HomeDir, "/")+"/")
	options := []string{
		"rsync", "-a",
		"--chown=" + u.Uid + ":" + u.Gid,
		"--chmod=D0755,Fgo-w",
		"--no-xattrs",
	}
	check := append(append([]string{}, options...), "--dry-run", "--itemize-changes", "--out-format=%i", "--")
	check = append(check, paths...)
	out, err := r.Output(ctx, check)
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) == "" {
		return nil
	}
	copyCommand := append(append([]string{}, options...), "--")
	if err := r.Run(ctx, append(copyCommand, paths...)); err != nil {
		return err
	}
	out, err = r.Output(ctx, check)
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "" {
		return fmt.Errorf("skel replication to %s failed: destination still differs", u.HomeDir)
	}
	return nil
}

func currentNonRootUser() (*user.User, error) {
	getHome := func() string {
		if home := strings.TrimSpace(os.Getenv("CHEF_USER_HOME")); home != "" {
			return home
		}
		return strings.TrimSpace(os.Getenv("TAILOR_USER_HOME"))
	}
	for _, envKey := range []string{"CHEF_USER", "TAILOR_USER", "SUDO_USER", "DOAS_USER"} {
		val := strings.TrimSpace(os.Getenv(envKey))
		if val != "" && val != "root" {
			if u, err := user.Lookup(val); err == nil && u.HomeDir != "" {
				if home := getHome(); home != "" {
					u.HomeDir = home
				}
				return u, nil
			}
		}
	}
	if u, err := user.Current(); err == nil && u.Uid != "0" && u.HomeDir != "" {
		if home := getHome(); home != "" {
			u.HomeDir = home
		}
		return u, nil
	}
	return nil, nil
}
