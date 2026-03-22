# workstack-cli

Terminal UI for managing multiple AI coding agents (Claude, Codex) across repositories. Run `ws` and manage everything from one screen — no more juggling tmux sessions.

## Features

- **Split-pane TUI** — session list + live terminal output side by side
- **Responsive layout** — split view on wide screens, tab-toggle on narrow/mobile (Termius)
- **Live terminal capture** — see agent output without attaching to tmux
- **Git worktree isolation** — each agent works in its own worktree
- **Multiple agent types** — Claude and Codex support

## Install

Requires Go 1.24+, tmux, and gcc (for SQLite).

```bash
git clone https://github.com/ilyalosinski/workstack-cli.git
cd workstack-cli
CGO_ENABLED=1 go build -o ws .
sudo cp ws /usr/local/bin/
```

## Usage

```bash
ws
```

### Key bindings

| Key | Action |
|-----|--------|
| `n` | New session |
| `a` | Add agent to session |
| `s` | Start agent/session |
| `S` | Stop agent/session |
| `Enter` | Attach to agent tmux (Ctrl+B, D to detach back) |
| `Tab` | Toggle focus (wide) or switch view (narrow) |
| `↑/↓` or `j/k` | Navigate |
| `D` | Delete session |
| `q` | Quit (agents keep running) |

## Config

Optional config at `~/.config/workstack/config.json`:

```json
{
  "base_dir": "~/Development"
}
```

## Architecture

```
workstack-cli/
├── main.go           # entry point
├── db/db.go          # SQLite storage
├── session/manager.go # tmux + worktree management
├── config/config.go  # configuration
└── tui/
    ├── tui.go        # main model, layout, input handling
    ├── sidebar.go    # session/agent list with windowed scrolling
    └── terminal.go   # live tmux pane capture via viewport
```
