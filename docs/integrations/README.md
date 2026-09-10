# Agent Integrations

Each provider implementation lives under `internal/adapters` and owns discovery, parsing, previewing, and writing of provider-specific configuration.

Initial providers:

- OpenCode
- Claude
- Codex

No provider assumptions should be added to the frontend or domain layers.

## Why only Claude gets a propagated copy

Only Claude Code actually needs a physical, independent copy of a Project
skill. This was checked two ways: against each provider's published docs, and then
empirically against the real installed CLIs (`codex debug prompt-input`,
`opencode debug skill`) — because **the published docs turned out to be wrong or
incomplete for both Codex and OpenCode**. Trust the empirical findings below over
the docs if the two ever disagree again.

- **Claude Code** reads only `~/.claude/skills` and `.claude/skills`. It has no
  knowledge of `.agents/skills`, so `Adapter.SkillRoots`/`ProjectSkillRoot` point at a
  real, separate directory, and `skill_propagation.go` copies into it. (Docs and
  behavior agreed here.)
- **Codex** reads `.agents/skills` directly — walking from the working directory up
  to the repository root (repo scope) and `$HOME/.agents/skills` (user scope) — exactly
  Agent Studio's own Project scope root. OpenAI's published skills doc
  ([developers.openai.com/codex/skills](https://developers.openai.com/codex/skills))
  stops there, but the real CLI (0.153.0) *also* reads `$CODEX_HOME/skills` (defaults
  to `~/.codex/skills`) and `<project>/.codex/skills`, undocumented. Codex does **not**
  deduplicate: the same skill name in both roots is listed twice. So `.codex/skills`
  stays a real, independent scope in the UI, but Project never propagates into
  it — that would just create a duplicate listing.
- **OpenCode** reads `.agents/skills` *and* `.claude/skills` natively too, project- and
  user-scoped, alongside its own directory (`.opencode/skills` project-local,
  `~/.config/opencode/skills` *and*, also undocumented but confirmed with the real
  CLI 1.18.20, `~/.opencode/skills` user-level). Unlike Codex, OpenCode *does* deduplicate
  by name — but it prefers its own copy over the external one, confirmed by writing
  conflicting content to both and checking which one it reported. So propagating a
  copy there would not error, it would just quietly shadow future edits to the
  canonical skill with a stale duplicate. Its own directory remains a scope for a
  skill placed independently just for OpenCode, never an automatic propagation target.
- The Codex `skills.config.path` field in `config.toml`: the published
  [config reference](https://developers.openai.com/codex/config-reference) describes
  it as "a skill folder containing SKILL.md", but the real CLI silently ignores a
  folder there — only the `SKILL.md` file path actually disables the skill. Verified
  by writing both forms and checking which one removed the skill from
  `codex debug prompt-input`'s listing.

`internal/application/skill_propagation.go`'s `providersWithIndependentSkillCopy`
is the single place the propagation decision is encoded; the file-vs-folder
`skills.config.path` fix lives in `internal/application/codex_policy.go`.

If a future provider release changes any of this, re-verify against the installed
CLI directly (there's usually a `debug` subcommand that dumps what it actually
discovered) rather than trusting that provider's own docs alone.
