package tui

import (
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/rizkyizh/horse-lens/internal/workspace"
)

type FormStep int

const (
	FormStepName FormStep = iota
	FormStepLinks // edit mode only: manage existing links
	FormStepSrc
	FormStepAlias
	FormStepDone
)

type WorkspaceFormModel struct {
	step        FormStep
	nameInput   textinput.Model
	srcInput    textinput.Model
	aliasInput  textinput.Model
	links       []workspace.Link
	wsName      string
	err         string
	suggestions    []string
	suggestionIdx  int
	showSuggestions bool
	editMode     bool
	originalName string
	linkCursor  int // cursor position in FormStepLinks
	editingLink int // -1 = adding new, >= 0 = editing this index
}

func NewWorkspaceForm() WorkspaceFormModel {
	name := textinput.New()
	name.Placeholder = "workspace name"
	name.Focus()
	name.CharLimit = 64

	src := textinput.New()
	src.Placeholder = "~/path/to/folder"
	src.CharLimit = 256

	alias := textinput.New()
	alias.Placeholder = "alias (e.g. project)"
	alias.CharLimit = 64

	return WorkspaceFormModel{
		step:          FormStepName,
		nameInput:     name,
		srcInput:      src,
		aliasInput:    alias,
		suggestionIdx: -1,
		editingLink:   -1,
	}
}

type FormDoneMsg struct {
	Workspace    workspace.Workspace
	IsEdit       bool
	OriginalName string
}
type FormCancelMsg struct{}

func NewWorkspaceFormEdit(ws workspace.Workspace) WorkspaceFormModel {
	f := NewWorkspaceForm()
	f.editMode = true
	f.originalName = ws.Name
	f.wsName = ws.Name
	f.links = make([]workspace.Link, len(ws.Links))
	copy(f.links, ws.Links)
	f.nameInput.SetValue(ws.Name)
	return f
}

