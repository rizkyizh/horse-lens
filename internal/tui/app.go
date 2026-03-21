package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rizkyizh/horse-lens/internal/workspace"
)

type focusPanel int

const (
	focusSidebar focusPanel = iota
	focusTerminal
)

type AppMode int

const (
	ModeNormal AppMode = iota
	ModeForm
	ModeConfirmDelete
)

const version = "v0.1.0"

type AppModel struct {
	sidebar   SidebarModel
	terminals map[string]TerminalModel // all running terminals keyed by workspace name
	activeWS  string                   // workspace currently displayed in the terminal panel
	form            WorkspaceFormModel
	focus           focusPanel
	mode            AppMode
	pendingDeleteWS string
	width     int
	height    int
	status    string

	// Panel dimensions and bounding boxes for mouse hit-testing
	termW, termH             int
	sidebarX, sidebarY       int
	sidebarW, sidebarH       int
	terminalX, terminalY     int
	terminalW, terminalH     int
}

func NewAppModel(workspaces []workspace.Workspace) AppModel {
	m := AppModel{
		sidebar:   NewSidebar(workspaces),
		terminals: make(map[string]TerminalModel),
		focus:     focusSidebar,
		mode:      ModeNormal,
	}
	m.sidebar.SetFocused(true)
	return m
}

func (m AppModel) Init() tea.Cmd {
	return nil
}

// activeTerminal returns the terminal for the currently displayed workspace.
func (m AppModel) activeTerminal() TerminalModel {
	if t, ok := m.terminals[m.activeWS]; ok {
		return t
	}
	idle := NewTerminal()
	idle.width = m.termW
	idle.height = m.termH
	return idle
}

// setActiveTerminal writes back a mutated terminal for the active workspace.
func (m *AppModel) setActiveTerminal(t TerminalModel) {
	m.terminals[m.activeWS] = t
}

