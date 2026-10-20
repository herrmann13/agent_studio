package application

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/herrmann13/agent_studio/internal/adapters"
	"github.com/herrmann13/agent_studio/internal/domain"
)

// projectRootFromScopeRoot derives the project directory from a project scope's
// Root (which is <project>/.agents/skills).
func projectRootFromScopeRoot(scopeRoot string) string {
	return filepath.Dir(filepath.Dir(scopeRoot))
}

// globalCopyMarker marks a user-level copy that Agent Studio made from the Global
// scope. Only marked copies are ever replaced or removed, and they are hidden from
// that agent's own lane so the skill is listed once, under Global.
const globalCopyMarker = ".agent-studio-global"

func (s *DiscoveryService) globalSkillRoot() string {
	return filepath.Join(s.home, ".agents", "skills")
}

// installedGlobalCopyRoots lists the user-level skill directories of installed
// agents that cannot read `~/.agents/skills` themselves (today only Claude Code).
func (s *DiscoveryService) installedGlobalCopyRoots() []string {
	needsCopy := providersWithIndependentSkillCopy()
	var roots []string
	for _, adapter := range s.adapters {
		if needsCopy[adapter.Provider()] && len(adapter.Detect(s.env)) > 0 {
			roots = append(roots, adapter.UserSkillRoot(s.home))
		}
	}
	return roots
}

// checkGlobalCopyTargets refuses before anything is written when an installed
// agent already has its own, unmanaged skill with the same folder name.
func (s *DiscoveryService) checkGlobalCopyTargets(scope domain.Scope, skillDirName string) error {
	if scope.Kind != "global" {
		return nil
	}
	for _, root := range s.installedGlobalCopyRoots() {
		destination := filepath.Join(root, skillDirName)
		if _, err := os.Stat(destination); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return fmt.Errorf("inspect %s: %w", destination, err)
		}
		if !isGlobalCopy(destination) {
			return fmt.Errorf("%s already contains its own %q skill; remove or rename it before making it global", root, skillDirName)
		}
	}
	return nil
}

func (s *DiscoveryService) propagateToGlobalAgents(sourceDir string) error {
	skillDirName := filepath.Base(sourceDir)
	if err := s.checkGlobalCopyTargets(domain.Scope{Kind: "global"}, skillDirName); err != nil {
		return err
	}
	for _, root := range s.installedGlobalCopyRoots() {
		destination := filepath.Join(root, skillDirName)
		if err := copyDirectoryForce(sourceDir, destination); err != nil {
			return fmt.Errorf("propagate skill to %s: %w", root, err)
		}
		if err := os.WriteFile(filepath.Join(destination, globalCopyMarker), []byte("Managed by Agent Studio: copy of "+sourceDir+"\n"), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// removeGlobalCopies removes marked copies even for agents that were uninstalled
// since, so a deleted global skill never lingers anywhere.
func (s *DiscoveryService) removeGlobalCopies(skillDirName string) error {
	needsCopy := providersWithIndependentSkillCopy()
	for _, adapter := range s.adapters {
		if !needsCopy[adapter.Provider()] {
			continue
		}
		destination := filepath.Join(adapter.UserSkillRoot(s.home), skillDirName)
		if !isGlobalCopy(destination) {
			continue
		}
		if err := os.RemoveAll(destination); err != nil {
			return fmt.Errorf("remove global copy %s: %w", destination, err)
		}
	}
	return nil
}

func isGlobalCopy(skillDir string) bool {
	_, err := os.Stat(filepath.Join(skillDir, globalCopyMarker))
	return err == nil
}

// propagateToProjectAgents copies a skill directory into every agent's canonical
// project-level skill location so all supported agents can use it inside the project.
// The source (`.agents/skills/<skill>`) remains the canonical copy and wins: existing
// propagated copies are replaced unconditionally.
func (s *DiscoveryService) propagateToProjectAgents(sourceDir, projectPath string) error {
	return s.propagateToAgentRoots(sourceDir, func(adapter adapters.Adapter) string {
		return adapter.ProjectSkillRoot(projectPath)
	})
}

// providersWithIndependentSkillCopy lists the agents that actually need a physical,
// separate copy of a Project skill, rather than already seeing it at its
// canonical `.agents/skills` path. Confirmed empirically against the real CLIs, not
// just their docs (which turned out to be incomplete for Codex and OpenCode):
//   - Claude Code only ever reads `~/.claude/skills` and `.claude/skills` — it has no
//     knowledge of `.agents/skills`, so it needs a real copy to see the skill at all.
//   - Codex reads `.agents/skills` (repo scope, walking to the repository root) and
//     `$HOME/.agents/skills` (user scope) directly — the same canonical directory
//     Agent Studio already uses for Project scope. It ALSO reads its own
//     `.codex/skills`/`$CODEX_HOME/skills` (undocumented, found by running
//     `codex debug prompt-input`), but does not deduplicate: a copy placed there
//     under the same name is listed twice.
//   - OpenCode reads `.agents/skills` and `.claude/skills` natively too, alongside
//     its own directory. Unlike Codex it does deduplicate by
//     name, but (confirmed with `opencode debug skill`) it prefers its own copy over
//     the external one — so a stale propagated copy would silently shadow edits made
//     to the canonical skill instead of erroring or disappearing.
//
// Either way, copying into Codex's or OpenCode's own directory on top of the
// canonical one is actively worse than not copying at all, so only Claude gets one.
func providersWithIndependentSkillCopy() map[domain.Provider]bool {
	return map[domain.Provider]bool{domain.ProviderClaude: true}
}

func (s *DiscoveryService) propagateToAgentRoots(sourceDir string, rootFor func(adapters.Adapter) string) error {
	skillDirName := filepath.Base(sourceDir)
	needsCopy := providersWithIndependentSkillCopy()
	for _, adapter := range s.adapters {
		if !needsCopy[adapter.Provider()] {
			continue
		}
		root := rootFor(adapter)
		if root == "" {
			continue
		}
		destination := filepath.Join(root, skillDirName)
		if err := copyDirectoryForce(sourceDir, destination); err != nil {
			return fmt.Errorf("propagate skill to %s: %w", root, err)
		}
	}
	return nil
}

// removePropagatedCopies removes a skill's propagated copies from each agent's
// project-level skill location without touching the canonical `.agents/skills` copy.
func (s *DiscoveryService) removePropagatedCopies(projectPath, skillDirName string) error {
	return s.removePropagatedCopiesFromRoots(skillDirName, func(adapter adapters.Adapter) string {
		return adapter.ProjectSkillRoot(projectPath)
	})
}

func (s *DiscoveryService) removePropagatedCopiesFromRoots(skillDirName string, rootFor func(adapters.Adapter) string) error {
	needsCopy := providersWithIndependentSkillCopy()
	for _, adapter := range s.adapters {
		if !needsCopy[adapter.Provider()] {
			continue
		}
		root := rootFor(adapter)
		if root == "" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, skillDirName)); err != nil {
			return fmt.Errorf("remove propagated skill from %s: %w", root, err)
		}
	}
	return nil
}

// copyDirectoryForce copies source into destination, replacing destination first so
// the operation is idempotent even when the destination already exists.
func copyDirectoryForce(source, destination string) error {
	if err := os.RemoveAll(destination); err != nil {
		return err
	}
	return copyDirectory(source, destination)
}
