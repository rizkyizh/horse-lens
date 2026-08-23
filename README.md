# HorseLens

[![CI](https://github.com/rizkyizh/horse-lens/actions/workflows/ci.yml/badge.svg)](https://github.com/rizkyizh/horse-lens/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/rizkyizh/horse-lens)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

> Instant Context Isolation for AI-Assisted Coding

## The Problem

When you run an AI coding agent (Claude Code, Aider, etc.) from a parent folder that contains many projects, the agent reads too many irrelevant files. This wastes context, slows responses, and causes the agent to make changes in the wrong places.

HorseLens solves this by creating **symlink-based virtual workspaces** — a focused directory containing only the projects you want the agent to see.

## Features

- CRUD for named workspaces, each a curated set of project symlinks
- Declarative reconcile: `apply` makes the directory match the config, pruning links you removed
- Never destructive — only symlinks HorseLens created are ever removed; real files are reported and left alone
- Configurable locations, globally or per workspace
- TOML config file — version-control your workspace definitions
- Full-screen interface for every action, plus a fully scriptable CLI with `--json`
- 20+ built-in themes, switchable at runtime
- Single static binary, no runtime dependencies

## Installation

There are no tagged releases; install from source.

```sh
git clone https://github.com/rizkyizh/horse-lens.git
cd horse-lens
go install ./cmd/horselens
```

To update later, `git pull && go install ./cmd/horselens`.

`go install github.com/rizkyizh/horse-lens/cmd/horselens@latest` also works and
resolves to the newest commit on `main`, though the module proxy can lag behind
by a while.

## Quick Start

```sh
horselens new auth-feature
horselens add auth-feature ~/Developer/backend        # alias defaults to "backend"
horselens add auth-feature ~/Developer/auth-lib auth  # or name it yourself
horselens enter auth-feature
```

You are now in a directory containing only `backend/` and `auth/`. Run your agent there and it sees nothing else. Type `exit` to leave.

Run `horselens` with no arguments for the full-screen interface, where every action above is also available.

## Commands

| Command | Description |
| --- | --- |
| `horselens` | Open the full-screen interface |
| `list` (`ls`) | List workspaces and their state |
| `status [name]` | Show what `apply` would change, without changing it |
| `new <name>` | Create an empty workspace |
| `add <name> <src> [alias]` | Add a link; alias defaults to the source folder name |
| `rm <name> <alias>` | Remove a link |
| `rename <old> <new>` | Rename a workspace, moving its directory as-is |
| `delete <name>` | Remove a workspace and its symlinks; refuses if unmanaged files are present |
| `apply [name]` | Reconcile symlinks with the config (all workspaces if omitted) |
| `path <name>` | Print the workspace directory |
| `enter <name>` | Apply, then open a subshell inside the workspace |
| `shell-init <shell>` | Print shell integration |
| `help`, `version` | Usage summary, build version |

Flags, accepted in any position: `--config <path>`, `--root <path>`, `--json` (`list`, `status`), `--force` (`delete`).

`add`, `rm` and `rename` apply automatically, so the directory is never out of date with the config.

### When you need `apply`

The config file is the source of truth; the workspace directory is only its shadow. `apply` makes the directory match the config — creating missing links, repointing moved ones, and pruning links you removed.

Most of the time you never type it: `add`, `rm`, `rename`, `enter` and the picker all apply for you. Reach for it when the config changed behind their backs:

- you edited `config.toml` in an editor
- you cloned the config on another machine — commit it to your dotfiles, run `horselens apply`, and every workspace materialises
- a project folder moved, so you fixed its `src`

`horselens apply` with no name reconciles every workspace. `status` is the same calculation printed instead of performed, so `status` then `apply` is the safe habit.

## Interface

Running `horselens` with no arguments opens the full-screen interface, built with [dado](https://github.com/atterpac/dado). Everything the CLI does is reachable here.

**Workspace list**

| Key | Action |
| --- | --- |
| `↑` `↓` / `k` `j` | Move |
| `g` / `G` | Jump to first / last |
| Mouse | Click a row to select, wheel to scroll |
| `↵` | Apply and enter the workspace |
| `e` / `l` / `→` | Edit the workspace's links |
| `n` | New workspace |
| `r` | Rename |
| `d` | Delete (with confirmation) |
| `a` / `A` | Apply selected / apply all |
| `R` | Reload the config from disk |
| `t` | Switch theme |
| `q` | Quit |

A workspace is just a name and a set of links, so there is no single edit screen: `r` renames it, and `e` opens the link view below to add or remove projects.

**Link view** (`e` from the list)

| Key | Action |
| --- | --- |
| `a` | Add a project |
| `e` | Edit the selected link's path or alias |
| `d` | Remove a link |
| `A` | Apply |
| `Esc` / `h` / `←` | Back |
| `q` | Quit |

Movement and the mouse behave as they do on the workspace list.

In a form, `^U` clears the current field, `↵` saves and `Esc` cancels. Forms with more than one field — Add and Edit project — also take `Tab` to move between them. Entries the workspace holds that are not symlinks are listed too, marked `left alone`; editing or removing those is refused.

Entering a workspace closes the interface first, then opens the shell, so you land in a normal terminal rather than one nested inside a UI.

## Entering a workspace

A child process cannot change its parent shell's working directory, so there are two routes.

**Subshell** — works everywhere, no setup:

```sh
horselens enter auth-feature   # exit to come back
```

**Shell function** — a real `cd`, no nesting. Add to your shell rc:

```sh
eval "$(horselens shell-init zsh)"   # bash, zsh, sh, ksh
```

```fish
horselens shell-init fish | source   # fish
```

Then `hl auth-feature` applies and cds in one step, and a bare `hl` lists your workspaces.

## How It Works

Each workspace is a directory of symlinks pointing at your real project folders:

```
~/.local/share/horselens/workspaces/auth-feature/
├── backend -> ~/Developer/backend
└── auth    -> ~/Developer/auth-lib
```

Nothing is moved or copied. `apply` reconciles that directory against the config:

```
$ horselens status auth-feature
auth-feature
  ~/.local/share/horselens/workspaces/auth-feature
  + api             -> ~/Developer/backend
  ~ auth            -> ~/Developer/auth-lib (was ~/Developer/old-auth)
  - removed-lib     (stale, will be removed)
  ! NOTES.md        (not a symlink — left alone)
```

### What is never touched

**HorseLens only ever removes symlinks it manages.** Anything that is not a symlink is reported as `!` and skipped. So a `.claude/` directory, scratch notes, or anything else you keep inside a workspace is safe:

| Command | Your unmanaged files |
| --- | --- |
| `apply`, `rm`, `status`, `list` | untouched |
| `rename` | move with the workspace |
| `delete` | refuses to run, and says which files are in the way |
| `delete --force` | **removed** — the only command that deletes them |

Your source folders are never at risk either: symlinks are removed, never followed.

## Configuration

Default location `~/.config/horselens/config.toml`:

```toml
# Optional: where workspaces are materialised.
root = "~/.local/share/horselens/workspaces"

[[workspaces]]
  name = "auth-feature"

  [[workspaces.links]]
    src   = "~/Developer/backend"
    alias = "api"

  [[workspaces.links]]
    src   = "~/Developer/auth-lib"
    alias = "auth"

[[workspaces]]
  name = "data-pipeline"
  # Optional: put this one next to the projects instead of under root.
  path = "~/Developer/_workspaces/data"

  [[workspaces.links]]
    src   = "~/Developer/ingestion"
    alias = "ingestion"
```

| Field | Description |
| --- | --- |
| `root` | Directory holding all workspaces |
| `workspaces[].name` | Workspace name; also the directory name under `root` |
| `workspaces[].path` | Optional per-workspace directory, overriding `root` |
| `workspaces[].links[].src` | Source directory (`~` is expanded) |
| `workspaces[].links[].alias` | Symlink name inside the workspace |

Names and aliases must be letters, digits, dot, dash or underscore, starting with a letter or digit, and at most 64 characters. A name becomes a directory name and an alias a filename, so anything that could escape the workspace root is rejected — on load as well as on input, so a hand-edited config is checked too.

### Where things live

Both locations are resolved in this order, highest first:

| | Config file | Workspace root |
| --- | --- | --- |
| 1 | `--config` | `--root` |
| 2 | `$HORSELENS_CONFIG` | `$HORSELENS_ROOT` |
| 3 | — | `root` key in the config |
| 4 | `$XDG_CONFIG_HOME/horselens/config.toml` | `$XDG_DATA_HOME/horselens/workspaces` |
| 5 | `~/.config/horselens/config.toml` | `~/.local/share/horselens/workspaces` |

Configs using the pre-1.0 `[[profiles]]` key are still read; they are rewritten as `[[workspaces]]` on the next save.

## Contributing

Contributions are welcome. Please open an issue first to discuss significant changes.

```sh
git clone https://github.com/rizkyizh/horse-lens.git
cd horse-lens
make test    # go test ./...
make lint    # go vet ./...
make build   # ./horselens
make run     # go run ./cmd/horselens
```

`make build` puts the binary in the repo as `./horselens`; `go install ./cmd/horselens` puts it on your `PATH` instead.

## License

[MIT](LICENSE) © 2026 rizkyizh
