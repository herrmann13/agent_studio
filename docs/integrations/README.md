# Agent Integrations

Each provider implementation lives under `internal/adapters` and owns discovery, parsing, previewing, and writing of provider-specific configuration.

Initial providers:

- OpenCode
- Claude
- Codex

No provider assumptions should be added to the frontend or domain layers.

## Which agents are shown

Only agents whose executable is found are listed (a leftover config directory is
not enough). Besides `PATH`, common install directories such as `~/.local/bin`,
`~/.opencode/bin`, `/opt/homebrew/bin` and `/usr/local/bin` are searched, because
the desktop app launched from Finder or a `.desktop` file does not inherit the
login shell's `PATH` (see `internal/adapters/environment.go`).

OpenCode is classified by each executable's `opencode --version` major number
(`opencode` and its `opencode2` alias are both checked), and v1 and v2 appear as
separate agents when both are installed. They share `~/.config/opencode/skills`, so
a skill in that folder appears in both lanes but is one file: it is not flagged as
duplicated, and deleting it removes it for both. OpenCode v2 identifies skills
differently from v1 (confirmed with the 2.1.0 CLI's `GET /api/skill` against a
scratch `HOME`):

- it reads `~/.config/opencode/skill` (singular) in addition to `skills`;
- a loose `<name>.md` directly in either root is a skill, not only `<name>/SKILL.md`;
- folders are searched up to six levels deep, stopping at the first `SKILL.md`, and
  hidden entries are ignored;
- it still accepts the v1 `opencode.json` format (`permission.skill`,
  `instructions`) and `commands/`, so invocation policies are written the same way.

Copying a loose v2 skill to another agent turns it into `<name>/SKILL.md`.

## Global scope

The Global lane is `~/.agents/skills`, a skill there is available to every installed
agent. Codex (confirmed with `codex debug prompt-input`, 0.162.0) and OpenCode v1/v2
(confirmed with `GET /api/skill`, 2.1.0) read that directory natively. Claude Code
does not, so when it is installed it gets a copy in `~/.claude/skills/<name>`
marked with a `.agent-studio-global` file:

- marked copies are hidden from the Claude Code lane, so the skill is listed once;
- an existing, unmarked Claude skill with the same name is never overwritten: the
  copy is refused before anything is written;
- deleting the global skill removes its marked copies;
- invocation policies for a global skill are written only to installed agents'
  user-level configs;
- a symlink in `~/.claude/skills` pointing at the global skill (what `npx skills`
  creates) already gives Claude access, so it is kept instead of copied over, and
  removed together with the global skill;
- **Sync to agents** creates copies for skills placed in `~/.agents/skills` outside
  Agent Studio, refreshes stale ones, covers agents installed later, and removes
  marked copies (and links into `~/.agents/skills`) whose global skill was deleted
  outside Agent Studio. Each marker records its source directory, so global skills
  in nested folders are matched correctly.

Executables are also searched in nvm (`~/.nvm/versions/node/*/bin`, newest first),
pnpm, Yarn, asdf and mise directories. `--version` runs with the executable's own
directory on `PATH` (so an npm shim finds the `node` next to it), times out after
5 seconds even when a wrapper leaves a child process behind, and a failed run is
cached for one minute only, so a slow first start is retried without a broken
binary costing a timeout on every scan.

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

## skills.sh

Search uses the same public endpoint as `npx skills find`:
`GET https://skills.sh/api/search?q=<query>&limit=20` (queries shorter than two
characters are rejected by the API, so they are not sent). Each result's `source`
is a GitHub `owner/repository` slug and `skillId` the skill's registry slug;
installing one is exactly `npx skills add https://github.com/<source> --skill <skillId>`
through the existing installer (Git clone with ZIP fallback, slug resolution), so a
Global install also reaches Claude Code. Both values are validated before a URL is
built, and the query is the only data sent to skills.sh.
