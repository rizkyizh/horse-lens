// Package config loads and saves the on-disk TOML file and resolves where
// that file and the workspace root live.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Link is one symlink entry inside a workspace.
type Link struct {
	Src   string `toml:"src"`
	Alias string `toml:"alias"`
}

// Workspace is a named set of links, optionally materialised somewhere other
// than the global root.
type Workspace struct {
	Name  string `toml:"name"`
	Path  string `toml:"path,omitempty"`
	Links []Link `toml:"links"`
}

// File mirrors the TOML document.
type File struct {
	Root       string      `toml:"root,omitempty"`
	Workspaces []Workspace `toml:"workspaces"`

	// Profiles is the pre-1.0 spelling of Workspaces. It is read for backwards
	// compatibility and never written back; Load folds it into Workspaces.
	Profiles []Workspace `toml:"profiles,omitempty"`
}

// Find returns the workspace with the given name.
func (f *File) Find(name string) (*Workspace, bool) {
	for i := range f.Workspaces {
		if f.Workspaces[i].Name == name {
			return &f.Workspaces[i], true
		}
	}
	return nil, false
}

// Remove deletes the named workspace, reporting whether it existed.
func (f *File) Remove(name string) bool {
	for i := range f.Workspaces {
		if f.Workspaces[i].Name == name {
			f.Workspaces = append(f.Workspaces[:i], f.Workspaces[i+1:]...)
			return true
		}
	}
	return false
}

// Load reads the TOML file at path. A missing file yields an empty document
// rather than an error, so a first run needs no setup.
func Load(path string) (*File, error) {
	f := &File{}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return f, nil
		}
		return nil, fmt.Errorf("stat config %s: %w", path, err)
	}
	if _, err := toml.DecodeFile(path, f); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	// Fold the legacy [[profiles]] spelling into [[workspaces]].
	if len(f.Profiles) > 0 {
		f.Workspaces = append(f.Workspaces, f.Profiles...)
		f.Profiles = nil
	}
	return f, nil
}

// Save writes the document atomically: encode to a temporary file in the same
// directory, then rename over the target. A failure part-way through therefore
// leaves the previous config intact.
func Save(path string, f *File) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config dir %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".config-*.toml")
	if err != nil {
		return fmt.Errorf("create temp config in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds

	if err := toml.NewEncoder(tmp).Encode(f); err != nil {
		tmp.Close()
		return fmt.Errorf("encode config: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp config: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace config %s: %w", path, err)
	}
	return nil
}

// ExpandPath resolves a leading ~ and makes the result absolute. Unlike the
// previous implementation it reports a missing home directory instead of
// silently producing a relative path.
func ExpandPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("empty path")
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory for %q: %w", p, err)
		}
		if p == "~" {
			p = home
		} else {
			p = filepath.Join(home, p[2:])
		}
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", p, err)
	}
	return filepath.Clean(abs), nil
}
