package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type harness struct {
	t    *testing.T
	root string
	cfg  string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	h := &harness{t: t, root: filepath.Join(dir, "ws"), cfg: filepath.Join(dir, "config.toml")}
	t.Setenv("HORSELENS_CONFIG", h.cfg)
	t.Setenv("HORSELENS_ROOT", h.root)
	return h
}

// run executes one command and returns exit code, stdout and stderr.
func (h *harness) run(args ...string) (int, string, string) {
	h.t.Helper()
	var out, errOut bytes.Buffer
	code := Run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

// ok runs a command that must succeed.
func (h *harness) ok(args ...string) string {
	h.t.Helper()
	code, out, errOut := h.run(args...)
	if code != 0 {
		h.t.Fatalf("%v exited %d\nstdout: %s\nstderr: %s", args, code, out, errOut)
	}
	return out
}

// srcDir makes a real directory to link to.
func (h *harness) srcDir(name string) string {
	h.t.Helper()
	p := filepath.Join(h.t.TempDir(), name)
	if err := os.MkdirAll(p, 0o755); err != nil {
		h.t.Fatal(err)
	}
	return p
}

func TestLifecycle(t *testing.T) {
	h := newHarness(t)
	api := h.srcDir("api")
	lib := h.srcDir("lib")

	h.ok("new", "auth")
	h.ok("add", "auth", api)
	h.ok("add", "auth", lib, "authlib")

	// Both symlinks exist and point at the sources.
	for alias, want := range map[string]string{"api": api, "authlib": lib} {
		got, err := os.Readlink(filepath.Join(h.root, "auth", alias))
		if err != nil || got != want {
			t.Errorf("link %s = %q err %v, want %q", alias, got, err, want)
		}
	}

	if out := h.ok("list"); !strings.Contains(out, "auth") || !strings.Contains(out, "in sync") {
		t.Errorf("list output unexpected:\n%s", out)
	}

	// Removing a link prunes the symlink.
	h.ok("rm", "auth", "authlib")
	if _, err := os.Lstat(filepath.Join(h.root, "auth", "authlib")); !os.IsNotExist(err) {
		t.Error("rm left a stale symlink behind")
	}

	// Rename moves the directory and leaves no orphan.
	h.ok("rename", "auth", "auth-feature")
	if _, err := os.Stat(filepath.Join(h.root, "auth")); !os.IsNotExist(err) {
		t.Error("rename left the old directory behind")
	}
	if _, err := os.Readlink(filepath.Join(h.root, "auth-feature", "api")); err != nil {
		t.Errorf("renamed workspace missing its link: %v", err)
	}

	h.ok("delete", "auth-feature")
	if _, err := os.Stat(filepath.Join(h.root, "auth-feature")); !os.IsNotExist(err) {
		t.Error("delete left the directory behind")
	}
	if out := h.ok("list"); !strings.Contains(out, "no workspaces") {
		t.Errorf("workspace still listed after delete:\n%s", out)
	}
}

// Go's flag package stops at the first positional argument; the permuting
// parser must accept flags on either side of it.
func TestFlagsAcceptedAfterPositionalArgs(t *testing.T) {
	h := newHarness(t)
	h.ok("new", "web")
	if err := os.MkdirAll(filepath.Join(h.root, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.root, "web", "note.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Without --force this must refuse.
	if code, _, _ := h.run("delete", "web"); code == 0 {
		t.Fatal("delete removed a workspace containing a real file")
	}
	// Flag after the positional argument must be honoured.
	if code, _, errOut := h.run("delete", "web", "--force"); code != 0 {
		t.Fatalf("delete --force exited %d: %s", code, errOut)
	}
	if _, err := os.Stat(filepath.Join(h.root, "web")); !os.IsNotExist(err) {
		t.Error("--force did not remove the directory")
	}
}

// A global flag given before the subcommand must survive the subcommand's own
// flag registration.
func TestGlobalFlagBeforeSubcommand(t *testing.T) {
	h := newHarness(t)
	h.ok("new", "web")
	alt := filepath.Join(t.TempDir(), "alt-root")

	before := strings.TrimSpace(h.ok("--root", alt, "path", "web"))
	after := strings.TrimSpace(h.ok("path", "web", "--root", alt))
	want := filepath.Join(alt, "web")

	if before != want {
		t.Errorf("--root before subcommand = %q, want %q", before, want)
	}
	if after != want {
		t.Errorf("--root after subcommand = %q, want %q", after, want)
	}
}

func TestPerWorkspacePathOverridesRoot(t *testing.T) {
	h := newHarness(t)
	custom := filepath.Join(t.TempDir(), "beside-my-projects")
	src := h.srcDir("api")

	cfg := "[[workspaces]]\n  name = \"web\"\n  path = \"" + custom + "\"\n" +
		"  [[workspaces.links]]\n    src = \"" + src + "\"\n    alias = \"api\"\n"
	if err := os.MkdirAll(filepath.Dir(h.cfg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.cfg, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := strings.TrimSpace(h.ok("path", "web")); got != custom {
		t.Errorf("path = %q, want %q", got, custom)
	}
	h.ok("apply", "web")
	if _, err := os.Readlink(filepath.Join(custom, "api")); err != nil {
		t.Errorf("link not created in custom path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(h.root, "web")); !os.IsNotExist(err) {
		t.Error("workspace was also created under the global root")
	}
}

// A hand-edited config containing a traversal name must fail loudly.
func TestPoisonedConfigIsRejected(t *testing.T) {
	h := newHarness(t)
	if err := os.MkdirAll(filepath.Dir(h.cfg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.cfg, []byte("[[workspaces]]\n  name = \"../../escape\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := h.run("list")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "invalid") {
		t.Errorf("stderr = %q, want a validation error", errOut)
	}
}

func TestRejectsBadNames(t *testing.T) {
	h := newHarness(t)
	for _, name := range []string{"../../../..", "..", "a/b"} {
		if code, _, _ := h.run("new", name); code == 0 {
			t.Errorf("new %q was accepted", name)
		}
	}
}

func TestJSONOutput(t *testing.T) {
	h := newHarness(t)
	src := h.srcDir("api")
	h.ok("new", "web")
	h.ok("add", "web", src)

	var list []jsonWorkspace
	if err := json.Unmarshal([]byte(h.ok("list", "--json")), &list); err != nil {
		t.Fatalf("list --json: %v", err)
	}
	if len(list) != 1 || list[0].Name != "web" || list[0].Links != 1 {
		t.Errorf("list json = %+v", list)
	}

	var plans []jsonPlan
	if err := json.Unmarshal([]byte(h.ok("status", "web", "--json")), &plans); err != nil {
		t.Fatalf("status --json: %v", err)
	}
	if len(plans) != 1 || plans[0].Changes {
		t.Errorf("status json = %+v", plans)
	}
}

func TestAddDefaultsAliasAndWarnsOnMissingSource(t *testing.T) {
	h := newHarness(t)
	h.ok("new", "w")
	out := h.ok("add", "w", filepath.Join(t.TempDir(), "not-there"))
	if !strings.Contains(out, "does not exist") {
		t.Errorf("expected a missing-source warning, got:\n%s", out)
	}
	// The alias defaults to the base name and the link is still created.
	if _, err := os.Lstat(filepath.Join(h.root, "w", "not-there")); err != nil {
		t.Errorf("dangling link not created: %v", err)
	}
}

func TestUnknownCommandAndHelp(t *testing.T) {
	h := newHarness(t)
	if code, _, _ := h.run("frobnicate"); code != 2 {
		t.Errorf("unknown command exit = %d, want 2", code)
	}
	if code, out, _ := h.run("help"); code != 0 || !strings.Contains(out, "shell-init") {
		t.Errorf("help exit=%d out=%q", code, out)
	}
	if code, out, _ := h.run("version"); code != 0 || !strings.Contains(out, Version) {
		t.Errorf("version exit=%d out=%q", code, out)
	}
}

func TestDuplicateNamesAndAliasesRejected(t *testing.T) {
	h := newHarness(t)
	src := h.srcDir("a")
	h.ok("new", "w")
	if code, _, _ := h.run("new", "w"); code == 0 {
		t.Error("duplicate workspace name accepted")
	}
	h.ok("add", "w", src, "same")
	if code, _, _ := h.run("add", "w", src, "same"); code == 0 {
		t.Error("duplicate alias accepted")
	}
}
