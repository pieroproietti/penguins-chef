package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	// Norton Commander / NCD classic panel styling
	borderStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#4A6984"))

	panelTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#80DEEA"))

	// Tree panel styles
	categoryStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#64B5F6"))

	treeBranchStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#78909C"))

	recipeFileStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ECEFF1"))

	// Classic Norton selection bar: bold white on deep blue
	selectedItemStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(lipgloss.Color("#005F87"))

	// Sober YAML preview styles
	yamlCommentStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#78909C"))

	yamlKeyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#80CBC4"))

	yamlValStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ECEFF1"))

	// Bottom status / help line
	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#90A4AE"))
)

// CategoryGroup holds recipes under a single category.
type CategoryGroup struct {
	Name      string
	Recipes   []RecipeItem
	Collapsed bool
}

// TreeNode represents a single row in the left-panel tree.
type TreeNode struct {
	IsCategory bool
	Category   string
	Recipe     *RecipeItem
	Prefix     string // e.g. "├── " or "└── "
	Label      string // e.g. "costumes/" or "duck.yaml"
}

// Model is the Bubble Tea model for the two-panel recipe picker.
type Model struct {
	allRecipes     []RecipeItem
	categories     []string
	groups         []CategoryGroup
	visibleNodes   []TreeNode
	cursor         int
	treeOffset     int
	previewOffset  int
	previewLines   []string
	previewTitle   string
	previewCache   map[string][]string
	width          int
	height         int
	selectedRecipe string
	cancelled      bool
}

// NewModel creates a new recipe picker model.
func NewModel(recipes []RecipeItem) Model {
	catMap := make(map[string][]RecipeItem)
	for _, r := range recipes {
		cat := r.Category
		if cat == "" {
			cat = "general"
		}
		catMap[cat] = append(catMap[cat], r)
	}

	var catNames []string
	for c := range catMap {
		catNames = append(catNames, c)
	}
	sort.Strings(catNames)

	var groups []CategoryGroup
	for _, c := range catNames {
		items := catMap[c]
		sort.Slice(items, func(i, j int) bool {
			return items[i].Name < items[j].Name
		})
		groups = append(groups, CategoryGroup{
			Name:      c,
			Recipes:   items,
			Collapsed: false,
		})
	}

	m := Model{
		allRecipes:   recipes,
		categories:   catNames,
		groups:       groups,
		previewCache: make(map[string][]string),
	}
	m.buildVisibleNodes()

	// Place cursor on first recipe if available, otherwise first category
	m.cursor = 0
	for i, node := range m.visibleNodes {
		if !node.IsCategory {
			m.cursor = i
			break
		}
	}
	m.updateSelection()

	return m
}

func (m *Model) buildVisibleNodes() {
	m.visibleNodes = nil
	for _, g := range m.groups {
		label := g.Name + "/"
		if g.Collapsed {
			label = g.Name + "/ [+]"
		}
		m.visibleNodes = append(m.visibleNodes, TreeNode{
			IsCategory: true,
			Category:   g.Name,
			Label:      label,
		})

		if g.Collapsed {
			continue
		}

		n := len(g.Recipes)
		for i := range g.Recipes {
			r := &g.Recipes[i]
			prefix := "├── "
			if i == n-1 {
				prefix = "└── "
			}
			filename := filepath.Base(r.Path)
			if filename == "" || filename == "." {
				filename = r.Name + ".yaml"
			}
			m.visibleNodes = append(m.visibleNodes, TreeNode{
				IsCategory: false,
				Category:   g.Name,
				Recipe:     r,
				Prefix:     prefix,
				Label:      filename,
			})
		}
	}
}

func (m *Model) updateSelection() {
	if m.cursor < 0 || m.cursor >= len(m.visibleNodes) {
		m.selectedRecipe = ""
		m.previewTitle = ""
		m.previewLines = nil
		return
	}

	node := m.visibleNodes[m.cursor]
	if !node.IsCategory && node.Recipe != nil {
		m.selectedRecipe = node.Recipe.Path
		m.previewTitle = node.Label
		m.previewLines = m.getRecipePreviewLines(node.Recipe.Path)
	} else {
		m.selectedRecipe = ""
		m.previewTitle = node.Category + "/"
		m.previewLines = m.getCategoryPreviewLines(node.Category)
	}
	m.previewOffset = 0
}

