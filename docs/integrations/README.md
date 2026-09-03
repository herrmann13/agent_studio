# Agent Integrations

Each provider implementation lives under `internal/adapters` and owns discovery, parsing, previewing, and writing of provider-specific configuration.

Initial providers:

- OpenCode
- Claude
- Codex

No provider assumptions should be added to the frontend or domain layers.