// closeAllTerminals closes every running PTY (called on quit).
func (m *AppModel) closeAllTerminals() {
	for ws := range m.terminals {
		t := m.terminals[ws]
		t.Close()
		// no need to write back — we're quitting
	}
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layoutPanels()
		return m, nil

	case ptyReadMsg:
		t, ok := m.terminals[msg.workspace]
		if !ok {
			return m, nil
		}
		updated, cmd := t.Update(msg)
		m.terminals[msg.workspace] = updated
		if !updated.IsReady() && msg.workspace == m.activeWS && m.focus == focusTerminal {
			m.focus = focusSidebar
			m.sidebar.SetFocused(true)
			m.status = "Terminal exited."
		}
		return m, cmd

	case ptyErrorMsg:
		t, ok := m.terminals[msg.workspace]
		if !ok {
			return m, nil
		}
		updated, cmd := t.Update(msg)
		m.terminals[msg.workspace] = updated
		if !updated.IsReady() && msg.workspace == m.activeWS && m.focus == focusTerminal {
			m.focus = focusSidebar
			m.sidebar.SetFocused(true)
			m.status = "Terminal exited."
		}
		return m, cmd

	case tea.KeyMsg:
		// Confirmation modal: handle y/n/esc before anything else
		if m.mode == ModeConfirmDelete {
			switch msg.String() {
			case "y", "enter":
				ws := m.pendingDeleteWS
				m.mode = ModeNormal
				m.pendingDeleteWS = ""
				deleted := m.sidebar.DeleteByName(ws)
				if deleted {
					workspace.DeleteProfile(ws)    //nolint:errcheck
					workspace.RemoveWorkspaceDir(ws) //nolint:errcheck
					if t, ok := m.terminals[ws]; ok {
						t.Close()
						delete(m.terminals, ws)
					}
					if m.activeWS == ws {
						m.activeWS = ""
					}
					m.status = fmt.Sprintf("Deleted workspace %q", ws)
				}
			case "n", "esc":
				m.mode = ModeNormal
				m.pendingDeleteWS = ""
			}
			return m, nil
		}

		// ctrl+b toggles sidebar from anywhere (before terminal forwarding)
		if msg.String() == "ctrl+b" && m.mode == ModeNormal {
			m.sidebar.ToggleHidden()
			m.layoutPanels()
			if m.sidebar.IsHidden() && m.focus == focusSidebar {
				m.focus = focusTerminal
				m.sidebar.SetFocused(false)
				if t, ok := m.terminals[m.activeWS]; ok {
					t.SetFocused(true)
					m.terminals[m.activeWS] = t
				}
			}
			return m, nil
		}

		// Terminal focused and ready: forward all keys except Ctrl+H/PgUp/PgDown
		if m.focus == focusTerminal && m.activeWS != "" {
			if t, ok := m.terminals[m.activeWS]; ok && t.IsReady() {
				switch msg.String() {
				case "ctrl+h":
					t.SetFocused(false)
					m.terminals[m.activeWS] = t
					m.focus = focusSidebar
					m.sidebar.SetFocused(true)
					return m, nil
				case "pgup":
					if !t.IsAltScreen() {
						t.ScrollBy(scrollStep)
						m.terminals[m.activeWS] = t
						return m, nil
					}
				case "pgdown":
					if !t.IsAltScreen() {
						t.ScrollBy(-scrollStep)
						m.terminals[m.activeWS] = t
						return m, nil
					}
				default:
					// Any other key while scrolled snaps back to live view
					if t.IsScrolled() {
						t.ScrollToBottom()
					}
				}
				updated, cmd := t.Update(msg)
				m.terminals[m.activeWS] = updated
				return m, cmd
			}
		}

		// Global keys (sidebar focused or terminal not ready)
		switch msg.String() {
		case "ctrl+c":
			m.closeAllTerminals()
			return m, tea.Quit

		case "ctrl+l":
			if m.mode == ModeNormal {
				m.toggleFocus()
				return m, nil
			}

		case "q":
			if m.mode == ModeNormal && m.focus == focusSidebar {
				m.closeAllTerminals()
				return m, tea.Quit
			}
		}

		if m.mode == ModeForm {
			var cmd tea.Cmd
			m.form, cmd = m.form.Update(msg)
			return m, cmd
		}

		// Sidebar-specific keys when sidebar is focused
		if m.focus == focusSidebar {
			switch msg.String() {
			case "n":
				m.mode = ModeForm
				m.form = NewWorkspaceForm()
				return m, nil


			case "e":
				ws := m.sidebar.Selected()
				if ws != nil {
					m.mode = ModeForm
					m.form = NewWorkspaceFormEdit(*ws)
				}
				return m, nil
			case "d":
				ws := m.sidebar.Selected()
				if ws != nil {
					m.pendingDeleteWS = ws.Name
					m.mode = ModeConfirmDelete
				}
				return m, nil

			case "enter":
				ws := m.sidebar.ActivateCurrent()
				if ws != nil {
					if err := workspace.CreateSymlinks(*ws); err != nil {
						m.status = "Error creating symlinks: " + err.Error()
						return m, nil
					}

					// Unfocus old active terminal
					if old, ok := m.terminals[m.activeWS]; ok {
						old.SetFocused(false)
						m.terminals[m.activeWS] = old
					}

					m.activeWS = ws.Name
					m.focus = focusTerminal
					m.sidebar.SetFocused(false)
					m.status = fmt.Sprintf("Workspace %q", ws.Name)

					// Reuse existing terminal if already running
					if t, ok := m.terminals[ws.Name]; ok && t.IsReady() {
						t.SetFocused(true)
						m.terminals[ws.Name] = t
						return m, nil
					}

					// Start new terminal for this workspace
					t := NewTerminal()
					t.SetSize(m.termW, m.termH)
					t.SetWorkspaceName(ws.Name)
					t.SetFocused(true)
					cmd := t.Start(workspace.WorkspaceRoot(ws.Name))
					m.terminals[ws.Name] = t
					return m, cmd
				}
				return m, nil
			}

			var cmd tea.Cmd
			m.sidebar, cmd = m.sidebar.Update(msg)
			return m, cmd
		}

	case FormDoneMsg:
		ws := msg.Workspace
		m.mode = ModeNormal
		if msg.IsEdit {
			if err := workspace.SaveProfile(ws); err != nil {
				m.status = "Error saving: " + err.Error()
			} else {
				m.sidebar.UpdateWorkspace(ws, msg.OriginalName)
				if msg.OriginalName != ws.Name {
					if t, ok := m.terminals[msg.OriginalName]; ok {
						t.Close()
						delete(m.terminals, msg.OriginalName)
					}
					if m.activeWS == msg.OriginalName {
						m.activeWS = ""
					}
				}
				m.status = fmt.Sprintf("Updated workspace %q", ws.Name)
			}
		} else {
			if err := workspace.SaveProfile(ws); err != nil {
				m.status = "Error saving profile: " + err.Error()
			} else {
				m.sidebar.AddWorkspace(ws)
				m.status = fmt.Sprintf("Created workspace %q", ws.Name)
			}
		}
		return m, nil

	case FormCancelMsg:
		m.mode = ModeNormal
		return m, nil

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				if t, ok := m.terminals[m.activeWS]; ok && t.IsReady() && !t.IsAltScreen() {
					t.ScrollBy(scrollStep)
					m.terminals[m.activeWS] = t
					return m, nil
				}
			case tea.MouseButtonWheelDown:
				if t, ok := m.terminals[m.activeWS]; ok && t.IsReady() && !t.IsAltScreen() {
					t.ScrollBy(-scrollStep)
					m.terminals[m.activeWS] = t
					return m, nil
				}
			}
		}
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			x, y := msg.X, msg.Y
			if !m.sidebar.IsHidden() &&
				x >= m.sidebarX && x < m.sidebarX+m.sidebarW &&
				y >= m.sidebarY && y < m.sidebarY+m.sidebarH {
				if m.focus != focusSidebar {
					m.focus = focusSidebar
					m.sidebar.SetFocused(true)
					if t, ok := m.terminals[m.activeWS]; ok {
						t.SetFocused(false)
						m.terminals[m.activeWS] = t
					}
				}
				relY := y - m.sidebarY - 2 // skip border + title row
				m.sidebar.SetCursorByClick(relY)
			} else if x >= m.terminalX && x < m.terminalX+m.terminalW &&
				y >= m.terminalY && y < m.terminalY+m.terminalH {
				if m.focus != focusTerminal {
					m.focus = focusTerminal
					m.sidebar.SetFocused(false)
					if t, ok := m.terminals[m.activeWS]; ok {
						t.SetFocused(true)
						m.terminals[m.activeWS] = t
					}
				}
			}
		}
		return m, nil
	}

	return m, nil
}

