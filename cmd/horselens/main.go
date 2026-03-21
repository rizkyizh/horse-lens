package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rizkyizh/horse-lens/internal/tui"
	"github.com/rizkyizh/horse-lens/internal/workspace"
)

func main() {
	workspaces, err := workspace.AllProfiles()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading profiles: %v\n", err)
		os.Exit(1)
	}

	model := tui.NewAppModel(workspaces)

	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
