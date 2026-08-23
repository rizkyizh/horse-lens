package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rizkyizh/horse-lens/internal/config"
)

func open(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	root := filepath.Join(dir, "ws")
	t.Setenv("HORSELENS_CONFIG", filepath.Join(dir, "config.toml"))
	t.Setenv("HORSELENS_ROOT", root)
	s, err := Open(config.Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	return s, root
}

// Reload must pick up a config edited outside the process — the case that
// makes a standalone `apply` necessary in the first place.
func TestReloadPicksUpExternalEdits(t *testing.T) {
	s, _ := open(t)
	if err := s.Create("one"); err != nil {
		t.Fatal(err)
	}

	extra := "\n[[workspaces]]\n  name = \"two\"\n"
	f, err := os.OpenFile(s.Paths().Config, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(extra); err != nil {
		t.Fatal(err)
	}
	f.Close()

	if rows, _ := s.Summaries(); len(rows) != 1 {
		t.Fatalf("stale store already sees %d workspaces", len(rows))
	}
	if err := s.Reload(); err != nil {
		t.Fatal(err)
	}
	rows, err := s.Summaries()
	if err != nil || len(rows) != 2 {
		t.Fatalf("after reload: %d rows, err %v", len(rows), err)
	}
}

func TestSummariesCountDriftAndForeign(t *testing.T) {
	s, root := open(t)
	src := filepath.Join(t.TempDir(), "api")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := s.Create("w"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AddLink("w", src, "api"); err != nil {
		t.Fatal(err)
	}

	// Not applied yet: one pending change.
	rows, _ := s.Summaries()
	if rows[0].Drift != 1 || !rows[0].InSync() == false {
		t.Errorf("before apply: %+v", rows[0])
	}

	if _, err := s.Apply("w"); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.Summaries()
	if !rows[0].InSync() {
		t.Errorf("after apply: %+v", rows[0])
	}

	// An unmanaged file is counted but is not drift.
	if err := os.WriteFile(filepath.Join(root, "w", "NOTES.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.Summaries()
	if rows[0].Foreign != 1 || !rows[0].InSync() {
		t.Errorf("with foreign file: %+v", rows[0])
	}
}

func TestAddLinkDefaultsAlias(t *testing.T) {
	s, _ := open(t)
	if err := s.Create("w"); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "my-project")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	alias, abs, err := s.AddLink("w", src, "")
	if err != nil {
		t.Fatal(err)
	}
	if alias != "my-project" {
		t.Errorf("alias = %q, want my-project", alias)
	}
	if !SourceExists(abs) {
		t.Errorf("SourceExists(%q) = false", abs)
	}
}
