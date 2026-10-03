package provision

import (
	"fmt"
	"strings"
)

// Package and init adapters are independent: the distro family selects only
// package operations, while the running init selects service operations.
type packageBackend interface {
	prepare() []string
	availability(string) []string
	validateAvailability(string) error
	installed(string) []string
	isInstalled(string) bool
	install([]string) []string
	validateRepository(File) error
}

func packagesFor(family string) (packageBackend, error) {
	switch family {
	case "debian":
		return aptBackend{}, nil
	case "archlinux":
		return pacmanBackend{}, nil
	default:
		return nil, fmt.Errorf("unsupported package backend %q", family)
	}
}

type aptBackend struct{}

func (aptBackend) prepare() []string                { return []string{"apt-get", "update", "--error-on=any"} }
func (aptBackend) availability(pkg string) []string { return []string{"apt-cache", "policy", pkg} }
func (aptBackend) installed(pkg string) []string {
	return []string{"dpkg-query", "-W", "-f=${Status}", pkg}
}
func (aptBackend) isInstalled(out string) bool {
	return strings.TrimSpace(out) == "install ok installed"
}
func (aptBackend) install(pkgs []string) []string {
	return append([]string{"apt-get", "install", "-y", "--"}, pkgs...)
}
func (aptBackend) validateAvailability(out string) error {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Candidate:") {
			candidate := strings.TrimSpace(strings.TrimPrefix(line, "Candidate:"))
			if candidate != "" && candidate != "(none)" {
				return nil
			}
		}
	}
	return fmt.Errorf("no install candidate in configured repositories")
}
func (aptBackend) validateRepository(f File) error {
	if !strings.HasPrefix(f.Path, "/etc/apt/sources.list.d/") || strings.Contains(strings.TrimPrefix(f.Path, "/etc/apt/sources.list.d/"), "/") || (!strings.HasSuffix(f.Path, ".sources") && !strings.HasSuffix(f.Path, ".list")) {
		return fmt.Errorf("unsupported APT repository file %q", f.Path)
	}
	return nil
}

type pacmanBackend struct{}

func (pacmanBackend) prepare() []string                { return []string{"pacman", "-Syu", "--noconfirm"} }
func (pacmanBackend) availability(pkg string) []string { return []string{"pacman", "-Si", pkg} }
func (pacmanBackend) installed(pkg string) []string    { return []string{"pacman", "-Q", pkg} }
func (pacmanBackend) isInstalled(out string) bool      { return strings.TrimSpace(out) != "" }
func (pacmanBackend) install(pkgs []string) []string {
	return append([]string{"pacman", "-S", "--needed", "--noconfirm", "--"}, pkgs...)
}
func (pacmanBackend) validateAvailability(out string) error {
	if strings.TrimSpace(out) == "" {
		return fmt.Errorf("empty repository response")
	}
	return nil
}
func (pacmanBackend) validateRepository(f File) error {
	return fmt.Errorf("adding pacman repositories is not implemented: %q", f.Path)
}

type initBackend interface {
	check(string) []string
	enable(string) []string
	isEnabled(string) bool
}

func initFor(name string) (initBackend, error) {
	if name == "systemd" {
		return systemdBackend{}, nil
	}
	return nil, fmt.Errorf("unsupported init backend %q", name)
}

type systemdBackend struct{}

func (systemdBackend) check(service string) []string {
	return []string{"systemctl", "is-enabled", service}
}
func (systemdBackend) enable(service string) []string {
	return []string{"systemctl", "enable", service}
}
func (systemdBackend) isEnabled(out string) bool { return strings.TrimSpace(out) == "enabled" }
