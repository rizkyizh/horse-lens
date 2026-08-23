package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// ActionKind is what reconciliation intends to do with one directory entry.
type ActionKind int

const (
	// ActionUnchanged means the symlink already points where it should.
	ActionUnchanged ActionKind = iota
	// ActionCreate means the symlink is missing.
	ActionCreate
	// ActionRetarget means a managed symlink points somewhere else.
	ActionRetarget
	// ActionRemove means a symlink exists that the config no longer lists.
	ActionRemove
	// ActionForeign means the entry is not a symlink. These are reported and
	// never touched, so real files inside a workspace are always safe.
	ActionForeign
)

func (k ActionKind) Symbol() string {
	switch k {
	case ActionCreate:
		return "+"
	case ActionRetarget:
		return "~"
	case ActionRemove:
		return "-"
	case ActionForeign:
		return "!"
	default:
		return "="
	}
}

func (k ActionKind) String() string {
	switch k {
	case ActionCreate:
		return "create"
	case ActionRetarget:
		return "retarget"
	case ActionRemove:
		return "remove"
	case ActionForeign:
		return "foreign"
	default:
		return "unchanged"
	}
}

// Action is one planned change to one entry in the workspace directory.
type Action struct {
	Kind    ActionKind
	Alias   string
	Target  string // desired symlink target, for create/retarget/unchanged
	Current string // existing symlink target, for retarget/remove
	// Dangling reports that Target does not currently exist. It is advisory:
	// the symlink is still created, because the source may appear later.
	Dangling bool
}

// Plan is the full set of changes needed to bring Dir in line with the config.
type Plan struct {
	Name    string
	Dir     string
	Actions []Action
}

// Changes reports whether applying the plan would modify anything.
func (p Plan) Changes() bool {
	for _, a := range p.Actions {
		switch a.Kind {
		case ActionCreate, ActionRetarget, ActionRemove:
			return true
		}
	}
	return false
}

// Counts tallies actions by kind, for summaries.
func (p Plan) Counts() map[ActionKind]int {
	m := make(map[ActionKind]int, len(p.Actions))
	for _, a := range p.Actions {
		m[a.Kind]++
	}
	return m
}

// BuildPlan diffs the workspace directory against the desired link set. It
// only reads the filesystem; nothing is modified.
func BuildPlan(ws Workspace) (Plan, error) {
	p := Plan{Name: ws.Name, Dir: ws.Dir}

	desired := make(map[string]Link, len(ws.Links))
	for _, l := range ws.Links {
		desired[l.Alias] = l
	}

	// Existing symlinks, plus anything that is not a symlink.
	existing := map[string]string{}
	entries, err := os.ReadDir(ws.Dir)
	if err != nil && !os.IsNotExist(err) {
		return Plan{}, fmt.Errorf("read workspace dir %s: %w", ws.Dir, err)
	}
	for _, e := range entries {
		name := e.Name()
		full := filepath.Join(ws.Dir, name)
		info, err := os.Lstat(full)
		if err != nil {
			return Plan{}, fmt.Errorf("inspect %s: %w", full, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			p.Actions = append(p.Actions, Action{Kind: ActionForeign, Alias: name})
			continue
		}
		target, err := os.Readlink(full)
		if err != nil {
			return Plan{}, fmt.Errorf("read symlink %s: %w", full, err)
		}
		existing[name] = target
	}

	for _, l := range ws.Links {
		dangling := false
		if _, err := os.Stat(l.Src); err != nil {
			dangling = true
		}
		current, ok := existing[l.Alias]
		switch {
		case !ok:
			p.Actions = append(p.Actions, Action{
				Kind: ActionCreate, Alias: l.Alias, Target: l.Src, Dangling: dangling,
			})
		case current != l.Src:
			p.Actions = append(p.Actions, Action{
				Kind: ActionRetarget, Alias: l.Alias, Target: l.Src,
				Current: current, Dangling: dangling,
			})
		default:
			p.Actions = append(p.Actions, Action{
				Kind: ActionUnchanged, Alias: l.Alias, Target: l.Src, Dangling: dangling,
			})
		}
	}

	// Managed symlinks the config no longer lists.
	var stale []string
	for alias := range existing {
		if _, want := desired[alias]; !want {
			stale = append(stale, alias)
		}
	}
	sort.Strings(stale)
	for _, alias := range stale {
		p.Actions = append(p.Actions, Action{
			Kind: ActionRemove, Alias: alias, Current: existing[alias],
		})
	}

	return p, nil
}

// Apply executes the plan. Foreign entries are skipped, and every removal
// re-checks that the target is still a symlink immediately before unlinking,
// so a file that appeared since the plan was built is never destroyed.
func (p Plan) Apply() error {
	if err := os.MkdirAll(p.Dir, 0o755); err != nil {
		return fmt.Errorf("create workspace dir %s: %w", p.Dir, err)
	}
	for _, a := range p.Actions {
		full := filepath.Join(p.Dir, a.Alias)
		switch a.Kind {
		case ActionCreate:
			if err := os.Symlink(a.Target, full); err != nil {
				return fmt.Errorf("link %s -> %s: %w", full, a.Target, err)
			}
		case ActionRetarget, ActionRemove:
			if err := removeSymlink(full); err != nil {
				return err
			}
			if a.Kind == ActionRetarget {
				if err := os.Symlink(a.Target, full); err != nil {
					return fmt.Errorf("link %s -> %s: %w", full, a.Target, err)
				}
			}
		}
	}
	return nil
}

// removeSymlink unlinks path only if it is still a symlink.
func removeSymlink(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("refusing to remove %s: not a symlink", path)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

// Destroy removes the workspace directory. Only symlinks are unlinked; if any
// real file or directory is present the call fails and lists them, unless
// force is set.
func Destroy(ws Workspace, force bool) error {
	entries, err := os.ReadDir(ws.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read workspace dir %s: %w", ws.Dir, err)
	}

	var foreign []string
	for _, e := range entries {
		full := filepath.Join(ws.Dir, e.Name())
		info, err := os.Lstat(full)
		if err != nil {
			return fmt.Errorf("inspect %s: %w", full, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			foreign = append(foreign, e.Name())
		}
	}

	if len(foreign) > 0 && !force {
		sort.Strings(foreign)
		return fmt.Errorf(
			"workspace %q contains files that are not symlinks: %v\n"+
				"these are not managed by horselens; move them out or re-run with --force",
			ws.Name, foreign)
	}

	if force {
		if err := os.RemoveAll(ws.Dir); err != nil {
			return fmt.Errorf("remove %s: %w", ws.Dir, err)
		}
		return nil
	}

	for _, e := range entries {
		if err := removeSymlink(filepath.Join(ws.Dir, e.Name())); err != nil {
			return err
		}
	}
	if err := os.Remove(ws.Dir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", ws.Dir, err)
	}
	return nil
}
