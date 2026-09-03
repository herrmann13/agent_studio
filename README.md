# Agent Studio

Local-first desktop companion for configuring skills and profiles for OpenCode, Claude, and Codex without changing the terminal-first workflow.

## Stack

- Go and Wails for desktop integration, filesystem access, and agent adapters.
- React and TypeScript for the user interface.

## Development

```sh
wails dev
```

Use the Wails binary installed by Go if it is not in your `PATH`:

```sh
"$(go env GOPATH)/bin/wails" dev
```

## Architecture

- `internal/domain`: core models and business rules.
- `internal/application`: use cases that coordinate the domain.
- `internal/adapters`: integrations for OpenCode, Claude, and Codex.
- `internal/infrastructure`: filesystem, storage, and platform implementations.
- `frontend/src/features`: UI grouped by product responsibility.

Further decisions are recorded in [`docs/architecture/overview.md`](docs/architecture/overview.md).
