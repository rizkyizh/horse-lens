package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/atterpac/dado/components"
	"github.com/atterpac/dado/core"
	"github.com/atterpac/dado/layout"

	"github.com/rizkyizh/horse-lens/internal/config"
	"github.com/rizkyizh/horse-lens/internal/store"
)

func newCtl(t *testing.T) (*Controller, string) {
	t.Helper()
	dir := t.TempDir()
	root := filepath.Join(dir, "ws")
	t.Setenv("HORSELENS_CONFIG", filepath.Join(dir, "config.toml"))
	t.Setenv("HORSELENS_ROOT", root)

	st, err := store.Open(config.Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	c := NewController(st)
	c.Refresh()
	return c, root
}

func srcDir(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func mustOK(t *testing.T, c *Controller, what string) {
	t.Helper()
	if msg, failed := c.Status(); failed {
		t.Fatalf("%s failed: %s", what, msg)
	}
}

func TestControllerFullLifecycle(t *testing.T) {
	c, root := newCtl(t)
	api := srcDir(t, "api")

	if !c.Create("auth") {
		t.Fatalf("Create: %s", mustMsg(c))
	}
	if len(c.Rows()) != 1 || c.Rows()[0].Name != "auth" {
		t.Fatalf("rows = %+v", c.Rows())
	}

	if !c.AddLink("auth", api, "") {
		t.Fatalf("AddLink: %s", mustMsg(c))
	}
	mustOK(t, c, "AddLink")
	if got, err := os.Readlink(filepath.Join(root, "auth", "api")); err != nil || got != api {
		t.Errorf("symlink = %q err %v, want %q", got, err, api)
	}

	links := c.Links("auth")
	if len(links) != 1 || links[0].Alias != "api" || links[0].State != "ok" {
		t.Errorf("links = %+v", links)
	}

	if !c.Rename("auth", "auth-feature") {
		t.Fatalf("Rename: %s", mustMsg(c))
	}
	if _, err := os.Stat(filepath.Join(root, "auth")); !os.IsNotExist(err) {
		t.Error("old directory left behind")
	}

	if !c.RemoveLink("auth-feature", "api") {
		t.Fatalf("RemoveLink: %s", mustMsg(c))
	}
	if _, err := os.Lstat(filepath.Join(root, "auth-feature", "api")); !os.IsNotExist(err) {
		t.Error("symlink not pruned")
	}

	if !c.Delete("auth-feature", false) {
		t.Fatalf("Delete: %s", mustMsg(c))
	}
	if len(c.Rows()) != 0 {
		t.Errorf("rows after delete = %+v", c.Rows())
	}
}

func mustMsg(c *Controller) string { m, _ := c.Status(); return m }

func TestControllerReportsFailuresWithoutMutating(t *testing.T) {
	c, _ := newCtl(t)
	c.Create("ok")

	// Invalid name.
	if c.Create("../escape") {
		t.Error("Create accepted a traversal name")
	}
	if _, failed := c.Status(); !failed {
		t.Error("status not marked as failed")
	}
	// Duplicate.
	if c.Create("ok") {
		t.Error("Create accepted a duplicate")
	}
	// Unknown workspace.
	if c.AddLink("nope", "/tmp", "x") {
		t.Error("AddLink accepted an unknown workspace")
	}
	if len(c.Rows()) != 1 {
		t.Errorf("failed operations changed the list: %+v", c.Rows())
	}
}

// Deleting a workspace holding unmanaged files must be refused and surfaced.
func TestControllerDeleteRefusesForeignFiles(t *testing.T) {
	c, root := newCtl(t)
	c.Create("w")
	if err := os.MkdirAll(filepath.Join(root, "w"), 0o755); err != nil {
		t.Fatal(err)
	}
	note := filepath.Join(root, "w", "NOTES.md")
	if err := os.WriteFile(note, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.Refresh()

	if got := c.Foreign("w"); len(got) != 1 || got[0] != "NOTES.md" {
		t.Fatalf("Foreign = %v", got)
	}
	if c.Delete("w", false) {
		t.Fatal("Delete succeeded despite an unmanaged file")
	}
	msg, failed := c.Status()
	if !failed || !strings.Contains(msg, "not symlinks") {
		t.Errorf("status = %q failed=%v", msg, failed)
	}
	if _, err := os.Stat(note); err != nil {
		t.Error("refused delete still removed the file")
	}
}

func TestControllerApplyReportsDrift(t *testing.T) {
	c, _ := newCtl(t)
	api := srcDir(t, "api")
	c.Create("w")
	c.AddLink("w", api, "api")

	// Already reconciled by AddLink.
	c.Apply("w")
	if msg, failed := c.Status(); failed || !strings.Contains(msg, "in sync") {
		t.Errorf("status = %q failed=%v, want in sync", msg, failed)
	}

	c.ApplyAll()
	if msg, failed := c.Status(); failed || !strings.Contains(msg, "1 workspaces") {
		t.Errorf("ApplyAll status = %q failed=%v", msg, failed)
	}
}

func TestControllerFlagsDanglingSource(t *testing.T) {
	c, _ := newCtl(t)
	c.Create("w")
	c.AddLink("w", filepath.Join(t.TempDir(), "gone"), "gone")

	links := c.Links("w")
	if len(links) != 1 || links[0].State != "source missing" {
		t.Errorf("links = %+v", links)
	}
	if c.Rows()[0].Dangling != 1 {
		t.Errorf("dangling count = %d, want 1", c.Rows()[0].Dangling)
	}
}

// The list widget must render without a terminal attached.
func TestListWidgetRenders(t *testing.T) {
	c, _ := newCtl(t)
	api := srcDir(t, "api")
	c.Create("auth")
	c.AddLink("auth", api, "api")
	c.Create("web")

	u := &ui{ctl: c}
	u.buildList()
	u.fillList()

	if got := u.list.GetDataRowCount(); got != 2 {
		t.Fatalf("row count = %d, want 2", got)
	}
	if got := u.list.GetRowData(0); len(got) == 0 || got[0] != "auth" {
		t.Errorf("row 0 = %v", got)
	}

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(100, 20)
	u.list.SetRect(0, 0, 100, 20)
	u.list.Draw(screen) // must not panic
	screen.Show()       // GetContents reads the front buffer

	if !strings.Contains(renderScreen(screen), "auth") {
		t.Error("workspace name not drawn")
	}
}

func renderScreen(s tcell.SimulationScreen) string {
	cells, w, h := s.GetContents()
	var b strings.Builder
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			b.WriteString(string(cells[y*w+x].Runes))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func TestSummaryText(t *testing.T) {
	cases := []struct {
		in   store.Summary
		want string
	}{
		{store.Summary{}, "in sync"},
		{store.Summary{Drift: 2}, "2 pending"},
		{store.Summary{Dangling: 1}, "1 dangling"},
		{store.Summary{Drift: 2, Dangling: 1}, "2 pending, 1 dangling"},
		{store.Summary{Foreign: 3}, "in sync, 3 foreign"},
	}
	for _, c := range cases {
		if got := summaryText(c.in); got != c.want {
			t.Errorf("summaryText(%+v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// --- key bindings -----------------------------------------------------------

func runeKey(r rune) *tcell.EventKey { return tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone) }
func namedKey(k tcell.Key) *tcell.EventKey {
	return tcell.NewEventKey(k, 0, tcell.ModNone)
}

func TestListKeyBindings(t *testing.T) {
	cases := []struct {
		ev   *tcell.EventKey
		want action
	}{
		{namedKey(tcell.KeyEnter), actEnter},
		{namedKey(tcell.KeyRight), actOpenLinks},
		{runeKey('l'), actOpenLinks},
		{runeKey('q'), actQuit},
		{runeKey('n'), actNew},
		{runeKey('r'), actRename},
		{runeKey('d'), actDelete},
		{runeKey('a'), actApply},
		{runeKey('A'), actApplyAll},
		{runeKey('R'), actReload},
		{runeKey('t'), actTheme},
		// Navigation keys must fall through to the table.
		{runeKey('j'), actPass},
		{runeKey('k'), actPass},
		{runeKey('g'), actPass},
		{runeKey('G'), actPass},
		{namedKey(tcell.KeyDown), actPass},
		{namedKey(tcell.KeyUp), actPass},
	}
	for _, c := range cases {
		if got := routeList(c.ev); got != c.want {
			t.Errorf("routeList(%v/%q) = %v, want %v", c.ev.Key(), c.ev.Rune(), got, c.want)
		}
	}
}

func TestDetailKeyBindings(t *testing.T) {
	cases := []struct {
		ev   *tcell.EventKey
		want action
	}{
		{namedKey(tcell.KeyEscape), actCloseLinks},
		{namedKey(tcell.KeyLeft), actCloseLinks},
		{runeKey('h'), actCloseLinks},
		{runeKey('a'), actAddLink},
		{runeKey('d'), actRemoveLink},
		{runeKey('A'), actApply},
		{runeKey('q'), actQuit},
		{runeKey('j'), actPass},
		{runeKey('k'), actPass},
		// 'n' and 'r' belong to the list only; they must not act here.
		{runeKey('n'), actPass},
		{runeKey('r'), actPass},
	}
	for _, c := range cases {
		if got := routeDetail(c.ev); got != c.want {
			t.Errorf("routeDetail(%v/%q) = %v, want %v", c.ev.Key(), c.ev.Rune(), got, c.want)
		}
	}
}

// 'd' must mean two different things depending on the page, and neither may
// leak into the other.
func TestDeleteKeyIsPageScoped(t *testing.T) {
	if routeList(runeKey('d')) != actDelete {
		t.Error("d on the list should delete the workspace")
	}
	if routeDetail(runeKey('d')) != actRemoveLink {
		t.Error("d on the link view should remove the link")
	}
}

// --- page adapter -----------------------------------------------------------

// core.Widget carries neither HandleKey nor HandleMouse, so embedding it in the
// page adapter silently hides them on the wrapped widget and the app can no
// longer deliver keys or clicks. Both must be forwarded.
func TestPageAdapterForwardsInput(t *testing.T) {
	var _ core.KeyHandler = (*page)(nil)
	var _ core.MouseHandler = (*page)(nil)

	c, _ := newCtl(t)
	for _, n := range []string{"alpha", "beta", "gamma"} {
		c.Create(n)
	}
	u := &ui{ctl: c}
	u.buildList()
	u.fillList()
	p := &page{Widget: u.list, name: "Workspaces"}

	if got := u.selectedName(); got != "alpha" {
		t.Fatalf("cursor starts at %q, want alpha", got)
	}

	// Arrow keys must reach the table through the adapter.
	if !p.HandleKey(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)) {
		t.Fatal("adapter did not forward KeyDown to the table")
	}
	if got := u.selectedName(); got != "beta" {
		t.Errorf("after Down, cursor = %q, want beta", got)
	}

	// Vim keys are handled by the table itself.
	p.HandleKey(tcell.NewEventKey(tcell.KeyRune, 'j', tcell.ModNone))
	if got := u.selectedName(); got != "gamma" {
		t.Errorf("after j, cursor = %q, want gamma", got)
	}
	p.HandleKey(tcell.NewEventKey(tcell.KeyRune, 'k', tcell.ModNone))
	if got := u.selectedName(); got != "beta" {
		t.Errorf("after k, cursor = %q, want beta", got)
	}
	p.HandleKey(tcell.NewEventKey(tcell.KeyRune, 'g', tcell.ModNone))
	if got := u.selectedName(); got != "alpha" {
		t.Errorf("after g, cursor = %q, want alpha", got)
	}
	p.HandleKey(tcell.NewEventKey(tcell.KeyRune, 'G', tcell.ModNone))
	if got := u.selectedName(); got != "gamma" {
		t.Errorf("after G, cursor = %q, want gamma", got)
	}

	// A click must select the row under the pointer.
	u.list.SetRect(0, 0, 80, 10)
	u.list.SelectRow(0)
	consumed, _ := p.HandleMouse(core.MouseLeftClick,
		tcell.NewEventMouse(2, 2, tcell.Button1, tcell.ModNone))
	if !consumed {
		t.Fatal("adapter did not forward a click to the table")
	}
	if u.selectedName() == "alpha" {
		t.Error("click did not move the cursor off the first row")
	}
}

// selectedName must read the cursor, not the multi-select marks.
func TestSelectedNameTracksCursorNotMarks(t *testing.T) {
	c, _ := newCtl(t)
	c.Create("first")
	c.Create("second")
	u := &ui{ctl: c}
	u.buildList()
	u.fillList()

	u.list.SelectRow(1)
	if got := u.selectedName(); got != "second" {
		t.Errorf("selectedName = %q, want second", got)
	}
	// Multi-select marks must not influence it.
	if got := len(u.list.GetSelectedRows()); got != 0 {
		t.Errorf("unexpected multi-select marks: %d", got)
	}
}

// A modal must receive every key. Letting shortcuts run first meant typing a
// name containing j, k, g or G moved the table behind the form instead.
func TestModalOwnsTheKeyboard(t *testing.T) {
	for _, r := range []rune{'j', 'k', 'g', 'G', 'n', 'd', 'q', 'a', 'e'} {
		tgt, _ := dispatch(true, false, runeKey(r))
		if tgt != toModal {
			t.Errorf("with a modal open, %q routed to %v, want toModal", r, tgt)
		}
	}
	for _, k := range []tcell.Key{tcell.KeyEnter, tcell.KeyEscape, tcell.KeyTab, tcell.KeyDown} {
		if tgt, _ := dispatch(true, false, namedKey(k)); tgt != toModal {
			t.Errorf("with a modal open, key %v routed to %v, want toModal", k, tgt)
		}
	}
}

func TestDispatchWithoutModal(t *testing.T) {
	// Shortcuts are ours.
	if tgt, a := dispatch(false, false, runeKey('n')); tgt != toShortcut || a != actNew {
		t.Errorf("n routed to %v/%v, want toShortcut/actNew", tgt, a)
	}
	// Navigation belongs to the table.
	for _, r := range []rune{'j', 'k', 'g', 'G'} {
		if tgt, _ := dispatch(false, false, runeKey(r)); tgt != toPage {
			t.Errorf("%q routed to %v, want toPage", r, tgt)
		}
	}
	if tgt, _ := dispatch(false, false, namedKey(tcell.KeyDown)); tgt != toPage {
		t.Error("Down should reach the table")
	}
	// The detail page has its own map.
	if _, a := dispatch(false, true, runeKey('a')); a != actAddLink {
		t.Errorf("a on the detail page = %v, want actAddLink", a)
	}
	if _, a := dispatch(false, false, runeKey('a')); a != actApply {
		t.Errorf("a on the list = %v, want actApply", a)
	}
}

// Users look for "edit", so e is an alias for opening the link view.
func TestEditAliasOpensLinks(t *testing.T) {
	for _, r := range []rune{'l', 'e'} {
		if got := routeList(runeKey(r)); got != actOpenLinks {
			t.Errorf("routeList(%q) = %v, want actOpenLinks", r, got)
		}
	}
}

func TestControllerUpdateLink(t *testing.T) {
	c, root := newCtl(t)
	oldSrc := srcDir(t, "old")
	newSrc := srcDir(t, "new")
	c.Create("w")
	c.AddLink("w", oldSrc, "api")

	// Repoint the source, keeping the alias.
	if !c.UpdateLink("w", "api", newSrc, "api") {
		t.Fatalf("UpdateLink: %s", mustMsg(c))
	}
	if got, _ := os.Readlink(filepath.Join(root, "w", "api")); got != newSrc {
		t.Errorf("after repoint, target = %q, want %q", got, newSrc)
	}

	// Rename the alias: the old symlink must be pruned by the reconcile.
	if !c.UpdateLink("w", "api", newSrc, "backend") {
		t.Fatalf("UpdateLink rename: %s", mustMsg(c))
	}
	if _, err := os.Lstat(filepath.Join(root, "w", "api")); !os.IsNotExist(err) {
		t.Error("old alias symlink survived the rename")
	}
	if got, _ := os.Readlink(filepath.Join(root, "w", "backend")); got != newSrc {
		t.Errorf("renamed alias target = %q, want %q", got, newSrc)
	}
	if links := c.Links("w"); len(links) != 1 || links[0].Alias != "backend" {
		t.Errorf("links = %+v", links)
	}
}

func TestControllerUpdateLinkRejectsClash(t *testing.T) {
	c, _ := newCtl(t)
	a, b := srcDir(t, "a"), srcDir(t, "b")
	c.Create("w")
	c.AddLink("w", a, "one")
	c.AddLink("w", b, "two")

	if c.UpdateLink("w", "one", a, "two") {
		t.Error("UpdateLink accepted an alias already in use")
	}
	if _, failed := c.Status(); !failed {
		t.Error("clash not reported as a failure")
	}
	if links := c.Links("w"); len(links) != 2 {
		t.Errorf("links changed despite the rejection: %+v", links)
	}
}

// The edit form should show the path as configured, not its expansion.
func TestLinksExposeRawSource(t *testing.T) {
	c, _ := newCtl(t)
	c.Create("w")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	c.AddLink("w", "~/some-project", "proj")

	links := c.Links("w")
	if len(links) != 1 {
		t.Fatalf("links = %+v", links)
	}
	if links[0].RawSrc != "~/some-project" {
		t.Errorf("RawSrc = %q, want the configured ~ form", links[0].RawSrc)
	}
	if links[0].Src != filepath.Join(home, "some-project") {
		t.Errorf("Src = %q, want the expanded path", links[0].Src)
	}
}

func TestDetailKeyBindingsIncludeEdit(t *testing.T) {
	if got := routeDetail(runeKey('e')); got != actEditLink {
		t.Errorf("routeDetail(e) = %v, want actEditLink", got)
	}
	// 'e' means different things per page: open links vs edit a link.
	if got := routeList(runeKey('e')); got != actOpenLinks {
		t.Errorf("routeList(e) = %v, want actOpenLinks", got)
	}
}

// A freshly filled table leaves the cursor on the header row, where
// SelectedRow reports -1 and every action on the selection quietly does
// nothing. Filling must place the cursor on the first data row.
func TestFilledTableSelectsFirstDataRow(t *testing.T) {
	c, _ := newCtl(t)
	src := srcDir(t, "api")
	c.Create("w")
	c.AddLink("w", src, "api")

	u := &ui{ctl: c}
	u.buildList()
	u.fillList()
	if got := u.list.SelectedRow(); got != 0 {
		t.Errorf("list cursor = %d after fill, want 0", got)
	}
	if got := u.selectedName(); got != "w" {
		t.Errorf("selectedName = %q, want w", got)
	}

	// The detail table is the one that regressed.
	u.detailFor = "w"
	u.links = newDetailTable()
	u.refreshDetail()
	if got := u.links.SelectedRow(); got != 0 {
		t.Errorf("detail cursor = %d after fill, want 0", got)
	}
	link, ok := u.selectedLink()
	if !ok || link.Alias != "api" {
		t.Errorf("selectedLink = %+v, ok=%v, want api", link, ok)
	}
}

// Unmanaged entries are listed after the links; the cursor parked on one must
// not resolve to a link.
func TestForeignRowIsNotALink(t *testing.T) {
	c, root := newCtl(t)
	src := srcDir(t, "api")
	c.Create("w")
	c.AddLink("w", src, "api")
	if err := os.WriteFile(filepath.Join(root, "w", "NOTES.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.Refresh()

	u := &ui{ctl: c, detailFor: "w"}
	u.links = newDetailTable()
	u.refreshDetail()

	u.links.SelectRow(0)
	if link, ok := u.selectedLink(); !ok || link.Alias != "api" {
		t.Errorf("row 0 = %+v ok=%v, want the api link", link, ok)
	}
	u.links.SelectRow(1) // NOTES.md
	if _, ok := u.selectedLink(); ok {
		t.Error("an unmanaged entry was reported as a link")
	}
}

// Enter invokes the modal's submit handler but does not dismiss the dialog;
// the handler has to close it. Without that the confirmation stayed on screen
// after confirming.
func TestConfirmModalClosesOnEnter(t *testing.T) {
	var order []string
	m := confirmModal("Delete workspace", "Delete \"x\"?", func() {
		order = append(order, "action")
	})
	m.SetOnClose(func() { order = append(order, "close") })

	if !m.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)) {
		t.Fatal("Enter was not handled")
	}
	if len(order) != 2 || order[0] != "close" || order[1] != "action" {
		t.Fatalf("order = %v, want [close action]", order)
	}
}

// Esc must dismiss without running the action.
func TestConfirmModalCancels(t *testing.T) {
	var ran []string
	m := confirmModal("Delete workspace", "Delete \"x\"?", func() {
		ran = append(ran, "action")
	})
	m.SetOnClose(func() { ran = append(ran, "close") })

	m.HandleKey(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	for _, r := range ran {
		if r == "action" {
			t.Fatal("Esc ran the confirmed action")
		}
	}
	if len(ran) == 0 {
		t.Error("Esc did not dismiss the dialog")
	}
}

// dado defines a StatusBar in both components and layout. The top bar used to
// be created from one package and type-asserted to the other, so the comma-ok
// assertion silently failed and no status or error ever reached the screen.
// The ui now keeps a typed reference; this pins the type it must be.
func TestStatusBarTypeRoundTrips(t *testing.T) {
	bar := layout.NewStatusBar()
	app := layout.NewApp(layout.AppConfig{TopBar: bar})

	got, ok := app.TopBar().(*layout.StatusBar)
	if !ok {
		t.Fatalf("TopBar() is %T, not *layout.StatusBar", app.TopBar())
	}
	if got != bar {
		t.Error("TopBar() returned a different status bar than was configured")
	}

	// The section type must match too, or nothing renders.
	bar.SetSections([]layout.StatusSection{{Text: "hello"}})
	if bar.SectionCount() != 1 {
		t.Errorf("SectionCount = %d, want 1", bar.SectionCount())
	}
	if bar.GetSection(0).Text != "hello" {
		t.Errorf("section text = %q", bar.GetSection(0).Text)
	}
}

// Tab only goes somewhere when a form has more than one field. Advertising it
// on the single-field New workspace dialog reads as a broken binding.
func TestFormHintsMatchFieldCount(t *testing.T) {
	has := func(hints []components.KeyHint, key string) bool {
		for _, h := range hints {
			if h.Key == key {
				return true
			}
		}
		return false
	}

	one := formHints(1)
	if has(one, "Tab") {
		t.Error("single-field form advertises Tab")
	}
	for _, k := range []string{"^U", "↵", "Esc"} {
		if !has(one, k) {
			t.Errorf("single-field form missing %s", k)
		}
	}

	two := formHints(2)
	if !has(two, "Tab") {
		t.Error("multi-field form should advertise Tab")
	}
}
