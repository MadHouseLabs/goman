// Package storybook provides a comprehensive interactive documentation system for TUI components
package storybook

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// App represents the main Storybook application
type App struct {
	width             int
	height            int
	registry          *Registry
	currentCategory   int
	currentStory      int
	currentView       ViewMode
	propertyPanel     *PropertyPanel
	codeViewer        *CodeViewer
	navigation        *Navigation
	focused           FocusState
	showPropertyPanel bool
	showCodeViewer    bool
}

// ViewMode represents different view modes in the storybook
type ViewMode int

const (
	ViewModeStory ViewMode = iota
	ViewModeProperty
	ViewModeCode
	ViewModeNavigation
)

// FocusState tracks which part of the UI has focus
type FocusState int

const (
	FocusStory FocusState = iota
	FocusNavigation
	FocusPropertyPanel
	FocusCodeViewer
)

// NewApp creates a new Storybook application
func NewApp() *App {
	registry := NewRegistry()
	
	// Register all component stories
	registerAllStories(registry)
	
	return &App{
		registry:          registry,
		currentCategory:   0,
		currentStory:      0,
		currentView:      ViewModeStory,
		propertyPanel:    NewPropertyPanel(),
		codeViewer:       NewCodeViewer(),
		navigation:       NewNavigation(registry),
		focused:          FocusStory,
		showPropertyPanel: true,
		showCodeViewer:   false,
	}
}

// Init initializes the Storybook app
func (a *App) Init() tea.Cmd {
	// Initialize the current story
	if story := a.getCurrentStory(); story != nil {
		return story.Init()
	}
	return nil
}

// Update handles messages and updates the app state
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.updateLayout()

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return a, tea.Quit

		case "tab":
			a.cycleFocus()
			
		case "shift+tab":
			a.cycleFocusReverse()

		case "1":
			a.currentView = ViewModeStory
			a.focused = FocusStory

		case "2":
			a.currentView = ViewModeProperty
			a.focused = FocusPropertyPanel
			a.showPropertyPanel = true

		case "3":
			a.currentView = ViewModeCode
			a.focused = FocusCodeViewer
			a.showCodeViewer = true

		case "4":
			a.currentView = ViewModeNavigation
			a.focused = FocusNavigation

		case "p":
			a.showPropertyPanel = !a.showPropertyPanel

		case "c":
			a.showCodeViewer = !a.showCodeViewer

		case "h", "left":
			if a.focused == FocusNavigation {
				a.previousCategory()
			} else {
				a.previousStory()
			}

		case "l", "right":
			if a.focused == FocusNavigation {
				a.nextCategory()
			} else {
				a.nextStory()
			}

		case "k", "up":
			if a.focused == FocusNavigation {
				a.previousStory()
			}

		case "j", "down":
			if a.focused == FocusNavigation {
				a.nextStory()
			}

		case "enter":
			if a.focused == FocusNavigation {
				// Navigation handled by arrow keys
			}
		}
	}

	// Update focused component
	switch a.focused {
	case FocusStory:
		if story := a.getCurrentStory(); story != nil {
			storyModel, storyCmd := story.Update(msg)
			if s, ok := storyModel.(*Story); ok {
				a.updateCurrentStory(s)
			}
			cmds = append(cmds, storyCmd)
		}

	case FocusNavigation:
		navModel, navCmd := a.navigation.Update(msg)
		if nav, ok := navModel.(*Navigation); ok {
			a.navigation = nav
			// Check if navigation selection changed
			if nav.selectedCategory != a.currentCategory || nav.selectedStory != a.currentStory {
				a.currentCategory = nav.selectedCategory
				a.currentStory = nav.selectedStory
				a.updatePropertyPanel()
			}
		}
		cmds = append(cmds, navCmd)

	case FocusPropertyPanel:
		if a.showPropertyPanel {
			propModel, propCmd := a.propertyPanel.Update(msg)
			if prop, ok := propModel.(*PropertyPanel); ok {
				a.propertyPanel = prop
			}
			cmds = append(cmds, propCmd)
		}

	case FocusCodeViewer:
		if a.showCodeViewer {
			codeModel, codeCmd := a.codeViewer.Update(msg)
			if code, ok := codeModel.(*CodeViewer); ok {
				a.codeViewer = code
			}
			cmds = append(cmds, codeCmd)
		}
	}

	// Update property panel with current story state
	a.updatePropertyPanel()

	return a, tea.Batch(cmds...)
}

