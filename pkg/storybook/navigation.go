package storybook

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Navigation handles the category and story navigation
type Navigation struct {
	registry         *Registry
	selectedCategory int
	selectedStory    int
	width            int
	height           int
	scrollOffset     int
	searchQuery      string
	searchMode       bool
	searchResults    []*Story
}

// NewNavigation creates a new navigation component
func NewNavigation(registry *Registry) *Navigation {
	return &Navigation{
		registry:         registry,
		selectedCategory: 0,
		selectedStory:    0,
		searchResults:    make([]*Story, 0),
	}
}

// SetDimensions sets the navigation panel dimensions
func (n *Navigation) SetDimensions(width, height int) {
	n.width = width
	n.height = height
}

// Update handles navigation updates
func (n *Navigation) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if n.searchMode {
			return n.handleSearchInput(msg)
		}
		
		switch msg.String() {
		case "/":
			n.searchMode = true
			n.searchQuery = ""
			n.searchResults = make([]*Story, 0)
			
		case "k", "up":
			n.moveUp()
			
		case "j", "down":
			n.moveDown()
			
		case "h", "left":
			n.collapseCategory()
			
		case "l", "right", "enter":
			n.expandCategory()
			
		case "gg":
			n.goToTop()
			
		case "G":
			n.goToBottom()
		}
	}
	
	return n, nil
}

// handleSearchInput handles search input
func (n *Navigation) handleSearchInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		n.executeSearch()
		n.searchMode = false
		
	case "esc":
		n.searchMode = false
		n.searchQuery = ""
		n.searchResults = make([]*Story, 0)
		
	case "backspace":
		if len(n.searchQuery) > 0 {
			n.searchQuery = n.searchQuery[:len(n.searchQuery)-1]
		}
		
	default:
		if len(msg.String()) == 1 {
			n.searchQuery += msg.String()
		}
	}
	
	return n, nil
}

// View renders the navigation panel
func (n *Navigation) View() string {
	if n.width == 0 || n.height == 0 {
		return "Loading navigation..."
	}
	
	content := ""
	
	// Search bar
	searchBar := n.renderSearchBar()
	content += searchBar + "\n"
	
	availableHeight := n.height - 3 // Account for search bar and padding
	
	if len(n.searchResults) > 0 {
		// Show search results
		content += n.renderSearchResults(availableHeight)
	} else {
		// Show category navigation
		content += n.renderCategoryNavigation(availableHeight)
	}
	
	return content
}

// renderSearchBar renders the search input
func (n *Navigation) renderSearchBar() string {
	searchStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Width(n.width - 4)
	
	if n.searchMode {
		searchStyle = searchStyle.BorderForeground(lipgloss.Color("69"))
	}
	
	var searchText string
	if n.searchMode {
		searchText = "🔍 " + n.searchQuery + "█"
	} else {
		searchText = "🔍 Press '/' to search"
	}
	
	return searchStyle.Render(searchText)
}

// renderSearchResults renders search results
func (n *Navigation) renderSearchResults(height int) string {
	if len(n.searchResults) == 0 {
		noResultsStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Align(lipgloss.Center).
			Width(n.width)
		return noResultsStyle.Render("No results found")
	}
	
	resultStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("250"))
	
	selectedStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("69")).
		Foreground(lipgloss.Color("230")).
		Bold(true)
	
	var lines []string
	
	for i, story := range n.searchResults {
		line := fmt.Sprintf("  %s", story.Name)
		if i == n.selectedStory {
			lines = append(lines, selectedStyle.Render(line))
		} else {
			lines = append(lines, resultStyle.Render(line))
		}
	}
	
	// Handle scrolling
	visibleLines := n.getVisibleLines(lines, height-1)
	
	return strings.Join(visibleLines, "\n")
}

// renderCategoryNavigation renders the category tree
func (n *Navigation) renderCategoryNavigation(height int) string {
	var lines []string
	currentLine := 0
	
	categories := n.registry.GetCategories()
	
	for catIndex, category := range categories {
		// Category line
		categoryLine := n.renderCategoryLine(category, catIndex == n.selectedCategory, currentLine == n.getSelectedLine())
		lines = append(lines, categoryLine)
		currentLine++
		
		// Story lines (if category is selected)
		if catIndex == n.selectedCategory {
			for storyIndex, story := range category.Stories {
				storyLine := n.renderStoryLine(story, storyIndex == n.selectedStory, currentLine == n.getSelectedLine())
				lines = append(lines, storyLine)
				currentLine++
			}
		}
	}
	
	// Handle scrolling
	visibleLines := n.getVisibleLines(lines, height-1)
	
	return strings.Join(visibleLines, "\n")
}

