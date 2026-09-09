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

// SkillRoots returns OpenCode's own global skill directory. OpenCode also reads
// `~/.agents/skills` and `~/.claude/skills` natively (see ProjectSkillRoot), so
// Global- and Project-scope skills are never propagated here; this directory
// remains available for a skill placed independently, just for OpenCode.
func (Adapter) SkillRoots(home string) []string {
	return []string{
		filepath.Join(home, ".config", "opencode", "skills"),
	}
}

// ProjectSkillRoot returns OpenCode's own project-local skill directory. As with
// SkillRoots, this is never a propagation target for Global or Project scope
// skills: OpenCode already discovers `.agents/skills` and `.claude/skills` in the
// project natively.
func (Adapter) ProjectSkillRoot(projectPath string) string {
	return filepath.Join(projectPath, ".opencode", "skills")
}

func (Adapter) Provider() domain.Provider {
	return domain.ProviderOpenCode
}
