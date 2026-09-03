package claude

import (
	"os"
	"os/exec"
	"path/filepath"

	"agent-studio/internal/domain"
)

type Adapter struct{}

func (Adapter) Detect(home string) (domain.Agent, *domain.ConfigFile) {
	configPath := filepath.Join(home, ".claude", "settings.json")
	commandPath, _ := exec.LookPath("claude")
	_, err := os.Stat(configPath)
	status := "not found"
	var config *domain.ConfigFile
	if err == nil {
		status = "configured"
		config = &domain.ConfigFile{Provider: domain.ProviderClaude, Path: configPath, Scope: "global"}
	} else if commandPath != "" {
		status = "installed"
	}

	return domain.Agent{ID: "claude", Name: "Claude Code", Provider: domain.ProviderClaude, Status: status, ConfigPath: configPath, CommandPath: commandPath}, config
}

func (Adapter) SkillRoots(home string) []string {
	return []string{
		filepath.Join(home, ".claude", "skills"),
		filepath.Join(home, ".agents", "skills"),
	}
}