// View renders the Storybook app
func (a *App) View() string {
	if a.width == 0 || a.height == 0 {
		return "Initializing Storybook..."
	}

	// Header
	header := a.renderHeader()

	// Main content area
	contentHeight := a.height - 4 // Account for header and footer
	content := a.renderContent(contentHeight)

	// Footer
	footer := a.renderFooter()

	return lipgloss.JoinVertical(lipgloss.Left, header, content, footer)
}

// renderHeader renders the application header
func (a *App) renderHeader() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("69")).
		Background(lipgloss.Color("236")).
		Padding(0, 2).
		Width(a.width)

	category := a.getCurrentCategory()
	story := a.getCurrentStory()
	
	var title string
	if category != nil && story != nil {
		title = fmt.Sprintf("📚 TUI Storybook - %s › %s", category.Name, story.Name)
	} else {
		title = "📚 TUI Component Storybook"
	}

	return titleStyle.Render(title)
}

// renderContent renders the main content area
func (a *App) renderContent(height int) string {
	leftWidth := a.width / 4
	centerWidth := a.width / 2
	rightWidth := a.width - leftWidth - centerWidth

	// Left panel - Navigation
	leftPanel := a.renderLeftPanel(leftWidth, height)

	// Center panel - Story view
	centerPanel := a.renderCenterPanel(centerWidth, height)

	// Right panel - Property panel and/or code viewer
	rightPanel := a.renderRightPanel(rightWidth, height)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, centerPanel, rightPanel)
}

// renderLeftPanel renders the navigation panel
func (a *App) renderLeftPanel(width, height int) string {
	style := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Border(lipgloss.NormalBorder()).
		BorderForeground(a.getBorderColor(FocusNavigation))

	a.navigation.SetDimensions(width-2, height-2)
	content := a.navigation.View()

	return style.Render(content)
}

// renderCenterPanel renders the main story view
func (a *App) renderCenterPanel(width, height int) string {
	style := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Border(lipgloss.NormalBorder()).
		BorderForeground(a.getBorderColor(FocusStory)).
		Padding(1)

	story := a.getCurrentStory()
	if story == nil {
		return style.Render("No story selected")
	}

	// Update story dimensions
	story.SetDimensions(width-4, height-4)
	content := story.View()

	return style.Render(content)
}

// renderRightPanel renders the property panel and code viewer
func (a *App) renderRightPanel(width, height int) string {
	if !a.showPropertyPanel && !a.showCodeViewer {
		return lipgloss.NewStyle().Width(width).Height(height).Render("")
	}

	var panels []string
	panelHeight := height

	if a.showPropertyPanel && a.showCodeViewer {
		panelHeight = height / 2
	}

	if a.showPropertyPanel {
		propStyle := lipgloss.NewStyle().
			Width(width).
			Height(panelHeight).
			Border(lipgloss.NormalBorder()).
			BorderForeground(a.getBorderColor(FocusPropertyPanel))

		a.propertyPanel.SetDimensions(width-2, panelHeight-2)
		panels = append(panels, propStyle.Render(a.propertyPanel.View()))
	}

	if a.showCodeViewer {
		codeStyle := lipgloss.NewStyle().
			Width(width).
			Height(panelHeight).
			Border(lipgloss.NormalBorder()).
			BorderForeground(a.getBorderColor(FocusCodeViewer))

		a.codeViewer.SetDimensions(width-2, panelHeight-2)
		a.codeViewer.SetCode(a.getCurrentStoryCode())
		panels = append(panels, codeStyle.Render(a.codeViewer.View()))
	}

	return lipgloss.JoinVertical(lipgloss.Left, panels...)
}

