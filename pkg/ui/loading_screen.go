package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
)

// LoadingScreen represents a professional loading screen component
type LoadingScreen struct {
	Title       string
	Message     string
	SubMessage  string
	ShowSpinner bool
	Width       int
	Height      int
}

// RenderLoadingScreen renders a professional loading screen with optional spinner
func RenderLoadingScreen(ls LoadingScreen, s spinner.Model) string {
	// Ensure minimum dimensions
	width := ls.Width
	height := ls.Height
	if width < 80 {
		width = 80
	}
	if height < 20 {
		height = 20
	}

	// Header section
	headerStyle := lipgloss.NewStyle().
		Foreground(ColorWhite).
		Bold(true).
		Padding(0, 1)
	
	header := headerStyle.Render(strings.ToUpper(ls.Title))
	
	// Separator
	separator := strings.Repeat("─", width)
	sepStyle := lipgloss.NewStyle().Foreground(ColorBorder)
	
	// Calculate content area height
	headerHeight := 3   // Title + separator + spacing
	footerHeight := 3   // Help + status
	contentHeight := height - headerHeight - footerHeight
	if contentHeight < 5 {
		contentHeight = 5
	}
	
	// Content area
	contentStyle := lipgloss.NewStyle().
		Width(width).
		Height(contentHeight).
		Align(lipgloss.Center, lipgloss.Center)
	
	// Create loading box
	loadingBoxStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(2, 4).
		Width(60)
	
	// Message styles
	messageStyle := lipgloss.NewStyle().
		Foreground(ColorWhite).
		Bold(true)
	
	subMessageStyle := lipgloss.NewStyle().
		Foreground(ColorGray)
	
	// Build loading content
	var loadingLines []string
	
	// Main message with optional spinner
	if ls.ShowSpinner && ls.Message != "" {
		mainLine := lipgloss.JoinHorizontal(
			lipgloss.Top,
			s.View(),
			" ",
			messageStyle.Render(ls.Message),
		)
		loadingLines = append(loadingLines, mainLine)
	} else if ls.Message != "" {
		loadingLines = append(loadingLines, messageStyle.Render(ls.Message))
	}
	
	// Add spacing if we have both message and submessage
	if ls.Message != "" && ls.SubMessage != "" {
		loadingLines = append(loadingLines, "")
	}
	
	// Sub message
	if ls.SubMessage != "" {
		loadingLines = append(loadingLines, subMessageStyle.Render(ls.SubMessage))
	}
	
	// Join all lines
	loadingContent := lipgloss.JoinVertical(
		lipgloss.Center,
		loadingLines...,
	)
	
	// Apply box styling
	loadingBox := loadingBoxStyle.Render(loadingContent)
	
	// Center the loading box
	centeredContent := contentStyle.Render(loadingBox)
	
	// Footer with status and navigation - consistent layout
	// Status text
	statusText := "◐ Loading..."
	statusStyle := lipgloss.NewStyle().
		Foreground(ColorYellow)
	
	// Navigation help on the right
	navStyle := lipgloss.NewStyle().
		Foreground(ColorGray)
	
	navText := "Please wait..."
	
	// Calculate padding for alignment
	statusWidth := lipgloss.Width(statusText)
	navWidth := lipgloss.Width(navText)
	paddingWidth := width - statusWidth - navWidth - 4 // 4 for margins
	
	if paddingWidth < 0 {
		paddingWidth = 1
	}
	
	// Create the footer line with proper spacing
	footerLine := lipgloss.JoinHorizontal(
		lipgloss.Top,
		" ", // Left margin
		statusStyle.Render(statusText),
		strings.Repeat(" ", paddingWidth), // Dynamic spacing
		navStyle.Render(navText),
		" ", // Right margin
	)
	
	// Add separator above footer
	footerSeparator := strings.Repeat("─", width)
	footerSepStyle := lipgloss.NewStyle().Foreground(ColorBorder)
	
	footer := lipgloss.JoinVertical(
		lipgloss.Left,
		footerSepStyle.Render(footerSeparator),
		footerLine,
	)
	
	// Combine all components
	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		sepStyle.Render(separator),
		centeredContent,
		footer,
	)
}

// RenderSimpleLoadingWithSpinner renders a simple centered loading message with spinner
func RenderSimpleLoadingWithSpinner(message string, s spinner.Model, width, height int) string {
	// Center the content
	centerStyle := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Align(lipgloss.Center, lipgloss.Center)
	
	// Loading style
	loadingStyle := lipgloss.NewStyle().
		Foreground(ColorGray)
	
	// Combine spinner and message
	content := lipgloss.JoinHorizontal(
		lipgloss.Top,
		s.View(),
		" ",
		loadingStyle.Render(message),
	)
	
	return centerStyle.Render(content)
}