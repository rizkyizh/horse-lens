package tui

import (
	"github.com/gdamore/tcell/v2"

	"github.com/atterpac/dado/components"
	"github.com/atterpac/dado/core"
)

// page adapts a plain dado widget into a nav.Component, which Pages requires.
//
// core.Widget does not include HandleKey or HandleMouse, so embedding it alone
// hides those methods on the wrapped widget and the app can no longer deliver
// keys or clicks to it. Both are forwarded explicitly.
type page struct {
	core.Widget
	name  string
	hints []components.KeyHint
}

func (p *page) Name() string                { return p.name }
func (p *page) Start()                      {}
func (p *page) Stop()                       {}
func (p *page) Hints() []components.KeyHint { return p.hints }

// HandleKey forwards to the wrapped widget, which is how the table keeps its
// own arrow and j/k/g/G navigation.
func (p *page) HandleKey(ev *tcell.EventKey) bool {
	if h, ok := p.Widget.(core.KeyHandler); ok {
		return h.HandleKey(ev)
	}
	return false
}

// HandleMouse forwards clicks and scroll to the wrapped widget.
func (p *page) HandleMouse(action core.MouseAction, ev *tcell.EventMouse) (bool, core.Widget) {
	if h, ok := p.Widget.(core.MouseHandler); ok {
		return h.HandleMouse(action, ev)
	}
	return false, nil
}

// action is what a key means. Routing is kept separate from performing the
// action so the bindings can be tested without a running terminal.
type action int

const (
	actPass action = iota // not ours; let the focused widget handle it
	actQuit
	actEnter
	actOpenLinks
	actCloseLinks
	actNew
	actRename
	actDelete
	actApply
	actApplyAll
	actReload
	actTheme
	actAddLink
	actRemoveLink
)

// routeList maps a key on the workspace list.
func routeList(ev *tcell.EventKey) action {
	switch ev.Key() {
	case tcell.KeyEnter:
		return actEnter
	case tcell.KeyRight:
		return actOpenLinks
	}
	switch ev.Rune() {
	case 'q':
		return actQuit
	case 'l':
		return actOpenLinks
	case 'n':
		return actNew
	case 'r':
		return actRename
	case 'd':
		return actDelete
	case 'a':
		return actApply
	case 'A':
		return actApplyAll
	case 'R':
		return actReload
	case 't':
		return actTheme
	}
	return actPass
}

// routeDetail maps a key on the link view.
func routeDetail(ev *tcell.EventKey) action {
	switch ev.Key() {
	case tcell.KeyEscape, tcell.KeyLeft:
		return actCloseLinks
	}
	switch ev.Rune() {
	case 'h':
		return actCloseLinks
	case 'q':
		return actQuit
	case 'a':
		return actAddLink
	case 'd':
		return actRemoveLink
	case 'A':
		return actApply
	}
	return actPass
}

// keys routes global shortcuts. Anything unrecognised falls through to the
// focused widget, so the table keeps its own j/k/g/G navigation.
func (u *ui) keys(ev *tcell.EventKey) *tcell.EventKey {
	// A modal owns the keyboard while it is open.
	if u.app.Pages().HasModal() {
		return ev
	}
	if u.detailFor != "" {
		return u.perform(routeDetail(ev), ev)
	}
	return u.perform(routeList(ev), ev)
}

func (u *ui) perform(a action, ev *tcell.EventKey) *tcell.EventKey {
	if a == actPass {
		return ev
	}
	name := u.selectedName()
	if u.detailFor != "" {
		name = u.detailFor
	}

	switch a {
	case actQuit:
		u.app.Stop()

	case actEnter:
		if name == "" {
			break
		}
		u.ctl.Apply(name)
		if _, failed := u.ctl.Status(); failed {
			u.showStatus()
			break
		}
		u.res.Enter = name
		u.app.Stop()

	case actOpenLinks:
		u.openDetail(name)
	case actCloseLinks:
		u.closeDetail()
	case actNew:
		u.newWorkspace()
	case actRename:
		u.renameWorkspace(name)
	case actDelete:
		u.deleteWorkspace(name)

	case actApply:
		if name == "" {
			break
		}
		u.ctl.Apply(name)
		if u.detailFor != "" {
			u.refreshDetail()
			u.showStatus()
		} else {
			u.refreshList()
		}
	case actApplyAll:
		u.ctl.ApplyAll()
		u.refreshList()
	case actReload:
		u.ctl.Refresh()
		u.refreshList()
	case actTheme:
		u.app.OpenThemeSelector()

	case actAddLink:
		u.addLink()
	case actRemoveLink:
		u.removeLink()
	}
	return nil
}
