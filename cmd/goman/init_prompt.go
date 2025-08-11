package main

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/madhouselabs/goman/pkg/setup"
)

// initPromptModel represents the initialization prompt screen
type initPromptModel struct {
	width        int
	height       int
	initializing bool
	initialized  bool
	spinner      spinner.Model
	result       *setup.InitializeResult
	err          error
	startTime    time.Time
	elapsedTime  time.Duration
}

// keyMap for initialization prompt
type initKeyMap struct {
	Init key.Binding
	Quit key.Binding
}

var initKeys = initKeyMap{
	Init: key.NewBinding(
		key.WithKeys("i", "enter"),
		key.WithHelp("i/enter", "initialize"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
}

func newInitPromptModel() initPromptModel {
	s := spinner.New()
	s.Spinner = spinner.Points
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ff00"))

	return initPromptModel{
		spinner: s,
	}
}

func (m initPromptModel) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		m.spinner.Tick,
	)
}

type initStartMsg struct{}
type initProgressMsg struct {
	message string
}
type initPromptCompleteMsg struct {
	result *setup.InitializeResult
	err    error
}
type tickMsg time.Time

func (m initPromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if m.initializing {
			// During initialization, only allow quit
			if key.Matches(msg, initKeys.Quit) {
				return m, tea.Quit
			}
			return m, nil
		}

		switch {
		case key.Matches(msg, initKeys.Init):
			if !m.initialized && !m.initializing {
				m.initializing = true
				m.startTime = time.Now()
				return m, tea.Batch(
					m.runInitialization(),
					m.tickTimer(),
				)
			}
		case key.Matches(msg, initKeys.Quit):
			return m, tea.Quit
		}

	case tea.MouseMsg:
		if !m.initializing && !m.initialized {
			if msg.Type == tea.MouseRelease && msg.Button == tea.MouseButtonLeft {
				if zone.Get("init_button").InBounds(msg) {
					m.initializing = true
					m.startTime = time.Now()
					return m, tea.Batch(
						m.runInitialization(),
						m.tickTimer(),
					)
				}
			}
		}

	case initPromptCompleteMsg:
		m.initializing = false
		m.initialized = true
		m.result = msg.result
		m.err = msg.err

		if msg.err == nil && msg.result != nil {
			// Save initialization status
			saveInitStatus(msg.result)

			// Auto-transition to main TUI after 2 seconds
			return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg {
				return transitionToMainMsg{}
			})
		}
		return m, nil

	case tickMsg:
		if m.initializing {
			m.elapsedTime = time.Since(m.startTime)
			return m, m.tickTimer()
		}

	case transitionToMainMsg:
		// Quit this program and let the CLI start the main TUI
		return m, tea.Quit

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m initPromptModel) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	// Styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#00ff00")).
		MarginBottom(2)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#606060")).
		Padding(2, 4).
		Width(60).
		Align(lipgloss.Left)

	buttonStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#00ff00")).
		Foreground(lipgloss.Color("#000000")).
		Padding(0, 3).
		Bold(true)

	warningStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#ff9900")).
		Bold(true)

	successStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#00ff00"))

	var content string

	if m.initialized {
		// Show completion message
		title := titleStyle.Render("✓ Initialization Complete!")

		var details string
		if m.result != nil {
			details = "\n"
			if m.result.StorageReady {
				details += successStyle.Render("✓") + " Storage configured\n"
			}
			if m.result.FunctionReady {
				details += successStyle.Render("✓") + " Function deployed\n"
			}
			if m.result.LockServiceReady {
				details += successStyle.Render("✓") + " Lock service created\n"
			}
			if m.result.AuthReady {
				details += successStyle.Render("✓") + " Auth configured\n"
			}

			if len(m.result.Errors) > 0 {
				details += "\n" + warningStyle.Render("Some warnings occurred:\n")
				for _, err := range m.result.Errors {
					details += fmt.Sprintf("  • %s\n", err)
				}
			}

			details += "\n\nStarting main interface..."
		}

		// Left-align everything
		content = lipgloss.JoinVertical(
			lipgloss.Left,
			title,
			"\n"+details,
		)

	} else if m.initializing {
		// Show progress
		title := titleStyle.Render("Initializing Goman Infrastructure")

		progress := fmt.Sprintf("%s Setting up AWS resources...\n\n", m.spinner.View())
		progress += fmt.Sprintf("Time elapsed: %s\n\n", m.elapsedTime.Round(time.Second))
		progress += "This may take a few minutes:\n"
		progress += "  • Creating S3 bucket\n"
		progress += "  • Deploying Lambda function\n"
		progress += "  • Setting up DynamoDB table\n"
		progress += "  • Configuring IAM roles"

		// Left-align everything
		content = lipgloss.JoinVertical(
			lipgloss.Left,
			title,
			"\n"+progress,
		)

	} else {
		// Show initialization prompt
		title := titleStyle.Render("Welcome to Goman!")

		warning := warningStyle.Render("⚠ Infrastructure Not Initialized")

		description := `Goman needs to set up AWS infrastructure before you can manage clusters.

This will create:
  • S3 bucket for state storage
  • Lambda function for reconciliation
  • DynamoDB table for locking
  • IAM roles and policies

Press 'i' or click the button below to begin initialization.`

		button := zone.Mark("init_button", buttonStyle.Render("Initialize Infrastructure"))

		help := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#606060")).
			Render("\nPress 'q' to quit")

		// Left-align everything
		content = lipgloss.JoinVertical(
			lipgloss.Left,
			title,
			warning,
			"\n"+description,
			"\n"+lipgloss.PlaceHorizontal(52, lipgloss.Center, button),
			lipgloss.PlaceHorizontal(52, lipgloss.Center, help),
		)
	}

	// Center the box
	box := boxStyle.Render(content)

	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		box,
	)
}

func (m initPromptModel) runInitialization() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		result, err := setup.EnsureFullSetup(ctx)

		return initPromptCompleteMsg{
			result: result,
			err:    err,
		}
	}
}

func (m initPromptModel) tickTimer() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

type transitionToMainMsg struct{}
