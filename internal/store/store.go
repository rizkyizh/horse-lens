// Package store is the shared service layer over the config file and the
// symlink reconciler. Both the CLI and the TUI drive it, so the rules about
// duplicates, validation and when to apply live in exactly one place.
package store

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rizkyizh/horse-lens/internal/config"
	"github.com/rizkyizh/horse-lens/internal/workspace"
)

// Store holds the resolved locations and the parsed config file.
type Store struct {
	paths config.Paths
	file  *config.File
}

// Open resolves paths and loads the config.
func Open(over config.Overrides) (*Store, error) {
	paths, f, err := config.Resolve(over)
	if err != nil {
		return nil, err
	}
	return &Store{paths: paths, file: f}, nil
}

// Paths reports where the config file and workspace root live.
func (s *Store) Paths() config.Paths { return s.paths }

// Reload re-reads the config from disk, picking up external edits.
func (s *Store) Reload() error {
	f, err := config.Load(s.paths.Config)
	if err != nil {
		return err
	}
	s.file = f
	return nil
}

func (s *Store) save() error { return config.Save(s.paths.Config, s.file) }

// Workspaces resolves every workspace, failing if any is invalid.
func (s *Store) Workspaces() ([]workspace.Workspace, error) {
	return workspace.ResolveAll(s.file, s.paths.Root)
}

// Resolve looks up and validates a single workspace.
func (s *Store) Resolve(name string) (workspace.Workspace, error) {
	cw, ok := s.file.Find(name)
	if !ok {
		return workspace.Workspace{}, fmt.Errorf("no workspace named %q", name)
	}
	return workspace.Resolve(*cw, s.paths.Root)
}

// Links returns the configured links of a workspace, unresolved.
func (s *Store) Links(name string) ([]config.Link, error) {
	cw, ok := s.file.Find(name)
	if !ok {
		return nil, fmt.Errorf("no workspace named %q", name)
	}
	return cw.Links, nil
}

// Plan reports what Apply would change, without changing anything.
func (s *Store) Plan(name string) (workspace.Plan, error) {
	ws, err := s.Resolve(name)
	if err != nil {
		return workspace.Plan{}, err
	}
	return workspace.BuildPlan(ws)
}

// Apply reconciles one workspace and returns the plan that was executed.
func (s *Store) Apply(name string) (workspace.Plan, error) {
	ws, err := s.Resolve(name)
	if err != nil {
		return workspace.Plan{}, err
	}
	p, err := workspace.BuildPlan(ws)
	if err != nil {
		return workspace.Plan{}, err
	}
	if err := p.Apply(); err != nil {
		return p, err
	}
	return p, nil
}

// ApplyAll reconciles every workspace, stopping at the first failure.
func (s *Store) ApplyAll() ([]workspace.Plan, error) {
	all, err := s.Workspaces()
	if err != nil {
		return nil, err
	}
	plans := make([]workspace.Plan, 0, len(all))
	for _, ws := range all {
		p, err := workspace.BuildPlan(ws)
		if err != nil {
			return plans, err
		}
		if err := p.Apply(); err != nil {
			return plans, err
		}
		plans = append(plans, p)
	}
	return plans, nil
}

// Summary is one row of the workspace list.
type Summary struct {
	Name     string
	Dir      string
	Links    int
	Drift    int
	Dangling int
	Foreign  int
}

// InSync reports whether the workspace needs no changes.
func (s Summary) InSync() bool { return s.Drift == 0 }

// Summaries plans every workspace and reduces each to counts.
func (s *Store) Summaries() ([]Summary, error) {
	all, err := s.Workspaces()
	if err != nil {
		return nil, err
	}
	out := make([]Summary, 0, len(all))
	for _, ws := range all {
		p, err := workspace.BuildPlan(ws)
		if err != nil {
			return nil, err
		}
		c := p.Counts()
		row := Summary{
			Name: ws.Name, Dir: ws.Dir, Links: len(ws.Links),
			Drift:   c[workspace.ActionCreate] + c[workspace.ActionRetarget] + c[workspace.ActionRemove],
			Foreign: c[workspace.ActionForeign],
		}
		for _, a := range p.Actions {
			if a.Dangling {
				row.Dangling++
			}
		}
		out = append(out, row)
	}
	return out, nil
}

