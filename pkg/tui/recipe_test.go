package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindRecipesInWorkspace(t *testing.T) {
	// Locate repository recipes
	recipesDir := "../../recipes"
	if _, err := os.Stat(recipesDir); err != nil {
		recipesDir = "recipes"
		if _, err := os.Stat(recipesDir); err != nil {
			t.Skip("recipes directory not found from test runner")
		}
	}

	items, err := FindRecipes(recipesDir)
	if err != nil {
		t.Fatalf("FindRecipes returned error: %v", err)
	}

	if len(items) == 0 {
		t.Fatalf("expected to find recipes, got 0")
	}

	// Verify categories were extracted
	cats := GetCategories(items)
	if len(cats) < 3 {
		t.Fatalf("expected multiple categories, got: %v", cats)
	}

	if cats[0] != "All" {
		t.Errorf("expected first category to be 'All', got %q", cats[0])
	}

	// Check specific recipes and their extracted descriptions
	foundDuck := false
	foundVLC := false
	foundBase := false

	for _, item := range items {
		if item.Name == "duck" {
			foundDuck = true
			if item.Category != "costumes" {
				t.Errorf("duck category: expected 'costumes', got %q", item.Category)
			}
			if item.Desc == "" {
				t.Errorf("expected duck to have a description, got empty")
			}
		}
		if item.Name == "vlc" {
			foundVLC = true
			if item.Category != "multimedia" {
				t.Errorf("vlc category: expected 'multimedia', got %q", item.Category)
			}
			if item.Desc == "" {
				t.Errorf("expected vlc to have a description, got empty")
			}
		}
		if item.Name == "base" {
			foundBase = true
			if item.Category != "base" {
				t.Errorf("base category: expected 'base', got %q", item.Category)
			}
		}
	}

	if !foundDuck {
		t.Errorf("recipe 'duck' was not found")
	}
	if !foundVLC {
		t.Errorf("recipe 'vlc' was not found")
	}
	if !foundBase {
		t.Errorf("recipe 'base' was not found")
	}
}

func TestFindRecipesCustomDirectory(t *testing.T) {
	tempDir := t.TempDir()

	// Create structure:
	// costumes/custom/custom.yaml
	// de/customde.yaml
	// root.yaml
	// sysroot/ignored.yaml
	costumeDir := filepath.Join(tempDir, "costumes", "custom")
	deDir := filepath.Join(tempDir, "de")
	sysrootDir := filepath.Join(tempDir, "costumes", "custom", "sysroot")

	if err := os.MkdirAll(costumeDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(deDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sysrootDir, 0755); err != nil {
		t.Fatal(err)
	}

	yaml1 := `version: 1
name: mycostume
description: "My custom costume recipe"
hostname: customhost
`
	yaml2 := `version: 1
name: myde
description: "My custom DE recipe"
`
	yaml3 := `version: 1
name: myroot
`
	ignoredYaml := `version: 1
name: ignored
`

	if err := os.WriteFile(filepath.Join(costumeDir, "custom.yaml"), []byte(yaml1), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deDir, "customde.yaml"), []byte(yaml2), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "root.yaml"), []byte(yaml3), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysrootDir, "ignored.yaml"), []byte(ignoredYaml), 0644); err != nil {
		t.Fatal(err)
	}

	items, err := FindRecipes(tempDir)
	if err != nil {
		t.Fatalf("FindRecipes failed: %v", err)
	}

	if len(items) != 3 {
		t.Fatalf("expected exactly 3 recipes (ignoring sysroot), got %d", len(items))
	}

	catMap := make(map[string]RecipeItem)
	for _, it := range items {
		catMap[it.Name] = it
	}

	costumeItem, ok := catMap["mycostume"]
	if !ok {
		t.Fatal("mycostume not found")
	}
	if costumeItem.Category != "costumes" {
		t.Errorf("expected category 'costumes', got %q", costumeItem.Category)
	}
	if costumeItem.Desc != "My custom costume recipe" {
		t.Errorf("expected description 'My custom costume recipe', got %q", costumeItem.Desc)
	}
	if costumeItem.Hostname != "customhost" {
		t.Errorf("expected hostname 'customhost', got %q", costumeItem.Hostname)
	}

	deItem, ok := catMap["myde"]
	if !ok {
		t.Fatal("myde not found")
	}
	if deItem.Category != "de" {
		t.Errorf("expected category 'de', got %q", deItem.Category)
	}

	rootItem, ok := catMap["myroot"]
	if !ok {
		t.Fatal("myroot not found")
	}
	if rootItem.Category != "general" {
		t.Errorf("expected category 'general' for root file, got %q", rootItem.Category)
	}
}