func (m *Model) getRecipePreviewLines(path string) []string {
	if lines, ok := m.previewCache[path]; ok {
		return lines
	}

	data, err := os.ReadFile(path)
	if err == nil {
		lines := strings.Split(string(data), "\n")
		m.previewCache[path] = lines
		return lines
	}

	// Fallback when file is not yet on disk (e.g. tests or synthetic items)
	var item *RecipeItem
	for i := range m.allRecipes {
		if m.allRecipes[i].Path == path {
			item = &m.allRecipes[i]
			break
		}
	}
	if item != nil {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("name: %s\n", item.Name))
		if item.Desc != "" {
			sb.WriteString(fmt.Sprintf("description: %q\n", item.Desc))
		}
		if item.Hostname != "" {
			sb.WriteString(fmt.Sprintf("hostname: %s\n", item.Hostname))
		}
		if len(item.Profiles) > 0 {
			sb.WriteString("profiles:\n")
			for _, p := range item.Profiles {
				sb.WriteString(fmt.Sprintf("  - %s\n", p))
			}
		}
		if len(item.Include) > 0 {
			sb.WriteString("include:\n")
			for _, inc := range item.Include {
				sb.WriteString(fmt.Sprintf("  - %s\n", inc))
			}
		}
		if item.Sysroot != "" {
			sb.WriteString(fmt.Sprintf("sysroot: %s\n", item.Sysroot))
		}
		lines := strings.Split(strings.TrimRight(sb.String(), "\n"), "\n")
		m.previewCache[path] = lines
		return lines
	}

	lines := []string{fmt.Sprintf("# Error reading file: %v", err)}
	return lines
}

func (m *Model) getCategoryPreviewLines(category string) []string {
	var recipes []RecipeItem
	for _, g := range m.groups {
		if g.Name == category {
			recipes = g.Recipes
			break
		}
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("# Category: %s/", category))
	lines = append(lines, fmt.Sprintf("# Recipes found: %d", len(recipes)))
	lines = append(lines, "#")
	for i, r := range recipes {
		prefix := "├── "
		if i == len(recipes)-1 {
			prefix = "└── "
		}
		name := filepath.Base(r.Path)
		if name == "" || name == "." {
			name = r.Name + ".yaml"
		}
		lines = append(lines, fmt.Sprintf("#   %s%s", prefix, name))
		if r.Desc != "" {
			lines = append(lines, fmt.Sprintf("#       (%s)", r.Desc))
		}
	}
	lines = append(lines, "#")
	lines = append(lines, "# Use ↑/↓ or j/k to select a recipe YAML file")
	lines = append(lines, "# Press Enter or Space to expand/collapse directory")
	return lines
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.cancelled = true
			m.selectedRecipe = ""
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.updateSelection()
			}
			return m, nil

		case "down", "j":
			if m.cursor < len(m.visibleNodes)-1 {
				m.cursor++
				m.updateSelection()
			}
			return m, nil

		case "home", "g":
			if len(m.visibleNodes) > 0 {
				m.cursor = 0
				m.updateSelection()
			}
			return m, nil

		case "end", "G":
			if len(m.visibleNodes) > 0 {
				m.cursor = len(m.visibleNodes) - 1
				m.updateSelection()
			}
			return m, nil

		case "enter":
			if m.cursor >= 0 && m.cursor < len(m.visibleNodes) {
				node := m.visibleNodes[m.cursor]
				if !node.IsCategory && node.Recipe != nil {
					m.selectedRecipe = node.Recipe.Path
					return m, tea.Quit
				}
				m.toggleCategory(node.Category)
			}
			return m, nil

		case " ":
			if m.cursor >= 0 && m.cursor < len(m.visibleNodes) {
				node := m.visibleNodes[m.cursor]
				if node.IsCategory {
					m.toggleCategory(node.Category)
				}
			}
			return m, nil

		case "left", "h":
			if m.cursor >= 0 && m.cursor < len(m.visibleNodes) {
				node := m.visibleNodes[m.cursor]
				if node.IsCategory {
					m.setCategoryCollapsed(node.Category, true)
				} else {
					m.jumpToCategory(node.Category)
				}
			}
			return m, nil

		case "right", "l":
			if m.cursor >= 0 && m.cursor < len(m.visibleNodes) {
				node := m.visibleNodes[m.cursor]
				if node.IsCategory {
					m.setCategoryCollapsed(node.Category, false)
				}
			}
			return m, nil

		case "tab", "]":
			m.jumpNextCategory()
			return m, nil

		case "shift+tab", "[":
			m.jumpPrevCategory()
			return m, nil

		case "pgdown", "ctrl+d":
			m.previewOffset += 10
			maxOff := len(m.previewLines) - 5
			if maxOff < 0 {
				maxOff = 0
			}
			if m.previewOffset > maxOff {
				m.previewOffset = maxOff
			}
			return m, nil

		case "pgup", "ctrl+u":
			m.previewOffset -= 10
			if m.previewOffset < 0 {
				m.previewOffset = 0
			}
			return m, nil
		}
	}
	return m, nil
}

