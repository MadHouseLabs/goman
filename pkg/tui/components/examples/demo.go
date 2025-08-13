package main

import (
	"fmt"
	"log"
	"time"
	
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/madhouselabs/goman/pkg/tui/components"
)

// DemoApp demonstrates the component library
type DemoApp struct {
	width    int
	height   int
	context  *components.Context
	provider *components.Provider
	layout   *components.FlexComponent
	pages    []Page
	current  int
}

// Page represents a demo page
type Page struct {
	Name      string
	Component components.Component
}

// NewDemoApp creates a new demo application
func NewDemoApp() *DemoApp {
	// Create global context
	ctx := components.NewContext()
	ctx.Set(components.ContextKeyTheme, components.DefaultTheme())
	ctx.Set(components.ContextKeyState, components.NewStateContext())
	
	// Create provider
	provider := components.NewProvider("root", ctx)
	
	// Create main layout
	layout := components.NewFlex("main", "column")
	
	app := &DemoApp{
		context:  ctx,
		provider: provider,
		layout:   layout,
		pages:    []Page{},
	}
	
	// Create demo pages
	app.createPages()
	
	// Set initial page
	app.setPage(0)
	
	return app
}

// createPages creates all demo pages
func (app *DemoApp) createPages() {
	// Button demo
	buttonPage := app.createButtonDemo()
	app.pages = append(app.pages, Page{Name: "Buttons", Component: buttonPage})
	
	// Input demo
	inputPage := app.createInputDemo()
	app.pages = append(app.pages, Page{Name: "Inputs", Component: inputPage})
	
	// Progress demo
	progressPage := app.createProgressDemo()
	app.pages = append(app.pages, Page{Name: "Progress", Component: progressPage})
	
	// Form demo
	formPage := app.createFormDemo()
	app.pages = append(app.pages, Page{Name: "Form", Component: formPage})
	
	// Layout demo
	layoutPage := app.createLayoutDemo()
	app.pages = append(app.pages, Page{Name: "Layout", Component: layoutPage})
}

// createButtonDemo creates button demonstration
func (app *DemoApp) createButtonDemo() components.Component {
	container := components.NewFlex("button-demo", "column")
	container.SetProps(components.Props{"gap": 2})
	
	// Title
	title := components.NewBox("button-title")
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("69"))
	title.SetProps(components.Props{
		"style": titleStyle,
	})
	// Note: In a real implementation, we'd need a Text component
	// For now, using Box with content
	
	// Primary button
	primaryBtn := components.NewPrimaryButton("primary", "Primary Button")
	primaryBtn.SetOnClick(func() {
		log.Println("Primary button clicked!")
	})
	
	// Secondary button
	secondaryBtn := components.NewSecondaryButton("secondary", "Secondary Button")
	secondaryBtn.SetOnClick(func() {
		log.Println("Secondary button clicked!")
	})
	
	// Danger button
	dangerBtn := components.NewDangerButton("danger", "Danger Button")
	dangerBtn.SetOnClick(func() {
		log.Println("Danger button clicked!")
	})
	
	// Disabled button
	disabledBtn := components.NewButton("disabled", "Disabled Button")
	disabledBtn.Disable()
	
	container.AddChild(primaryBtn)
	container.AddChild(secondaryBtn)
	container.AddChild(dangerBtn)
	container.AddChild(disabledBtn)
	
	return container
}

// createInputDemo creates input demonstration
func (app *DemoApp) createInputDemo() components.Component {
	container := components.NewFlex("input-demo", "column")
	container.SetProps(components.Props{"gap": 2})
	
	// Text input
	textInput := components.NewTextInput("text-input")
	textInput.SetLabel("Name")
	textInput.SetPlaceholder("Enter your name...")
	
	// Password input
	passwordInput := components.NewPasswordInput("password-input")
	passwordInput.SetLabel("Password")
	
	// Email input
	emailInput := components.NewEmailInput("email-input")
	emailInput.SetLabel("Email")
	
	// Text area
	textArea := components.NewTextArea("textarea")
	textArea.SetLabel("Comments")
	textArea.SetPlaceholder("Enter your comments here...")
	textArea.SetDimensions(40, 5)
	
	container.AddChild(textInput)
	container.AddChild(passwordInput)
	container.AddChild(emailInput)
	container.AddChild(textArea)
	
	return container
}

