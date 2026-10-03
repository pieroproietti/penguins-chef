// Package provision implements the experimental declarative execution engine.
package provision

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Recipe is deliberately distinct from Wardrobe v2; profiles are complete,
// explicit definitions, rather than implicit merges of distro recipes.
type Recipe struct {
	Version  int                `yaml:"version"`
	Name     string             `yaml:"name"`
	Profiles map[string]Profile `yaml:"profiles"`
	Sysroot  string             `yaml:"sysroot"`
	baseDir  string
}

type Profile struct {
	Repositories  []File   `yaml:"repositories"`
	Packages      []string `yaml:"packages"`
	Files         []File   `yaml:"files"`
	Services      []string `yaml:"services"`
	DefaultTarget string   `yaml:"default_target"`
}

type File struct {
	Path    string `yaml:"path"`
	Content string `yaml:"content"`
}

func Load(r io.Reader) (Recipe, error) {
	var recipe Recipe
	d := yaml.NewDecoder(r)
	d.KnownFields(true)
	if err := d.Decode(&recipe); err != nil {
		return recipe, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return recipe, fmt.Errorf("expected a single YAML document")
	}
	if recipe.Version != 1 || strings.TrimSpace(recipe.Name) == "" || len(recipe.Profiles) == 0 {
		return recipe, fmt.Errorf("recipe requires version: 1, a name and profiles")
	}
	if f, ok := r.(*os.File); ok {
		var err error
		recipe.baseDir, err = filepath.Abs(filepath.Dir(f.Name()))
		if err != nil {
			return recipe, err
		}
	}
	return recipe, nil
}

var identifier = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.+@:-]*$`)

// Build validates the whole selected profile before creating any operation.
func Build(recipe Recipe, family, init string) (Plan, error) {
	plan := Plan{Name: recipe.Name, Family: family, Init: init}
	profile, ok := recipe.Profiles[family]
	if !ok {
		return plan, fmt.Errorf("no explicit profile for %q", family)
	}
	backend, err := packagesFor(family)
	if err != nil {
		return plan, err
	}
	if len(profile.Services) > 0 || profile.DefaultTarget != "" {
		if _, err := initFor(init); err != nil {
			return plan, err
		}
	}
	if profile.DefaultTarget != "" && (!identifier.MatchString(profile.DefaultTarget) || !strings.HasSuffix(profile.DefaultTarget, ".target")) {
		return plan, fmt.Errorf("invalid default target %q", profile.DefaultTarget)
	}
	paths := map[string]bool{}
	for _, f := range append(append([]File{}, profile.Repositories...), profile.Files...) {
		if !filepath.IsAbs(f.Path) || filepath.Clean(f.Path) != f.Path || f.Path == "/" || paths[f.Path] {
			return plan, fmt.Errorf("invalid or duplicate file path %q", f.Path)
		}
		paths[f.Path] = true
	}
	for path := range paths {
		for parent := filepath.Dir(path); parent != "/"; parent = filepath.Dir(parent) {
			if paths[parent] {
				return plan, fmt.Errorf("managed file %q is a parent of %q", parent, path)
			}
		}
	}
	for _, f := range profile.Repositories {
		if err := backend.validateRepository(f); err != nil {
			return plan, err
		}
	}
	for _, name := range append(append([]string{}, profile.Packages...), profile.Services...) {
		if !identifier.MatchString(name) {
			return plan, fmt.Errorf("invalid package or service name %q", name)
		}
	}
	for _, f := range profile.Repositories {
		plan.Steps = append(plan.Steps, Step{ID: "repository:" + f.Path, Phase: "repositories", File: &f})
	}
	if len(profile.Packages) > 0 || len(profile.Repositories) > 0 {
		plan.Steps = append(plan.Steps, Step{ID: "repositories:prepare", Phase: "repositories", Command: backend.prepare()})
	}
	seen := map[string]bool{}
	var packages []string
	for _, pkg := range profile.Packages {
		if seen[pkg] {
			continue
		}
		seen[pkg] = true
		packages = append(packages, pkg)
		plan.Steps = append(plan.Steps, Step{ID: "available:" + pkg, Phase: "repositories", Availability: backend.availability(pkg)})
	}
	// Verify availability of ALL targets before installing any of them. Install
	// one package transaction, but retain individual checks and verification.
	if len(packages) > 0 {
		plan.Steps = append(plan.Steps, Step{ID: "packages:install", Phase: "packages", Packages: packages})
	}
	for _, f := range profile.Files {
		plan.Steps = append(plan.Steps, Step{ID: "file:" + f.Path, Phase: "configuration", File: &f})
	}
	for _, service := range profile.Services {
		plan.Steps = append(plan.Steps, Step{ID: "service:" + service, Phase: "init", Service: service})
	}
	if profile.DefaultTarget != "" {
		plan.Steps = append(plan.Steps, Step{ID: "default-target:" + profile.DefaultTarget, Phase: "init", DefaultTarget: profile.DefaultTarget})
	}
	if recipe.Sysroot != "" {
		source := recipe.Sysroot
		if !filepath.IsAbs(source) {
			if recipe.baseDir == "" {
				return plan, fmt.Errorf("relative sysroot requires loading the recipe from a file")
			}
			source = filepath.Join(recipe.baseDir, source)
		}
		if err := plan.AddSysroot(source); err != nil {
			return plan, err
		}
	}
	return plan, nil
}
