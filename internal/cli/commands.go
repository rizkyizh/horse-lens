package cli

import (
	"encoding/json"
	"fmt"

	"github.com/rizkyizh/horse-lens/internal/shell"
	"github.com/rizkyizh/horse-lens/internal/store"
	"github.com/rizkyizh/horse-lens/internal/workspace"
)

// --- read-only --------------------------------------------------------------

type jsonWorkspace struct {
	Name     string `json:"name"`
	Dir      string `json:"dir"`
	Links    int    `json:"links"`
	Drift    int    `json:"drift"`
	Dangling int    `json:"dangling"`
	Foreign  int    `json:"foreign"`
}

func (a *app) cmdList(args []string) error {
	fs := a.newFlags("list", withJSON(a))
	if err := a.parse(fs, args); err != nil {
		return err
	}
	if err := argsExactly(fs, 0, "list [--json]"); err != nil {
		return err
	}
	st, err := a.store()
	if err != nil {
		return err
	}
	rows, err := st.Summaries()
	if err != nil {
		return err
	}

	if a.json {
		out := make([]jsonWorkspace, 0, len(rows))
		for _, r := range rows {
			out = append(out, jsonWorkspace{
				Name: r.Name, Dir: r.Dir, Links: r.Links,
				Drift: r.Drift, Dangling: r.Dangling, Foreign: r.Foreign,
			})
		}
		return json.NewEncoder(a.out).Encode(out)
	}

	p := st.Paths()
	if len(rows) == 0 {
		fmt.Fprintf(a.out,
			"no workspaces yet\n\n  config: %s\n  root:   %s\n\ncreate one:  horselens new <name>\n",
			p.Config, p.Root)
		return nil
	}
	for _, r := range rows {
		notes := joinNonEmpty([]string{
			ifPositive(r.Drift, plural(r.Drift, "change pending", "changes pending")),
			ifPositive(r.Dangling, plural(r.Dangling, "dangling", "dangling")),
			ifPositive(r.Foreign, plural(r.Foreign, "foreign file", "foreign files")),
		}, ", ")
		if notes == "" {
			notes = "in sync"
		}
		fmt.Fprintf(a.out, "%-20s %-8s %s\n", r.Name, plural(r.Links, "link", "links"), notes)
	}
	fmt.Fprintf(a.out, "\nconfig: %s\nroot:   %s\n", p.Config, p.Root)
	return nil
}

func ifPositive(n int, s string) string {
	if n > 0 {
		return s
	}
	return ""
}

func (a *app) cmdStatus(args []string) error {
	fs := a.newFlags("status", withJSON(a))
	if err := a.parse(fs, args); err != nil {
		return err
	}
	if err := argsAtMost(fs, 1, "status [name] [--json]"); err != nil {
		return err
	}
	st, err := a.store()
	if err != nil {
		return err
	}
	plans, err := a.plansFor(st, fs.Arg(0))
	if err != nil {
		return err
	}
	if a.json {
		return json.NewEncoder(a.out).Encode(plansToJSON(plans))
	}
	for i, p := range plans {
		if i > 0 {
			fmt.Fprintln(a.out)
		}
		a.printPlan(p)
	}
	return nil
}

func (a *app) cmdNew(args []string) error {
	fs := a.newFlags("new")
	if err := a.parse(fs, args); err != nil {
		return err
	}
	if err := argsExactly(fs, 1, "new <name>"); err != nil {
		return err
	}
	name := fs.Arg(0)

	st, err := a.store()
	if err != nil {
		return err
	}
	if err := st.Create(name); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "created %q\n\nadd a project:  horselens add %s <path>\n", name, name)
	return nil
}

