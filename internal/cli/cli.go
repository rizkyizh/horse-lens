// Package cli implements the command surface. Every command resolves its
// paths the same way, so --config and --root behave identically everywhere.
package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/rizkyizh/horse-lens/internal/config"
	"github.com/rizkyizh/horse-lens/internal/workspace"
)

// Version is the reported build version.
const Version = "v0.2.1"

type app struct {
	out    io.Writer
	errOut io.Writer
	over   config.Overrides
	json   bool
	force  bool
}

// Run dispatches one invocation and returns the process exit code.
func Run(args []string, out, errOut io.Writer) int {
	a := &app{out: out, errOut: errOut}

	// No command, or only global flags: open the picker.
	if len(args) == 0 || strings.HasPrefix(args[0], "-") && !isHelpFlag(args[0]) {
		fs := a.newFlags("horselens")
		if err := a.parse(fs, args); err != nil {
			return 2
		}
		if fs.NArg() > 0 {
			args = fs.Args()
		} else {
			return Picker(out, errOut, a.over)
		}
	}

	cmd, rest := args[0], args[1:]
	var err error
	switch cmd {
	case "list", "ls":
		err = a.cmdList(rest)
	case "status":
		err = a.cmdStatus(rest)
	case "new":
		err = a.cmdNew(rest)
	case "add":
		err = a.cmdAdd(rest)
	case "rm":
		err = a.cmdRm(rest)
	case "rename":
		err = a.cmdRename(rest)
	case "delete":
		err = a.cmdDelete(rest)
	case "apply":
		err = a.cmdApply(rest)
	case "path":
		err = a.cmdPath(rest)
	case "enter":
		err = a.cmdEnter(rest)
	case "shell-init":
		err = a.cmdShellInit(rest)
	case "version", "--version", "-v":
		fmt.Fprintln(out, "horselens", Version)
		return 0
	case "help", "--help", "-h":
		a.usage()
		return 0
	default:
		fmt.Fprintf(errOut, "horselens: unknown command %q\n\n", cmd)
		a.usage()
		return 2
	}

	if err != nil {
		fmt.Fprintln(a.errOut, "horselens:", err)
		return 1
	}
	return 0
}

func isHelpFlag(s string) bool {
	switch s {
	case "-h", "--help", "-v", "--version":
		return true
	}
	return false
}

func (a *app) usage() {
	fmt.Fprint(a.out, `horselens — focused symlink workspaces for AI coding agents

usage: horselens <command> [args]

  (no command)          open the workspace picker
  list                  list workspaces and their state
  status [name]         show what apply would change, without changing it
  new <name>            create an empty workspace
  add <name> <src> [alias]
                        add a link (alias defaults to the source folder name)
  rm <name> <alias>     remove a link
  rename <old> <new>    rename a workspace
  delete <name>         remove a workspace and its symlinks
  apply [name]          reconcile symlinks with the config
  path <name>           print the workspace directory
  enter <name>          apply, then open a subshell inside the workspace
  shell-init <shell>    print shell integration for a real cd

flags (any command):
  --config <path>       config file          [$HORSELENS_CONFIG]
  --root <path>         workspace root       [$HORSELENS_ROOT]
  --json                machine-readable output (list, status)
  --force               allow delete to remove non-symlink files

`)
}

// newFlags builds a FlagSet carrying the flags every command accepts.
func (a *app) newFlags(name string, extra ...func(*flag.FlagSet)) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	// Seed each flag with the value already parsed, so a global flag given
	// before the subcommand (horselens --root X path web) is not reset to ""
	// when the subcommand registers the same flag.
	fs.StringVar(&a.over.Config, "config", a.over.Config, "config file path")
	fs.StringVar(&a.over.Root, "root", a.over.Root, "workspace root directory")
	for _, f := range extra {
		f(fs)
	}
	return fs
}

func withJSON(a *app) func(*flag.FlagSet) {
	return func(fs *flag.FlagSet) { fs.BoolVar(&a.json, "json", false, "machine-readable output") }
}

func withForce(a *app) func(*flag.FlagSet) {
	return func(fs *flag.FlagSet) { fs.BoolVar(&a.force, "force", false, "remove non-symlink files too") }
}

// load resolves paths and reads the config in one step.
func (a *app) load() (config.Paths, *config.File, error) {
	return config.Resolve(a.over)
}

// resolveOne finds a workspace by name and resolves it against the root.
func (a *app) resolveOne(f *config.File, root, name string) (workspace.Workspace, error) {
	cw, ok := f.Find(name)
	if !ok {
		return workspace.Workspace{}, fmt.Errorf("no workspace named %q (try: horselens list)", name)
	}
	return workspace.Resolve(*cw, root)
}

// argsExactly reports a usage error unless exactly n positional args are given.
func argsExactly(fs *flag.FlagSet, n int, usage string) error {
	if fs.NArg() != n {
		return fmt.Errorf("usage: horselens %s", usage)
	}
	return nil
}

func argsAtMost(fs *flag.FlagSet, n int, usage string) error {
	if fs.NArg() > n {
		return fmt.Errorf("usage: horselens %s", usage)
	}
	return nil
}

// plural is a small formatting helper used across the summaries.
func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func joinNonEmpty(parts []string, sep string) string {
	out := parts[:0]
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}
