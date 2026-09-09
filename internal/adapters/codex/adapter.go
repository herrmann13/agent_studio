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

// SkillRoots returns Codex's own skill directory. Confirmed empirically (Codex CLI
// 0.153.0, `codex debug prompt-input`) that Codex reads `$CODEX_HOME/skills` (which
// defaults to here) IN ADDITION to `.agents/skills` -- OpenAI's own published skills
// doc only documents the latter, so don't trust that doc alone for this. Codex does
// not deduplicate: the same skill name present in both roots is listed twice. So
// Global/Project skills are never propagated here (see skill_propagation.go); this
// directory remains available for a skill placed independently, just for Codex.
func (Adapter) SkillRoots(home string) []string {
	return []string{
		filepath.Join(home, ".codex", "skills"),
	}
}

// ProjectSkillRoot returns Codex's own project-local skill directory (also confirmed
// empirically). As with SkillRoots, this is never a propagation target.
func (Adapter) ProjectSkillRoot(projectPath string) string {
	return filepath.Join(projectPath, ".codex", "skills")
}

func (Adapter) Provider() domain.Provider {
	return domain.ProviderCodex
}
