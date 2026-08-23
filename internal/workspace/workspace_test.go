package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rizkyizh/horse-lens/internal/config"
)

// The names below used to make `delete` run os.RemoveAll outside the root —
// a workspace called "../../../.." erased the user's home directory.
func TestValidateNameRejectsTraversal(t *testing.T) {
	bad := []string{
		"", " ", "..", ".", "../../../..", "a/b", "a\\b", "~", "/abs",
		" leading", "trailing ", "-flag", ".hidden", strings.Repeat("x", MaxNameLen+1),
	}
	for _, n := range bad {
		if err := ValidateName(n); err == nil {
			t.Errorf("ValidateName(%q) = nil, want error", n)
		}
		if err := ValidateAlias(n); err == nil {
			t.Errorf("ValidateAlias(%q) = nil, want error", n)
		}
	}
	for _, n := range []string{"auth", "auth-feature", "a_b.c", "x1", "9lives"} {
		if err := ValidateName(n); err != nil {
			t.Errorf("ValidateName(%q) = %v, want nil", n, err)
		}
	}
}

// Even if a bad name reached Resolve, the containment check must stop it.
func TestResolveCannotEscapeRoot(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{"..", "../../etc", "a/b"} {
		if _, err := Resolve(config.Workspace{Name: n}, root); err == nil {
			t.Errorf("Resolve(name=%q) = nil error, want rejection", n)
		}
	}
	ws, err := Resolve(config.Workspace{Name: "ok"}, root)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if ws.Dir != filepath.Join(root, "ok") {
		t.Errorf("Dir = %q, want %q", ws.Dir, filepath.Join(root, "ok"))
	}
}

func TestResolvePerWorkspacePathOverridesRoot(t *testing.T) {
	root := t.TempDir()
	custom := filepath.Join(t.TempDir(), "elsewhere")
	ws, err := Resolve(config.Workspace{Name: "auth", Path: custom}, root)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if ws.Dir != custom {
		t.Errorf("Dir = %q, want %q", ws.Dir, custom)
	}
}

func TestResolveRejectsDuplicateAlias(t *testing.T) {
	root := t.TempDir()
	_, err := Resolve(config.Workspace{
		Name: "x",
		Links: []config.Link{
			{Src: "/tmp/a", Alias: "same"},
			{Src: "/tmp/b", Alias: "same"},
		},
	}, root)
	if err == nil || !strings.Contains(err.Error(), "duplicate alias") {
		t.Errorf("err = %v, want duplicate alias", err)
	}
}

func TestResolveAllRejectsDuplicateNames(t *testing.T) {
	root := t.TempDir()
	f := &config.File{Workspaces: []config.Workspace{{Name: "dup"}, {Name: "dup"}}}
	if _, err := ResolveAll(f, root); err == nil {
		t.Error("ResolveAll accepted duplicate workspace names")
	}
}

// --- reconciliation ---------------------------------------------------------

func mustResolve(t *testing.T, root string, w config.Workspace) Workspace {
	t.Helper()
	ws, err := Resolve(w, root)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return ws
}

func kinds(p Plan) map[string]ActionKind {
	m := map[string]ActionKind{}
	for _, a := range p.Actions {
		m[a.Alias] = a.Kind
	}
	return m
}

