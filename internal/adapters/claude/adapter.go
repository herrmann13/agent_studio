package claude

import (
	"os"
	"path/filepath"

	"github.com/herrmann13/agent_studio/internal/adapters"
	"github.com/herrmann13/agent_studio/internal/domain"
)

type Adapter struct{}

func (Adapter) Detect(env adapters.Environment) []adapters.Installation {
	commandPath := env.FindCommand("claude")
	if commandPath == "" {
		return nil
	}
	configPath := filepath.Join(env.Home, ".claude", "settings.json")
	status := "installed"
	var config *domain.ConfigFile
	if _, err := os.Stat(configPath); err == nil {
		status = "configured"
		config = &domain.ConfigFile{Provider: domain.ProviderClaude, Path: configPath, Scope: "global"}
	}
	agent := domain.Agent{ID: "claude", Name: "Claude Code", Provider: domain.ProviderClaude, Status: status, ConfigPath: configPath, CommandPath: commandPath}
	return []adapters.Installation{{
		Agent:  agent,
		Config: config,
		Scope:  domain.Scope{ID: agent.ID, Name: agent.Name, Kind: "agent", Provider: agent.Provider, Root: Adapter{}.UserSkillRoot(env.Home)},
	}}
}

func (Adapter) UserSkillRoot(home string) string {
	return filepath.Join(home, ".claude", "skills")
}

func (Adapter) ProjectSkillRoot(projectPath string) string {
	return filepath.Join(projectPath, ".claude", "skills")
}

func (Adapter) Provider() domain.Provider {
	return domain.ProviderClaude
}