func (a *app) cmdAdd(args []string) error {
	fs := a.newFlags("add")
	if err := a.parse(fs, args); err != nil {
		return err
	}
	if fs.NArg() < 2 || fs.NArg() > 3 {
		return fmt.Errorf("usage: horselens add <name> <src> [alias]")
	}
	name, src, alias := fs.Arg(0), fs.Arg(1), fs.Arg(2)

	st, err := a.store()
	if err != nil {
		return err
	}
	resolvedAlias, abs, err := st.AddLink(name, src, alias)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "added %s -> %s\n", resolvedAlias, abs)
	if !store.SourceExists(abs) {
		fmt.Fprintf(a.out, "warning: %s does not exist yet\n", abs)
	}
	return a.applyAndReport(st, name)
}

func (a *app) cmdRm(args []string) error {
	fs := a.newFlags("rm")
	if err := a.parse(fs, args); err != nil {
		return err
	}
	if err := argsExactly(fs, 2, "rm <name> <alias>"); err != nil {
		return err
	}
	name, alias := fs.Arg(0), fs.Arg(1)

	st, err := a.store()
	if err != nil {
		return err
	}
	if err := st.RemoveLink(name, alias); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "removed %s from %s\n", alias, name)
	return a.applyAndReport(st, name)
}

func (a *app) cmdRename(args []string) error {
	fs := a.newFlags("rename")
	if err := a.parse(fs, args); err != nil {
		return err
	}
	if err := argsExactly(fs, 2, "rename <old> <new>"); err != nil {
		return err
	}
	oldName, newName := fs.Arg(0), fs.Arg(1)

	st, err := a.store()
	if err != nil {
		return err
	}
	if err := st.Rename(oldName, newName); err != nil {
		return err
	}
	if oldName == newName {
		return nil
	}
	fmt.Fprintf(a.out, "renamed %q -> %q\n", oldName, newName)
	return a.applyAndReport(st, newName)
}

func (a *app) cmdDelete(args []string) error {
	fs := a.newFlags("delete", withForce(a))
	if err := a.parse(fs, args); err != nil {
		return err
	}
	if err := argsExactly(fs, 1, "delete <name> [--force]"); err != nil {
		return err
	}
	name := fs.Arg(0)

	st, err := a.store()
	if err != nil {
		return err
	}
	if err := st.Delete(name, a.force); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "deleted %q\n", name)
	return nil
}

func (a *app) cmdApply(args []string) error {
	fs := a.newFlags("apply")
	if err := a.parse(fs, args); err != nil {
		return err
	}
	if err := argsAtMost(fs, 1, "apply [name]"); err != nil {
		return err
	}
	st, err := a.store()
	if err != nil {
		return err
	}
	if name := fs.Arg(0); name != "" {
		return a.applyAndReport(st, name)
	}
	plans, err := st.ApplyAll()
	for _, p := range plans {
		a.reportPlan(p)
	}
	return err
}

func (a *app) cmdEnter(args []string) error {
	fs := a.newFlags("enter")
	if err := a.parse(fs, args); err != nil {
		return err
	}
	if err := argsExactly(fs, 1, "enter <name>"); err != nil {
		return err
	}
	st, err := a.store()
	if err != nil {
		return err
	}
	name := fs.Arg(0)
	if err := a.applyAndReport(st, name); err != nil {
		return err
	}
	ws, err := st.Resolve(name)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "entering %s (exit to leave)\n", ws.Dir)
	return shell.Enter(ws.Dir, ws.Name)
}

func (a *app) cmdShellInit(args []string) error {
	fs := a.newFlags("shell-init")
	if err := a.parse(fs, args); err != nil {
		return err
	}
	if err := argsExactly(fs, 1, "shell-init <bash|zsh|fish>"); err != nil {
		return err
	}
	src, err := shell.Init(fs.Arg(0))
	if err != nil {
		return err
	}
	fmt.Fprint(a.out, src)
	return nil
}

// --- shared helpers ---------------------------------------------------------

// store opens the shared service layer with the resolved overrides.
func (a *app) store() (*store.Store, error) { return store.Open(a.over) }