// --- mutations --------------------------------------------------------------

// Create adds an empty workspace.
func (s *Store) Create(name string) error {
	if err := workspace.ValidateName(name); err != nil {
		return err
	}
	if _, exists := s.file.Find(name); exists {
		return fmt.Errorf("workspace %q already exists", name)
	}
	s.file.Workspaces = append(s.file.Workspaces, config.Workspace{Name: name})
	return s.save()
}

// AddLink adds a link and reconciles. The alias defaults to the source folder
// name; the resolved alias and absolute source are returned for reporting.
func (s *Store) AddLink(name, src, alias string) (string, string, error) {
	abs, err := config.ExpandPath(src)
	if err != nil {
		return "", "", err
	}
	if alias == "" {
		alias = filepath.Base(abs)
	}
	if err := workspace.ValidateAlias(alias); err != nil {
		return "", "", err
	}

	cw, ok := s.file.Find(name)
	if !ok {
		return "", "", fmt.Errorf("no workspace named %q", name)
	}
	for _, l := range cw.Links {
		if l.Alias == alias {
			return "", "", fmt.Errorf("workspace %q already has an alias %q", name, alias)
		}
	}
	cw.Links = append(cw.Links, config.Link{Src: src, Alias: alias})
	if err := s.save(); err != nil {
		return "", "", err
	}
	return alias, abs, nil
}

// RemoveLink drops a link.
func (s *Store) RemoveLink(name, alias string) error {
	cw, ok := s.file.Find(name)
	if !ok {
		return fmt.Errorf("no workspace named %q", name)
	}
	for i, l := range cw.Links {
		if l.Alias == alias {
			cw.Links = append(cw.Links[:i], cw.Links[i+1:]...)
			return s.save()
		}
	}
	return fmt.Errorf("workspace %q has no alias %q", name, alias)
}

// Rename moves the workspace directory and rewrites the config entry.
func (s *Store) Rename(oldName, newName string) error {
	if err := workspace.ValidateName(newName); err != nil {
		return err
	}
	cw, ok := s.file.Find(oldName)
	if !ok {
		return fmt.Errorf("no workspace named %q", oldName)
	}
	if oldName == newName {
		return nil
	}
	if _, exists := s.file.Find(newName); exists {
		return fmt.Errorf("workspace %q already exists", newName)
	}

	oldWS, err := workspace.Resolve(*cw, s.paths.Root)
	if err != nil {
		return err
	}
	renamed := *cw
	renamed.Name = newName
	newWS, err := workspace.Resolve(renamed, s.paths.Root)
	if err != nil {
		return err
	}

	// Move keeps unmanaged files; the rebuild fallback is only for a
	// cross-filesystem rename and still refuses to discard them.
	if _, err := workspace.Move(oldWS.Dir, newWS.Dir); err != nil {
		if derr := workspace.Destroy(oldWS, false); derr != nil {
			return fmt.Errorf("could not move %s to %s: %v\n%w\n(the rename was not applied)",
				oldWS.Dir, newWS.Dir, err, derr)
		}
	}
	cw.Name = newName
	return s.save()
}

// Delete removes the workspace directory and its config entry.
func (s *Store) Delete(name string, force bool) error {
	ws, err := s.Resolve(name)
	if err != nil {
		return err
	}
	if err := workspace.Destroy(ws, force); err != nil {
		return err
	}
	s.file.Remove(name)
	return s.save()
}

// SourceExists reports whether a link source is present on disk.
func SourceExists(abs string) bool {
	_, err := os.Stat(abs)
	return err == nil
}
