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

// Recipe profiles are complete, explicit definitions rather than implicit
// merges of distro recipes.
type Recipe struct {
	Version  int                `yaml:"version"`
	Name     string             `yaml:"name"`
	Hostname string             `yaml:"hostname"`
	Include  []string           `yaml:"include"`
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
	if recipe.Version != 1 || strings.TrimSpace(recipe.Name) == "" || (len(recipe.Profiles) == 0 && len(recipe.Include) == 0) {
		return recipe, fmt.Errorf("recipe requires version: 1, a name and profiles or include")
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

// Resolve resolves all included recipes recursively, merging repositories, packages,
// files (with child override), services and default target into a unified recipe.
func (r Recipe) Resolve() (Recipe, error) {
	return r.resolve(map[string]bool{})
}

func (r Recipe) resolve(visited map[string]bool) (Recipe, error) {
	if len(r.Include) == 0 {
		return r, nil
	}

	merged := Recipe{
		Version:  r.Version,
		Name:     r.Name,
		Hostname: r.Hostname,
		Profiles: make(map[string]Profile),
		Sysroot:  r.Sysroot,
		baseDir:  r.baseDir,
	}

	for _, inc := range r.Include {
		incPath := inc
		if !filepath.IsAbs(incPath) {
			if r.baseDir == "" {
				return r, fmt.Errorf("relative include %q requires loading the recipe from a file", inc)
			}
			incPath = filepath.Join(r.baseDir, incPath)
		}
		incPath = filepath.Clean(incPath)
		if visited[incPath] {
			return r, fmt.Errorf("circular include detected: %s", incPath)
		}
		visited[incPath] = true

		f, err := os.Open(incPath)
		if err != nil {
			return r, fmt.Errorf("failed to open included recipe %q: %w", inc, err)
		}
		parent, err := Load(f)
		f.Close()
		if err != nil {
			return r, fmt.Errorf("failed to parse included recipe %q: %w", inc, err)
		}

		resolvedParent, err := parent.resolve(visited)
		if err != nil {
			return r, err
		}

		merged = mergeRecipes(merged, resolvedParent)
	}

	merged = mergeRecipes(merged, r)
	merged.Include = nil
	return merged, nil
}

func mergeRecipes(base, override Recipe) Recipe {
	res := Recipe{
		Version:  base.Version,
		Name:     base.Name,
		Hostname: base.Hostname,
		Profiles: make(map[string]Profile),
		Sysroot:  base.Sysroot,
		baseDir:  base.baseDir,
	}
	if override.Version != 0 {
		res.Version = override.Version
	}
	if override.Name != "" {
		res.Name = override.Name
	}
	if override.Hostname != "" {
		res.Hostname = override.Hostname
	}
	if override.Sysroot != "" {
		res.Sysroot = override.Sysroot
	}
	if override.baseDir != "" {
		res.baseDir = override.baseDir
	}

	for k, v := range base.Profiles {
		res.Profiles[k] = v
	}
	for k, v := range override.Profiles {
		if existing, ok := res.Profiles[k]; ok {
			res.Profiles[k] = mergeProfiles(existing, v)
		} else {
			res.Profiles[k] = v
		}
	}
	return res
}

func mergeProfiles(base, override Profile) Profile {
	res := Profile{
		DefaultTarget: base.DefaultTarget,
	}
	if override.DefaultTarget != "" {
		res.DefaultTarget = override.DefaultTarget
	}

	repoMap := make(map[string]int)
	for _, repo := range base.Repositories {
		repoMap[repo.Path] = len(res.Repositories)
		res.Repositories = append(res.Repositories, repo)
	}
	for _, repo := range override.Repositories {
		if idx, found := repoMap[repo.Path]; found {
			res.Repositories[idx] = repo
		} else {
			repoMap[repo.Path] = len(res.Repositories)
			res.Repositories = append(res.Repositories, repo)
		}
	}

	seenPkg := make(map[string]bool)
	for _, pkg := range base.Packages {
		if !seenPkg[pkg] {
			seenPkg[pkg] = true
			res.Packages = append(res.Packages, pkg)
		}
	}
	for _, pkg := range override.Packages {
		if !seenPkg[pkg] {
			seenPkg[pkg] = true
			res.Packages = append(res.Packages, pkg)
		}
	}

	fileMap := make(map[string]int)
	for _, f := range base.Files {
		fileMap[f.Path] = len(res.Files)
		res.Files = append(res.Files, f)
	}
	for _, f := range override.Files {
		if idx, found := fileMap[f.Path]; found {
			res.Files[idx] = f
		} else {
			fileMap[f.Path] = len(res.Files)
			res.Files = append(res.Files, f)
		}
	}

	seenSvc := make(map[string]bool)
	for _, svc := range base.Services {
		if !seenSvc[svc] {
			seenSvc[svc] = true
			res.Services = append(res.Services, svc)
		}
	}
	for _, svc := range override.Services {
		if !seenSvc[svc] {
			seenSvc[svc] = true
			res.Services = append(res.Services, svc)
		}
	}

	return res
}

var (
	identifier    = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.+@:-]*$`)
	hostnameRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)
)

func isCostume(r Recipe) bool {
	if r.Hostname != "" {
		return true
	}
	clean := filepath.ToSlash(r.baseDir)
	return strings.Contains("/"+clean+"/", "/costumes/")
}

// Build validates the whole selected profile before creating any operation.
func Build(recipe Recipe, family, init string) (Plan, error) {
	resolved, err := recipe.Resolve()
	if err != nil {
		return Plan{Name: recipe.Name, Family: family, Init: init}, err
	}
	recipe = resolved
	plan := Plan{Name: recipe.Name, Family: family, Init: init}
	profile, ok := recipe.Profiles[family]
	if !ok {
		return plan, fmt.Errorf("no explicit profile for %q", family)
	}
	backend, err := packagesFor(family)
	if err != nil {
		return plan, err
	}
	hostname := recipe.Hostname
	if hostname == "" && isCostume(recipe) {
		hostname = recipe.Name
	}
	_, initErr := initFor(init)
	if family != "debian" {
		if len(profile.Services) > 0 || profile.DefaultTarget != "" || hostname != "" {
			if initErr != nil {
				return plan, initErr
			}
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
	if hostname != "" {
		if !hostnameRegex.MatchString(hostname) {
			return plan, fmt.Errorf("invalid hostname %q", hostname)
		}
		plan.Steps = append(plan.Steps, Step{ID: "hostname:" + hostname, Phase: "configuration", Hostname: hostname})
	}
	if initErr == nil {
		for _, service := range profile.Services {
			plan.Steps = append(plan.Steps, Step{ID: "service:" + service, Phase: "init", Service: service})
		}
		if profile.DefaultTarget != "" {
			plan.Steps = append(plan.Steps, Step{ID: "default-target:" + profile.DefaultTarget, Phase: "init", DefaultTarget: profile.DefaultTarget})
		}
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
