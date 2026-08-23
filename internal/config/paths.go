package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Env var names honoured when resolving locations.
const (
	EnvConfig = "HORSELENS_CONFIG"
	EnvRoot   = "HORSELENS_ROOT"
)

// Overrides carries the command-line flags that outrank every other source.
type Overrides struct {
	Config string
	Root   string
}

// Paths is the resolved set of locations for one invocation.
type Paths struct {
	Config string // the TOML file
	Root   string // directory holding the materialised workspaces
}

// ResolveConfig picks the config file location, in descending precedence:
// flag, HORSELENS_CONFIG, $XDG_CONFIG_HOME/horselens/config.toml,
// ~/.config/horselens/config.toml.
func ResolveConfig(o Overrides) (string, error) {
	if o.Config != "" {
		return ExpandPath(o.Config)
	}
	if v := os.Getenv(EnvConfig); v != "" {
		return ExpandPath(v)
	}
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		base, err := ExpandPath(v)
		if err != nil {
			return "", err
		}
		return filepath.Join(base, "horselens", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config", "horselens", "config.toml"), nil
}

// ResolveRoot picks the workspace root, in descending precedence: flag,
// HORSELENS_ROOT, the config file's `root` key, $XDG_DATA_HOME/horselens/
// workspaces, ~/.local/share/horselens/workspaces.
func ResolveRoot(o Overrides, f *File) (string, error) {
	if o.Root != "" {
		return ExpandPath(o.Root)
	}
	if v := os.Getenv(EnvRoot); v != "" {
		return ExpandPath(v)
	}
	if f != nil && f.Root != "" {
		return ExpandPath(f.Root)
	}
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		base, err := ExpandPath(v)
		if err != nil {
			return "", err
		}
		return filepath.Join(base, "horselens", "workspaces"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", "horselens", "workspaces"), nil
}

// Resolve loads the config file and resolves both locations together.
func Resolve(o Overrides) (Paths, *File, error) {
	cfgPath, err := ResolveConfig(o)
	if err != nil {
		return Paths{}, nil, err
	}
	f, err := Load(cfgPath)
	if err != nil {
		return Paths{}, nil, err
	}
	root, err := ResolveRoot(o, f)
	if err != nil {
		return Paths{}, nil, err
	}
	return Paths{Config: cfgPath, Root: root}, f, nil
}
