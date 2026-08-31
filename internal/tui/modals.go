package tui

import (
	"fmt"
	"strings"

	"github.com/atterpac/dado/components"
	"github.com/atterpac/dado/core"
	"github.com/atterpac/dado/theme"
	"github.com/gdamore/tcell/v2"
)

// field describes one text input in a modal form. A field marked complete
// offers directory completions with the down and up keys.
type field struct {
	name, label, placeholder, initial string
	complete                          bool
}

// formModalSize returns a size that fits the fields without the form having to
// scroll. dado modals do not size to their content — whatever the config says
// is what gets drawn — so leaving the height short made the second field slide
// under the hint bar.
func formModalSize(fieldCount int, withSuggestions bool) (width, height int) {
	const (
		// A labelled text field draws its label plus a three-row bordered
		// input, and Form adds one row of spacing after each.
		fieldRows = 5
		// Panel border top and bottom, the hint bar, and a row of breathing
		// room above it.
		chrome = 4
	)
	if fieldCount < 1 {
		fieldCount = 1
	}
	h := chrome + fieldCount*fieldRows
	if withSuggestions {
		h += suggestionRows
	}
	return 72, h
}

// chromeRows is the non-field part of a form modal: panel border top and
// bottom, the hint bar, and a row of breathing room.
const chromeRows = 4

// anyComplete reports whether any field offers path completion.
func anyComplete(fields []field) bool {
	for _, f := range fields {
		if f.complete {
			return true
		}
	}
	return false
}

// formHints describes the keys a form actually responds to. Tab only moves
// somewhere when there is more than one field, and advertising it on a
// single-field dialog reads as a broken binding.
func formHints(fieldCount int) []components.KeyHint {
	hints := make([]components.KeyHint, 0, 5)
	if fieldCount > 1 {
		hints = append(hints, components.KeyHint{Key: "Tab", Description: "next field"})
	}
	hints = append(hints,
		components.KeyHint{Key: "↓↑", Description: "complete path"},
		components.KeyHint{Key: "^U", Description: "clear field"},
		components.KeyHint{Key: "↵", Description: "save"},
		components.KeyHint{Key: "Esc", Description: "cancel"},
	)
	return hints
}

// formModal shows a modal form and calls onSubmit with the trimmed values.
func (u *ui) formModal(title string, fields []field, onSubmit func(map[string]string)) {
	form := components.NewForm()
	completes := map[string]bool{}
	for _, f := range fields {
		if f.complete {
			completes[f.name] = true
		}
	}
	for _, f := range fields {
		form.AddTextField(f.name, f.label, f.placeholder)
	}
	for _, f := range fields {
		if f.initial == "" {
			continue
		}
		if tf, ok := form.GetTextField(f.name); ok {
			_ = tf.SetFieldValue(f.initial)
		}
	}

	width, height := formModalSize(len(fields), anyComplete(fields))
	modal := components.NewModal(components.ModalConfig{
		Title: title, Width: width, Height: height, Backdrop: true,
	})
	modal.SetHints(formHints(len(fields)))

	form.SetOnSubmit(func(values map[string]any) {
		out := make(map[string]string, len(values))
		for k, v := range values {
			out[k] = strings.TrimSpace(fmt.Sprint(v))
		}
		modal.Close()
		onSubmit(out)
	})
	form.SetOnCancel(func() { modal.Cancel() })

	if len(completes) > 0 {
		// Form and suggestion list are stacked so the candidates are visible
		// while cycling; the modal is sized for both.
		view := core.NewTextView()
		view.SetDynamicColors(true)
		view.SetTextAlign(core.AlignLeft)

		body := core.NewFlex().SetDirection(core.Column)
		body.AddItem(form, height-chromeRows-suggestionRows, 0, true)
		body.AddItem(view, suggestionRows, 0, false)

		state := newCompletionState(form, completes, view)
		for name := range completes {
			if tf, ok := form.GetTextField(name); ok {
				tf.SetOnChange(func(*components.ChangeEvent[string]) { state.reset() })
			}
		}
		state.render()

		u.modalKey = state.handleKey
		modal.SetOnClose(func() { u.modalKey = nil })
		modal.SetContent(body).SetFocusOnShow(form)
		u.app.ShowModal(modal)
		return
	}

	modal.SetOnClose(func() { u.modalKey = nil })
	modal.SetContent(form).SetFocusOnShow(form)
	u.app.ShowModal(modal)
}

// completionState tracks the candidate list for the focused path field and
// keeps the visible list in step with it.
type completionState struct {
	form       *components.Form
	completes  map[string]bool
	view       *core.TextView
	candidates []string
	index      int
}

func newCompletionState(form *components.Form, completes map[string]bool, view *core.TextView) *completionState {
	return &completionState{form: form, completes: completes, view: view, index: -1}
}

// focused returns the completing field that currently has focus.
func (c *completionState) focused() (*components.TextField, bool) {
	for name := range c.completes {
		if tf, ok := c.form.GetTextField(name); ok && tf.HasFocus() {
			return tf, true
		}
	}
	return nil, false
}

// reset recomputes the candidates from what is typed. Called on every edit.
func (c *completionState) reset() {
	c.index = -1
	c.candidates = nil
	if tf, ok := c.focused(); ok {
		c.candidates = completePath(tf.GetValue())
	}
	c.render()
}

