// Package tui is the full-screen interface. Every action the CLI offers is
// reachable here too.
//
// All state and rules live in Controller, which knows nothing about dado or
// tcell; the widgets in tui.go only render it and forward keys. That keeps the
// behaviour testable without a terminal.
package tui

import (
	"fmt"
	"sort"

	"github.com/rizkyizh/horse-lens/internal/store"
	"github.com/rizkyizh/horse-lens/internal/workspace"
)

// LinkRow is one link of a workspace, with its reconciliation state. Src is
// resolved for display; RawSrc is what the config holds, which is what an edit
// form should show.
type LinkRow struct {
	Alias  string
	Src    string
	RawSrc string
	State  string
}

// Controller holds the screen state and performs every mutation through the
// store, recording a human-readable status message for the UI to show.
type Controller struct {
	st     *store.Store
	rows   []store.Summary
	status string
	failed bool
}

func NewController(st *store.Store) *Controller { return &Controller{st: st} }

// Rows is the workspace list.
func (c *Controller) Rows() []store.Summary { return c.rows }

// Status is the last message, and whether it reports a failure.
func (c *Controller) Status() (string, bool) { return c.status, c.failed }

func (c *Controller) ok(format string, args ...any) {
	c.status, c.failed = fmt.Sprintf(format, args...), false
}

func (c *Controller) fail(err error) {
	c.status, c.failed = err.Error(), true
}

// Refresh re-reads the config from disk and re-plans every workspace.
func (c *Controller) Refresh() {
	if err := c.st.Reload(); err != nil {
		c.fail(err)
		return
	}
	rows, err := c.st.Summaries()
	if err != nil {
		c.fail(err)
		return
	}
	c.rows = rows
}

// Names lists workspace names in display order.
func (c *Controller) Names() []string {
	out := make([]string, 0, len(c.rows))
	for _, r := range c.rows {
		out = append(out, r.Name)
	}
	return out
}

// Row returns the summary at an index, if it exists.
func (c *Controller) Row(i int) (store.Summary, bool) {
	if i < 0 || i >= len(c.rows) {
		return store.Summary{}, false
	}
	return c.rows[i], true
}

// Dir is the directory a workspace materialises into.
func (c *Controller) Dir(name string) string {
	ws, err := c.st.Resolve(name)
	if err != nil {
		return ""
	}
	return ws.Dir
}

// Links describes a workspace's links and how each currently stands.
func (c *Controller) Links(name string) []LinkRow {
	p, err := c.st.Plan(name)
	if err != nil {
		c.fail(err)
		return nil
	}
	var out []LinkRow
	for _, a := range p.Actions {
		var state string
		switch {
		case a.Kind == workspace.ActionForeign:
			continue // not a link; shown separately
		case a.Dangling:
			state = "source missing"
		case a.Kind == workspace.ActionUnchanged:
			state = "ok"
		case a.Kind == workspace.ActionRemove:
			state = "stale"
		default:
			state = a.Kind.String() + " pending"
		}
		src := a.Target
		if src == "" {
			src = a.Current
		}
		row := LinkRow{Alias: a.Alias, Src: src, RawSrc: src, State: state}
		if l, ok := c.st.Link(name, a.Alias); ok {
			row.RawSrc = l.Src
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Alias < out[j].Alias })
	return out
}

// Foreign lists entries in the workspace directory that horselens does not
// manage. They are never modified, but the user should be able to see them.
func (c *Controller) Foreign(name string) []string {
	p, err := c.st.Plan(name)
	if err != nil {
		return nil
	}
	var out []string
	for _, a := range p.Actions {
		if a.Kind == workspace.ActionForeign {
			out = append(out, a.Alias)
		}
	}
	sort.Strings(out)
	return out
}

// --- actions ----------------------------------------------------------------

func (c *Controller) Apply(name string) {
	p, err := c.st.Apply(name)
	if err != nil {
		c.fail(err)
		return
	}
	if !p.Changes() {
		c.ok("%s already in sync", name)
	} else {
		cnt := p.Counts()
		c.ok("applied %s (+%d ~%d -%d)", name,
			cnt[workspace.ActionCreate], cnt[workspace.ActionRetarget], cnt[workspace.ActionRemove])
	}
	c.Refresh()
}