// renderCategoryLine renders a single category line
func (n *Navigation) renderCategoryLine(category *Category, isSelected, isFocused bool) string {
	var style lipgloss.Style
	
	if isFocused {
		style = lipgloss.NewStyle().
			Background(lipgloss.Color("69")).
			Foreground(lipgloss.Color("230")).
			Bold(true)
	} else {
		style = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")).
			Bold(true)
	}
	
	icon := category.Icon
	if icon == "" {
		icon = "📁"
	}
	
	expandIcon := "▶"
	if isSelected {
		expandIcon = "▼"
	}
	
	line := fmt.Sprintf("%s %s %s (%d)", expandIcon, icon, category.Name, len(category.Stories))
	return style.Render(line)
}

// renderStoryLine renders a single story line
func (n *Navigation) renderStoryLine(story *Story, isSelected, isFocused bool) string {
	var style lipgloss.Style
	
	if isFocused {
		style = lipgloss.NewStyle().
			Background(lipgloss.Color("69")).
			Foreground(lipgloss.Color("230"))
	} else if isSelected {
		style = lipgloss.NewStyle().
			Foreground(lipgloss.Color("69"))
	} else {
		style = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))
	}
	
	line := fmt.Sprintf("    • %s", story.Name)
	return style.Render(line)
}

// getVisibleLines returns the visible lines based on scroll offset
func (n *Navigation) getVisibleLines(lines []string, maxLines int) []string {
	if len(lines) <= maxLines {
		return lines
	}
	
	start := n.scrollOffset
	end := start + maxLines
	
	if end > len(lines) {
		end = len(lines)
		start = end - maxLines
		if start < 0 {
			start = 0
		}
	}
	
	return lines[start:end]
}

// getSelectedLine calculates the currently selected line number
func (n *Navigation) getSelectedLine() int {
	line := n.selectedCategory
	
	// Add story offset if in a category
	if n.selectedCategory < len(n.registry.GetCategories()) {
		line += n.selectedStory + 1 // +1 for the category line itself
	}
	
	return line
}

// Movement methods

func (n *Navigation) moveUp() {
	if len(n.searchResults) > 0 {
		// Navigate search results
		if n.selectedStory > 0 {
			n.selectedStory--
		}
		return
	}
	
	categories := n.registry.GetCategories()
	if len(categories) == 0 {
		return
	}
	
	if n.selectedStory > 0 {
		// Move up within stories
		n.selectedStory--
	} else if n.selectedCategory > 0 {
		// Move to previous category
		n.selectedCategory--
		// Go to last story of previous category
		prevCategory := categories[n.selectedCategory]
		if len(prevCategory.Stories) > 0 {
			n.selectedStory = len(prevCategory.Stories) - 1
		}
	}
	
	n.ensureVisible()
}

func (n *Navigation) moveDown() {
	if len(n.searchResults) > 0 {
		// Navigate search results
		if n.selectedStory < len(n.searchResults)-1 {
			n.selectedStory++
		}
		return
	}
	
	categories := n.registry.GetCategories()
	if len(categories) == 0 {
		return
	}
	
	currentCategory := categories[n.selectedCategory]
	
	if n.selectedStory < len(currentCategory.Stories)-1 {
		// Move down within stories
		n.selectedStory++
	} else if n.selectedCategory < len(categories)-1 {
		// Move to next category
		n.selectedCategory++
		n.selectedStory = 0
	}
	
	n.ensureVisible()
}

func (n *Navigation) collapseCategory() {
	n.selectedStory = 0
}

func (n *Navigation) expandCategory() {
	// Already expanded by selection logic
}

func (n *Navigation) goToTop() {
	n.selectedCategory = 0
	n.selectedStory = 0
	n.scrollOffset = 0
}

func (n *Navigation) goToBottom() {
	categories := n.registry.GetCategories()
	if len(categories) > 0 {
		n.selectedCategory = len(categories) - 1
		lastCategory := categories[n.selectedCategory]
		if len(lastCategory.Stories) > 0 {
			n.selectedStory = len(lastCategory.Stories) - 1
		}
	}
	n.ensureVisible()
}

func (n *Navigation) ensureVisible() {
	selectedLine := n.getSelectedLine()
	maxVisibleLines := n.height - 4 // Account for UI elements
	
	if selectedLine < n.scrollOffset {
		n.scrollOffset = selectedLine
	} else if selectedLine >= n.scrollOffset+maxVisibleLines {
		n.scrollOffset = selectedLine - maxVisibleLines + 1
	}
}

func (n *Navigation) executeSearch() {
	if n.searchQuery == "" {
		return
	}
	
	n.searchResults = n.registry.SearchStories(n.searchQuery)
	n.selectedStory = 0
}

// Getters for external access

func (n *Navigation) GetSelectedCategory() int {
	return n.selectedCategory
}

func (n *Navigation) GetSelectedStory() int {
	return n.selectedStory
}