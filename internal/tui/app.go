// Package tui provides the workspace picker shown when horselens is run with
// no arguments. It only selects, applies and deletes; creating and editing
// workspaces is the CLI's job, where the shell's own path completion works.
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rizkyizh/horse-lens/internal/workspace"
)

// Row is one workspace as displayed in the picker.
type Row struct {
	Name     string
	Dir      string
	Links    int
	Drift    int
	Dangling int
	Foreign  int
}

// Summary is the short state description shown beside the name.
func (r Row) Summary() string {
	var parts []string
	if r.Drift > 0 {
		parts = append(parts, fmt.Sprintf("%d pending", r.Drift))
	}
	if r.Dangling > 0 {
		parts = append(parts, fmt.Sprintf("%d dangling", r.Dangling))
	}
	if r.Foreign > 0 {
		parts = append(parts, fmt.Sprintf("%d foreign", r.Foreign))
	}
	if len(parts) == 0 {
		return "in sync"
	}
	return strings.Join(parts, ", ")
}

// Actions is what the picker asks the caller to do once it exits. The shell is
// only entered after the TUI has released the terminal.
type Actions struct {
	Enter  string // workspace to enter, empty if none
	Delete string // workspace the user confirmed deleting
}

type mode int

const (
	modeList mode = iota
	modeConfirmDelete
)

// Model is the picker.
type Model struct {
	rows    []Row
	cursor  int
	mode    mode
	status  string
	result  Actions
	width   int
	height  int
	applyFn func(name string) error
}

// New builds a picker over the given rows. applyFn reconciles one workspace
// and is called when the user presses "a".
func New(rows []Row, applyFn func(string) error) Model {
	return Model{rows: rows, applyFn: applyFn}
}

// Result reports what the user chose before quitting.
func (m Model) Result() Actions { return m.result }

func (m Model) Init() tea.Cmd { return nil }

func (m Model) selected() *Row {
	if len(m.rows) == 0 || m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return &m.rows[m.cursor]
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		if m.mode == modeConfirmDelete {
			switch msg.String() {
			case "y", "enter":
				if r := m.selected(); r != nil {
					m.result.Delete = r.Name
					return m, tea.Quit
				}
				m.mode = modeList
			case "n", "esc", "q":
				m.mode = modeList
				m.status = ""
			}
			return m, nil
		}

		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.rows)-1 {
				m.cursor++
			}

		case "a":
			r := m.selected()
			if r == nil {
				return m, nil
			}
			if m.applyFn == nil {
				return m, nil
			}
			if err := m.applyFn(r.Name); err != nil {
				m.status = "apply failed: " + err.Error()
			} else {
				m.status = fmt.Sprintf("applied %q", r.Name)
				m.rows[m.cursor].Drift = 0
			}

		case "d":
			if r := m.selected(); r != nil {
				m.mode = modeConfirmDelete
			}

		case "enter":
			if r := m.selected(); r != nil {
				m.result.Enter = r.Name
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m Model) View() string {
	var b strings.Builder

	b.WriteString(styleTitle.Render("HorseLens") + styleVersion.Render(" workspaces") + "\n\n")

	if len(m.rows) == 0 {
		b.WriteString(styleDimItem.Render("  no workspaces yet") + "\n\n")
		b.WriteString(styleDimItem.Render("  create one:  horselens new <name>") + "\n\n")
		b.WriteString(styleHelp.Render("[q] quit") + "\n")
		return b.String()
	}

	for i, r := range m.rows {
		prefix := "  "
		render := styleDimItem.Render
		if i == m.cursor {
			prefix = "> "
			render = styleActiveItem.Render
		}
		line := fmt.Sprintf("%s%-22s %-9s %s",
			prefix, r.Name, fmt.Sprintf("%d links", r.Links), r.Summary())
		b.WriteString(render(line) + "\n")
	}

	if r := m.selected(); r != nil {
		b.WriteString("\n" + styleDimItem.Render("  "+r.Dir) + "\n")
	}

	if m.mode == modeConfirmDelete {
		r := m.selected()
		name := ""
		if r != nil {
			name = r.Name
		}
		b.WriteString("\n" + styleModal.Render(
			styleTitle.Render("DELETE WORKSPACE")+"\n\n"+
				styleNormalItem.Render(fmt.Sprintf("Delete %q?", name))+"\n"+
				styleDimItem.Render("Removes its symlinks and config entry. Sources are never touched.")+"\n\n"+
				styleDimItem.Render("[y/Enter] confirm   [n/Esc] cancel")) + "\n")
		return b.String()
	}

	b.WriteString("\n")
	if m.status != "" {
		b.WriteString(styleNormalItem.Render("  "+m.status) + "\n")
	}
	b.WriteString(styleHelp.Render("[↵] enter  [a] apply  [d] delete  [↑↓/jk] move  [q] quit") + "\n")
	return b.String()
}

// RowsFrom builds picker rows by planning each workspace.
func RowsFrom(all []workspace.Workspace) ([]Row, error) {
	rows := make([]Row, 0, len(all))
	for _, ws := range all {
		p, err := workspace.BuildPlan(ws)
		if err != nil {
			return nil, err
		}
		c := p.Counts()
		row := Row{
			Name: ws.Name, Dir: ws.Dir, Links: len(ws.Links),
			Drift:   c[workspace.ActionCreate] + c[workspace.ActionRetarget] + c[workspace.ActionRemove],
			Foreign: c[workspace.ActionForeign],
		}
		for _, act := range p.Actions {
			if act.Dangling {
				row.Dangling++
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}