func (c *Controller) ApplyAll() {
	plans, err := c.st.ApplyAll()
	if err != nil {
		c.fail(err)
		return
	}
	changed := 0
	for _, p := range plans {
		if p.Changes() {
			changed++
		}
	}
	c.ok("applied %d workspaces, %d changed", len(plans), changed)
	c.Refresh()
}

// Root is the directory all workspaces materialise under, unless a workspace
// overrides it with its own path.
func (c *Controller) Root() string { return c.st.Paths().Root }

// RootOverridden reports whether a flag or environment variable is outranking
// the config's root key, which makes editing it here take no effect.
func (c *Controller) RootOverridden() (bool, string) {
	src := c.st.RootSource()
	return src.OverridesRoot(), string(src)
}

// SetRoot moves the workspace root and records it in the config.
func (c *Controller) SetRoot(raw string) bool {
	old := c.st.Paths().Root
	moved, err := c.st.SetRoot(raw)
	if err != nil {
		c.fail(err)
		return false
	}
	// An override means the saved key will not take effect, and nothing was
	// moved; say that instead of reporting a change that did not happen.
	if overridden, src := c.RootOverridden(); overridden {
		c.fail(fmt.Errorf("saved to the config, but %s overrides it — unset it for this to take effect", src))
		c.Refresh()
		return true
	}
	switch {
	case c.st.Paths().Root == old:
		c.ok("root unchanged")
	case moved:
		c.ok("root moved to %s", c.st.Paths().Root)
	default:
		c.ok("root is now %s — %s was left in place", c.st.Paths().Root, old)
	}
	c.Refresh()
	return true
}

func (c *Controller) Create(name string) bool {
	if err := c.st.Create(name); err != nil {
		c.fail(err)
		return false
	}
	c.ok("created %q — press l to add projects", name)
	c.Refresh()
	return true
}

func (c *Controller) Rename(oldName, newName string) bool {
	if err := c.st.Rename(oldName, newName); err != nil {
		c.fail(err)
		return false
	}
	c.ok("renamed %q to %q", oldName, newName)
	c.Refresh()
	return true
}

func (c *Controller) Delete(name string, force bool) bool {
	if err := c.st.Delete(name, force); err != nil {
		c.fail(err)
		return false
	}
	c.ok("deleted %q", name)
	c.Refresh()
	return true
}

func (c *Controller) AddLink(name, src, alias string) bool {
	resolved, abs, err := c.st.AddLink(name, src, alias)
	if err != nil {
		c.fail(err)
		return false
	}
	if _, applyErr := c.st.Apply(name); applyErr != nil {
		c.fail(applyErr)
		return false
	}
	if store.SourceExists(abs) {
		c.ok("added %s -> %s", resolved, abs)
	} else {
		c.ok("added %s -> %s (source does not exist yet)", resolved, abs)
	}
	c.Refresh()
	return true
}

func (c *Controller) UpdateLink(name, oldAlias, src, alias string) bool {
	resolved, abs, err := c.st.UpdateLink(name, oldAlias, src, alias)
	if err != nil {
		c.fail(err)
		return false
	}
	if _, applyErr := c.st.Apply(name); applyErr != nil {
		c.fail(applyErr)
		return false
	}
	if store.SourceExists(abs) {
		c.ok("updated %s -> %s", resolved, abs)
	} else {
		c.ok("updated %s -> %s (source does not exist yet)", resolved, abs)
	}
	c.Refresh()
	return true
}

func (c *Controller) RemoveLink(name, alias string) bool {
	if err := c.st.RemoveLink(name, alias); err != nil {
		c.fail(err)
		return false
	}
	if _, err := c.st.Apply(name); err != nil {
		c.fail(err)
		return false
	}
	c.ok("removed %s from %s", alias, name)
	c.Refresh()
	return true
}
