package opencode

import (
	"os"
	"path/filepath"

	"github.com/herrmann13/agent_studio/internal/adapters"
	"github.com/herrmann13/agent_studio/internal/domain"
)

type Adapter struct{}

// Detect reports OpenCode v1 and v2 as separate agents, because both can be
// installed side by side (v2's installer keeps `opencode` and adds an `opencode2`
// alias, while a v1 build may still live in Homebrew or npm). Every executable is
// classified by its `--version` major number; the first one found per major wins.
//
// Both versions read the same `~/.config/opencode` directory, so their scopes share
// `~/.config/opencode/skills`. They differ in how skills are identified there:
//   - v1 (confirmed with `opencode debug skill`, 1.18.20) reads `<name>/SKILL.md`
//     folders in `~/.config/opencode/skills` and the undocumented `~/.opencode/skills`.
//   - v2 (confirmed against `GET /api/skill`, 2.1.0) reads both
//     `~/.config/opencode/skills` and `~/.config/opencode/skill`, nested folders up
//     to six levels deep, and also a loose `<name>.md` file directly in either root.
//     See domain.SkillLayoutOpenCodeV2.
//
// Both versions also read `.agents/skills` and `.claude/skills` natively and prefer
// their own copy by name over those when both exist -- so Project skills are never
// propagated here: doing so would shadow the canonical copy with a duplicate that
// silently goes stale.
func (Adapter) Detect(env adapters.Environment) []adapters.Installation {
	configPath := filepath.Join(env.Home, ".config", "opencode", "opencode.json")
	var config *domain.ConfigFile
	status := "installed"
	if _, err := os.Stat(configPath); err == nil {
		status = "configured"
		config = &domain.ConfigFile{Provider: domain.ProviderOpenCode, Path: configPath, Scope: "global"}
	}
	skillsRoot := Adapter{}.UserSkillRoot(env.Home)

	if env.FindCommands == nil {
		return nil
	}
	var installations []adapters.Installation
	found := map[int]bool{}
	for _, commandPath := range env.FindCommands("opencode", "opencode2") {
		version, major := "", 2
		if env.CommandVersion != nil {
			if parsed, parsedMajor, ok := adapters.ParseVersion(env.CommandVersion(commandPath)); ok {
				version, major = parsed, parsedMajor
			}
		}
		if major < 2 {
			major = 1
		}
		if found[major] {
			continue
		}
		found[major] = true

		agent := domain.Agent{ID: "opencode", Name: "OpenCode v1", Provider: domain.ProviderOpenCode, Status: status, ConfigPath: configPath, CommandPath: commandPath, Version: version}
		scope := domain.Scope{Kind: "agent", Provider: domain.ProviderOpenCode, Root: skillsRoot}
		if major >= 2 {
			agent.ID, agent.Name = "opencode-v2", "OpenCode v2"
			scope.SkillLayout = domain.SkillLayoutOpenCodeV2
		}
		scope.ID, scope.Name = agent.ID, agent.Name
		installations = append(installations, adapters.Installation{Agent: agent, Config: config, Scope: scope})
		// Both versions share one config file; report it once.
		config = nil
	}
	// Keep v1 before v2 regardless of which executable PATH lists first.
	if len(installations) == 2 && installations[0].Agent.ID == "opencode-v2" {
		installations[0], installations[1] = installations[1], installations[0]
	}
	return installations
}

func (Adapter) UserSkillRoot(home string) string {
	return filepath.Join(home, ".config", "opencode", "skills")
}

// ProjectSkillRoot returns OpenCode's own project-local skill directory. This is
// never a propagation target for Project scope skills: OpenCode already discovers
// `.agents/skills` and `.claude/skills` in the project natively.
func (Adapter) ProjectSkillRoot(projectPath string) string {
	return filepath.Join(projectPath, ".opencode", "skills")
}

func (Adapter) Provider() domain.Provider {
	return domain.ProviderOpenCode
}
