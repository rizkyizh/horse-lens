package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorBorder      = lipgloss.Color("#444444")
	colorBorderFocus = lipgloss.Color("#00FF88")
	colorHeader      = lipgloss.Color("#00FF88")
	colorSubtle      = lipgloss.Color("#666666")
	colorActive      = lipgloss.Color("#00FF88")
	colorText        = lipgloss.Color("#DDDDDD")
	colorOverlay     = lipgloss.Color("#0d0d0d")

	styleHeader = lipgloss.NewStyle().
			Foreground(colorHeader).
			Bold(true).
			Padding(0, 1)

	styleSidebarBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorBorder)

	styleSidebarBorderFocus = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorBorderFocus)

	styleTerminalBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorBorder)

	styleTerminalBorderFocus = lipgloss.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(colorBorderFocus)

	styleTitle = lipgloss.NewStyle().
			Foreground(colorHeader).
			Bold(true).
			Padding(0, 1)

	styleVersion = lipgloss.NewStyle().
			Foreground(colorSubtle).
			Padding(0, 1)

	styleStatusBar = lipgloss.NewStyle().
			Foreground(colorSubtle).
			Padding(0, 1)

	styleActiveItem = lipgloss.NewStyle().
			Foreground(colorActive).
			Bold(true)

	styleNormalItem = lipgloss.NewStyle().
			Foreground(colorText)

	styleDimItem = lipgloss.NewStyle().
			Foreground(colorSubtle)

	styleHelp = lipgloss.NewStyle().
			Foreground(colorSubtle).
			Padding(0, 1)

	styleModal = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(colorBorderFocus).
			Padding(1, 3).
			Width(68)

	styleSuggestionBox = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(colorBorder)

	styleSuggestionItem     = styleDimItem
	styleSuggestionSelected = styleActiveItem
)
