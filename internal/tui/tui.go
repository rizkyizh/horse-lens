package tui

import (
	"fmt"

	"github.com/atterpac/dado/components"
	"github.com/atterpac/dado/layout"
	"github.com/atterpac/dado/theme"
	"github.com/atterpac/dado/theme/themes"

	"github.com/rizkyizh/horse-lens/internal/store"
)

// Result reports what the user asked for on the way out. Entering a workspace
// happens after the UI has released the terminal, never inside it.
type Result struct {
	Enter string
}

type ui struct {
	app  *layout.App
	ctl  *Controller
	res  Result
	list *components.Table
	// links is the detail table for the workspace named in detailFor.
	links     *components.Table
	detailFor string
}

// Run opens the interface and blocks until the user quits.
func Run(st *store.Store) (Result, error) {
	theme.SetProvider(themes.TokyoNightStorm)

	u := &ui{ctl: NewController(st)}
	u.ctl.Refresh()

	status := components.NewStatusBar()
	u.app = layout.NewApp(layout.AppConfig{
		TopBar:          status,
		BottomBar:       layout.NewMenu(),
		BottomBarHeight: 1,
	})

	u.buildList()
	u.app.Pages().Push(&page{Widget: u.list, name: "Workspaces", hints: u.hints()})
	u.app.SetInputCapture(u.keys)
	u.list.Focus()
	u.refreshList()

	if err := u.app.Run(); err != nil {
		return Result{}, err
	}
	return u.res, nil
}

// --- workspace list ---------------------------------------------------------

func (u *ui) buildList() {
	t := components.NewTable()
	t.SetHeaders("WORKSPACE", "LINKS", "STATE", "DIRECTORY")
	t.ConfigureEmpty("", "No workspaces", "Press n to create one")
	u.list = t
}

func (u *ui) refreshList() {
	u.fillList()
	u.showStatus()
}

// fillList rewrites the table rows from the controller. It touches no app
// state, so it can be exercised without a running terminal.
func (u *ui) fillList() {
	sel := u.selectedRow()
	u.list.ClearRows()
	for _, r := range u.ctl.Rows() {
		u.list.AddRow(r.Name, fmt.Sprintf("%d", r.Links), summaryText(r), r.Dir)
	}
	u.restoreRow(sel)
}

func summaryText(r store.Summary) string {
	switch {
	case r.Drift > 0 && r.Dangling > 0:
		return fmt.Sprintf("%d pending, %d dangling", r.Drift, r.Dangling)
	case r.Drift > 0:
		return fmt.Sprintf("%d pending", r.Drift)
	case r.Dangling > 0:
		return fmt.Sprintf("%d dangling", r.Dangling)
	case r.Foreign > 0:
		return fmt.Sprintf("in sync, %d foreign", r.Foreign)
	default:
		return "in sync"
	}
}

// selectedRow is the cursor position, in data rows. GetSelectedRows reports
// multi-select marks instead, which are never set here.
func (u *ui) selectedRow() int {
	return u.list.SelectedRow()
}

func (u *ui) restoreRow(i int) {
	n := u.list.GetDataRowCount()
	if n == 0 {
		return
	}
	if i >= n {
		i = n - 1
	}
	if i < 0 {
		i = 0
	}
	u.list.SelectRow(i)
}

// selectedName is the workspace under the cursor on the list page.
func (u *ui) selectedName() string {
	names := u.ctl.Names()
	i := u.selectedRow()
	if i < 0 || i >= len(names) {
		return ""
	}
	return names[i]
}

func (u *ui) showStatus() {
	if u.app == nil {
		return
	}
	msg, failed := u.ctl.Status()
	if msg == "" {
		p := u.ctl.st.Paths()
		msg = fmt.Sprintf("%d workspaces   root: %s", len(u.ctl.Rows()), p.Root)
	}
	if bar, ok := u.app.TopBar().(*layout.StatusBar); ok {
		_ = failed
		bar.SetSections([]layout.StatusSection{{Text: msg}})
	}
	u.app.UpdateMenuHints(u.hints())
}

func (u *ui) hints() []components.KeyHint {
	if u.detailFor != "" {
		return []components.KeyHint{
			{Key: "a", Description: "add link"},
			{Key: "d", Description: "remove link"},
			{Key: "A", Description: "apply"},
			{Key: "Esc", Description: "back"},
		}
	}
	return []components.KeyHint{
		{Key: "↵", Description: "enter"},
		{Key: "e/l", Description: "edit links"},
		{Key: "n", Description: "new"},
		{Key: "r", Description: "rename"},
		{Key: "d", Description: "delete"},
		{Key: "a/A", Description: "apply/all"},
		{Key: "q", Description: "quit"},
	}
}

// --- link detail ------------------------------------------------------------

func (u *ui) openDetail(name string) {
	if name == "" {
		return
	}
	u.detailFor = name
	t := components.NewTable()
	t.SetHeaders("ALIAS", "SOURCE", "STATE")
	t.ConfigureEmpty("", "No links", "Press a to add a project")
	u.links = t
	u.refreshDetail()

	// The table is pushed directly rather than wrapped in a Panel: every extra
	// layer has to forward HandleKey and HandleMouse, and the page name already
	// shows which workspace this is.
	u.app.Pages().Push(&page{Widget: t, name: name, hints: u.hints()})
	t.Focus()
	u.showStatus()
}

func (u *ui) refreshDetail() {
	if u.links == nil {
		return
	}
	u.links.ClearRows()
	for _, l := range u.ctl.Links(u.detailFor) {
		u.links.AddRow(l.Alias, l.Src, l.State)
	}
	for _, f := range u.ctl.Foreign(u.detailFor) {
		u.links.AddRow(f, "(not managed by horselens)", "left alone")
	}
}

func (u *ui) selectedAlias() string {
	links := u.ctl.Links(u.detailFor)
	i := u.links.SelectedRow()
	if i < 0 || i >= len(links) {
		return ""
	}
	return links[i].Alias
}

func (u *ui) closeDetail() {
	u.detailFor = ""
	u.links = nil
	u.app.Pages().Pop()
	u.refreshList()
}
