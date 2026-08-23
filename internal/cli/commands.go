package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/rizkyizh/horse-lens/internal/config"
	"github.com/rizkyizh/horse-lens/internal/shell"
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

	paths, f, err := a.load()
	if err != nil {
		return err
	}
	all, err := workspace.ResolveAll(f, paths.Root)
	if err != nil {
		return err
	}

	if len(all) == 0 && !a.json {
		fmt.Fprintf(a.out, "no workspaces yet\n\n  config: %s\n  root:   %s\n\ncreate one:  horselens new <name>\n",
			paths.Config, paths.Root)
		return nil
	}

	rows := make([]jsonWorkspace, 0, len(all))
	for _, ws := range all {
		p, err := workspace.BuildPlan(ws)
		if err != nil {
			return err
		}
		c := p.Counts()
		row := jsonWorkspace{
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

	if a.json {
		return json.NewEncoder(a.out).Encode(rows)
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
	fmt.Fprintf(a.out, "\nconfig: %s\nroot:   %s\n", paths.Config, paths.Root)
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

	paths, f, err := a.load()
	if err != nil {
		return err
	}
	targets, err := a.selectWorkspaces(f, paths.Root, fs.Arg(0))
	if err != nil {
		return err
	}

	var plans []workspace.Plan
	for _, ws := range targets {
		p, err := workspace.BuildPlan(ws)
		if err != nil {
			return err
		}
		plans = append(plans, p)
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
	paths, f, err := a.load()
	if err != nil {
		return err
	}
	ws, err := a.resolveOne(f, paths.Root, fs.Arg(0))
	if err != nil {
		return err
	}
	fmt.Fprintln(a.out, ws.Dir)
	return nil
}

// --- mutating ---------------------------------------------------------------

func (a *app) cmdNew(args []string) error {
	fs := a.newFlags("new")
	if err := a.parse(fs, args); err != nil {
		return err
	}
	if err := argsExactly(fs, 1, "new <name>"); err != nil {
		return err
	}
	name := fs.Arg(0)
	if err := workspace.ValidateName(name); err != nil {
		return err
	}

	paths, f, err := a.load()
	if err != nil {
		return err
	}
	if _, exists := f.Find(name); exists {
		return fmt.Errorf("workspace %q already exists", name)
	}
	f.Workspaces = append(f.Workspaces, config.Workspace{Name: name})
	if err := config.Save(paths.Config, f); err != nil {
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
	name, src := fs.Arg(0), fs.Arg(1)

	abs, err := config.ExpandPath(src)
	if err != nil {
		return err
	}
	alias := fs.Arg(2)
	if alias == "" {
		alias = filepath.Base(abs)
	}
	if err := workspace.ValidateAlias(alias); err != nil {
		return fmt.Errorf("%w\n(pass an explicit alias: horselens add %s %s <alias>)", err, name, src)
	}

	paths, f, err := a.load()
	if err != nil {
		return err
	}
	cw, ok := f.Find(name)
	if !ok {
		return fmt.Errorf("no workspace named %q (create it: horselens new %s)", name, name)
	}
	for _, l := range cw.Links {
		if l.Alias == alias {
			return fmt.Errorf("workspace %q already has an alias %q", name, alias)
		}
	}
	cw.Links = append(cw.Links, config.Link{Src: src, Alias: alias})
	if err := config.Save(paths.Config, f); err != nil {
		return err
	}

	fmt.Fprintf(a.out, "added %s -> %s\n", alias, abs)
	if _, err := statPath(abs); err != nil {
		fmt.Fprintf(a.out, "warning: %s does not exist yet\n", abs)
	}
	return a.applyNamed(paths, f, name)
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

	paths, f, err := a.load()
	if err != nil {
		return err
	}
	cw, ok := f.Find(name)
	if !ok {
		return fmt.Errorf("no workspace named %q", name)
	}
	found := false
	for i, l := range cw.Links {
		if l.Alias == alias {
			cw.Links = append(cw.Links[:i], cw.Links[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("workspace %q has no alias %q", name, alias)
	}
	if err := config.Save(paths.Config, f); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "removed %s from %s\n", alias, name)
	return a.applyNamed(paths, f, name)
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
	if err := workspace.ValidateName(newName); err != nil {
		return err
	}

	paths, f, err := a.load()
	if err != nil {
		return err
	}
	cw, ok := f.Find(oldName)
	if !ok {
		return fmt.Errorf("no workspace named %q", oldName)
	}
	if oldName == newName {
		return nil
	}
	if _, exists := f.Find(newName); exists {
		return fmt.Errorf("workspace %q already exists", newName)
	}

	// Tear the old directory down before renaming, so no orphan is left
	// behind. Symlinks are cheap to recreate under the new name.
	oldWS, err := workspace.Resolve(*cw, paths.Root)
	if err != nil {
		return err
	}
	if err := workspace.Destroy(oldWS, false); err != nil {
		return fmt.Errorf("%w\n(the rename was not applied)", err)
	}

	cw.Name = newName
	if err := config.Save(paths.Config, f); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "renamed %q -> %q\n", oldName, newName)
	return a.applyNamed(paths, f, newName)
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

	paths, f, err := a.load()
	if err != nil {
		return err
	}
	ws, err := a.resolveOne(f, paths.Root, name)
	if err != nil {
		return err
	}
	if err := workspace.Destroy(ws, a.force); err != nil {
		return err
	}
	f.Remove(name)
	if err := config.Save(paths.Config, f); err != nil {
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
	paths, f, err := a.load()
	if err != nil {
		return err
	}
	targets, err := a.selectWorkspaces(f, paths.Root, fs.Arg(0))
	if err != nil {
		return err
	}
	for _, ws := range targets {
		if err := a.applyOne(ws); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) cmdEnter(args []string) error {
	fs := a.newFlags("enter")
	if err := a.parse(fs, args); err != nil {
		return err
	}
	if err := argsExactly(fs, 1, "enter <name>"); err != nil {
		return err
	}
	paths, f, err := a.load()
	if err != nil {
		return err
	}
	ws, err := a.resolveOne(f, paths.Root, fs.Arg(0))
	if err != nil {
		return err
	}
	if err := a.applyOne(ws); err != nil {
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

// selectWorkspaces returns one named workspace, or all of them when name is "".
func (a *app) selectWorkspaces(f *config.File, root, name string) ([]workspace.Workspace, error) {
	if name == "" {
		return workspace.ResolveAll(f, root)
	}
	ws, err := a.resolveOne(f, root, name)
	if err != nil {
		return nil, err
	}
	return []workspace.Workspace{ws}, nil
}

func (a *app) applyOne(ws workspace.Workspace) error {
	p, err := workspace.BuildPlan(ws)
	if err != nil {
		return err
	}
	if err := p.Apply(); err != nil {
		return err
	}
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
	return nil
}

func (a *app) applyNamed(paths config.Paths, f *config.File, name string) error {
	ws, err := a.resolveOne(f, paths.Root, name)
	if err != nil {
		return err
	}
	return a.applyOne(ws)
}
