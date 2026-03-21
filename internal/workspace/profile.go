package workspace

import (
	"fmt"

	"github.com/rizkyizh/horse-lens/internal/config"
)

func SaveProfile(ws Workspace) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	p := ToProfile(ws)
	for i, existing := range cfg.Profiles {
		if existing.Name == ws.Name {
			cfg.Profiles[i] = p
			return config.Save(cfg)
		}
	}
	cfg.Profiles = append(cfg.Profiles, p)
	return config.Save(cfg)
}

func LoadProfile(name string) (Workspace, error) {
	cfg, err := config.Load()
	if err != nil {
		return Workspace{}, err
	}
	for _, p := range cfg.Profiles {
		if p.Name == name {
			return FromProfile(p), nil
		}
	}
	return Workspace{}, fmt.Errorf("profile %q not found", name)
}

func DeleteProfile(name string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	filtered := cfg.Profiles[:0]
	for _, p := range cfg.Profiles {
		if p.Name != name {
			filtered = append(filtered, p)
		}
	}
	cfg.Profiles = filtered
	return config.Save(cfg)
}

func AllProfiles() ([]Workspace, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	var workspaces []Workspace
	for _, p := range cfg.Profiles {
		workspaces = append(workspaces, FromProfile(p))
	}
	return workspaces, nil
}
