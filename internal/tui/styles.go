package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorAccent = lipgloss.Color("#00FF88")
	colorSubtle = lipgloss.Color("#666666")
	colorText   = lipgloss.Color("#DDDDDD")

	styleTitle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true).
			Padding(0, 1)

	styleVersion = lipgloss.NewStyle().Foreground(colorSubtle)

	styleActiveItem = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	styleNormalItem = lipgloss.NewStyle().Foreground(colorText)
	styleDimItem    = lipgloss.NewStyle().Foreground(colorSubtle)

	styleHelp = lipgloss.NewStyle().Foreground(colorSubtle).Padding(0, 1)

	styleModal = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(1, 3)
)
