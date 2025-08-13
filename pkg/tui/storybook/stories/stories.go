// Package stories contains all component story definitions for the storybook
package stories

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/madhouselabs/goman/pkg/tui/components"
	"github.com/madhouselabs/goman/pkg/tui/storybook/types"
	"github.com/madhouselabs/goman/pkg/tui/storybook/wrappers"
)

// GetAllCategories returns all story categories
func GetAllCategories(logFunc func(string)) []types.Category {
	return []types.Category{
		GetButtonStories(logFunc),
		GetTextInputStories(logFunc),
		GetCheckboxStories(),
		GetRadioStories(),
		GetProgressStories(),
		GetSpinnerStories(),
		GetTimerStories(logFunc),
		GetPaginatorStories(logFunc),
		GetLayoutStories(),
		GetListStories(),
		GetFormStories(),
	}
}

// GetButtonStories returns button component stories
func GetButtonStories(logFunc func(string)) types.Category {
	return types.Category{
		Name: "Buttons",
		Stories: []types.Story{
			{
				Name:        "Button Variants",
				Description: "Different button styles",
				Component: func() components.Component {
					buttons := []*components.ButtonComponent{
						components.NewPrimaryButton("btn-primary", "Primary"),
						components.NewSecondaryButton("btn-secondary", "Secondary"),
						components.NewDangerButton("btn-danger", "Danger"),
						components.NewWarningButton("btn-warning", "Warning"),
					}
					return wrappers.NewButtonDemoWrapper(buttons, logFunc)
				},
			},
			{
				Name:        "Button States",
				Description: "Different button states",
				Component: func() components.Component {
					normalBtn := components.NewPrimaryButton("btn-normal", "Normal")
					focusedBtn := components.NewPrimaryButton("btn-focused", "Focused")
					focusedBtn.Focus()
					disabledBtn := components.NewPrimaryButton("btn-disabled", "Disabled")
					disabledBtn.Disable()

					buttons := []*components.ButtonComponent{normalBtn, focusedBtn, disabledBtn}
					return wrappers.NewButtonDemoWrapper(buttons, logFunc)
				},
			},
		},
	}
}

// GetTextInputStories returns text input component stories
func GetTextInputStories(logFunc func(string)) types.Category {
	return types.Category{
		Name: "Text Inputs",
		Stories: []types.Story{
			{
				Name:        "Basic Inputs",
				Description: "Various text input configurations",
				Component: func() components.Component {
					inputs := []*components.TextInputComponent{
						components.NewTextInput("input-basic"),
						components.NewTextInput("input-value"),
						components.NewPasswordInput("input-password"),
						components.NewEmailInput("input-email"),
					}
					
					// Set placeholders and values
					inputs[0].SetPlaceholder("Enter text...")
					inputs[1].SetValue("Hello World")
					inputs[2].SetPlaceholder("Enter password...")
					inputs[3].SetPlaceholder("email@example.com")

					names := []string{"Basic", "With Value", "Password", "Email"}
					return wrappers.NewInputDemoWrapper(inputs, names)
				},
			},
		},
	}
}

// GetCheckboxStories returns checkbox component stories
func GetCheckboxStories() types.Category {
	return types.Category{
		Name: "Checkboxes",
		Stories: []types.Story{
			{
				Name:        "Single Checkbox",
				Description: "Single checkbox component",
				Component: func() components.Component {
					return components.NewCheckbox("checkbox-single", "Accept terms and conditions")
				},
			},
			{
				Name:        "Checkbox Group",
				Description: "Multiple checkboxes in a group",
				Component: func() components.Component {
					group := components.NewCheckboxGroup("checkbox-group", "Select options")
					group.AddCheckbox("option1", "Option 1", false)
					group.AddCheckbox("option2", "Option 2", false)
					group.AddCheckbox("option3", "Option 3", false)
					return group
				},
			},
		},
	}
}