func (f WorkspaceFormModel) Update(msg tea.Msg) (WorkspaceFormModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		f.err = ""

		// Handle autocomplete navigation in FormStepSrc when suggestions are shown
		if f.step == FormStepSrc && f.showSuggestions {
			switch msg.String() {
			case "down":
				if f.suggestionIdx < len(f.suggestions)-1 {
					f.suggestionIdx++
				}
				return f, nil
			case "up":
				if f.suggestionIdx > 0 {
					f.suggestionIdx--
				}
				return f, nil
			case "tab":
				if f.suggestionIdx >= 0 && f.suggestionIdx < len(f.suggestions) {
					f.srcInput.SetValue(f.suggestions[f.suggestionIdx])
					f.srcInput.CursorEnd()
				}
				f.showSuggestions = false
				f.suggestions = nil
				f.suggestionIdx = -1
				return f, nil
			case "esc":
				f.showSuggestions = false
				f.suggestions = nil
				f.suggestionIdx = -1
				return f, nil
			}
		}

		switch msg.String() {
		case "esc":
			// In edit mode, esc from Src/Alias goes back to links list
			if f.editMode && (f.step == FormStepSrc || f.step == FormStepAlias) {
				f.step = FormStepLinks
				f.srcInput.Blur()
				f.aliasInput.Blur()
				f.editingLink = -1
				return f, nil
			}
			return f, func() tea.Msg { return FormCancelMsg{} }

		case "enter":
			switch f.step {
			case FormStepName:
				name := strings.TrimSpace(f.nameInput.Value())
				if name == "" {
					f.err = "Name cannot be empty"
					break
				}
				f.wsName = name
				f.nameInput.Blur()
				if f.editMode {
					f.step = FormStepLinks
					f.linkCursor = 0
				} else {
					f.step = FormStepSrc
					f.srcInput.Focus()
				}

			case FormStepLinks:
				if f.linkCursor < len(f.links) {
					// Edit selected link
					f.editingLink = f.linkCursor
					f.srcInput.SetValue(f.links[f.linkCursor].Src)
					f.srcInput.CursorEnd()
					f.aliasInput.SetValue(f.links[f.linkCursor].Alias)
					f.step = FormStepSrc
					f.srcInput.Focus()
				} else {
					// Cursor on "[ + add new ]"
					f.editingLink = -1
					f.srcInput.SetValue("")
					f.aliasInput.SetValue("")
					f.step = FormStepSrc
					f.srcInput.Focus()
				}

			case FormStepSrc:
				src := strings.TrimSpace(f.srcInput.Value())
				if src == "" {
					if len(f.links) > 0 && !f.editMode {
						ws := workspace.Workspace{Name: f.wsName, Links: f.links}
						return f, func() tea.Msg {
							return FormDoneMsg{Workspace: ws, IsEdit: f.editMode, OriginalName: f.originalName}
						}
					}
					f.err = "Source path cannot be empty"
					break
				}
				f.showSuggestions = false
				f.suggestions = nil
				f.suggestionIdx = -1
				f.step = FormStepAlias
				f.srcInput.Blur()
				f.aliasInput.Focus()

			case FormStepAlias:
				alias := strings.TrimSpace(f.aliasInput.Value())
				src := strings.TrimSpace(f.srcInput.Value())
				if alias == "" {
					f.err = "Alias cannot be empty"
					break
				}
				if f.editMode {
					if f.editingLink >= 0 && f.editingLink < len(f.links) {
						f.links[f.editingLink] = workspace.Link{Src: src, Alias: alias}
					} else {
						f.links = append(f.links, workspace.Link{Src: src, Alias: alias})
					}
					f.editingLink = -1
					f.srcInput.SetValue("")
					f.aliasInput.SetValue("")
					f.aliasInput.Blur()
					f.step = FormStepLinks
				} else {
					f.links = append(f.links, workspace.Link{Src: src, Alias: alias})
					ws := workspace.Workspace{Name: f.wsName, Links: f.links}
					return f, func() tea.Msg {
						return FormDoneMsg{Workspace: ws, IsEdit: f.editMode, OriginalName: f.originalName}
					}
				}
			}

		case "tab":
			switch f.step {
			case FormStepAlias:
				// Add current link and go back to src for another
				alias := strings.TrimSpace(f.aliasInput.Value())
				src := strings.TrimSpace(f.srcInput.Value())
				if alias != "" && src != "" {
					if f.editMode && f.editingLink >= 0 && f.editingLink < len(f.links) {
						f.links[f.editingLink] = workspace.Link{Src: src, Alias: alias}
					} else {
						f.links = append(f.links, workspace.Link{Src: src, Alias: alias})
					}
				}
				f.editingLink = -1
				f.aliasInput.SetValue("")
				f.srcInput.SetValue("")
				f.step = FormStepSrc
				f.aliasInput.Blur()
				f.srcInput.Focus()
			}

		// FormStepLinks navigation and actions
		case "up", "k":
			if f.step == FormStepLinks && f.linkCursor > 0 {
				f.linkCursor--
				return f, nil
			}
		case "down", "j":
			if f.step == FormStepLinks {
				maxIdx := len(f.links) // last position = "add new"
				if f.linkCursor < maxIdx {
					f.linkCursor++
				}
				return f, nil
			}
		case "e":
			if f.step == FormStepLinks && f.linkCursor < len(f.links) {
				f.editingLink = f.linkCursor
				f.srcInput.SetValue(f.links[f.linkCursor].Src)
				f.srcInput.CursorEnd()
				f.aliasInput.SetValue(f.links[f.linkCursor].Alias)
				f.step = FormStepSrc
				f.srcInput.Focus()
				return f, nil
			}
		case "d":
			if f.step == FormStepLinks && f.linkCursor < len(f.links) {
				f.links = append(f.links[:f.linkCursor], f.links[f.linkCursor+1:]...)
				if f.linkCursor >= len(f.links) && f.linkCursor > 0 {
					f.linkCursor--
				}
				return f, nil
			}
		case "n":
			if f.step == FormStepLinks {
				f.editingLink = -1
				f.srcInput.SetValue("")
				f.aliasInput.SetValue("")
				f.step = FormStepSrc
				f.srcInput.Focus()
				return f, nil
			}
		case "s":
			if f.step == FormStepLinks {
				ws := workspace.Workspace{Name: f.wsName, Links: f.links}
				return f, func() tea.Msg {
					return FormDoneMsg{Workspace: ws, IsEdit: f.editMode, OriginalName: f.originalName}
				}
			}
		}
	}

	var cmd tea.Cmd
	switch f.step {
	case FormStepName:
		f.nameInput, cmd = f.nameInput.Update(msg)
		cmds = append(cmds, cmd)
	case FormStepSrc:
		f.srcInput, cmd = f.srcInput.Update(msg)
		cmds = append(cmds, cmd)
		// Refresh suggestions based on current input value
		sug := fetchSuggestions(f.srcInput.Value())
		f.suggestions = sug
		f.showSuggestions = len(sug) > 0
		if !f.showSuggestions {
			f.suggestionIdx = -1
		} else if f.suggestionIdx >= len(sug) {
			f.suggestionIdx = len(sug) - 1
		}
	case FormStepAlias:
		f.aliasInput, cmd = f.aliasInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return f, tea.Batch(cmds...)
}

