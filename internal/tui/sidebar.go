package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rizkyizh/horse-lens/internal/workspace"
)

type SidebarModel struct {
	workspaces    []workspace.Workspace
	cursor        int
	activeIndex   int // which workspace is activated (-1 = none)
	focused       bool
	hidden        bool
	width, height int
}

func NewSidebar(workspaces []workspace.Workspace) SidebarModel {
	return SidebarModel{
		workspaces:  workspaces,
		cursor:      0,
		activeIndex: -1,
	}
}

func (s SidebarModel) Selected() *workspace.Workspace {
	if len(s.workspaces) == 0 {
		return nil
	}
	ws := s.workspaces[s.cursor]
	return &ws
}

func (s SidebarModel) Active() *workspace.Workspace {
	if s.activeIndex < 0 || s.activeIndex >= len(s.workspaces) {
		return nil
	}
	ws := s.workspaces[s.activeIndex]
	return &ws
}

func (s SidebarModel) Update(msg tea.Msg) (SidebarModel, tea.Cmd) {
	if !s.focused {
		return s, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if s.cursor > 0 {
				s.cursor--
			}
		case "down", "j":
			if s.cursor < len(s.workspaces)-1 {
				s.cursor++
			}
		}
	}
	return s, nil
}

func (s SidebarModel) View() string {
	inner := s.height - 2 // subtract border lines

	var sb strings.Builder

	title := styleTitle.Render("WORKSPACES")
	sb.WriteString(title + "\n")

	listHeight := inner - 4 // title + help lines
	if listHeight < 1 {
		listHeight = 1
	}

	for i, ws := range s.workspaces {
		if i >= listHeight {
			break
		}
		prefix := "  "
		if i == s.cursor && s.focused {
			prefix = "> "
		} else if i == s.cursor {
			prefix = "› "
		}
		name := ws.Name
		var line string
		if i == s.activeIndex {
			line = styleActiveItem.Render(fmt.Sprintf("%s%s ●", prefix, name))
		} else if i == s.cursor {
			line = styleNormalItem.Render(fmt.Sprintf("%s%s", prefix, name))
		} else {
			line = styleDimItem.Render(fmt.Sprintf("%s%s", prefix, name))
		}
		sb.WriteString(line + "\n")
	}

	// Fill remaining space
	linesUsed := 1 + min(len(s.workspaces), listHeight)
	for i := linesUsed; i < inner-2; i++ {
		sb.WriteString("\n")
	}

	help := styleHelp.Render("[n]ew [e]dit [d]el [↵]open")
	sb.WriteString(help)

	borderStyle := styleSidebarBorder
	if s.focused {
		borderStyle = styleSidebarBorderFocus
	}

	return borderStyle.
		Width(s.width - 2).
		Height(s.height - 2).
		Render(sb.String())
}

func (s *SidebarModel) SetSize(w, h int) {
	s.width = w
	s.height = h
}

func (s *SidebarModel) SetFocused(f bool) {
	s.focused = f
}

func (s *SidebarModel) UpdateWorkspace(ws workspace.Workspace, originalName string) {
	for i, w := range s.workspaces {
		if w.Name == originalName {
			s.workspaces[i] = ws
			return
		}
	}
}

func (s *SidebarModel) AddWorkspace(ws workspace.Workspace) {
	s.workspaces = append(s.workspaces, ws)
	s.cursor = len(s.workspaces) - 1
}

func (s *SidebarModel) DeleteCurrent() *workspace.Workspace {
	if len(s.workspaces) == 0 {
		return nil
	}
	ws := s.workspaces[s.cursor]
	s.workspaces = append(s.workspaces[:s.cursor], s.workspaces[s.cursor+1:]...)
	if s.cursor >= len(s.workspaces) && s.cursor > 0 {
		s.cursor--
	}
	if s.activeIndex == s.cursor {
		s.activeIndex = -1
	} else if s.activeIndex > s.cursor {
		s.activeIndex--
	}
	return &ws
}

func (s *SidebarModel) DeleteByName(name string) bool {
	for i, ws := range s.workspaces {
		if ws.Name == name {
			s.workspaces = append(s.workspaces[:i], s.workspaces[i+1:]...)
			if s.cursor >= len(s.workspaces) && s.cursor > 0 {
				s.cursor--
			}
			if s.activeIndex == i {
				s.activeIndex = -1
			} else if s.activeIndex > i {
				s.activeIndex--
			}
			return true
		}
	}
	return false
}

func (s *SidebarModel) ActivateCurrent() *workspace.Workspace {
	if len(s.workspaces) == 0 {
		return nil
	}
	s.activeIndex = s.cursor
	ws := s.workspaces[s.cursor]
	return &ws
}

func (s *SidebarModel) SetWorkspaces(wss []workspace.Workspace) {
	s.workspaces = wss
	s.cursor = 0
	s.activeIndex = -1
}

func (s *SidebarModel) ToggleHidden() { s.hidden = !s.hidden }
func (s SidebarModel) IsHidden() bool  { return s.hidden }

func (s *SidebarModel) SetCursorByClick(relY int) {
	if relY >= 0 && relY < len(s.workspaces) {
		s.cursor = relY
	}
}

func lipglossMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func min(a, b int) int {
	return lipglossMin(a, b)
}

// Needed for lipgloss width calculation
var _ = lipgloss.NewStyle