func TestPlanAndApplyReconcile(t *testing.T) {
	root := t.TempDir()
	srcA := filepath.Join(t.TempDir(), "a")
	srcB := filepath.Join(t.TempDir(), "b")
	for _, d := range []string{srcA, srcB} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	ws := mustResolve(t, root, config.Workspace{
		Name:  "w",
		Links: []config.Link{{Src: srcA, Alias: "a"}},
	})

	// First apply creates the link.
	p, err := BuildPlan(ws)
	if err != nil {
		t.Fatal(err)
	}
	if kinds(p)["a"] != ActionCreate {
		t.Fatalf("first plan = %v, want create", kinds(p))
	}
	if err := p.Apply(); err != nil {
		t.Fatal(err)
	}
	got, err := os.Readlink(filepath.Join(ws.Dir, "a"))
	if err != nil || got != srcA {
		t.Fatalf("symlink target = %q err %v, want %q", got, err, srcA)
	}

	// Re-planning is a no-op.
	p, _ = BuildPlan(ws)
	if p.Changes() {
		t.Errorf("second plan wants changes: %v", kinds(p))
	}

	// Repointing the alias retargets rather than duplicating.
	ws.Links[0].Src = srcB
	p, _ = BuildPlan(ws)
	if kinds(p)["a"] != ActionRetarget {
		t.Fatalf("plan = %v, want retarget", kinds(p))
	}
	if err := p.Apply(); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.Readlink(filepath.Join(ws.Dir, "a")); got != srcB {
		t.Errorf("after retarget target = %q, want %q", got, srcB)
	}

	// Dropping the link prunes the stale symlink — the bug the old
	// CreateSymlinks had, where removed projects stayed visible.
	ws.Links = nil
	p, _ = BuildPlan(ws)
	if kinds(p)["a"] != ActionRemove {
		t.Fatalf("plan = %v, want remove", kinds(p))
	}
	if err := p.Apply(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(ws.Dir, "a")); !os.IsNotExist(err) {
		t.Error("stale symlink survived apply")
	}
}