func fetchSuggestions(inputPath string) []string {
	if inputPath == "" {
		return nil
	}

	endsWithSlash := strings.HasSuffix(inputPath, "/")

	// Expand ~
	expanded := inputPath
	if expanded == "~" || expanded == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		expanded = home
		endsWithSlash = true
	} else if strings.HasPrefix(expanded, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		expanded = filepath.Join(home, expanded[2:])
	}

	var dir, partial string
	if endsWithSlash {
		dir = expanded
		partial = ""
	} else {
		dir = filepath.Dir(expanded)
		partial = filepath.Base(expanded)
		if partial == "." || partial == string(filepath.Separator) {
			partial = ""
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var suggestions []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if partial == "" || strings.HasPrefix(e.Name(), partial) {
			suggestions = append(suggestions, filepath.Join(dir, e.Name())+"/")
			if len(suggestions) >= 6 {
				break
			}
		}
	}
	return suggestions
}

func (f WorkspaceFormModel) View() string {
	var sb strings.Builder
	title := "NEW WORKSPACE"
	if f.editMode {
		title = "EDIT WORKSPACE"
	}
	sb.WriteString(styleTitle.Render(title) + "\n\n")

	switch f.step {
	case FormStepName:
		sb.WriteString(styleNormalItem.Render("Workspace name:") + "\n")
		sb.WriteString(f.nameInput.View() + "\n")

	case FormStepLinks:
		sb.WriteString(styleActiveItem.Render(f.wsName) + "\n\n")
		if len(f.links) == 0 {
			sb.WriteString(styleDimItem.Render("  No links yet.") + "\n")
		} else {
			for i, l := range f.links {
				prefix := "  "
				if i == f.linkCursor {
					prefix = "> "
				}
				line := prefix + l.Src + " → " + l.Alias
				if i == f.linkCursor {
					sb.WriteString(styleNormalItem.Render(line) + "\n")
				} else {
					sb.WriteString(styleDimItem.Render(line) + "\n")
				}
			}
		}
		// "add new" item
		addNew := "  [ + add new ]"
		if f.linkCursor == len(f.links) {
			sb.WriteString(styleActiveItem.Render("> [ + add new ]") + "\n")
		} else {
			sb.WriteString(styleDimItem.Render(addNew) + "\n")
		}
		sb.WriteString("\n")
		sb.WriteString(styleDimItem.Render("[e/Enter] edit  [d] delete  [n] add new  [s] save  [Esc] cancel") + "\n")

	case FormStepSrc:
		if len(f.links) > 0 {
			sb.WriteString(styleActiveItem.Render(f.wsName) + "\n")
			for i, l := range f.links {
				prefix := "  "
				if f.editMode && i == f.editingLink {
					prefix = "* " // mark the link being edited
				}
				sb.WriteString(styleDimItem.Render(prefix+l.Src+" → "+l.Alias) + "\n")
			}
			sb.WriteString("\n")
		}
		sb.WriteString(styleNormalItem.Render("Source path:") + "\n")
		sb.WriteString(f.srcInput.View() + "\n")
		if f.showSuggestions {
			for i, s := range f.suggestions {
				if i == f.suggestionIdx {
					sb.WriteString(styleSuggestionSelected.Render("> "+s) + "\n")
				} else {
					sb.WriteString(styleSuggestionItem.Render("  "+s) + "\n")
				}
			}
		}
		if f.editMode {
			sb.WriteString(styleDimItem.Render("[Tab] autocomplete  [↑↓] navigate  [Esc] back") + "\n")
		} else if len(f.links) > 0 {
			sb.WriteString(styleDimItem.Render("[Enter] done  [Tab] autocomplete  [↑↓] navigate  [Esc] cancel") + "\n")
		} else {
			sb.WriteString(styleDimItem.Render("[Tab] autocomplete  [↑↓] navigate  [Esc] cancel") + "\n")
		}

	case FormStepAlias:
		sb.WriteString(styleNormalItem.Render("Alias for "+f.srcInput.Value()+":") + "\n")
		sb.WriteString(f.aliasInput.View() + "\n")
		if f.editMode {
			sb.WriteString(styleDimItem.Render("[Enter] save  [Tab] add another  [Esc] back") + "\n")
		} else {
			sb.WriteString(styleDimItem.Render("[Enter] save  [Tab] add another  [Esc] cancel") + "\n")
		}
	}

	if f.err != "" {
		sb.WriteString("\n" + styleDimItem.Render("Error: "+f.err) + "\n")
	}

	return styleModal.Render(sb.String())
}
