# HorseLens

[![CI](https://github.com/rizkyizh/horse-lens/actions/workflows/ci.yml/badge.svg)](https://github.com/rizkyizh/horse-lens/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/rizkyizh/horse-lens)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

> Instant Context Isolation for AI-Assisted Coding

<!-- Screenshot/demo GIF placeholder -->

## The Problem

When you run an AI coding agent (Claude CLI, Aider, etc.) from a parent folder that contains many projects, the agent reads too many irrelevant files. This wastes context, slows responses, and causes the agent to make changes in the wrong places.

HorseLens solves this by creating **symlink-based virtual workspaces** — a focused directory that only contains the projects you want the agent to see.

## Features

- Manage multiple named workspaces, each with a curated set of project symlinks
- Embedded terminal: open a shell directly inside a workspace with one keypress
- Fast switching between workspaces without leaving the TUI
- TOML config file — version-control your workspace definitions
- Single static binary, no runtime dependencies

## Installation

### go install (recommended)

```sh
go install github.com/rizkyizh/horse-lens/cmd/horselens@latest
```

### Download binary from Releases

Download the latest binary for your platform from the [Releases page](https://github.com/rizkyizh/horse-lens/releases), then move it to a directory on your `PATH`:

```sh
# Example for macOS arm64
curl -L https://github.com/rizkyizh/horse-lens/releases/latest/download/horselens-darwin-arm64.tar.gz | tar xz
mv horselens /usr/local/bin/
```

## Quick Start

1. Create a config file at `~/.config/horselens/config.toml`:

```toml
[[profiles]]
  name = "my-project"

  [[profiles.links]]
    src = "~/Developer/my-api"
    alias = "api"

  [[profiles.links]]
    src = "~/Developer/my-frontend"
    alias = "frontend"
```

2. Run HorseLens:

```sh
horselens
```

3. Select a workspace and press `↵` to open a terminal inside it. Your AI agent now only sees `api/` and `frontend/`.

## Keybindings

| Key             | Action                                |
| --------------- | ------------------------------------- |
| `n`             | New workspace                         |
| `e`             | Edit workspace                        |
| `d`             | Delete workspace (with confirm modal) |
| `↵`             | Open terminal in workspace            |
| `Ctrl+L`        | Focus terminal                        |
| `Ctrl+H`        | Focus sidebar                         |
| `Ctrl+B`        | Toggle sidebar                        |
| `PgUp` / `PgDn` | Scroll terminal history               |
| Mouse wheel     | Scroll terminal history               |
| `q`             | Quit                                  |

## How It Works

When you open a workspace, HorseLens creates a directory at:

```
~/.local/share/horselens/workspaces/{name}/
```

Inside that directory, each `link` from your config becomes a symlink pointing to the original source folder. When you open a terminal in that workspace, your shell's working directory is this folder — so tools like `claude`, `aider`, or `grep` only traverse the symlinked projects.

No files are moved or copied. Deleting a workspace only removes the symlinks, never the originals.

## Configuration

**Location:** `~/.config/horselens/config.toml`

```toml
[[profiles]]
  name = "auth-feature"

  [[profiles.links]]
    src = "~/Developer/backend"
    alias = "backend"

  [[profiles.links]]
    src = "~/Developer/auth-lib"
    alias = "auth"

[[profiles]]
  name = "data-pipeline"

  [[profiles.links]]
    src = "~/Developer/ingestion"
    alias = "ingestion"

  [[profiles.links]]
    src = "~/Developer/transforms"
    alias = "transforms"
```

| Field                      | Description                                    |
| -------------------------- | ---------------------------------------------- |
| `profiles[].name`          | Workspace name (used as directory name)        |
| `profiles[].links[].src`   | Path to the source directory (`~` is expanded) |
| `profiles[].links[].alias` | Name of the symlink inside the workspace       |

## Contributing

Contributions are welcome. Please open an issue first to discuss significant changes.

```sh
git clone https://github.com/rizkyizh/horse-lens.git
cd horse-lens
make build
./horselens
```

## License

[MIT](LICENSE) © 2026 rizkyizh