// createProgressDemo creates progress demonstration
func (app *DemoApp) createProgressDemo() components.Component {
	container := components.NewFlex("progress-demo", "column")
	container.SetProps(components.Props{"gap": 2})
	
	// Progress bar
	progress := components.NewProgress("progress")
	progress.SetLabel("Downloading...")
	progress.SetPercent(0.45)
	progress.SetWidth(40)
	
	// Styled progress
	styledProgress := components.NewStyledProgress("styled-progress", 40)
	styledProgress.SetLabel("Processing...")
	styledProgress.SetPercent(0.75)
	
	// Spinner
	spinner := components.NewLoadingSpinner("spinner", "Loading data...")
	
	// Timer
	timer := components.NewCountdownTimer("timer", 30*time.Second)
	
	container.AddChild(progress)
	container.AddChild(styledProgress)
	container.AddChild(spinner)
	container.AddChild(timer)
	
	return container
}

// createFormDemo creates form demonstration
func (app *DemoApp) createFormDemo() components.Component {
	form := components.NewLoginForm("login-form")
	
	form.SetOnSubmit(func(values map[string]interface{}) {
		log.Printf("Form submitted: %+v", values)
	})
	
	form.SetOnCancel(func() {
		log.Println("Form cancelled")
	})
	
	return form
}

// createLayoutDemo creates layout demonstration
func (app *DemoApp) createLayoutDemo() components.Component {
	container := components.NewFlex("layout-demo", "column")
	container.SetProps(components.Props{"gap": 2})
	
	// Horizontal box
	hbox := components.NewHBox("hbox")
	for i := 1; i <= 3; i++ {
		box := components.NewBorderedBox(
			fmt.Sprintf("box%d", i),
			nil, // Would need content component
		)
		box.SetProps(components.Props{
			"width":  20,
			"height": 5,
		})
		hbox.AddChild(box)
	}
	
	// Grid
	grid := components.NewGrid("grid", 2, 3)
	for row := 0; row < 2; row++ {
		for col := 0; col < 3; col++ {
			cell := components.NewBox(fmt.Sprintf("cell-%d-%d", row, col))
			cell.SetProps(components.Props{
				"style": lipgloss.NewStyle().
					Border(lipgloss.NormalBorder()).
					Width(15).
					Height(3).
					Align(lipgloss.Center, lipgloss.Center),
			})
			grid.SetCell(row, col, cell)
		}
	}
	
	container.AddChild(hbox)
	container.AddChild(grid)
	
	return container
}

// setPage sets the current page
func (app *DemoApp) setPage(index int) {
	if index >= 0 && index < len(app.pages) {
		app.current = index
		app.updateLayout()
	}
}

// updateLayout updates the main layout
func (app *DemoApp) updateLayout() {
	app.layout = components.NewFlex("main", "column")
	
	// Header
	header := app.createHeader()
	
	// Current page content
	content := app.pages[app.current].Component
	
	// Footer with navigation
	footer := app.createFooter()
	
	app.layout.AddChild(header)
	app.layout.AddChild(content)
	app.layout.AddChild(footer)
	
	app.provider.SetChild(app.layout)
}

// createHeader creates the header
func (app *DemoApp) createHeader() components.Component {
	header := components.NewBox("header")
	headerStyle := lipgloss.NewStyle().
		Width(app.width).
		Padding(1).
		Background(lipgloss.Color("69")).
		Foreground(lipgloss.Color("230")).
		Bold(true)
	
	header.SetProps(components.Props{
		"style": headerStyle,
	})
	
	// Would need a Text component for the title
	return header
}

// createFooter creates the footer with navigation
func (app *DemoApp) createFooter() components.Component {
	footer := components.NewBox("footer")
	
	// Navigation help
	help := components.NewDefaultHelp("help")
	help.SetWidth(app.width)
	
	// Pagination
	paginator := components.NewNumberedPaginator("paginator", len(app.pages))
	paginator.SetPage(app.current)
	
	footerContent := components.NewFlex("footer-content", "row")
	footerContent.AddChild(help)
	footerContent.AddChild(paginator)
	
	footer.SetContent(footerContent)
	
	return footer
}

// Init initializes the app
func (app *DemoApp) Init() tea.Cmd {
	return app.provider.Init()
}

// Update handles messages
func (app *DemoApp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		app.width = msg.Width
		app.height = msg.Height
		app.updateLayout()
		
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return app, tea.Quit
		case "left", "h":
			if app.current > 0 {
				app.setPage(app.current - 1)
			}
		case "right", "l":
			if app.current < len(app.pages)-1 {
				app.setPage(app.current + 1)
			}
		}
	}
	
	// Update provider (which updates all children)
	model, cmd := app.provider.Update(msg)
	if provider, ok := model.(*components.Provider); ok {
		app.provider = provider
	}
	
	return app, cmd
}

// View renders the app
func (app *DemoApp) View() string {
	return app.provider.View()
}

func main() {
	app := NewDemoApp()
	
	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}