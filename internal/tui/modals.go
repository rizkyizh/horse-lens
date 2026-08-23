package tui

import (
	"fmt"
	"strings"

	"github.com/atterpac/dado/components"
)

// field describes one text input in a modal form.
type field struct{ name, label, placeholder, initial string }

// formModal shows a modal form and calls onSubmit with the trimmed values.
func (u *ui) formModal(title string, fields []field, onSubmit func(map[string]string)) {
	form := components.NewForm()
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

	modal := components.NewModal(components.ModalConfig{
		Title: title, Width: 68, MinHeight: 9, Backdrop: true,
	})
	modal.SetHints([]components.KeyHint{
		{Key: "Tab", Description: "next field"},
		{Key: "^U", Description: "clear field"},
		{Key: "↵", Description: "save"},
		{Key: "Esc", Description: "cancel"},
	})

	form.SetOnSubmit(func(values map[string]any) {
		out := make(map[string]string, len(values))
		for k, v := range values {
			out[k] = strings.TrimSpace(fmt.Sprint(v))
		}
		modal.Close()
		onSubmit(out)
	})
	form.SetOnCancel(func() { modal.Cancel() })

	modal.SetContent(form).SetFocusOnShow(form)
	u.app.ShowModal(modal)
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
			{name: "src", label: "Source path", placeholder: "~/Developer/backend"},
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
			{name: "src", label: "Source path", placeholder: "~/Developer/backend", initial: link.RawSrc},
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