func (m *AppModel) toggleFocus() {
	if m.focus == focusSidebar {
		m.focus = focusTerminal
		m.sidebar.SetFocused(false)
		if t, ok := m.terminals[m.activeWS]; ok {
			t.SetFocused(true)
			m.terminals[m.activeWS] = t
		}
	} else {
		m.focus = focusSidebar
		m.sidebar.SetFocused(true)
		if t, ok := m.terminals[m.activeWS]; ok {
			t.SetFocused(false)
			m.terminals[m.activeWS] = t
		}
	}
}

func (m *AppModel) layoutPanels() {
	sidebarW := 0
	if !m.sidebar.IsHidden() {
		sidebarW = 20
		if m.width < 60 {
			sidebarW = m.width / 3
		}
	}
	termW := m.width - sidebarW
	panelH := m.height - 3 // header + status bar

	m.sidebar.SetSize(sidebarW, panelH)
	m.termW = termW
	m.termH = panelH

	// Resize all running terminals
	for ws := range m.terminals {
		t := m.terminals[ws]
		t.SetSize(termW, panelH)
		m.terminals[ws] = t
	}

	m.sidebarX = 0
	m.sidebarY = 1 // after header
	m.sidebarW = sidebarW
	m.sidebarH = panelH

	m.terminalX = sidebarW
	m.terminalY = 1
	m.terminalW = termW
	m.terminalH = panelH
}

func (m AppModel) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	// Header
	title := styleTitle.Render("HorseLens")
	ver := styleVersion.Render(version)
	headerContent := lipgloss.JoinHorizontal(lipgloss.Top, title, ver)
	headerPad := m.width - lipgloss.Width(headerContent)
	if headerPad < 0 {
		headerPad = 0
	}
	header := headerContent + styleVersion.Render(repeatStr(" ", headerPad))
	header = lipgloss.NewStyle().
		Background(lipgloss.Color("#111111")).
		Width(m.width).
		Render(header)

	termView := m.activeTerminal().View()
	var body string
	if m.sidebar.IsHidden() {
		body = termView
	} else {
		body = lipgloss.JoinHorizontal(lipgloss.Top, m.sidebar.View(), termView)
	}

	if m.mode == ModeForm {
		panelH := m.height - 3
		body = lipgloss.Place(
			m.width,
			panelH,
			lipgloss.Center,
			lipgloss.Center,
			m.form.View(),
			lipgloss.WithWhitespaceBackground(colorOverlay),
		)
	}

	if m.mode == ModeConfirmDelete {
		panelH := m.height - 3
		modalMsg := styleTitle.Render("DELETE WORKSPACE") + "\n\n" +
			styleNormalItem.Render("Delete "+fmt.Sprintf("%q", m.pendingDeleteWS)+"?") + "\n" +
			styleDimItem.Render("This will remove the workspace and its symlink folder.") + "\n\n" +
			styleDimItem.Render("[y/Enter] confirm  [n/Esc] cancel")
		body = lipgloss.Place(
			m.width,
			panelH,
			lipgloss.Center,
			lipgloss.Center,
			styleModal.Render(modalMsg),
			lipgloss.WithWhitespaceBackground(colorOverlay),
		)
	}

	// Status bar
	tabHint := "[Tab] switch panel"
	if m.mode == ModeNormal && m.focus == focusSidebar {
		tabHint = "[Ctrl+L] → terminal  [n]ew  [e]dit  [d]el  [↵] open  [q]uit  [Ctrl+B] hide sidebar"
	} else if m.mode == ModeNormal && m.focus == focusTerminal {
		if t, ok := m.terminals[m.activeWS]; ok && t.IsScrolled() {
			tabHint = "[Ctrl+H] → sidebar  [PgUp/Dn] scroll  [any key] back to live  [Ctrl+B] hide sidebar"
		} else {
			tabHint = "[Ctrl+H] → sidebar  [PgUp] scroll  [Ctrl+B] hide sidebar"
		}
	}
	status := m.status
	if status == "" {
		status = tabHint
	}
	statusBar := styleStatusBar.Width(m.width).Render(status)

	return lipgloss.JoinVertical(lipgloss.Left, header, body, statusBar)
}

func repeatStr(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}
