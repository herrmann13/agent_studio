package codex

import (
	"os"
	"os/exec"
	"path/filepath"

	"agent-studio/internal/domain"
)

type Adapter struct{}

func (Adapter) Detect(home string) (domain.Agent, *domain.ConfigFile) {
	configPath := filepath.Join(home, ".codex", "config.toml")
	commandPath, _ := exec.LookPath("codex")
	_, err := os.Stat(configPath)
	status := "not found"
	var config *domain.ConfigFile
	if err == nil {
		status = "configured"
		config = &domain.ConfigFile{Provider: domain.ProviderCodex, Path: configPath, Scope: "global"}
	} else if commandPath != "" {
		status = "installed"
	}

	return domain.Agent{ID: "codex", Name: "Codex", Provider: domain.ProviderCodex, Status: status, ConfigPath: configPath, CommandPath: commandPath}, config
}

func (Adapter) SkillRoots(home string) []string {
	return []string{
		filepath.Join(home, ".codex", "skills"),
	}
}