// renderFooter renders the application footer with help text
func (a *App) renderFooter() string {
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Background(lipgloss.Color("236")).
		Padding(0, 1).
		Width(a.width)

	var helpText []string
	
	helpText = append(helpText, 
		"q: quit",
		"tab: cycle focus",
		"1-4: view modes",
		"p: toggle properties",
		"c: toggle code",
		"←/→: navigate",
	)

	return helpStyle.Render(strings.Join(helpText, " • "))
}

// Helper methods

func (a *App) getBorderColor(focus FocusState) lipgloss.Color {
	if a.focused == focus {
		return lipgloss.Color("69") // Active blue
	}
	return lipgloss.Color("240") // Inactive gray
}

func (a *App) cycleFocus() {
	switch a.focused {
	case FocusStory:
		a.focused = FocusNavigation
	case FocusNavigation:
		if a.showPropertyPanel {
			a.focused = FocusPropertyPanel
		} else if a.showCodeViewer {
			a.focused = FocusCodeViewer
		} else {
			a.focused = FocusStory
		}
	case FocusPropertyPanel:
		if a.showCodeViewer {
			a.focused = FocusCodeViewer
		} else {
			a.focused = FocusStory
		}
	case FocusCodeViewer:
		a.focused = FocusStory
	}
}

func (a *App) cycleFocusReverse() {
	switch a.focused {
	case FocusStory:
		if a.showCodeViewer {
			a.focused = FocusCodeViewer
		} else if a.showPropertyPanel {
			a.focused = FocusPropertyPanel
		} else {
			a.focused = FocusNavigation
		}
	case FocusNavigation:
		a.focused = FocusStory
	case FocusPropertyPanel:
		a.focused = FocusNavigation
	case FocusCodeViewer:
		if a.showPropertyPanel {
			a.focused = FocusPropertyPanel
		} else {
			a.focused = FocusNavigation
		}
	}
}

func (a *App) getCurrentCategory() *Category {
	if a.currentCategory >= 0 && a.currentCategory < len(a.registry.categories) {
		return a.registry.categories[a.currentCategory]
	}
	return nil
}

func (a *App) getCurrentStory() *Story {
	category := a.getCurrentCategory()
	if category != nil && a.currentStory >= 0 && a.currentStory < len(category.Stories) {
		return category.Stories[a.currentStory]
	}
	return nil
}

func (a *App) getCurrentStoryCode() string {
	story := a.getCurrentStory()
	if story != nil {
		return story.GetCode()
	}
	return ""
}

func (a *App) updateCurrentStory(story *Story) {
	category := a.getCurrentCategory()
	if category != nil && a.currentStory >= 0 && a.currentStory < len(category.Stories) {
		category.Stories[a.currentStory] = story
	}
}

func (a *App) previousCategory() {
	if a.currentCategory > 0 {
		a.currentCategory--
		a.currentStory = 0
		a.updatePropertyPanel()
	}
}

func (a *App) nextCategory() {
	if a.currentCategory < len(a.registry.categories)-1 {
		a.currentCategory++
		a.currentStory = 0
		a.updatePropertyPanel()
	}
}

func (a *App) previousStory() {
	category := a.getCurrentCategory()
	if category != nil && a.currentStory > 0 {
		a.currentStory--
		a.updatePropertyPanel()
	}
}

func (a *App) nextStory() {
	category := a.getCurrentCategory()
	if category != nil && a.currentStory < len(category.Stories)-1 {
		a.currentStory++
		a.updatePropertyPanel()
	}
}

func (a *App) updatePropertyPanel() {
	story := a.getCurrentStory()
	if story != nil {
		a.propertyPanel.SetStory(story)
	}
}

func (a *App) updateLayout() {
	// Recalculate layout when window size changes
	a.navigation.SetDimensions(a.width/4-2, a.height-6)
	a.propertyPanel.SetDimensions(a.width/4-2, a.height/2-3)
	a.codeViewer.SetDimensions(a.width/4-2, a.height/2-3)
}