// plansFor builds plans for one workspace, or all of them when name is "".
func (a *app) plansFor(st *store.Store, name string) ([]workspace.Plan, error) {
	if name != "" {
		p, err := st.Plan(name)
		if err != nil {
			return nil, err
		}
		return []workspace.Plan{p}, nil
	}
	all, err := st.Workspaces()
	if err != nil {
		return nil, err
	}
	plans := make([]workspace.Plan, 0, len(all))
	for _, ws := range all {
		p, err := workspace.BuildPlan(ws)
		if err != nil {
			return nil, err
		}
		plans = append(plans, p)
	}
	return plans, nil
}

// applyAndReport reconciles one workspace and prints what changed.
func (a *app) applyAndReport(st *store.Store, name string) error {
	p, err := st.Apply(name)
	if err != nil {
		return err
	}
	a.reportPlan(p)
	return nil
}

func (a *app) reportPlan(p workspace.Plan) {
	for _, act := range p.Actions {
		switch act.Kind {
		case workspace.ActionCreate, workspace.ActionRetarget, workspace.ActionRemove:
			fmt.Fprintf(a.out, "  %s %s\n", act.Kind.Symbol(), act.Alias)
		case workspace.ActionForeign:
			fmt.Fprintf(a.out, "  %s %s (not a symlink — left alone)\n", act.Kind.Symbol(), act.Alias)
		}
		if act.Dangling && act.Kind != workspace.ActionRemove {
			fmt.Fprintf(a.out, "    warning: %s does not exist\n", act.Target)
		}
	}
}

type jsonAction struct {
	Action   string `json:"action"`
	Alias    string `json:"alias"`
	Target   string `json:"target,omitempty"`
	Current  string `json:"current,omitempty"`
	Dangling bool   `json:"dangling,omitempty"`
}

type jsonPlan struct {
	Name    string       `json:"name"`
	Dir     string       `json:"dir"`
	Changes bool         `json:"changes"`
	Actions []jsonAction `json:"actions"`
}

func plansToJSON(plans []workspace.Plan) []jsonPlan {
	out := make([]jsonPlan, 0, len(plans))
	for _, p := range plans {
		jp := jsonPlan{Name: p.Name, Dir: p.Dir, Changes: p.Changes(), Actions: []jsonAction{}}
		for _, a := range p.Actions {
			jp.Actions = append(jp.Actions, jsonAction{
				Action: a.Kind.String(), Alias: a.Alias,
				Target: a.Target, Current: a.Current, Dangling: a.Dangling,
			})
		}
		out = append(out, jp)
	}
	return out
}

func (a *app) printPlan(p workspace.Plan) {
	fmt.Fprintf(a.out, "%s\n  %s\n", p.Name, p.Dir)
	if len(p.Actions) == 0 {
		fmt.Fprintln(a.out, "  (no links)")
		return
	}
	for _, act := range p.Actions {
		line := fmt.Sprintf("  %s %-16s", act.Kind.Symbol(), act.Alias)
		switch act.Kind {
		case workspace.ActionRetarget:
			line += fmt.Sprintf("-> %s (was %s)", act.Target, act.Current)
		case workspace.ActionRemove:
			line += "(stale, will be removed)"
		case workspace.ActionForeign:
			line += "(not a symlink — left alone)"
		default:
			line += "-> " + act.Target
		}
		if act.Dangling {
			line += "  [source missing]"
		}
		fmt.Fprintln(a.out, line)
	}
	if !p.Changes() {
		fmt.Fprintln(a.out, "  in sync")
	}
}

func (a *app) cmdPath(args []string) error {
	fs := a.newFlags("path")
	if err := a.parse(fs, args); err != nil {
		return err
	}
	if err := argsExactly(fs, 1, "path <name>"); err != nil {
		return err
	}
	st, err := a.store()
	if err != nil {
		return err
	}
	ws, err := st.Resolve(fs.Arg(0))
	if err != nil {
		return err
	}
	fmt.Fprintln(a.out, ws.Dir)
	return nil
}
