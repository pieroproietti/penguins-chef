package provision

import (
	"encoding/xml"
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
	case "fedora":
		return dnfBackend{}, nil
	case "opensuse":
		return zypperBackend{}, nil
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

// DNF's repoquery can succeed without matches; use a marker so diagnostic
// output from CombinedOutput cannot be mistaken for an available package.
type dnfBackend struct{}

func (dnfBackend) prepare() []string { return []string{"dnf", "--refresh", "makecache"} }
func (dnfBackend) availability(pkg string) []string {
	return []string{"dnf", "-q", "repoquery", "--available", "--queryformat", "chef-package:%{name}\\n", pkg}
}
func (dnfBackend) validateAvailability(out string) error {
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "chef-package:") && identifier.MatchString(strings.TrimPrefix(line, "chef-package:")) {
			return nil
		}
	}
	return fmt.Errorf("no install candidate in configured repositories")
}
func (dnfBackend) installed(pkg string) []string {
	return []string{"rpm", "-q", "--queryformat", "%{NAME}\n", pkg}
}
func (dnfBackend) isInstalled(out string) bool { return strings.TrimSpace(out) != "" }
func (dnfBackend) install(pkgs []string) []string {
	// DNF5 rejects the end-of-options separator for install.
	return append([]string{"dnf", "install", "-y"}, pkgs...)
}
func (dnfBackend) validateRepository(f File) error {
	return fmt.Errorf("adding DNF repositories is not implemented: %q", f.Path)
}

type zypperBackend struct{}

func (zypperBackend) prepare() []string {
	return []string{"zypper", "--non-interactive", "refresh"}
}
func (zypperBackend) availability(pkg string) []string {
	return []string{"zypper", "--non-interactive", "--xmlout", "--no-refresh", "search", "--match-exact", "--case-sensitive", "--details", "--type", "package", pkg}
}
func (zypperBackend) validateAvailability(out string) error {
	var result struct {
		XMLName   xml.Name `xml:"stream"`
		Solvables []struct {
			Kind       string `xml:"kind,attr"`
			Name       string `xml:"name,attr"`
			Repository string `xml:"repository,attr"`
		} `xml:"search-result>solvable-list>solvable"`
	}
	if err := xml.Unmarshal([]byte(out), &result); err != nil {
		return fmt.Errorf("invalid zypper search response: %w", err)
	}
	for _, pkg := range result.Solvables {
		if pkg.Kind == "package" && identifier.MatchString(pkg.Name) && pkg.Repository != "" && pkg.Repository != "@System" && pkg.Repository != "(System Packages)" {
			return nil
		}
	}
	return fmt.Errorf("no install candidate in configured repositories")
}
func (zypperBackend) installed(pkg string) []string { return dnfBackend{}.installed(pkg) }
func (zypperBackend) isInstalled(out string) bool   { return dnfBackend{}.isInstalled(out) }
func (zypperBackend) install(pkgs []string) []string {
	return append([]string{"zypper", "--non-interactive", "install"}, pkgs...)
}
func (zypperBackend) validateRepository(f File) error {
	return fmt.Errorf("adding Zypper repositories is not implemented: %q", f.Path)
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