func (m *Model) toggleCategory(catName string) {
	for i := range m.groups {
		if m.groups[i].Name == catName {
			m.groups[i].Collapsed = !m.groups[i].Collapsed
			break
		}
	}
	m.buildVisibleNodes()
	for i, node := range m.visibleNodes {
		if node.IsCategory && node.Category == catName {
			m.cursor = i
			break
		}
	}
	m.updateSelection()
}

func (m *Model) setCategoryCollapsed(catName string, collapsed bool) {
	for i := range m.groups {
		if m.groups[i].Name == catName {
			if m.groups[i].Collapsed == collapsed {
				return
			}
			m.groups[i].Collapsed = collapsed
			break
		}
	}
	m.buildVisibleNodes()
	for i, node := range m.visibleNodes {
		if node.IsCategory && node.Category == catName {
			m.cursor = i
			break
		}
	}
	m.updateSelection()
}

func (m *Model) jumpToCategory(catName string) {
	for i, node := range m.visibleNodes {
		if node.IsCategory && node.Category == catName {
			m.cursor = i
			m.updateSelection()
			return
		}
	}
}

func (m *Model) jumpNextCategory() {
	if len(m.visibleNodes) == 0 {
		return
	}
	for i := m.cursor + 1; i < len(m.visibleNodes); i++ {
		if m.visibleNodes[i].IsCategory {
			m.cursor = i
			m.updateSelection()
			return
		}
	}
	for i := 0; i <= m.cursor; i++ {
		if m.visibleNodes[i].IsCategory {
			m.cursor = i
			m.updateSelection()
			return
		}
	}
}

func (m *Model) jumpPrevCategory() {
	if len(m.visibleNodes) == 0 {
		return
	}
	for i := m.cursor - 1; i >= 0; i-- {
		if m.visibleNodes[i].IsCategory {
			m.cursor = i
			m.updateSelection()
			return
		}
	}
	for i := len(m.visibleNodes) - 1; i >= m.cursor; i-- {
		if m.visibleNodes[i].IsCategory {
			m.cursor = i
			m.updateSelection()
			return
		}
	}
}

func (m Model) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	// Calculate panel dimensions
	panelHeight := m.height - 2
	if panelHeight < 6 {
		panelHeight = 6
	}

	leftWidth := m.width * 38 / 100
	if leftWidth < 28 {
		leftWidth = 28
	}
	if leftWidth > 42 {
		leftWidth = 42
	}
	rightWidth := m.width - leftWidth - 2
	if rightWidth < 20 {
		rightWidth = 20
	}

	leftContentWidth := leftWidth - 2
	if leftContentWidth < 10 {
		leftContentWidth = 10
	}
	rightContentWidth := rightWidth - 2
	if rightContentWidth < 10 {
		rightContentWidth = 10
	}

	contentHeight := panelHeight - 2
	if contentHeight < 4 {
		contentHeight = 4
	}

	// Render Left Panel (Albero delle ricette)
	leftView := m.renderLeftPanel(leftContentWidth, contentHeight)

	// Render Right Panel (Anteprima del file)
	rightView := m.renderRightPanel(rightContentWidth, contentHeight)

	// Border containers
	leftBox := borderStyle.Width(leftContentWidth).Height(contentHeight).Render(leftView)
	rightBox := borderStyle.Width(rightContentWidth).Height(contentHeight).Render(rightView)

	panels := lipgloss.JoinHorizontal(lipgloss.Top, leftBox, rightBox)

	// Minimal classic footer
	footer := footerStyle.Render(" ↑/↓: Navigate  •  Enter: Apply  •  Space: Toggle  •  q/Esc: Quit")

	return lipgloss.JoinVertical(lipgloss.Left, panels, footer)
}

