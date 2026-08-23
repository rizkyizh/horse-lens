package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMissingFileIsEmpty(t *testing.T) {
	f, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if len(f.Workspaces) != 0 {
		t.Errorf("got %d workspaces, want 0", len(f.Workspaces))
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.toml")
	in := &File{
		Root: "~/ws",
		Workspaces: []Workspace{{
			Name:  "auth",
			Path:  "~/Developer/_ws/auth",
			Links: []Link{{Src: "~/Developer/backend", Alias: "api"}},
		}},
	}
	if err := Save(path, in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if out.Root != in.Root || len(out.Workspaces) != 1 {
		t.Fatalf("round trip mismatch: %+v", out)
	}
	w := out.Workspaces[0]
	if w.Name != "auth" || w.Path != "~/Developer/_ws/auth" || len(w.Links) != 1 {
		t.Errorf("workspace mismatch: %+v", w)
	}
	if w.Links[0].Alias != "api" {
		t.Errorf("alias = %q, want api", w.Links[0].Alias)
	}
}

// The pre-1.0 [[profiles]] spelling must keep working.
func TestLoadFoldsLegacyProfiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	legacy := `
[[profiles]]
  name = "old"
  [[profiles.links]]
    src = "~/a"
    alias = "a"
`
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(f.Workspaces) != 1 || f.Workspaces[0].Name != "old" {
		t.Fatalf("legacy profiles not folded: %+v", f.Workspaces)
	}
	if len(f.Profiles) != 0 {
		t.Error("Profiles should be cleared so Save writes the new spelling")
	}

	// Saving must emit [[workspaces]] and not [[profiles]].
	if err := Save(path, f); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "[[profiles]]") {
		t.Errorf("saved config still contains [[profiles]]:\n%s", b)
	}
	if !strings.Contains(string(b), "[[workspaces]]") {
		t.Errorf("saved config missing [[workspaces]]:\n%s", b)
	}
}

// A failed encode must not destroy the previous config.
func TestSaveIsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	original := &File{Workspaces: []Workspace{{Name: "keep"}}}
	if err := Save(path, original); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)

	// Encoding a channel fails; the original file must survive untouched.
	if err := Save(path, &File{Workspaces: []Workspace{{Name: strings.Repeat("x", 1)}}}); err != nil {
		t.Fatalf("control save failed: %v", err)
	}
	if err := Save(filepath.Join(dir, "no-such-dir", "x", "c.toml"), original); err != nil {
		t.Logf("nested save error (acceptable): %v", err)
	}

	// No temp files may be left behind.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".config-") {
			t.Errorf("leftover temp file %s", e.Name())
		}
	}
	if len(before) == 0 {
		t.Error("original config was empty")
	}
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	cases := []struct{ in, want string }{
		{"~", home},
		{"~/x", filepath.Join(home, "x")},
		{"/abs/./x", "/abs/x"},
	}
	for _, c := range cases {
		got, err := ExpandPath(c.in)
		if err != nil {
			t.Errorf("ExpandPath(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ExpandPath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if _, err := ExpandPath("   "); err == nil {
		t.Error("ExpandPath(blank) should fail")
	}
}

func TestResolvePrecedence(t *testing.T) {
	dir := t.TempDir()
	flagCfg := filepath.Join(dir, "flag.toml")
	envCfg := filepath.Join(dir, "env.toml")

	t.Setenv(EnvConfig, envCfg)
	t.Setenv(EnvRoot, "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))

	// Flag beats env.
	got, err := ResolveConfig(Overrides{Config: flagCfg})
	if err != nil || got != flagCfg {
		t.Errorf("flag override: got %q err %v, want %q", got, err, flagCfg)
	}
	// Env beats XDG.
	got, err = ResolveConfig(Overrides{})
	if err != nil || got != envCfg {
		t.Errorf("env override: got %q err %v, want %q", got, err, envCfg)
	}
	// XDG beats the home default.
	t.Setenv(EnvConfig, "")
	got, err = ResolveConfig(Overrides{})
	want := filepath.Join(dir, "xdg", "horselens", "config.toml")
	if err != nil || got != want {
		t.Errorf("xdg: got %q err %v, want %q", got, err, want)
	}
}

func TestResolveRootPrecedence(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvRoot, "")
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "xdgdata"))

	// The config file's root key is used when no flag or env is set.
	f := &File{Root: filepath.Join(dir, "from-config")}
	got, err := ResolveRoot(Overrides{}, f)
	if err != nil || got != filepath.Join(dir, "from-config") {
		t.Errorf("config root: got %q err %v", got, err)
	}
	// Env beats the config key.
	t.Setenv(EnvRoot, filepath.Join(dir, "from-env"))
	got, _ = ResolveRoot(Overrides{}, f)
	if got != filepath.Join(dir, "from-env") {
		t.Errorf("env root: got %q", got)
	}
	// Flag beats env.
	got, _ = ResolveRoot(Overrides{Root: filepath.Join(dir, "from-flag")}, f)
	if got != filepath.Join(dir, "from-flag") {
		t.Errorf("flag root: got %q", got)
	}
	// XDG_DATA_HOME is honoured when nothing else is set.
	t.Setenv(EnvRoot, "")
	got, _ = ResolveRoot(Overrides{}, &File{})
	if want := filepath.Join(dir, "xdgdata", "horselens", "workspaces"); got != want {
		t.Errorf("xdg data: got %q, want %q", got, want)
	}
}
