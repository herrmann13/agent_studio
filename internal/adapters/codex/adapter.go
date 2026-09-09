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

// SkillRoots returns no independent directory: per OpenAI's documented Codex skill
// discovery (REPO/USER/ADMIN/SYSTEM scopes), Codex only ever reads `.agents/skills`
// (repo scope, walking up to the repository root) and `$HOME/.agents/skills` (user
// scope) — the same canonical directories Agent Studio already treats as the Project
// and Global scopes. There is no separate `.codex/skills` location Codex scans, so
// showing one as a distinct, copyable scope would silently do nothing for Codex.
func (Adapter) SkillRoots(home string) []string {
	return nil
}

// ProjectSkillRoot is unused: Codex has no independent per-project skill directory to
// propagate into (see SkillRoots). It documents the real path Codex reads instead.
func (Adapter) ProjectSkillRoot(projectPath string) string {
	return filepath.Join(projectPath, ".agents", "skills")
}

func (Adapter) Provider() domain.Provider {
	return domain.ProviderCodex
}
