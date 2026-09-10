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

// SkillRoots returns OpenCode's own user-level skill directories. Confirmed
// empirically (OpenCode 1.18.20, `opencode debug skill`) that OpenCode reads both
// paths, even though only the first is documented; discovery.go only scans
// roots[0], so `.config/opencode/skills` is the one Agent Studio treats as
// OpenCode's scope. OpenCode also reads `~/.agents/skills` and `~/.claude/skills`
// natively, and (also confirmed empirically) prefers its own copy by name over
// those when both exist -- so Project skills are never propagated here: doing so
// would shadow the canonical copy with a duplicate that silently goes stale.
func (Adapter) SkillRoots(home string) []string {
	return []string{
		filepath.Join(home, ".config", "opencode", "skills"),
		filepath.Join(home, ".opencode", "skills"),
	}
}

// ProjectSkillRoot returns OpenCode's own project-local skill directory. As with
// SkillRoots, this is never a propagation target for Project scope skills:
// OpenCode already discovers `.agents/skills` and `.claude/skills` in the project
// natively.
func (Adapter) ProjectSkillRoot(projectPath string) string {
	return filepath.Join(projectPath, ".opencode", "skills")
}

func (Adapter) Provider() domain.Provider {
	return domain.ProviderOpenCode
}
