package provision

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Plan struct {
	Name   string
	Family string
	Init   string
	Steps  []Step
}

type Step struct {
	ID            string
	Phase         string
	Command       []string
	Availability  []string
	Packages      []string
	File          *File
	Hostname      string
	DefaultTarget string
	Service       string
	Sysroot       string
}

type Runner interface {
	Output(context.Context, []string) (string, error)
	Run(context.Context, []string) error
}

// HostRunner passes arguments directly to executables; recipes contain no shell.
type HostRunner struct{ Out, Err io.Writer }

func (r HostRunner) Output(ctx context.Context, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
func (r HostRunner) Run(ctx context.Context, args []string) error {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "DEBIAN_FRONTEND=noninteractive")
	cmd.Stdout, cmd.Stderr = r.Out, r.Err
	return cmd.Run()
}

func (p Plan) Describe(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "%s (packages: %s, init: %s)\n", p.Name, p.Family, p.Init); err != nil {
		return err
	}
	for _, s := range p.Steps {
		detail := strings.Join(s.Command, " ")
		if len(s.Availability) > 0 {
			detail = "check " + strings.Join(s.Availability, " ")
		}
		if len(s.Packages) > 0 {
			detail = "check/install/verify " + strings.Join(s.Packages, " ")
		}
		if s.File != nil {
			detail = "check/write/verify " + s.File.Path + " (0644)"
		}
		if s.Hostname != "" {
			detail = "check/set/verify " + s.Hostname
		}
		if s.Service != "" {
			detail = "check/enable/verify " + s.Service + " (no start)"
			if (p.Family == "opensuse" || p.Family == "archlinux") && isDisplayManagerService(s.Service) {
				detail += " (replace display-manager alias)"
			}
		}
		if s.DefaultTarget != "" {
			detail = "check/set-default/verify " + s.DefaultTarget + " (next boot)"
		}
		if s.Sysroot != "" {
			detail = "check/copy/verify " + s.Sysroot + " -> / (archive, ACLs, xattrs; no deletion)"
		}
		if _, err := fmt.Fprintf(w, "[%s] %s: %s\n", s.Phase, s.ID, detail); err != nil {
			return err
		}
	}
	return nil
}

// Execute stops on the first failure, including a failed postcondition. Re-run
// checks current system state rather than trusting a previously recorded success.
func (p Plan) Execute(ctx context.Context, r Runner, w io.Writer) error {
	// Reject unsuitable destinations before repository/package operations.
	for _, s := range p.Steps {
		if s.Sysroot != "" {
			if err := validateSysroot(s.Sysroot); err != nil {
				return fmt.Errorf("%s preflight: %w", s.ID, err)
			}
		}
		if s.File != nil {
			if err := regularPath(s.File.Path); err != nil {
				return fmt.Errorf("%s preflight: %w", s.ID, err)
			}
		}
		if s.Hostname != "" {
			if _, isHost := r.(HostRunner); isHost {
				if info, err := os.Lstat(defaultHostsPath); err == nil && info.IsDir() {
					return fmt.Errorf("%s preflight: %s is a directory", s.ID, defaultHostsPath)
				}
			}
		}
	}
	for _, s := range p.Steps {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "[%s] %s\n", s.Phase, s.ID); err != nil {
			return err
		}
		if err := p.executeStep(ctx, r, s); err != nil {
			return fmt.Errorf("%s failed (completed operations remain applied): %w", s.ID, err)
		}
	}
	return nil
}

func (p Plan) installed(ctx context.Context, r Runner, pkg string) (bool, error) {
	backend, err := packagesFor(p.Family)
	if err != nil {
		return false, err
	}
	out, err := r.Output(ctx, backend.installed(pkg))
	if err != nil {
		// Missing packages have status 1; missing executables and signals are errors.
		var exitErr interface{ ExitCode() int }
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, err
	}
	return backend.isInstalled(out), nil
}