// GetRadioStories returns radio button component stories
func GetRadioStories() types.Category {
	return types.Category{
		Name: "Radio Buttons",
		Stories: []types.Story{
			{
				Name:        "Radio Group",
				Description: "Radio button group",
				Component: func() components.Component {
					group := components.NewRadioGroup("radio-group", "Select size")
					group.AddOption("small", "Small", "sm")
					group.AddOption("medium", "Medium", "md")
					group.AddOption("large", "Large", "lg")
					return group
				},
			},
		},
	}
}

// GetProgressStories returns progress component stories
func GetProgressStories() types.Category {
	return types.Category{
		Name: "Progress",
		Stories: []types.Story{
			{
				Name:        "Progress Bar",
				Description: "Progress bar at different percentages",
				Component: func() components.Component {
					progress := components.NewProgress("progress")
					progress.SetPercent(0.65)
					return progress
				},
			},
			{
				Name:        "Progress Styles",
				Description: "Different progress bar styles",
				Component: func() components.Component {
					container := components.NewFlex("progress-flex", "column")
					
					progress1 := components.NewProgress("progress-1")
					progress1.SetPercent(0.30)
					
					progress2 := components.NewProgress("progress-2")
					progress2.SetPercent(0.60)
					
					progress3 := components.NewProgress("progress-3")
					progress3.SetPercent(0.90)
					
					container.AddChild(progress1)
					container.AddChild(progress2)
					container.AddChild(progress3)
					return container
				},
			},
		},
	}
}

// GetSpinnerStories returns spinner component stories
func GetSpinnerStories() types.Category {
	return types.Category{
		Name: "Spinners",
		Stories: []types.Story{
			{
				Name:        "Spinner Styles",
				Description: "Different spinner styles",
				Component: func() components.Component {
					container := components.NewFlex("spinner-flex", "row")
					
					dotSpinner := components.NewSpinner("spinner-dot")
					lineSpinner := components.NewSpinner("spinner-line")
					globeSpinner := components.NewSpinner("spinner-globe")
					moonSpinner := components.NewSpinner("spinner-moon")
					
					// Set different spinner types
					dotSpinner.SetMessage("Dot")
					lineSpinner.SetMessage("Line")
					globeSpinner.SetMessage("Globe")
					moonSpinner.SetMessage("Moon")
					
					container.AddChild(dotSpinner)
					container.AddChild(lineSpinner)
					container.AddChild(globeSpinner)
					container.AddChild(moonSpinner)
					return container
				},
			},
		},
	}
}

// GetTimerStories returns timer component stories
func GetTimerStories(logFunc func(string)) types.Category {
	return types.Category{
		Name: "Timers",
		Stories: []types.Story{
			{
				Name:        "Countdown Timer",
				Description: "Interactive countdown timer",
				Component: func() components.Component {
					timer := components.NewCountdownTimer("timer", 60*time.Second)
					return wrappers.NewTimerWrapper(timer, logFunc)
				},
			},
		},
	}
}

// GetPaginatorStories returns paginator component stories
func GetPaginatorStories(logFunc func(string)) types.Category {
	// Generate sample items
	items := make([]string, 100)
	for i := 0; i < 100; i++ {
		items[i] = fmt.Sprintf("Item %d", i+1)
	}

	return types.Category{
		Name: "Pagination",
		Stories: []types.Story{
			{
				Name:        "Paginator",
				Description: "Interactive paginator",
				Component: func() components.Component {
					paginator := components.NewPaginator("paginator", 10)
					return wrappers.NewPaginatorWrapper(paginator, items, logFunc)
				},
			},
		},
	}
}

