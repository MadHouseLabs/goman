# TUI Component Storybook Documentation

An interactive showcase and testing environment for TUI components built with Bubble Tea and Lipgloss.

## Overview

The TUI Storybook provides an interactive environment to explore, test, and demonstrate all TUI components in the library. It features two modes:
- **Navigation Mode**: Browse through different component stories
- **Interactive Mode**: Interact directly with components

## Project Structure

```
storybook/
├── cmd/
│   └── main.go              # Entry point for running the storybook
├── stories/
│   └── stories.go           # Component story definitions
├── types/
│   └── types.go            # Common types and interfaces
├── wrappers/
│   ├── button_wrapper.go   # Button demo wrapper with keyboard controls
│   ├── input_wrapper.go    # Input demo wrapper with focus management
│   ├── paginator_wrapper.go # Paginator wrapper with keyboard navigation
│   └── timer_wrapper.go    # Timer wrapper with keyboard controls
├── storybook.go            # Main storybook model and logic
└── docs/
    └── README.md           # This documentation file
```

## Running the Storybook

```bash
cd pkg/tui/storybook/cmd
go run main.go
```

## Keyboard Controls

### Navigation Mode
- `Tab` - Next story
- `Shift+Tab` - Previous story
- `i` - Enter interactive mode
- `c` - Toggle code view (future feature)
- `p` - Toggle props view (future feature)
- `?` - Toggle help
- `q` - Quit

### Interactive Mode
- `Esc` - Exit to navigation mode
- Component-specific controls are active

## Component Categories

### Buttons
- **Button Variants**: Primary, Secondary, Danger, Warning styles
- **Button States**: Normal, Focused, Disabled states
- Interactive controls: Tab/Arrow keys to navigate, Enter to click, 'd' to toggle disable

### Text Inputs
- **Basic Inputs**: Standard text, password, email inputs
- Interactive controls: Tab to navigate between inputs, type to enter text

### Checkboxes
- **Single Checkbox**: Individual checkbox component
- **Checkbox Group**: Multiple checkboxes in a group
- Interactive controls: Space to toggle, arrows to navigate

### Radio Buttons
- **Radio Group**: Mutually exclusive radio options
- Interactive controls: Arrows to select, Space/Enter to confirm

### Progress
- **Progress Bar**: Progress indicator at various percentages
- **Progress Styles**: Different color schemes (Blue, Green, Red)

### Spinners
- **Spinner Styles**: Dot, Line, Globe, Moon animations
- Animated spinners that update automatically

### Timers
- **Countdown Timer**: Interactive countdown with controls
- Interactive controls: 's' to start, 'p' to pause, 'r' to reset

### Pagination
- **Paginator**: Navigate through paginated content
- Interactive controls: Left/Right arrows or 'p'/'n' to navigate, 'f' for first, 'l' for last

### Layout
- **Stack Layout**: Vertical component stacking
- **Flex Layout**: Horizontal flexible layouts

### Lists
- **Basic List**: Simple list component with items

### Forms
- **Complete Form**: Full-featured form with all input types
  - Text fields, passwords, emails
  - Select dropdowns, radio groups
  - Checkboxes, text areas, number inputs
  - Scrollable with fixed height
  - Professional styling with sections

## Adding New Stories

To add a new component story:

1. Create a wrapper in `wrappers/` if the component needs special interaction handling:
```go
type MyComponentWrapper struct {
    component *components.MyComponent
    // wrapper-specific fields
}

func (w *MyComponentWrapper) Init() tea.Cmd { /* ... */ }
func (w *MyComponentWrapper) Update(msg tea.Msg) (tea.Model, tea.Cmd) { /* ... */ }
func (w *MyComponentWrapper) View() string { /* ... */ }
// Implement Component interface methods
```

2. Add the story definition in `stories/stories.go`:
```go
func GetMyComponentStories() types.Category {
    return types.Category{
        Name: "My Components",
        Stories: []types.Story{
            {
                Name:        "Basic Component",
                Description: "Description of the component",
                Component: func() components.Component {
                    return components.NewMyComponent("id")
                },
            },
        },
    }
}
```

3. Include the category in `GetAllCategories()`:
```go
func GetAllCategories(logFunc func(string)) []types.Category {
    return []types.Category{
        // ... existing categories
        GetMyComponentStories(),
    }
}
```

## Component Wrappers

Wrappers provide keyboard interaction for components that would normally use mouse or button clicks:

- **ButtonDemoWrapper**: Manages button navigation and clicking
- **InputDemoWrapper**: Handles focus between multiple inputs
- **TimerWrapper**: Provides keyboard controls for timer
- **PaginatorWrapper**: Enables keyboard navigation for pagination

## Architecture Notes

- All components implement the `components.Component` interface
- Wrappers are used when components need special interaction handling
- The storybook maintains two modes for browsing vs interacting
- Interaction logs are displayed in interactive mode
- Components are organized by category for easy navigation

## Best Practices

1. **Component Independence**: Each story should be self-contained
2. **Clear Naming**: Use descriptive names for stories and categories
3. **Interactive Examples**: Provide keyboard controls for all interactive components
4. **Documentation**: Include help text for component controls
5. **Visual Feedback**: Log interactions to show component state changes

## Future Enhancements

- [ ] Code view showing component initialization
- [ ] Props editor for real-time component configuration
- [ ] Export story configurations
- [ ] Search functionality for components
- [ ] Component performance metrics
- [ ] Accessibility testing tools