func (p Plan) executeStep(ctx context.Context, r Runner, s Step) error {
	if s.Sysroot != "" {
		return reconcileSysroot(ctx, r, s.Sysroot, "/")
	}
	if len(s.Command) > 0 {
		return r.Run(ctx, s.Command)
	}
	if len(s.Availability) > 0 {
		out, err := r.Output(ctx, s.Availability)
		if err != nil {
			return err
		}
		backend, err := packagesFor(p.Family)
		if err != nil {
			return err
		}
		return backend.validateAvailability(out)
	}
	if len(s.Packages) > 0 {
		var missing []string
		for _, pkg := range s.Packages {
			ok, err := p.installed(ctx, r, pkg)
			if err != nil {
				return err
			}
			if !ok {
				missing = append(missing, pkg)
			}
		}
		if len(missing) == 0 {
			return nil
		}
		backend, err := packagesFor(p.Family)
		if err != nil {
			return err
		}
		if err := r.Run(ctx, backend.install(missing)); err != nil {
			return err
		}
		for _, pkg := range missing {
			ok, err := p.installed(ctx, r, pkg)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("package %s was not installed", pkg)
			}
		}
		return nil
	}
	if s.File != nil {
		return reconcileFile(*s.File)
	}
	if s.Hostname != "" {
		return reconcileHostname(ctx, r, s.Hostname)
	}
	if s.DefaultTarget != "" {
		if _, err := initFor(p.Init); err != nil {
			return err
		}
		check := []string{"systemctl", "get-default"}
		out, err := r.Output(ctx, check)
		if err != nil {
			return err
		}
		if strings.TrimSpace(out) == s.DefaultTarget {
			return nil
		}
		if err := r.Run(ctx, []string{"systemctl", "set-default", s.DefaultTarget}); err != nil {
			return err
		}
		out, err = r.Output(ctx, check)
		if err != nil {
			return err
		}
		if strings.TrimSpace(out) != s.DefaultTarget {
			return fmt.Errorf("default target is not %s", s.DefaultTarget)
		}
		return nil
	}

	if s.Service != "" {
		backend, err := initFor(p.Init)
		if err != nil {
			return err
		}
		check := backend.check(s.Service)
		// openSUSE and Arch derivatives may retain another display-manager alias. Selecting
		// a display manager must reconcile that alias even if it is already enabled.
		replaceDisplayManager := (p.Family == "opensuse" || p.Family == "archlinux") && isDisplayManagerService(s.Service)
		aliasCheck := []string{"systemctl", "show", "--property=Id", "--value", "display-manager.service"}
		aliasMatches := func() (bool, error) {
			if !replaceDisplayManager {
				return true, nil
			}
			out, err := r.Output(ctx, aliasCheck)
			return strings.TrimSpace(out) == s.Service, err
		}
		out, err := r.Output(ctx, check)
		if err == nil && backend.isEnabled(out) {
			matches, aliasErr := aliasMatches()
			if aliasErr != nil {
				return aliasErr
			}
			if matches {
				return nil
			}
		}
		if err != nil {
			var exitErr interface{ ExitCode() int }
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
				return err
			}
		}
		enable := backend.enable(s.Service)
		if replaceDisplayManager {
			enable = []string{"systemctl", "enable", "--force", s.Service}
		}
		if err := r.Run(ctx, enable); err != nil {
			return err
		}
		out, err = r.Output(ctx, check)
		if err != nil {
			return err
		}
		if !backend.isEnabled(out) {
			return fmt.Errorf("service %s is not persistently enabled", s.Service)
		}
		matches, err := aliasMatches()
		if err != nil {
			return err
		}
		if !matches {
			return fmt.Errorf("display-manager.service does not select %s", s.Service)
		}
	}
	return nil
}

// Refuse symlinks rather than replacing or following an unexpected destination.
func regularPath(path string) error {
	for current := path; current != "/" && current != "."; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in managed path %s", current)
		}
		if current == path && !info.Mode().IsRegular() {
			return fmt.Errorf("destination is not a regular file: %s", path)
		}
		if current != path && !info.IsDir() {
			return fmt.Errorf("parent is not a directory: %s", current)
		}
	}
	return nil
}