// GetLayoutStories returns layout component stories
func GetLayoutStories() types.Category {
	return types.Category{
		Name: "Layout",
		Stories: []types.Story{
			{
				Name:        "Stack Layout",
				Description: "Vertical stack of components",
				Component: func() components.Component {
					stack := components.NewFlex("stack", "column")
					
					header := wrappers.NewStaticTextWrapper(
						"Header Section",
						lipgloss.NewStyle().
							Bold(true).
							Foreground(lipgloss.Color("86")).
							Border(lipgloss.NormalBorder()).
							Padding(1),
					)
					
					content := wrappers.NewStaticTextWrapper(
						"Main Content Area",
						lipgloss.NewStyle().
							Foreground(lipgloss.Color("241")).
							Padding(2),
					)
					
					footer := wrappers.NewStaticTextWrapper(
						"Footer Section",
						lipgloss.NewStyle().
							Italic(true).
							Foreground(lipgloss.Color("245")).
							Border(lipgloss.NormalBorder()).
							Padding(1),
					)
					
					stack.AddChild(header)
					stack.AddChild(content)
					stack.AddChild(footer)
					return stack
				},
			},
			{
				Name:        "Flex Layout",
				Description: "Horizontal flex layout",
				Component: func() components.Component {
					flex := components.NewFlex("flex", "row")
					
					left := wrappers.NewStaticTextWrapper(
						"Left Panel",
						lipgloss.NewStyle().
							Border(lipgloss.NormalBorder()).
							Padding(2).
							Width(20),
					)
					
					center := wrappers.NewStaticTextWrapper(
						"Center Content",
						lipgloss.NewStyle().
							Border(lipgloss.NormalBorder()).
							Padding(2),
					)
					
					right := wrappers.NewStaticTextWrapper(
						"Right Panel",
						lipgloss.NewStyle().
							Border(lipgloss.NormalBorder()).
							Padding(2).
							Width(20),
					)
					
					flex.AddChild(left)
					flex.AddChild(center)
					flex.AddChild(right)
					return flex
				},
			},
		},
	}
}

// GetListStories returns list component stories
func GetListStories() types.Category {
	return types.Category{
		Name: "Lists",
		Stories: []types.Story{
			{
				Name:        "Basic List",
				Description: "Simple list component",
				Component: func() components.Component {
					list := components.NewList("list", 40, 10)
					items := []components.ListItem{
						{ItemTitle: "First Item", ItemDescription: "This is the first item"},
						{ItemTitle: "Second Item", ItemDescription: "This is the second item"},
						{ItemTitle: "Third Item", ItemDescription: "This is the third item"},
						{ItemTitle: "Fourth Item", ItemDescription: "This is the fourth item"},
						{ItemTitle: "Fifth Item", ItemDescription: "This is the fifth item"},
					}
					list.SetItems(items)
					return list
				},
			},
		},
	}
}

// GetFormStories returns form component stories
func GetFormStories() types.Category {
	return types.Category{
		Name: "Forms",
		Stories: []types.Story{
			{
				Name:        "Complete Form",
				Description: "Full-featured form with all input types",
				Component: func() components.Component {
					form := components.NewForm("form")
					form.SetTitle("User Registration")
					form.SetDescription("Please fill in all required fields")
					
					// Personal Information Section
					form.AddSection("Personal Information")
					form.AddInput("name", "Full Name", "Enter your full name", true)
					form.AddInput("email", "Email Address", "Enter your email", true)
					form.AddPasswordInput("password", "Password", "Enter password", true)
					
					// Preferences Section
					form.AddSection("Preferences")
					languages := []string{"English", "Spanish", "French", "German"}
					form.AddRadioGroup("language", "Language", languages, false)
					
					form.AddCheckbox("newsletter", "Subscribe to newsletter")
					form.AddCheckbox("notifications", "Enable notifications")
					
					// Additional Information Section
					form.AddSection("Additional Information")
					form.AddTextArea("bio", "Bio", "Tell us about yourself", false)
					
					// Terms
					form.AddCheckbox("terms", "I agree to the terms and conditions")
					
					return form
				},
			},
		},
	}
}