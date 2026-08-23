package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitSupportedShells(t *testing.T) {
	for _, sh := range []string{"bash", "zsh", "sh"} {
		out, err := Init(sh)
		if err != nil {
			t.Fatalf("Init(%q): %v", sh, err)
		}
		if !strings.Contains(out, "hl()") || !strings.Contains(out, "horselens apply") {
			t.Errorf("Init(%q) missing the function body:\n%s", sh, out)
		}
	}
	out, err := Init("fish")
	if err != nil || !strings.Contains(out, "function hl") {
		t.Errorf("Init(fish) = %q, %v", out, err)
	}
	for _, bad := range []string{"", "powershell"} {
		if _, err := Init(bad); err == nil {
			t.Errorf("Init(%q) = nil error, want rejection", bad)
		}
	}
}

// Enter must run the shell with the workspace as its working directory.
func TestEnterRunsShellInDir(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(t.TempDir(), "captured")
	script := filepath.Join(t.TempDir(), "fake-shell")
	body := "#!/bin/sh\n{ pwd; echo \"$" + EnvWorkspace + "\"; } > " + out + "\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", script)

	if err := Enter(dir, "myws"); err != nil {
		t.Fatalf("Enter: %v", err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("shell did not run: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 {
		t.Fatalf("captured %q", b)
	}
	gotDir, _ := filepath.EvalSymlinks(lines[0])
	wantDir, _ := filepath.EvalSymlinks(dir)
	if gotDir != wantDir {
		t.Errorf("cwd = %q, want %q", gotDir, wantDir)
	}
	if lines[1] != "myws" {
		t.Errorf("%s = %q, want myws", EnvWorkspace, lines[1])
	}
}

// A non-zero exit from the user's shell is normal, not a horselens failure.
func TestEnterIgnoresShellExitCode(t *testing.T) {
	script := filepath.Join(t.TempDir(), "failing-shell")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", script)
	if err := Enter(t.TempDir(), "w"); err != nil {
		t.Errorf("Enter = %v, want nil for a non-zero shell exit", err)
	}
}

func TestEnterReportsMissingShell(t *testing.T) {
	t.Setenv("SHELL", filepath.Join(t.TempDir(), "does-not-exist"))
	if err := Enter(t.TempDir(), "w"); err == nil {
		t.Error("Enter = nil, want an error for a missing shell binary")
	}
}