var defaultHostsPath = "/etc/hosts"

func writeAtomicFile(path, content string, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".chef-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if _, err := tmp.WriteString(content); err != nil {
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if string(data) != content {
		return fmt.Errorf("file verification failed: %s", path)
	}
	return nil
}

func reconcileFile(f File) error {
	if err := regularPath(f.Path); err != nil {
		return err
	}
	data, err := os.ReadFile(f.Path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil && string(data) == f.Content {
		info, err := os.Stat(f.Path)
		if err != nil {
			return err
		}
		if info.Mode().Perm() == 0644 {
			return nil
		}
	}
	return writeAtomicFile(f.Path, f.Content, 0644)
}

func reconcileHostname(ctx context.Context, r Runner, hostname string) error {
	check := []string{"hostnamectl", "hostname"}
	out, err := r.Output(ctx, check)
	if err != nil {
		return err
	}
	current := strings.TrimSpace(out)
	if current != hostname {
		if err := r.Run(ctx, []string{"hostnamectl", "set-hostname", hostname}); err != nil {
			return err
		}
		out, err = r.Output(ctx, check)
		if err != nil {
			return err
		}
		if strings.TrimSpace(out) != hostname {
			return fmt.Errorf("hostname is not %s", hostname)
		}
	}
	if _, isHost := r.(HostRunner); isHost {
		if err := reconcileHostsFile(defaultHostsPath, current, hostname); err != nil {
			return err
		}
	}
	return nil
}

func reconcileHostsFile(path, oldHost, newHost string) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	content := string(data)
	updated := updateHostsContent(content, oldHost, newHost)
	if updated == content && err == nil {
		return nil
	}
	return writeAtomicFile(path, updated, 0644)
}

func updateHostsContent(content, oldHost, newHost string) string {
	if strings.TrimSpace(content) == "" {
		return fmt.Sprintf("127.0.0.1 localhost %s\n::1 localhost\n", newHost)
	}

	lines := strings.Split(content, "\n")
	hasTrailingNewline := strings.HasSuffix(content, "\n")
	if hasTrailingNewline && len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	canReplaceOld := oldHost != "" &&
		oldHost != "localhost" &&
		oldHost != "localhost.localdomain" &&
		oldHost != newHost

	newHostFound := false
	var targetLineIdx = -1
	var fallbackLineIdx = -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		ip := fields[0]
		isLoopback := strings.HasPrefix(ip, "127.") || ip == "::1"
		if !isLoopback {
			continue
		}

		if ip == "127.0.1.1" {
			targetLineIdx = i
		} else if ip == "127.0.0.1" && targetLineIdx == -1 {
			fallbackLineIdx = i
		}

		lineModified := false
		for j := 1; j < len(fields); j++ {
			host := fields[j]
			if host == newHost {
				newHostFound = true
			}
			if canReplaceOld {
				if host == oldHost {
					fields[j] = newHost
					newHostFound = true
					lineModified = true
				} else if host == oldHost+".localdomain" {
					fields[j] = newHost + ".localdomain"
					lineModified = true
				} else if host == oldHost+".localhost" {
					fields[j] = newHost + ".localhost"
					lineModified = true
				}
			}
		}
		if lineModified {
			lines[i] = strings.Join(fields, " ")
		}
	}

	if !newHostFound {
		chosenIdx := targetLineIdx
		if chosenIdx == -1 {
			chosenIdx = fallbackLineIdx
		}
		if chosenIdx != -1 {
			lines[chosenIdx] = lines[chosenIdx] + " " + newHost
		} else {
			lines = append(lines, fmt.Sprintf("127.0.0.1 localhost %s", newHost))
		}
	}

	res := strings.Join(lines, "\n")
	if hasTrailingNewline {
		res += "\n"
	}
	return res
}

func isDisplayManagerService(service string) bool {
	switch service {
	case "lightdm.service", "sddm.service", "gdm.service", "lxdm.service", "xdm.service":
		return true
	default:
		return false
	}
}
