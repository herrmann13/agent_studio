package application

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-studio/internal/domain"
)

const codexManagedMarker = "# Agent Studio managed skill policy: "

func (s *DiscoveryService) syncCodexPolicy(skill domain.Skill, scope domain.Scope, mode string) error {
	if mode == modeAlways {
		content, err := os.ReadFile(skill.Path)
		if err != nil {
			return fmt.Errorf("read Codex skill instructions: %w", err)
		}
		instructionsPath := filepath.Join(s.home, ".agent-studio", "generated", "codex", "instructions", skill.ID+".md")
		if err := writeAtomic(instructionsPath, append([]byte("# Agent Studio managed instruction: "+skill.Name+"\n\n"), content...), 0o644); err != nil {
			return err
		}
		instructionsFile := filepath.Join(s.home, ".codex", "AGENTS.md")
		if scope.Kind == "project" {
			instructionsFile = filepath.Join(filepath.Dir(filepath.Dir(scope.Root)), "AGENTS.md")
		}
		marker := codexManagedMarker + skill.ID
		block := marker + "\n" + string(content) + "\n# End Agent Studio managed skill policy"
		if err := updateManagedMarkdown(instructionsFile, marker, "# End Agent Studio managed skill policy", block, true); err != nil {
			return err
		}
	} else {
		instructionsFile := filepath.Join(s.home, ".codex", "AGENTS.md")
		if scope.Kind == "project" {
			instructionsFile = filepath.Join(filepath.Dir(filepath.Dir(scope.Root)), "AGENTS.md")
		}
		if err := updateManagedMarkdown(instructionsFile, codexManagedMarker+skill.ID, "# End Agent Studio managed skill policy", "", false); err != nil {
			return err
		}
	}

	if mode == modeExplicit {
		if err := removeCodexDisabledEntry(s, scope, skill.Path); err != nil {
			return err
		}
		return syncCodexSkillMetadata(skill)
	}
	if err := removeCodexSkillMetadata(skill); err != nil {
		return err
	}
	if mode != modeDisabled {
		return removeCodexDisabledEntry(s, scope, skill.Path)
	}
	return addCodexDisabledEntry(s, scope, skill.Path)
}

func syncCodexSkillMetadata(skill domain.Skill) error {
	return syncManagedCodexMetadata(filepath.Dir(skill.Path))
}

func syncManagedCodexMetadata(skillDirectory string) error {
	path := filepath.Join(skillDirectory, "agents", "openai.yaml")
	const marker = "# Agent Studio managed invocation policy"
	if existing, err := os.ReadFile(path); err == nil && !strings.Contains(string(existing), marker) {
		return fmt.Errorf("Codex metadata %q already exists and will not be overwritten", path)
	} else if err == nil {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	content := marker + "\npolicy:\n  allow_implicit_invocation: false\n"
	return writeAtomic(path, []byte(content), 0o644)
}

func removeCodexSkillMetadata(skill domain.Skill) error {
	return removeManagedCodexMetadata(filepath.Dir(skill.Path))
}

func removeManagedCodexMetadata(skillDirectory string) error {
	path := filepath.Join(skillDirectory, "agents", "openai.yaml")
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !strings.Contains(string(content), "# Agent Studio managed invocation policy") {
		return nil
	}
	return os.Remove(path)
}

func codexConfigPath(s *DiscoveryService, scope domain.Scope) string {
	if scope.Kind == "project" {
		return filepath.Join(filepath.Dir(filepath.Dir(scope.Root)), ".codex", "config.toml")
	}
	return filepath.Join(s.home, ".codex", "config.toml")
}

func codexPolicyMarker(path string) string {
	return codexManagedMarker + filepath.Clean(path)
}

func addCodexDisabledEntry(s *DiscoveryService, scope domain.Scope, skillPath string) error {
	path := codexConfigPath(s, scope)
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		content = nil
	} else if err != nil {
		return err
	}
	marker := codexPolicyMarker(skillPath)
	if strings.Contains(string(content), marker) {
		return nil
	}
	block := fmt.Sprintf("\n%s\n[[skills.config]]\npath = %q\nenabled = false\n# End Agent Studio managed skill policy\n", marker, skillPath)
	return writeWithBackup(path, append(content, []byte(block)...), 0o600)
}

func removeCodexDisabledEntry(s *DiscoveryService, scope domain.Scope, skillPath string) error {
	path := codexConfigPath(s, scope)
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	text := string(content)
	marker := codexPolicyMarker(skillPath)
	start := strings.Index(text, marker)
	if start < 0 {
		return nil
	}
	endRelative := strings.Index(text[start:], "# End Agent Studio managed skill policy")
	if endRelative < 0 {
		return nil
	}
	end := start + endRelative + len("# End Agent Studio managed skill policy")
	text = strings.TrimRight(text[:start], "\n") + text[end:]
	return writeWithBackup(path, []byte(text), 0o600)
}
