package application

import (
	"fmt"
	"os"
	"path/filepath"

	"agent-studio/internal/adapters"
	"agent-studio/internal/domain"
)

// projectRootFromScopeRoot derives the project directory from a project scope's
// Root (which is <project>/.agents/skills).
func projectRootFromScopeRoot(scopeRoot string) string {
	return filepath.Dir(filepath.Dir(scopeRoot))
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

// propagateToGlobalAgents copies a skill directory into every agent's canonical
// global skill location so all supported agents can use it regardless of project.
// The source (`~/.agents/skills/<skill>`) remains the canonical copy and wins: existing
// propagated copies are replaced unconditionally.
func (s *DiscoveryService) propagateToGlobalAgents(sourceDir string) error {
	return s.propagateToAgentRoots(sourceDir, func(adapter adapters.Adapter) string {
		roots := adapter.SkillRoots(s.home)
		if len(roots) == 0 {
			return ""
		}
		return roots[0]
	})
}

// providersWithIndependentSkillCopy lists the agents that actually need a physical,
// separate copy of a Global/Project skill, rather than already seeing it at its
// canonical `.agents/skills` path. Confirmed empirically against the real CLIs, not
// just their docs (which turned out to be incomplete for Codex and OpenCode):
//   - Claude Code only ever reads `~/.claude/skills` and `.claude/skills` — it has no
//     knowledge of `.agents/skills`, so it needs a real copy to see the skill at all.
//   - Codex reads `.agents/skills` (repo scope, walking to the repository root) and
//     `$HOME/.agents/skills` (user scope) directly — the same canonical directories
//     Agent Studio already uses for Project and Global scope. It ALSO reads its own
//     `.codex/skills`/`$CODEX_HOME/skills` (undocumented, found by running
//     `codex debug prompt-input`), but does not deduplicate: a copy placed there
//     under the same name is listed twice.
//   - OpenCode reads `.agents/skills` and `.claude/skills` natively too (both project
//     and global) alongside its own directory. Unlike Codex it does deduplicate by
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

// removePropagatedGlobalCopies removes a skill's propagated copies from each agent's
// global skill location without touching the canonical `~/.agents/skills` copy.
func (s *DiscoveryService) removePropagatedGlobalCopies(skillDirName string) error {
	return s.removePropagatedCopiesFromRoots(skillDirName, func(adapter adapters.Adapter) string {
		roots := adapter.SkillRoots(s.home)
		if len(roots) == 0 {
			return ""
		}
		return roots[0]
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
