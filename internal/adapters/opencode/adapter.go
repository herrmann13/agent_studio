package opencode

import (
	"os"
	"os/exec"
	"path/filepath"

	"agent-studio/internal/domain"
)

type Adapter struct{}

func (Adapter) Detect(home string) (domain.Agent, *domain.ConfigFile) {
	configPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	commandPath, _ := exec.LookPath("opencode")
	_, err := os.Stat(configPath)
	status := "not found"
	var config *domain.ConfigFile
	if err == nil {
		status = "configured"
		config = &domain.ConfigFile{Provider: domain.ProviderOpenCode, Path: configPath, Scope: "global"}
	} else if commandPath != "" {
		status = "installed"
	}

	return domain.Agent{ID: "opencode", Name: "OpenCode", Provider: domain.ProviderOpenCode, Status: status, ConfigPath: configPath, CommandPath: commandPath}, config
}

func (Adapter) SkillRoots(home string) []string {
	return []string{
		filepath.Join(home, ".config", "opencode", "skills"),
		filepath.Join(home, ".opencode", "skills"),
	}
}
