package workspace

import (
	"fmt"

	"github.com/rizkyizh/horse-lens/internal/config"
)

// Link is a resolved symlink entry: Src is absolute, Alias is a bare filename.
type Link struct {
	Src   string
	Alias string
}

// Workspace is a validated workspace with its materialisation directory
// already resolved.
type Workspace struct {
	Name  string
	Dir   string
	Links []Link
}

// Resolve validates a config workspace and works out where it materialises.
// A per-workspace `path` wins over the global root; otherwise the workspace
// lives at root/<name>, verified to stay inside root.
func Resolve(w config.Workspace, root string) (Workspace, error) {
	if err := ValidateName(w.Name); err != nil {
		return Workspace{}, err
	}

	var dir string
	if w.Path != "" {
		p, err := config.ExpandPath(w.Path)
		if err != nil {
			return Workspace{}, fmt.Errorf("workspace %q: %w", w.Name, err)
		}
		dir = p
	} else {
		p, err := childPath(root, w.Name)
		if err != nil {
			return Workspace{}, fmt.Errorf("workspace %q: %w", w.Name, err)
		}
		dir = p
	}

	out := Workspace{Name: w.Name, Dir: dir}
	seen := make(map[string]struct{}, len(w.Links))
	for _, l := range w.Links {
		if err := ValidateAlias(l.Alias); err != nil {
			return Workspace{}, fmt.Errorf("workspace %q: %w", w.Name, err)
		}
		if _, dup := seen[l.Alias]; dup {
			return Workspace{}, fmt.Errorf("workspace %q: duplicate alias %q", w.Name, l.Alias)
		}
		seen[l.Alias] = struct{}{}

		src, err := config.ExpandPath(l.Src)
		if err != nil {
			return Workspace{}, fmt.Errorf("workspace %q, alias %q: %w", w.Name, l.Alias, err)
		}
		out.Links = append(out.Links, Link{Src: src, Alias: l.Alias})
	}
	return out, nil
}

// ResolveAll resolves every workspace in the file, failing on the first
// invalid one so a broken config never half-applies.
func ResolveAll(f *config.File, root string) ([]Workspace, error) {
	seen := make(map[string]struct{}, len(f.Workspaces))
	out := make([]Workspace, 0, len(f.Workspaces))
	for _, w := range f.Workspaces {
		if _, dup := seen[w.Name]; dup {
			return nil, fmt.Errorf("duplicate workspace name %q", w.Name)
		}
		seen[w.Name] = struct{}{}

		rw, err := Resolve(w, root)
		if err != nil {
			return nil, err
		}
		out = append(out, rw)
	}
	return out, nil
}