func (c *completionState) render() {
	if c.view == nil {
		return
	}
	lines := renderSuggestions(c.candidates, c.index, suggestionRows)
	styled := make([]string, 0, len(lines))
	for _, l := range lines {
		colour := theme.TagFgMuted()
		if strings.HasPrefix(l, "> ") {
			colour = theme.TagAccent()
		}
		styled = append(styled, "["+colour+"]"+l+"[-]")
	}
	c.view.SetText(strings.Join(styled, "\n"))
}

// handleKey walks the candidates. It runs before the modal, because the form
// would otherwise consume the arrow keys first.
func (c *completionState) handleKey(ev *tcell.EventKey) bool {
	var step int
	switch ev.Key() {
	case tcell.KeyDown:
		step = 1
	case tcell.KeyUp:
		step = -1
	default:
		return false
	}

	tf, ok := c.focused()
	if !ok {
		return false
	}
	// Recompute when the text is not one this handler just inserted.
	if c.index < 0 || c.index >= len(c.candidates) || tf.GetValue() != c.candidates[c.index] {
		c.candidates = completePath(tf.GetValue())
		c.index = -1
	}
	if len(c.candidates) == 0 {
		c.render()
		return true
	}
	c.index = cycle(c.index, step, len(c.candidates))
	tf.SetValue(c.candidates[c.index])
	c.render()
	return true
}

// confirmModal builds a yes/no modal.
//
// Modal.handleBaseInput invokes onSubmit on Enter but does not dismiss the
// dialog; closing is the handler's job. Close runs before the action so the
// dialog disappears even when the action then fails and reports an error.
func confirmModal(title, message string, onYes func()) *components.Modal {
	modal := components.NewConfirmModal(title, message)
	modal.SetOnSubmit(func() {
		modal.Close()
		onYes()
	})
	return modal
}

// confirm shows a yes/no modal.
func (u *ui) confirm(title, message string, onYes func()) {
	u.app.ShowModal(confirmModal(title, message, onYes))
}

// --- workspace actions ------------------------------------------------------

func (u *ui) newWorkspace() {
	u.formModal("New workspace",
		[]field{{name: "name", label: "Name", placeholder: "auth-feature"}},
		func(v map[string]string) {
			u.ctl.Create(v["name"])
			u.refreshList()
		})
}

func (u *ui) renameWorkspace(name string) {
	if name == "" {
		return
	}
	u.formModal("Rename workspace",
		[]field{{name: "name", label: "New name", placeholder: name, initial: name}},
		func(v map[string]string) {
			u.ctl.Rename(name, v["name"])
			u.refreshList()
		})
}

func (u *ui) setRoot() {
	current := u.ctl.Root()
	label := "Workspace root"
	if overridden, src := u.ctl.RootOverridden(); overridden {
		label = "Workspace root (" + src + " currently overrides this)"
	}
	u.formModal("Workspace root",
		[]field{{name: "root", label: label, placeholder: "~/Developer/workspaces", initial: current, complete: true}},
		func(v map[string]string) {
			u.ctl.SetRoot(v["root"])
			u.refreshList()
		})
}

func (u *ui) deleteWorkspace(name string) {
	if name == "" {
		return
	}
	foreign := u.ctl.Foreign(name)
	msg := fmt.Sprintf("Delete %q?\n\nIts symlinks and config entry are removed.\nYour source folders are never touched.", name)
	if len(foreign) > 0 {
		msg += fmt.Sprintf("\n\nIt also holds %d unmanaged file(s): %s\nThose are not symlinks, so deletion will be refused.\nMove them out first.",
			len(foreign), strings.Join(foreign, ", "))
	}
	u.confirm("Delete workspace", msg, func() {
		u.ctl.Delete(name, false)
		u.refreshList()
	})
}

// --- link actions -----------------------------------------------------------

func (u *ui) addLink() {
	ws := u.detailFor
	u.formModal("Add project",
		[]field{
			{name: "src", label: "Source path", placeholder: "~/Developer/backend", complete: true},
			{name: "alias", label: "Alias (optional)", placeholder: "defaults to folder name"},
		},
		func(v map[string]string) {
			u.ctl.AddLink(ws, v["src"], v["alias"])
			u.refreshDetail()
			u.showStatus()
		})
}

func (u *ui) editLink() {
	link, ok := u.selectedLink()
	if !ok {
		u.notAManagedLink()
		return
	}
	ws := u.detailFor
	u.formModal("Edit project",
		[]field{
			{name: "src", label: "Source path", placeholder: "~/Developer/backend", initial: link.RawSrc, complete: true},
			{name: "alias", label: "Alias", placeholder: "defaults to folder name", initial: link.Alias},
		},
		func(v map[string]string) {
			u.ctl.UpdateLink(ws, link.Alias, v["src"], v["alias"])
			u.refreshDetail()
			u.showStatus()
		})
}

func (u *ui) removeLink() {
	link, ok := u.selectedLink()
	if !ok {
		u.notAManagedLink()
		return
	}
	ws := u.detailFor
	u.confirm("Remove link",
		fmt.Sprintf("Remove %q from %s?\n\nOnly the symlink goes; the source folder stays.", link.Alias, ws),
		func() {
			u.ctl.RemoveLink(ws, link.Alias)
			u.refreshDetail()
			u.showStatus()
		})
}

// notAManagedLink explains why nothing happened when the cursor sits on an
// entry horselens does not manage.
func (u *ui) notAManagedLink() {
	u.ctl.ok("that entry is not managed by horselens — nothing to change")
	u.showStatus()
}
