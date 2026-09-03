# Agent Studio

Local-first desktop companion for configuring skills and profiles for OpenCode, Claude, and Codex without changing the terminal-first workflow.

## Stack

- Go and Wails for desktop integration, filesystem access, and agent adapters.
- React and TypeScript for the user interface.

## Development

The root `Makefile` is the supported interface for local development:

```sh
make setup    # Verify Go, Bun, and Wails; install locked dependencies.
make dev      # Start the Wails desktop app with hot reload.
make dev-browser # Start Wails and expose the integrated browser endpoint.
make test     # Build the frontend and run Go tests.
make check    # Run tests and validate the worktree formatting.
make build    # Build the production app for the current platform.
make package  # Create a .dmg on macOS or .deb on Linux.
```

Pass an explicit version when packaging a release candidate locally:

```sh
make package VERSION=v0.1.0-rc.1
```

The frontend uses Bun. Do not create or update `package-lock.json`.

## Releases

GitHub Actions validates pushes and pull requests. Pushing a version tag builds native release artifacts and publishes a GitHub Release:

```sh
git tag v0.1.0
git push origin v0.1.0
```

The release contains:

```text
agent-studio-v0.1.0-macos-arm64.dmg
agent-studio-v0.1.0-macos-amd64.dmg
agent-studio-v0.1.0-linux-amd64.deb
```

macOS and Linux packages are created on their native GitHub runners because Wails uses platform-specific CGO dependencies.

## Architecture

- `internal/domain`: core models and business rules.
- `internal/application`: use cases that coordinate the domain.
- `internal/adapters`: integrations for OpenCode, Claude, and Codex.
- `internal/infrastructure`: filesystem, storage, and platform implementations.
- `frontend/src/features`: UI grouped by product responsibility.

Further decisions are recorded in [`docs/architecture/overview.md`](docs/architecture/overview.md).
