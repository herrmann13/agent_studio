# Architecture Overview

Agent Studio configures coding agents but does not replace them. Users continue running OpenCode, Claude, and Codex in their existing terminals.

## Boundaries

- The React frontend renders discovery results and configuration changes.
- Wails exposes application use cases from Go to the frontend.
- The application layer coordinates discovery, profiles, previews, backups, and writes.
- Agent adapters isolate provider-specific paths and file formats.
- The domain layer remains independent of Wails, filesystems, and providers.

## Initial Flow

1. An adapter discovers a provider's known configuration and skill locations.
2. The application normalizes discovered skills and configurations.
3. A user assigns a profile to an agent or project.
4. The adapter generates a preview of native-file changes.
5. The application creates a backup and applies the approved changes.

The first discovery release is read-only. Writing native configuration is added only after its adapter has validated paths, formats, previews, and backups.
