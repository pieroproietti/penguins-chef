package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pieroproietti/penguins-chef/pkg/chef"
	"gopkg.in/yaml.v3"
)

// RecipeItem represents a recipe found during scanning.
type RecipeItem struct {
	Name     string   `yaml:"name"`
	Category string   `yaml:"-"`
	Path     string   `yaml:"-"`
	RelPath  string   `yaml:"-"`
	Desc     string   `yaml:"description"`
	Hostname string   `yaml:"hostname"`
	Include  []string `yaml:"include"`
	Profiles []string `yaml:"-"`
	Sysroot  string   `yaml:"sysroot"`
}

// Title returns the display title for list views.
func (r RecipeItem) Title() string {
	if r.Category != "" && r.Category != "general" {
		return fmt.Sprintf("[%s] %s", r.Category, r.Name)
	}
	return r.Name
}

// Description is required by list.DefaultItem.
func (r RecipeItem) Description() string {
	if r.Desc != "" {
		return r.Desc
	}
	return r.RelPath
}

// FilterValue implements list.Item for fuzzy searching.
func (r RecipeItem) FilterValue() string {
	return fmt.Sprintf("%s %s %s %s", r.Category, r.Name, r.Desc, r.RelPath)
}

// rawYAML is a permissive parser for recipe discovery.
type rawYAML struct {
	Version     int                                `yaml:"version"`
	Name        string                             `yaml:"name"`
	Description string                             `yaml:"description"`
	Hostname    string                             `yaml:"hostname"`
	Include     []string                           `yaml:"include"`
	Profiles    map[string]map[string]any          `yaml:"profiles"`
	Sysroot     string                             `yaml:"sysroot"`
}

// DefaultRecipeDirs returns the list of default directories to scan for recipes.
func DefaultRecipeDirs() []string {
	var dirs []string

	if env := os.Getenv("CHEF_RECIPES_DIR"); env != "" {
		if info, err := os.Stat(env); err == nil && info.IsDir() {
			dirs = append(dirs, env)
		}
	}

	// 1. Current directory's "recipes" folder (e.g. inside repository)
	if info, err := os.Stat("recipes"); err == nil && info.IsDir() {
		dirs = append(dirs, "recipes")
	}

	// 2. ~/.chef/recipes or ~/.chef
	if chefRoot, err := chef.GetChefRoot(); err == nil && chefRoot != "" {
		chefRecipes := filepath.Join(chefRoot, "recipes")
		if info, err := os.Stat(chefRecipes); err == nil && info.IsDir() {
			dirs = append(dirs, chefRecipes)
		} else if info, err := os.Stat(chefRoot); err == nil && info.IsDir() {
			dirs = append(dirs, chefRoot)
		}
	}

	// 3. Fallback to current directory if nothing else found
	if len(dirs) == 0 {
		dirs = append(dirs, ".")
	}

	return dirs
}

// FindRecipes scans the specified directories recursively for recipes.
// It groups them by category and deduplicates items.
func FindRecipes(dirs ...string) ([]RecipeItem, error) {
	if len(dirs) == 0 {
		dirs = DefaultRecipeDirs()
	}

	seen := make(map[string]bool)
	var recipes []RecipeItem

	for _, dir := range dirs {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			absDir = dir
		}

		stat, err := os.Stat(absDir)
		if err != nil || !stat.IsDir() {
			continue
		}

		err = filepath.Walk(absDir, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return nil
			}

			// Skip directories
			if info.IsDir() {
				name := info.Name()
				if strings.HasPrefix(name, ".") || name == "sysroot" || name == "node_modules" || name == "vendor" {
					return filepath.SkipDir
				}
				return nil
			}

			// Only process .yaml and .yml files
			ext := filepath.Ext(info.Name())
			if ext != ".yaml" && ext != ".yml" {
				return nil
			}

			// Compute relative path from scanned root
			rel, err := filepath.Rel(absDir, path)
			if err != nil {
				rel = info.Name()
			}
			slashRel := filepath.ToSlash(rel)

			// Determine category from directory structure
			// e.g. "costumes/duck/duck.yaml" -> "costumes"
			// "de/xfce4.yaml" -> "de"
			parts := strings.Split(slashRel, "/")
			category := "general"
			if len(parts) > 1 {
				category = parts[0]
			}

			// Read and parse YAML file
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}

			var parsed rawYAML
			if err := yaml.Unmarshal(data, &parsed); err != nil {
				return nil
			}

			// Must look like a recipe (e.g. has name or version 1 or profiles or include)
			recipeName := parsed.Name
			if recipeName == "" {
				recipeName = strings.TrimSuffix(info.Name(), ext)
			}

			// Collect profile names (distro families)
			var profiles []string
			for p := range parsed.Profiles {
				profiles = append(profiles, p)
			}
			sort.Strings(profiles)

			dedupKey := category + "/" + recipeName
			if seen[dedupKey] {
				return nil
			}
			seen[dedupKey] = true

			// Relative path from CWD for cleaner display if inside CWD
			displayPath := path
			if cwd, err := os.Getwd(); err == nil {
				if relCwd, err := filepath.Rel(cwd, path); err == nil && !strings.HasPrefix(relCwd, "..") {
					displayPath = relCwd
				}
			}

			item := RecipeItem{
				Name:        recipeName,
				Category:    category,
				Path:        path,
				RelPath:     displayPath,
				Desc:        strings.TrimSpace(parsed.Description),
				Hostname:    parsed.Hostname,
				Include:     parsed.Include,
				Profiles:    profiles,
				Sysroot:     parsed.Sysroot,
			}

			recipes = append(recipes, item)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	// Sort recipes by Category, then by Name
	sort.Slice(recipes, func(i, j int) bool {
		if recipes[i].Category != recipes[j].Category {
			return recipes[i].Category < recipes[j].Category
		}
		return recipes[i].Name < recipes[j].Name
	})

	return recipes, nil
}

// GetCategories returns unique categories found in items, with "All" as first.
func GetCategories(items []RecipeItem) []string {
	catMap := make(map[string]bool)
	for _, it := range items {
		if it.Category != "" {
			catMap[it.Category] = true
		}
	}

	var cats []string
	for c := range catMap {
		cats = append(cats, c)
	}
	sort.Strings(cats)

	return append([]string{"All"}, cats...)
}
