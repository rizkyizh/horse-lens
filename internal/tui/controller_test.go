package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/atterpac/dado/core"

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
