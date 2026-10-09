package tui

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func sampleRecipes() []RecipeItem {
	return []RecipeItem{
		{
			Name:        "duck",
			Category:    "costumes",
			Path:        "/path/recipes/costumes/duck/duck.yaml",
			RelPath:     "recipes/costumes/duck/duck.yaml",
			Desc:        "Duck costume with Cinnamon desktop",
			Hostname:    "duck",
			Profiles:    []string{"debian", "archlinux"},
			Include:     []string{"../../base/base.yaml"},
		},
		{
			Name:        "xfce4",
			Category:    "de",
			Path:        "/path/recipes/de/xfce4.yaml",
			RelPath:     "recipes/de/xfce4.yaml",
			Desc:        "XFCE desktop environment",
			Profiles:    []string{"debian"},
		},
		{
			Name:        "vlc",
			Category:    "multimedia",
			Path:        "/path/recipes/multimedia/vlc.yaml",
			RelPath:     "recipes/multimedia/vlc.yaml",
			Desc:        "VLC media player",
			Profiles:    []string{"debian", "archlinux", "fedora"},
		},
	}
}

func TestModelInitialization(t *testing.T) {
	recipes := sampleRecipes()
	m := NewModel(recipes)

	if len(m.categories) != 3 { // "costumes", "de", "multimedia"
		t.Fatalf("expected 3 categories, got %d: %v", len(m.categories), m.categories)
	}

	// Resize model
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model := updated.(Model)

	view := model.View()

	// Must NOT contain old modern headers or banner
	if strings.Contains(view, "PENGUINS CHEF") {
		t.Errorf("expected view not to contain old header banner, got: %s", view)
	}

	// Left panel: Recipes Tree with ASCII tree
	if !strings.Contains(view, "Recipes Tree") {
		t.Errorf("expected view to contain 'Recipes Tree', got: %s", view)
	}
	if !strings.Contains(view, "costumes/") || !strings.Contains(view, "de/") || !strings.Contains(view, "multimedia/") {
		t.Errorf("expected view to contain tree categories, got: %s", view)
	}
	if !strings.Contains(view, "└── duck.yaml") && !strings.Contains(view, "├── duck.yaml") {
		t.Errorf("expected view to contain ASCII tree branch for duck.yaml, got: %s", view)
	}

	// Right panel: Preview with YAML content
	if !strings.Contains(view, "Preview: duck.yaml") {
		t.Errorf("expected view to contain 'Preview: duck.yaml', got: %s", view)
	}
	if !strings.Contains(view, "Duck costume with Cinnamon desktop") {
		t.Errorf("expected preview to contain recipe YAML description, got: %s", view)
	}
	if !strings.Contains(view, "name: duck") {
		t.Errorf("expected preview to contain YAML key name, got: %s", view)
	}
}

func TestModelNavigation(t *testing.T) {
	recipes := sampleRecipes()
	m := NewModel(recipes)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model := updated.(Model)

	// Initially on duck.yaml (index 1 in visibleNodes: costumes/ is 0, duck.yaml is 1)
	selected, _ := model.Selected()
	if selected != recipes[0].Path {
		t.Fatalf("expected initial selected recipe %q, got %q", recipes[0].Path, selected)
	}

	// Press down (moves to de/ category node)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)
	if model.selectedRecipe != "" {
		t.Errorf("expected category node to have empty selected recipe, got %q", model.selectedRecipe)
	}

	// Press down again (moves to xfce4.yaml)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)
	selected, _ = model.Selected()
	if selected != recipes[1].Path {
		t.Fatalf("expected selected recipe %q (xfce4), got %q", recipes[1].Path, selected)
	}

	// Verify preview updated dynamically to xfce4
	view := model.View()
	if !strings.Contains(view, "Preview: xfce4.yaml") {
		t.Errorf("expected preview to show xfce4.yaml, got: %s", view)
	}
	if !strings.Contains(view, "XFCE desktop environment") {
		t.Errorf("expected preview to contain xfce4 description, got: %s", view)
	}

	// Press up using 'k' (moves back to de/ category)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	model = updated.(Model)
	if model.selectedRecipe != "" {
		t.Errorf("expected empty recipe on category node, got %q", model.selectedRecipe)
	}

	// Press up again using 'k' (moves back to duck.yaml)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	model = updated.(Model)
	selected, _ = model.Selected()
	if selected != recipes[0].Path {
		t.Fatalf("expected duck.yaml after navigating up, got %q", selected)
	}
}

