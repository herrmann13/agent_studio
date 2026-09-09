package application

import (
	"fmt"
	"os"
	"path/filepath"

	"agent-studio/internal/adapters"
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

func (s *DiscoveryService) propagateToAgentRoots(sourceDir string, rootFor func(adapters.Adapter) string) error {
	skillDirName := filepath.Base(sourceDir)
	for _, adapter := range s.adapters {
		root := rootFor(adapter)
		if root == "" {
			continue
		}
		destination := filepath.Join(root, skillDirName)
		if err := copyDirectoryForce(sourceDir, destination); err != nil {
			return fmt.Errorf("propagate skill to %s: %w", root, err)
		}
		if adapter.Provider() == "codex" {
			if err := ensureCodexMetadata(destination); err != nil {
				return err
			}
		}
	}
	return nil
}

// ensureCodexMetadata writes a minimal Codex agent metadata file so Codex registers
// the skill. It preserves an existing non-empty metadata file if one is present.
func ensureCodexMetadata(skillDir string) error {
	metadataPath := filepath.Join(skillDir, "agents", "openai.yaml")
	if _, err := os.Stat(metadataPath); err == nil {
		return nil
	}
	skillMarkdown := filepath.Join(skillDir, "SKILL.md")
	skill, err := parseSkill(skillMarkdown)
	if err != nil {
		return fmt.Errorf("parse skill for Codex metadata: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(metadataPath), 0o755); err != nil {
		return err
	}
	content := fmt.Sprintf("name: %s\ndescription: %q\n", skill.Name, skill.Description)
	return writeAtomic(metadataPath, []byte(content), 0o644)
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
	for _, adapter := range s.adapters {
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
