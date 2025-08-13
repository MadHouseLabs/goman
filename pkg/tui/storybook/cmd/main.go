// Package main provides the entry point for the TUI component storybook
package main

import (
	"fmt"
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/madhouselabs/goman/pkg/tui/storybook"
)

func main() {
	// Create the storybook model
	model := storybook.New()

	// Create the Bubble Tea program
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	// Run the program
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running storybook: %v\n", err)
		os.Exit(1)
	}

	log.Println("Storybook exited successfully")
}