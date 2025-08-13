package storybook

import (
	"sort"
	"strings"
)

// Registry manages all component stories organized by categories
type Registry struct {
	categories []*Category
	index      map[string]*Story
}

// Category represents a group of related component stories
type Category struct {
	Name        string
	Description string
	Stories     []*Story
	Icon        string
}

// NewRegistry creates a new story registry
func NewRegistry() *Registry {
	return &Registry{
		categories: make([]*Category, 0),
		index:      make(map[string]*Story),
	}
}

// AddCategory adds a new category to the registry
func (r *Registry) AddCategory(name, description, icon string) *Category {
	category := &Category{
		Name:        name,
		Description: description,
		Icon:        icon,
		Stories:     make([]*Story, 0),
	}
	
	r.categories = append(r.categories, category)
	return category
}

// GetCategory returns a category by name
func (r *Registry) GetCategory(name string) *Category {
	for _, category := range r.categories {
		if category.Name == name {
			return category
		}
	}
	return nil
}

// GetCategories returns all categories
func (r *Registry) GetCategories() []*Category {
	return r.categories
}

// AddStory adds a story to a category
func (r *Registry) AddStory(categoryName string, story *Story) error {
	category := r.GetCategory(categoryName)
	if category == nil {
		return ErrCategoryNotFound
	}
	
	category.Stories = append(category.Stories, story)
	r.index[story.ID] = story
	
	// Sort stories alphabetically within category
	sort.Slice(category.Stories, func(i, j int) bool {
		return strings.ToLower(category.Stories[i].Name) < strings.ToLower(category.Stories[j].Name)
	})
	
	return nil
}

// GetStory returns a story by ID
func (r *Registry) GetStory(id string) *Story {
	return r.index[id]
}

// GetAllStories returns all stories across all categories
func (r *Registry) GetAllStories() []*Story {
	var stories []*Story
	for _, story := range r.index {
		stories = append(stories, story)
	}
	return stories
}

// SearchStories searches for stories by name or description
func (r *Registry) SearchStories(query string) []*Story {
	query = strings.ToLower(query)
	var matches []*Story
	
	for _, story := range r.index {
		if strings.Contains(strings.ToLower(story.Name), query) ||
		   strings.Contains(strings.ToLower(story.Description), query) {
			matches = append(matches, story)
		}
	}
	
	return matches
}

// GetCategoryCount returns the number of categories
func (r *Registry) GetCategoryCount() int {
	return len(r.categories)
}

// GetStoryCount returns the total number of stories
func (r *Registry) GetStoryCount() int {
	return len(r.index)
}

// GetCategoryByIndex returns a category by index
func (r *Registry) GetCategoryByIndex(index int) *Category {
	if index >= 0 && index < len(r.categories) {
		return r.categories[index]
	}
	return nil
}

// GetStoryByIndex returns a story from a category by index
func (r *Registry) GetStoryByIndex(categoryIndex, storyIndex int) *Story {
	category := r.GetCategoryByIndex(categoryIndex)
	if category != nil && storyIndex >= 0 && storyIndex < len(category.Stories) {
		return category.Stories[storyIndex]
	}
	return nil
}

// registerAllStories registers all component stories with the registry
func registerAllStories(registry *Registry) {
	// Input Components Category
	inputCategory := registry.AddCategory("Input", "Components for user input", "📝")
	
	// Button stories
	registry.AddStory("Input", NewButtonStory())
	registry.AddStory("Input", NewPrimaryButtonStory())
	registry.AddStory("Input", NewSecondaryButtonStory())
	registry.AddStory("Input", NewDangerButtonStory())
	
	// Text Input stories
	registry.AddStory("Input", NewTextInputStory())
	registry.AddStory("Input", NewPasswordInputStory())
	registry.AddStory("Input", NewEmailInputStory())
	
	// Textarea stories
	registry.AddStory("Input", NewTextAreaStory())
	registry.AddStory("Input", NewCodeEditorStory())
	registry.AddStory("Input", NewMarkdownEditorStory())
	
	// Form stories
	registry.AddStory("Input", NewFormStory())
	registry.AddStory("Input", NewLoginFormStory())
	registry.AddStory("Input", NewRegistrationFormStory())
	
	// Display Components Category
	displayCategory := registry.AddCategory("Display", "Components for displaying data", "📊")
	
	// Table stories
	registry.AddStory("Display", NewTableStory())
	registry.AddStory("Display", NewStyledTableStory())
	
	// List stories
	registry.AddStory("Display", NewListStory())
	registry.AddStory("Display", NewStyledListStory())
	
	// Viewport stories
	registry.AddStory("Display", NewViewportStory())
	
	// Help stories
	registry.AddStory("Display", NewHelpStory())
	registry.AddStory("Display", NewNavigationHelpStory())
	registry.AddStory("Display", NewFormHelpStory())
	
	// Feedback Components Category
	feedbackCategory := registry.AddCategory("Feedback", "Components for user feedback", "💬")
	
	// Progress stories
	registry.AddStory("Feedback", NewProgressStory())
	registry.AddStory("Feedback", NewStyledProgressStory())
	
	// Spinner stories
	registry.AddStory("Feedback", NewSpinnerStory())
	registry.AddStory("Feedback", NewLoadingSpinnerStory())
	registry.AddStory("Feedback", NewProgressSpinnerStory())
	registry.AddStory("Feedback", NewPulseSpinnerStory())
	
	// Timer stories
	registry.AddStory("Feedback", NewTimerStory())
	registry.AddStory("Feedback", NewCountdownTimerStory())
	registry.AddStory("Feedback", NewStopwatchStory())
	registry.AddStory("Feedback", NewClockStory())
	
	// Layout Components Category
	layoutCategory := registry.AddCategory("Layout", "Components for organizing content", "📐")
	
	// Box stories
	registry.AddStory("Layout", NewBoxStory())
	registry.AddStory("Layout", NewBorderedBoxStory())
	
	// Flex stories
	registry.AddStory("Layout", NewFlexStory())
	registry.AddStory("Layout", NewVBoxStory())
	registry.AddStory("Layout", NewHBoxStory())
	
	// Grid stories
	registry.AddStory("Layout", NewGridStory())
	
	// Navigation Components Category
	navCategory := registry.AddCategory("Navigation", "Components for navigation", "🧭")
	
	// Paginator stories
	registry.AddStory("Navigation", NewPaginatorStory())
	registry.AddStory("Navigation", NewNumberedPaginatorStory())
	registry.AddStory("Navigation", NewDotPaginatorStory())
	
	// Context Components Category
	contextCategory := registry.AddCategory("Context", "Components for state management", "🔄")
	
	// Context stories
	registry.AddStory("Context", NewContextProviderStory())
	registry.AddStory("Context", NewContextConsumerStory())
	registry.AddStory("Context", NewStateContextStory())
	
	// Sort categories alphabetically
	sort.Slice(registry.categories, func(i, j int) bool {
		return strings.ToLower(registry.categories[i].Name) < strings.ToLower(registry.categories[j].Name)
	})
}