// Real files inside a workspace must be reported and never touched.
func TestForeignFilesAreNeverTouched(t *testing.T) {
	root := t.TempDir()
	ws := mustResolve(t, root, config.Workspace{Name: "w"})
	if err := os.MkdirAll(ws.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	note := filepath.Join(ws.Dir, "notes.md")
	if err := os.WriteFile(note, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := BuildPlan(ws)
	if err != nil {
		t.Fatal(err)
	}
	if kinds(p)["notes.md"] != ActionForeign {
		t.Fatalf("plan = %v, want foreign", kinds(p))
	}
	if p.Changes() {
		t.Error("a foreign-only plan should report no changes")
	}
	if err := p.Apply(); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(note); err != nil || string(b) != "mine" {
		t.Errorf("foreign file altered: %q %v", b, err)
	}

	// Destroy must refuse while the real file is there.
	err = Destroy(ws, false)
	if err == nil || !strings.Contains(err.Error(), "not symlinks") {
		t.Fatalf("Destroy err = %v, want refusal", err)
	}
	if _, err := os.Stat(note); err != nil {
		t.Error("refused Destroy still removed the file")
	}

	// --force is the explicit escape hatch.
	if err := Destroy(ws, true); err != nil {
		t.Fatalf("forced Destroy: %v", err)
	}
	if _, err := os.Stat(ws.Dir); !os.IsNotExist(err) {
		t.Error("forced Destroy left the directory behind")
	}
}

func TestDestroyRemovesOnlySymlinks(t *testing.T) {
	root := t.TempDir()
	src := t.TempDir()
	ws := mustResolve(t, root, config.Workspace{
		Name:  "w",
		Links: []config.Link{{Src: src, Alias: "a"}},
	})
	p, _ := BuildPlan(ws)
	if err := p.Apply(); err != nil {
		t.Fatal(err)
	}
	if err := Destroy(ws, false); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if _, err := os.Stat(ws.Dir); !os.IsNotExist(err) {
		t.Error("workspace dir survived")
	}
	// The source must be untouched — symlinks are never followed.
	if _, err := os.Stat(src); err != nil {
		t.Errorf("source directory was removed: %v", err)
	}
}

func TestDestroyOnMissingDirIsNoOp(t *testing.T) {
	ws := mustResolve(t, t.TempDir(), config.Workspace{Name: "gone"})
	if err := Destroy(ws, false); err != nil {
		t.Errorf("Destroy on missing dir = %v, want nil", err)
	}
}

func TestPlanFlagsDanglingSource(t *testing.T) {
	root := t.TempDir()
	ws := mustResolve(t, root, config.Workspace{
		Name:  "w",
		Links: []config.Link{{Src: filepath.Join(t.TempDir(), "missing"), Alias: "gone"}},
	})
	p, err := BuildPlan(ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Actions) != 1 || !p.Actions[0].Dangling {
		t.Fatalf("actions = %+v, want one dangling", p.Actions)
	}
	// It is still created: the source may appear later.
	if err := p.Apply(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(ws.Dir, "gone")); err != nil {
		t.Errorf("dangling link not created: %v", err)
	}
}

// A file appearing where a symlink was planned for removal must abort the
// removal rather than delete the file.
func TestApplyRefusesToRemoveNonSymlink(t *testing.T) {
	root := t.TempDir()
	ws := mustResolve(t, root, config.Workspace{Name: "w"})
	if err := os.MkdirAll(ws.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(ws.Dir, "a")
	if err := os.WriteFile(victim, []byte("precious"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Hand-build a plan that (wrongly) believes "a" is a stale symlink.
	p := Plan{Name: ws.Name, Dir: ws.Dir, Actions: []Action{{Kind: ActionRemove, Alias: "a"}}}
	if err := p.Apply(); err == nil {
		t.Fatal("Apply removed a non-symlink without complaint")
	}
	if b, err := os.ReadFile(victim); err != nil || string(b) != "precious" {
		t.Errorf("file was destroyed: %q %v", b, err)
	}
}

// Renaming must carry the whole directory across, including files horselens
// does not manage. Rebuilding it would silently lose them.
func TestMovePreservesForeignFiles(t *testing.T) {
	root := t.TempDir()
	src := t.TempDir()
	old := mustResolve(t, root, config.Workspace{
		Name:  "before",
		Links: []config.Link{{Src: src, Alias: "api"}},
	})
	p, _ := BuildPlan(old)
	if err := p.Apply(); err != nil {
		t.Fatal(err)
	}
	// The kind of thing a user accumulates inside a workspace.
	if err := os.MkdirAll(filepath.Join(old.Dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old.Dir, ".claude", "settings.json"), []byte(`{"a":1}`), 0o644); err != nil {
		t.Fatal(err)
	}

	newWS := mustResolve(t, root, config.Workspace{
		Name:  "after",
		Links: []config.Link{{Src: src, Alias: "api"}},
	})
	moved, err := Move(old.Dir, newWS.Dir)
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if !moved {
		t.Error("Move reported nothing moved")
	}
	if _, err := os.Stat(old.Dir); !os.IsNotExist(err) {
		t.Error("old directory survived the move")
	}
	b, err := os.ReadFile(filepath.Join(newWS.Dir, ".claude", "settings.json"))
	if err != nil || string(b) != `{"a":1}` {
		t.Errorf("foreign file did not travel: %q %v", b, err)
	}
	if got, _ := os.Readlink(filepath.Join(newWS.Dir, "api")); got != src {
		t.Errorf("symlink after move = %q, want %q", got, src)
	}
}

func TestMoveEdgeCases(t *testing.T) {
	root := t.TempDir()

	// Nothing materialised yet is not an error.
	moved, err := Move(filepath.Join(root, "never-applied"), filepath.Join(root, "b"))
	if err != nil || moved {
		t.Errorf("Move of a missing dir = (%v, %v), want (false, nil)", moved, err)
	}

	// Same path is a no-op.
	if moved, err := Move(root, root); err != nil || moved {
		t.Errorf("Move to the same path = (%v, %v)", moved, err)
	}

	// Refuses to clobber an existing directory.
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	for _, d := range []string{a, b} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Move(a, b); err == nil {
		t.Error("Move overwrote an existing directory")
	}
	if _, err := os.Stat(a); err != nil {
		t.Error("source removed despite the refusal")
	}
}