func (m Model) renderLeftPanel(width, height int) string {
	var sb strings.Builder

	title := panelTitleStyle.Render("[ Recipes Tree ]")
	sb.WriteString(title + "\n")

	treeViewHeight := height - 1
	if treeViewHeight < 1 {
		treeViewHeight = 1
	}

	treeOffset := m.treeOffset
	if m.cursor < treeOffset {
		treeOffset = m.cursor
	}
	if m.cursor >= treeOffset+treeViewHeight {
		treeOffset = m.cursor - treeViewHeight + 1
	}
	if treeOffset < 0 {
		treeOffset = 0
	}

	renderedLines := 0
	for i := treeOffset; i < len(m.visibleNodes) && renderedLines < treeViewHeight; i++ {
		node := m.visibleNodes[i]
		isSelected := (i == m.cursor)

		var lineText string
		if node.IsCategory {
			if isSelected {
				lineText = selectedItemStyle.Width(width).Render("> " + node.Label)
			} else {
				lineText = "  " + categoryStyle.Render(node.Label)
			}
		} else {
			if isSelected {
				lineText = selectedItemStyle.Width(width).Render("> " + node.Prefix + node.Label)
			} else {
				lineText = "  " + treeBranchStyle.Render(node.Prefix) + recipeFileStyle.Render(node.Label)
			}
		}

		sb.WriteString(lineText + "\n")
		renderedLines++
	}

	for renderedLines < treeViewHeight {
		sb.WriteString("\n")
		renderedLines++
	}

	return strings.TrimRight(sb.String(), "\n")
}

func (m Model) renderRightPanel(width, height int) string {
	var sb strings.Builder

	titleText := "[ Preview ]"
	if m.previewTitle != "" {
		titleText = fmt.Sprintf("[ Preview: %s ]", m.previewTitle)
	}

	previewViewHeight := height - 1
	if previewViewHeight < 1 {
		previewViewHeight = 1
	}

	if len(m.previewLines) > previewViewHeight {
		total := len(m.previewLines)
		startL := m.previewOffset + 1
		endL := m.previewOffset + previewViewHeight
		if endL > total {
			endL = total
		}
		titleText += fmt.Sprintf(" (%d-%d/%d)", startL, endL, total)
	}
	sb.WriteString(panelTitleStyle.Render(titleText) + "\n")

	renderedLines := 0
	startIdx := m.previewOffset
	for i := startIdx; i < len(m.previewLines) && renderedLines < previewViewHeight; i++ {
		line := m.previewLines[i]
		styledLine := formatYAMLLine(line, width)
		sb.WriteString(styledLine + "\n")
		renderedLines++
	}

	for renderedLines < previewViewHeight {
		sb.WriteString("\n")
		renderedLines++
	}

	return strings.TrimRight(sb.String(), "\n")
}

func formatYAMLLine(line string, maxWidth int) string {
	line = strings.TrimRight(line, "\r")
	line = truncateRunes(line, maxWidth)

	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "#") {
		return yamlCommentStyle.Render(line)
	}

	if colonIdx := strings.Index(line, ":"); colonIdx != -1 && !strings.HasPrefix(trimmed, "- ") {
		keyPart := line[:colonIdx+1]
		valPart := line[colonIdx+1:]
		return yamlKeyStyle.Render(keyPart) + yamlValStyle.Render(valPart)
	}

	return yamlValStyle.Render(line)
}

func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes])
	}
	return s
}

// Selected returns the selected recipe path and whether the user cancelled.
func (m Model) Selected() (string, bool) {
	return m.selectedRecipe, m.cancelled
}

// Run launches the interactive recipe picker TUI.
// Returns the selected recipe path, or empty string if cancelled.
func Run(dirs ...string) (string, error) {
	recipes, err := FindRecipes(dirs...)
	if err != nil {
		return "", fmt.Errorf("failed to scan for recipes: %w", err)
	}

	if len(recipes) == 0 {
		return "", fmt.Errorf("no recipes found in ~/.chef or current directory (run 'chef get' to download recipes)")
	}

	m := NewModel(recipes)
	p := tea.NewProgram(m, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("tui error: %w", err)
	}

	final, ok := finalModel.(Model)
	if !ok {
		return "", nil
	}

	if final.cancelled || final.selectedRecipe == "" {
		return "", nil
	}

	return final.selectedRecipe, nil
}
