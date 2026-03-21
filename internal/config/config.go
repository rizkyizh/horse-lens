package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type LinkEntry struct {
	Src   string `toml:"src"`
	Alias string `toml:"alias"`
}

type Profile struct {
	Name  string      `toml:"name"`
	Links []LinkEntry `toml:"links"`
}

type Config struct {
	Profiles []Profile `toml:"profiles"`
}

func ConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "horselens")
}

func ConfigPath() string {
	return filepath.Join(ConfigDir(), "config.toml")
}

func Load() (*Config, error) {
	path := ConfigPath()
	cfg := &Config{}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func Save(cfg *Config) error {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.Create(ConfigPath())
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
}
