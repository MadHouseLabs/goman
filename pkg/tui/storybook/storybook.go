// Package storybook provides an interactive component showcase for the TUI library
package storybook

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/madhouselabs/goman/pkg/tui/components"
	"github.com/madhouselabs/goman/pkg/tui/storybook/stories"
	"github.com/madhouselabs/goman/pkg/tui/storybook/types"
)

// Model represents the storybook application state
type Model struct {
	categories       []types.Category
	currentCategory  int
	currentStory     int
	currentComponent components.Component
	mode             types.InteractiveMode
	keys             types.KeyMap
	help             help.Model
	width            int
	height           int
	showCode         bool
	showProps        bool
	interactions     []string
	viewport         viewport.Model
}

// New creates a new storybook model
func New() Model {
	m := Model{
		categories:   stories.GetAllCategories(nil),
		keys:         types.DefaultKeyMap(),
		help:         help.New(),
		mode:         types.ModeNavigation,
		interactions: []string{},
	}

	// Initialize with a log function
	m.categories = stories.GetAllCategories(m.logInteraction)

	// Load first story
	if len(m.categories) > 0 && len(m.categories[0].Stories) > 0 {
		m.currentComponent = m.categories[0].Stories[0].Component()
	}

	return m
}

// Init initializes the storybook
func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	if m.currentComponent != nil {
		cmds = append(cmds, m.currentComponent.Init())
	}
	return tea.Batch(cmds...)
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Initialize viewport for interactions log
		m.viewport = viewport.New(m.width/3, 10)
		m.viewport.SetContent(m.getInteractionsContent())

	case tea.KeyMsg:
		switch m.mode {
		case types.ModeNavigation:
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit

			case "tab":
				m.nextStory()
				if m.currentComponent != nil {
					cmds = append(cmds, m.currentComponent.Init())
				}

			case "shift+tab":
				m.prevStory()
				if m.currentComponent != nil {
					cmds = append(cmds, m.currentComponent.Init())
				}

			case "i":
				m.mode = types.ModeInteractive
				m.logInteraction("Entered interactive mode")

			case "c":
				m.showCode = !m.showCode

			case "p":
				m.showProps = !m.showProps

			case "?":
				m.help.ShowAll = !m.help.ShowAll
			}

		case types.ModeInteractive:
			if msg.String() == "esc" {
				m.mode = types.ModeNavigation
				m.logInteraction("Exited interactive mode")
			} else if m.currentComponent != nil {
				// Pass the message to the current component
				model, cmd := m.currentComponent.Update(msg)
				if comp, ok := model.(components.Component); ok {
					m.currentComponent = comp
				}
				cmds = append(cmds, cmd)
			}
		}
	}

	// Update viewport for interactions
	m.viewport.SetContent(m.getInteractionsContent())
	viewportModel, viewportCmd := m.viewport.Update(msg)
	m.viewport = viewportModel
	cmds = append(cmds, viewportCmd)

	return m, tea.Batch(cmds...)
}

// View renders the storybook
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	// Styles
	headerStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("62")).
		Foreground(lipgloss.Color("230")).
		Padding(0, 2).
		Bold(true).
		Width(m.width)

	modeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("86")).
		Bold(true)

	categoryStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Italic(true)

	storyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("251"))

	componentAreaStyle := lipgloss.NewStyle().
		Padding(2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Width(m.width - 4).
		Height(m.height - 15)

	interactionsStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1).
		Width(m.width/3 - 2).
		Height(12)

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))

	// Build header
	modeText := "Navigation"
	if m.mode == types.ModeInteractive {
		modeText = "Interactive"
	}

	currentCategory := m.categories[m.currentCategory]
	currentStory := currentCategory.Stories[m.currentStory]

	header := headerStyle.Render(fmt.Sprintf(
		"TUI Component Storybook | Mode: %s | %s > %s",
		modeStyle.Render(modeText),
		categoryStyle.Render(currentCategory.Name),
		storyStyle.Render(currentStory.Name),
	))

	// Component area
	var componentView string
	if m.currentComponent != nil {
		componentView = m.currentComponent.View()
	}

	componentArea := componentAreaStyle.Render(componentView)

	// Build layout
	mainContent := componentArea

	// Add interactions log if in interactive mode
	if m.mode == types.ModeInteractive && len(m.interactions) > 0 {
		interactionsLog := interactionsStyle.Render(
			lipgloss.JoinVertical(
				lipgloss.Left,
				lipgloss.NewStyle().Bold(true).Render("Interactions:"),
				m.viewport.View(),
			),
		)

		mainContent = lipgloss.JoinHorizontal(
			lipgloss.Top,
			componentArea,
			"  ",
			interactionsLog,
		)
	}

	// Help
	helpText := "TAB: next • SHIFT+TAB: prev • i: interactive • c: code • p: props • ?: help • q: quit"
	if m.mode == types.ModeInteractive {
		helpText = "ESC to exit interactive mode • Component controls are active"
	}
	help := helpStyle.Render(helpText)

	// Combine everything
	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		"",
		mainContent,
		"",
		help,
	)
}

// Helper methods

func (m *Model) nextStory() {
	currentCategory := m.categories[m.currentCategory]
	m.currentStory++
	if m.currentStory >= len(currentCategory.Stories) {
		m.currentStory = 0
		m.currentCategory = (m.currentCategory + 1) % len(m.categories)
	}
	m.loadCurrentStory()
}

func (m *Model) prevStory() {
	m.currentStory--
	if m.currentStory < 0 {
		m.currentCategory--
		if m.currentCategory < 0 {
			m.currentCategory = len(m.categories) - 1
		}
		m.currentStory = len(m.categories[m.currentCategory].Stories) - 1
	}
	m.loadCurrentStory()
}

func (m *Model) loadCurrentStory() {
	currentCategory := m.categories[m.currentCategory]
	if m.currentStory < len(currentCategory.Stories) {
		story := currentCategory.Stories[m.currentStory]
		m.currentComponent = story.Component()
		m.interactions = []string{} // Clear interactions on story change
	}
}

func (m *Model) logInteraction(msg string) {
	m.interactions = append(m.interactions, fmt.Sprintf("• %s", msg))
	// Keep only last 20 interactions
	if len(m.interactions) > 20 {
		m.interactions = m.interactions[len(m.interactions)-20:]
	}
}

func (m Model) getInteractionsContent() string {
	return strings.Join(m.interactions, "\n")
}