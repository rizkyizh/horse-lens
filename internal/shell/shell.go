// Package shell handles entering a workspace directory. A child process cannot
// change its parent's working directory, so there are two supported routes: a
// subshell (works everywhere, no setup) and a shell function emitted by Init
// that performs a real cd in the caller's shell.
package shell

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// EnvWorkspace names the active workspace inside a subshell, so prompts and
// scripts can tell they are in one.
const EnvWorkspace = "HORSELENS_WORKSPACE"

// Enter runs the user's shell with its working directory set to dir. A
// non-zero exit from the shell itself is normal (the user ran a failing
// command before exiting) and is not reported as an error.
func Enter(dir, name string) error {
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/sh"
	}
	cmd := exec.Command(sh)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), EnvWorkspace+"="+name)

	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("start shell %s in %s: %w", sh, dir, err)
	}
	return nil
}

const posixInit = `# horselens shell integration
# add to your shell rc:  eval "$(horselens shell-init %[1]s)"
hl() {
  if [ -z "$1" ]; then
    horselens list
    return
  fi
  horselens apply "$1" || return
  cd "$(horselens path "$1")" || return
}
`

const fishInit = `# horselens shell integration
# add to your config.fish:  horselens shell-init fish | source
function hl
    if test -z "$argv[1]"
        horselens list
        return
    end
    horselens apply $argv[1]; or return
    cd (horselens path $argv[1])
end
`

// Init returns the shell function to eval for a real cd, rather than a
// subshell. Supported: bash, zsh, sh, fish.
func Init(name string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "bash", "zsh", "sh", "ksh":
		return fmt.Sprintf(posixInit, name), nil
	case "fish":
		return fishInit, nil
	case "":
		return "", errors.New("shell-init needs a shell name: bash, zsh, sh or fish")
	default:
		return "", fmt.Errorf("unsupported shell %q: use bash, zsh, sh or fish", name)
	}
}
