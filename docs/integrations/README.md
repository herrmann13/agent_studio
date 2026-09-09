# Agent Integrations

Each provider implementation lives under `internal/adapters` and owns discovery, parsing, previewing, and writing of provider-specific configuration.

Initial providers:

- OpenCode
- Claude
- Codex

No provider assumptions should be added to the frontend or domain layers.

## Why only Claude gets a propagated copy

Per each provider's own documentation, only Claude Code actually needs a physical,
independent copy of a Global/Project skill:

- **Claude Code** reads only `~/.claude/skills` and `.claude/skills`. It has no
  knowledge of `.agents/skills`, so `Adapter.SkillRoots`/`ProjectSkillRoot` point at a
  real, separate directory, and `skill_propagation.go` copies into it.
- **Codex** reads `.agents/skills` directly — walking from the working directory up
  to the repository root (repo scope) and `$HOME/.agents/skills` (user scope). These
  are exactly Agent Studio's own Project and Global scope roots, so `Adapter.SkillRoots`
  returns nil: there is no separate `.codex/skills` location Codex scans, and showing
  one as a scope would silently do nothing.
- **OpenCode** reads `.agents/skills` *and* `.claude/skills` natively too (project and
  global). Its own directory (`.opencode/skills` / `~/.config/opencode/skills`) still
  exists as a scope for a skill placed independently just for OpenCode, but it is never
  an automatic propagation target for Global/Project skills — OpenCode already sees
  those directly, and a redundant same-named copy is a documented cause of a skill
  failing to load ("ensure skill names are unique across all locations").

`internal/application/skill_propagation.go`'s `providersWithIndependentSkillCopy`
is the single place this is encoded. Sources: [Claude Code skills](https://code.claude.com/docs/en/skills),
[Codex skills](https://developers.openai.com/codex/skills), [Codex config reference](https://developers.openai.com/codex/config-reference),
[OpenCode Agent Skills](https://opencode.ai/docs/skills/).

This was reverse-engineered before being checked against the above docs — if a future
provider release changes these paths, re-verify against the primary docs rather than
assuming this file is still accurate.
