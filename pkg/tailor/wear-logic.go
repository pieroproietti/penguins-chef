package tailor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pieroproietti/penguins-tailor/pkg/distro"
	"github.com/pieroproietti/penguins-tailor/pkg/utils"
	"gopkg.in/yaml.v3"
)

var tailorLogFile = distroLogPath(distro.NewDistro().Identity())

func distroLogPath(identity string) string {
	if identity == "" {
		return utils.DefaultTechnicalLogPath
	}
	return filepath.Join("/var/log/tailor", fmt.Sprintf("tailor-%s.log", identity))
}

// logToFile remains as a compatibility wrapper while callers are migrated to
// explicit log levels. New code should use TechnicalLogger directly.
func logToFile(message string) {
	_ = utils.NewTechnicalLogger(tailorLogFile).Info(message)
}

func findYaml(costumePath string) string {
	candidates := []string{
		"index.yaml",
		"index.yml",
	}

	d := distro.NewDistro()
	if d != nil {
		if d.DistroID != "" {
			candidates = append(candidates, strings.ToLower(d.DistroID)+".yaml", strings.ToLower(d.DistroID)+".yml")
		}
		if d.DistroLike != "" {
			candidates = append(candidates, strings.ToLower(d.DistroLike)+".yaml", strings.ToLower(d.DistroLike)+".yml")
		}
		if d.FamilyID != "" {
			candidates = append(candidates, strings.ToLower(d.FamilyID)+".yaml", strings.ToLower(d.FamilyID)+".yml")
		}
		if strings.EqualFold(d.FamilyID, "archlinux") {
			candidates = append(candidates, "arch.yaml", "arch.yml")
		}
		if strings.EqualFold(d.FamilyID, "debian") {
			candidates = append(candidates, "debian.yaml", "debian.yml", "ubuntu.yaml", "devuan.yaml")
		}
	}

	// Standard distro fallbacks
	candidates = append(candidates, "debian.yaml", "debian.yml", "arch.yaml", "alpine.yaml", "fedora.yaml", "opensuse.yaml")

	seen := make(map[string]struct{})
	for _, c := range candidates {
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		fullPath := filepath.Join(costumePath, c)
		if _, err := os.Stat(fullPath); err == nil {
			return fullPath
		}
	}

	return ""
}

func loadSuit(yamlFile string) (*Suit, error) {
	if yamlFile == "" {
		return nil, fmt.Errorf("costume/accessory definition yaml file not found")
	}
	data, err := os.ReadFile(yamlFile)
	if err != nil {
		return nil, err
	}
	var suit Suit
	if err := yaml.Unmarshal(data, &suit); err != nil {
		return nil, err
	}
	suit.normalize()

	// Auto-discovery: if packages.yaml or packages.yml is present in the directory,
	// automatically load and merge packages into suit.Packages
	dir := filepath.Dir(yamlFile)
	if extraPkgs := loadPackagesYaml(dir); len(extraPkgs) > 0 {
		seen := make(map[string]struct{}, len(suit.Packages)+len(extraPkgs))
		for _, p := range suit.Packages {
			seen[p] = struct{}{}
		}
		for _, p := range extraPkgs {
			if _, exists := seen[p]; !exists {
				seen[p] = struct{}{}
				suit.Packages = append(suit.Packages, p)
			}
		}
	}

	return &suit, nil
}

// loadPackagesYaml searches and loads packages.yaml or packages.yml in the specified directory
func loadPackagesYaml(dir string) []string {
	for _, name := range []string{"packages.yaml", "packages.yml"} {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		// Prova parsing con struttura standard YAML (packages: o sequence.packages:)
		var doc struct {
			Packages []string `yaml:"packages"`
			Sequence *struct {
				Packages []string `yaml:"packages"`
			} `yaml:"sequence"`
		}
		if err := yaml.Unmarshal(data, &doc); err == nil {
			var pkgs []string
			if len(doc.Packages) > 0 {
				pkgs = append(pkgs, doc.Packages...)
			}
			if doc.Sequence != nil && len(doc.Sequence.Packages) > 0 {
				pkgs = append(pkgs, doc.Sequence.Packages...)
			}
			if len(pkgs) > 0 {
				return pkgs
			}
		}

		// Prova parsing come lista semplice di stringhe (- pkg1 \n - pkg2)
		var list []string
		if err := yaml.Unmarshal(data, &list); err == nil && len(list) > 0 {
			return list
		}
	}
	return nil
}

// licensePromptPackages holds suit.PackagesInteractive: packages whose
// preinst asks a license question that cannot be answered noninteractively.
var licensePromptPackages []string

// SetLicensePromptPackages is called by Wear() before any install starts.
func SetLicensePromptPackages(pkgs []string) { licensePromptPackages = pkgs }

func isLicensePrompt(pkg string) bool {
	c := normalizePkgName(pkg)
	for _, p := range licensePromptPackages {
		if normalizePkgName(p) == c {
			return true
		}
	}
	return false
}