func TestModelSelection(t *testing.T) {
	recipes := sampleRecipes()
	m := NewModel(recipes)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model := updated.(Model)

	// Press enter on the selected recipe (duck.yaml)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	if cmd == nil {
		t.Errorf("expected non-nil tea.Cmd on selection")
	}

	selected, cancelled := model.Selected()
	if cancelled {
		t.Errorf("expected not cancelled")
	}
	if selected != recipes[0].Path {
		t.Errorf("expected selected recipe %q, got %q", recipes[0].Path, selected)
	}
}

func TestModelCancellation(t *testing.T) {
	recipes := sampleRecipes()
	m := NewModel(recipes)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model := updated.(Model)

	// Press 'q'
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	model = updated.(Model)

	if cmd == nil {
		t.Errorf("expected non-nil tea.Cmd on quit")
	}

	selected, cancelled := model.Selected()
	if !cancelled {
		t.Errorf("expected cancelled to be true")
	}
	if selected != "" {
		t.Errorf("expected empty selected path on cancel, got %q", selected)
	}

	// Test Esc key as well
	m2 := NewModel(recipes)
	updated2, cmd2 := m2.Update(tea.KeyMsg{Type: tea.KeyEscape})
	model2 := updated2.(Model)
	if cmd2 == nil {
		t.Errorf("expected non-nil tea.Cmd on Esc")
	}
	_, cancelled2 := model2.Selected()
	if !cancelled2 {
		t.Errorf("expected cancelled to be true on Esc")
	}
}

func TestModelToggleCategory(t *testing.T) {
	recipes := sampleRecipes()
	m := NewModel(recipes)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model := updated.(Model)

	// Initially all 3 categories expanded (3 categories + 3 recipes = 6 visible nodes)
	if len(model.visibleNodes) != 6 {
		t.Fatalf("expected 6 visible nodes initially, got %d", len(model.visibleNodes))
	}

	// Move up to costumes/ category (index 0)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = updated.(Model)
	if !model.visibleNodes[model.cursor].IsCategory {
		t.Fatalf("expected cursor on category node, got: %+v", model.visibleNodes[model.cursor])
	}

	// Press Space to collapse costumes/
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(Model)
	// Now duck.yaml is hidden: 5 visible nodes
	if len(model.visibleNodes) != 5 {
		t.Fatalf("expected 5 visible nodes after collapsing costumes, got %d", len(model.visibleNodes))
	}

	// Press Space again to expand costumes/
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(Model)
	if len(model.visibleNodes) != 6 {
		t.Fatalf("expected 6 visible nodes after expanding costumes, got %d", len(model.visibleNodes))
	}
}

func TestModelPreviewScrolling(t *testing.T) {
	recipes := sampleRecipes()
	m := NewModel(recipes)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model := updated.(Model)

	// Initially offset is 0
	if model.previewOffset != 0 {
		t.Errorf("expected initial previewOffset 0, got %d", model.previewOffset)
	}

	// Press pgdown
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = updated.(Model)
	// Offset should increase or clamp to max
	if model.previewOffset < 0 {
		t.Errorf("expected non-negative previewOffset, got %d", model.previewOffset)
	}

	// Press pgup
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	model = updated.(Model)
	if model.previewOffset != 0 {
		t.Errorf("expected previewOffset 0 after pgup, got %d", model.previewOffset)
	}
}

func TestRealWorkspaceRecipesTUI(t *testing.T) {
	recipesDir := "../../recipes"
	if _, err := os.Stat(recipesDir); err != nil {
		recipesDir = "recipes"
		if _, err := os.Stat(recipesDir); err != nil {
			t.Skip("recipes directory not found")
		}
	}

	recipes, err := FindRecipes(recipesDir)
	if err != nil {
		t.Fatalf("FindRecipes failed: %v", err)
	}
	if len(recipes) == 0 {
		t.Fatalf("no recipes found")
	}

	m := NewModel(recipes)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 35})
	model := updated.(Model)

	view := model.View()
	t.Logf("\n%s\n", view)

	// Verify tree has ASCII branches
	if !strings.Contains(view, "├──") && !strings.Contains(view, "└──") {
		t.Errorf("expected view to contain ASCII tree branches, got: %s", view)
	}

	// Verify preview panel header and YAML content
	if !strings.Contains(view, "[ Preview:") {
		t.Errorf("expected view to contain preview title, got: %s", view)
	}
	if !strings.Contains(view, "version:") {
		t.Errorf("expected view to contain YAML 'version:' keyword, got: %s", view)
	}
}

func TestRenderVisualOutput(t *testing.T) {
	recipes := sampleRecipes()
	m := NewModel(recipes)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 90, Height: 20})
	model := updated.(Model)
	t.Logf("\n%s\n", model.View())
}

