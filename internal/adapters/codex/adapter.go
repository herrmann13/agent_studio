package codex

import (
	"os"
	"path/filepath"

	"agent-studio/internal/adapters"
	"agent-studio/internal/domain"
)

type Adapter struct{}

// Detect uses Codex's own skill directory as its scope. Confirmed empirically
// (Codex CLI 0.153.0, `codex debug prompt-input`) that Codex reads
// `$CODEX_HOME/skills` (which defaults to here) IN ADDITION to `.agents/skills` --
// OpenAI's own published skills doc only documents the latter, so don't trust that
// doc alone for this. Codex does not deduplicate: the same skill name present in
// both roots is listed twice. So Project skills are never propagated here (see
// skill_propagation.go); this directory remains available for a skill placed
// independently, just for Codex.
func (Adapter) Detect(env adapters.Environment) []adapters.Installation {
	commandPath := env.FindCommand("codex")
	if commandPath == "" {
		return nil
	}
	configPath := filepath.Join(env.Home, ".codex", "config.toml")
	status := "installed"
	var config *domain.ConfigFile
	if _, err := os.Stat(configPath); err == nil {
		status = "configured"
		config = &domain.ConfigFile{Provider: domain.ProviderCodex, Path: configPath, Scope: "global"}
	}
	agent := domain.Agent{ID: "codex", Name: "Codex", Provider: domain.ProviderCodex, Status: status, ConfigPath: configPath, CommandPath: commandPath}
	return []adapters.Installation{{
		Agent:  agent,
		Config: config,
		Scope:  domain.Scope{ID: agent.ID, Name: agent.Name, Kind: "agent", Provider: agent.Provider, Root: Adapter{}.UserSkillRoot(env.Home)},
	}}
}

func (Adapter) UserSkillRoot(home string) string {
	return filepath.Join(home, ".codex", "skills")
}

// ProjectSkillRoot returns Codex's own project-local skill directory (also confirmed
// empirically). As with the user-level directory, this is never a propagation target.
func (Adapter) ProjectSkillRoot(projectPath string) string {
	return filepath.Join(projectPath, ".codex", "skills")
}

func (Adapter) Provider() domain.Provider {
	return domain.ProviderCodex
}
