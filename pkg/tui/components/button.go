package components

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ButtonComponent represents a clickable button
type ButtonComponent struct {
	*BaseComponent
	label    string
	focused  bool
	disabled bool
	onClick  func()
	style    ButtonStyle
}

// ButtonStyle contains button styling
type ButtonStyle struct {
	Normal   lipgloss.Style
	Focused  lipgloss.Style
	Disabled lipgloss.Style
}

// DefaultButtonStyle returns default button styling (gray outline)
func DefaultButtonStyle() ButtonStyle {
	baseStyle := lipgloss.NewStyle().
		Padding(0, 3).
		Width(20).
		Align(lipgloss.Center).
		Border(lipgloss.NormalBorder())
	
	return ButtonStyle{
		Normal: baseStyle.Copy().
			Foreground(lipgloss.Color("#6B7280")).
			BorderForeground(lipgloss.Color("#6B7280")),
		Focused: baseStyle.Copy().
			Foreground(lipgloss.Color("#4B5563")).
			BorderForeground(lipgloss.Color("#4B5563")).
			Bold(true).
			Background(lipgloss.Color("#F3F4F6")),
		Disabled: baseStyle.Copy().
			Foreground(lipgloss.Color("#9CA3AF")).
			BorderForeground(lipgloss.Color("#9CA3AF")),
	}
}

// NewButton creates a new button component
func NewButton(id string, label string) *ButtonComponent {
	return &ButtonComponent{
		BaseComponent: NewBaseComponent(id),
		label:         label,
		style:         DefaultButtonStyle(),
	}
}

// Init initializes the button
func (b *ButtonComponent) Init() tea.Cmd {
	return nil
}

// Update handles button messages
func (b *ButtonComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle key events when focused
	if b.focused && !b.disabled {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "enter", " ":
				if b.onClick != nil {
					b.onClick()
				}
				// Store clicked state
				b.state["clicked"] = true
				return b, nil
			}
		}
	}
	
	return b, nil
}

// View renders the button
func (b *ButtonComponent) View() string {
	// Select appropriate style based on state
	var style lipgloss.Style
	if b.disabled {
		style = b.style.Disabled
	} else if b.focused {
		style = b.style.Focused
	} else {
		style = b.style.Normal
	}
	
	return style.Render(b.label)
}

// SetLabel sets the button label
func (b *ButtonComponent) SetLabel(label string) {
	b.label = label
}

// SetOnClick sets the click handler
func (b *ButtonComponent) SetOnClick(handler func()) {
	b.onClick = handler
}

// Focus focuses the button
func (b *ButtonComponent) Focus() {
	b.focused = true
}

// Blur unfocuses the button
func (b *ButtonComponent) Blur() {
	b.focused = false
}

// Enable enables the button
func (b *ButtonComponent) Enable() {
	b.disabled = false
}

// Disable disables the button
func (b *ButtonComponent) Disable() {
	b.disabled = true
}

// IsFocused returns whether the button is focused
func (b *ButtonComponent) IsFocused() bool {
	return b.focused
}

// IsDisabled returns whether the button is disabled
func (b *ButtonComponent) IsDisabled() bool {
	return b.disabled
}

// Button style presets

// PrimaryButtonStyle returns primary button styling (blue)
func PrimaryButtonStyle() ButtonStyle {
	baseStyle := lipgloss.NewStyle().
		Padding(0, 3).
		Width(20).
		Align(lipgloss.Center).
		Border(lipgloss.RoundedBorder())
	
	return ButtonStyle{
		Normal: baseStyle.Copy().
			Foreground(lipgloss.Color("#3B82F6")).
			BorderForeground(lipgloss.Color("#3B82F6")).
			Bold(true),
		Focused: baseStyle.Copy().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#2563EB")).
			BorderForeground(lipgloss.Color("#2563EB")).
			Bold(true),
		Disabled: baseStyle.Copy().
			Foreground(lipgloss.Color("#9CA3AF")).
			BorderForeground(lipgloss.Color("#9CA3AF")),
	}
}

// SecondaryButtonStyle returns secondary button styling (gray outline)
func SecondaryButtonStyle() ButtonStyle {
	baseStyle := lipgloss.NewStyle().
		Padding(0, 3).
		Width(20).
		Align(lipgloss.Center).
		Border(lipgloss.RoundedBorder())
	
	return ButtonStyle{
		Normal: baseStyle.Copy().
			Foreground(lipgloss.Color("#6B7280")).
			BorderForeground(lipgloss.Color("#9CA3AF")),
		Focused: baseStyle.Copy().
			Foreground(lipgloss.Color("#374151")).
			BorderForeground(lipgloss.Color("#6B7280")).
			Bold(true),
		Disabled: baseStyle.Copy().
			Foreground(lipgloss.Color("#D1D5DB")).
			BorderForeground(lipgloss.Color("#E5E7EB")),
	}
}

// DangerButtonStyle returns danger button styling (red outline)
func DangerButtonStyle() ButtonStyle {
	baseStyle := lipgloss.NewStyle().
		Padding(0, 3).
		Width(20).
		Align(lipgloss.Center).
		Border(lipgloss.NormalBorder())
	
	return ButtonStyle{
		Normal: baseStyle.Copy().
			Foreground(lipgloss.Color("#EF4444")).
			BorderForeground(lipgloss.Color("#EF4444")),
		Focused: baseStyle.Copy().
			Foreground(lipgloss.Color("#DC2626")).
			BorderForeground(lipgloss.Color("#DC2626")).
			Bold(true).
			Background(lipgloss.Color("#FEF2F2")),
		Disabled: baseStyle.Copy().
			Foreground(lipgloss.Color("#9CA3AF")).
			BorderForeground(lipgloss.Color("#9CA3AF")),
	}
}

// WarningButtonStyle returns warning button styling (yellow/amber outline)
func WarningButtonStyle() ButtonStyle {
	baseStyle := lipgloss.NewStyle().
		Padding(0, 3).
		Width(20).
		Align(lipgloss.Center).
		Border(lipgloss.NormalBorder())
	
	return ButtonStyle{
		Normal: baseStyle.Copy().
			Foreground(lipgloss.Color("#F59E0B")).
			BorderForeground(lipgloss.Color("#F59E0B")),
		Focused: baseStyle.Copy().
			Foreground(lipgloss.Color("#D97706")).
			BorderForeground(lipgloss.Color("#D97706")).
			Bold(true).
			Background(lipgloss.Color("#FFFBEB")),
		Disabled: baseStyle.Copy().
			Foreground(lipgloss.Color("#9CA3AF")).
			BorderForeground(lipgloss.Color("#9CA3AF")),
	}
}

// NewPrimaryButton creates a primary styled button
func NewPrimaryButton(id string, label string) *ButtonComponent {
	b := NewButton(id, label)
	b.style = PrimaryButtonStyle()
	return b
}

// NewSecondaryButton creates a secondary styled button
func NewSecondaryButton(id string, label string) *ButtonComponent {
	b := NewButton(id, label)
	b.style = SecondaryButtonStyle()
	return b
}

// NewDangerButton creates a danger styled button
func NewDangerButton(id string, label string) *ButtonComponent {
	b := NewButton(id, label)
	b.style = DangerButtonStyle()
	return b
}

// NewWarningButton creates a warning styled button
func NewWarningButton(id string, label string) *ButtonComponent {
	b := NewButton(id, label)
	b.style = WarningButtonStyle()
	return b
}