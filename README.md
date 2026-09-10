# Agent Studio

> Local-first desktop companion for configuring skills and profiles for OpenCode, Claude, and Codex — without changing your terminal-first workflow.

<p align="center">
  <a href="https://github.com/herrmann13/agent_studio/blob/main/LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License"></a>
  <a href="https://github.com/herrmann13/agent_studio/releases"><img src="https://img.shields.io/github/v/release/herrmann13/agent_studio" alt="Latest release"></a>
  <a href="https://github.com/herrmann13/agent_studio/releases"><img src="https://img.shields.io/badge/platform-macOS%20%7C%20Linux-lightgrey" alt="Platform"></a>
</p>

Agent Studio gives you a single visual overview of every skill you have configured across your coding agents. Discover what is installed, copy skills between your agents and projects, and control exactly how each one is used — all while you keep working from the terminal.

Everything runs locally. No cloud, no accounts, no telemetry.

## Screenshots

<!-- TODO: add a screenshot showing the workspace overview (agents + skills) -->
<!-- ![Agent Studio workspace](docs/screenshots/workspace.png) -->

## Features

- **Discover your setup** — a read-only scan of your installed agents (OpenCode, Claude Code, and Codex) and the skills each one has.
- **Organize with scopes** — skills live in two places: **Agent** (per tool) and **Project** (per repository).
- **Copy with drag-and-drop** — move a skill from one scope to another with a simple drag.
- **Track projects** — add a project folder to get a dedicated `.agents/skills` destination for it.
- **Install from URL** — paste a public GitHub, GitLab, or Bitbucket repository and install a skill into any scope (Git clone with a ZIP fallback).
- **Control how skills are used** — set each skill to *Automatic*, *Always active*, *Explicit*, or *Disabled*, synced to the agent's native configuration.
- **Spot duplicates and conflicts** — skills with the same name are flagged so you can resolve them.
- **Safe by default** — every change creates a local backup first, and deletions require confirmation.
- **Automatic updates** — check for and install new versions from within the app, with checksum verification.

## Installation

Download the latest installer from the [Releases](https://github.com/herrmann13/agent_studio/releases) page:

| Platform | Package |
| --- | --- |
| macOS (Apple Silicon) | `agent-studio-<version>-macos-arm64.dmg` |
| macOS (Intel) | `agent-studio-<version>-macos-amd64.dmg` |
| Linux (amd64) | `agent-studio-<version>-linux-amd64.deb` |

## Getting started

1. Launch Agent Studio. It scans your machine for OpenCode, Claude Code, and Codex automatically.
2. **Track a project** to create its skill destination, or use the per-agent scopes.
3. **Copy skills** between scopes by dragging them, or **install** one from a public repository URL.
4. Set a **usage mode** on any skill to change how the agent may invoke it.
5. Check for **updates** from the toolbar to stay current.

## Development

Agent Studio is built with Go, Wails, and React. The root `Makefile` is the supported interface:

```sh
make setup    # Verify Go, Bun, and Wails; install locked dependencies.
make dev      # Start the desktop app with hot reload.
make test     # Build the frontend and run Go tests.
make check    # Run tests and validate formatting.
make build    # Build the production app for the current platform.
make package  # Create a .dmg on macOS or .deb on Linux.
```

On Ubuntu 24.04 and other distributions that provide WebKitGTK 4.1, install the native build dependencies first:

```sh
sudo apt install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
```

## Contributing

Contributions are welcome. See [`docs/architecture/overview.md`](docs/architecture/overview.md) for the architecture and [`docs/integrations/README.md`](docs/integrations/README.md) for how agent integrations work.

## License

[MIT](LICENSE) © Henrique Herrmann
