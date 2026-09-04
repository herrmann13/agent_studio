package application

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-studio/internal/domain"
)

const (
	claudeManagedStart = "<!-- BEGIN AGENT STUDIO SKILL: "
	claudeManagedEnd   = "<!-- END AGENT STUDIO SKILL -->"
)

func (s *DiscoveryService) syncClaudePolicy(skill domain.Skill, scope domain.Scope, mode string, policy *skillPolicy) error {
	instructionsPath := filepath.Join(s.home, ".agent-studio", "generated", "claude", "instructions", skill.ID+".md")
	claudeMDPath := filepath.Join(s.home, ".claude", "CLAUDE.md")
	settingsPath := filepath.Join(s.home, ".claude", "settings.json")
	if scope.Kind == "project" {
		projectRoot := filepath.Dir(filepath.Dir(scope.Root))
		claudeMDPath = filepath.Join(projectRoot, "CLAUDE.md")
		settingsPath = filepath.Join(projectRoot, ".claude", "settings.json")
	}

	if mode == modeAlways {
		content, err := os.ReadFile(skill.Path)
		if err != nil {
			return fmt.Errorf("read Claude skill instructions: %w", err)
		}
		if err := writeAtomic(instructionsPath, append([]byte("# Agent Studio managed instruction: "+skill.Name+"\n\n"), content...), 0o644); err != nil {
			return err
		}
		marker := claudeManagedStart + skill.ID + " -->"
		block := marker + "\n@" + instructionsPath + "\n" + claudeManagedEnd
		if err := updateManagedMarkdown(claudeMDPath, marker, claudeManagedEnd, block, true); err != nil {
			return err
		}
	} else if err := updateManagedMarkdown(claudeMDPath, claudeManagedStart+skill.ID+" -->", claudeManagedEnd, "", false); err != nil {
		return err
	}

	config, err := readJSONConfig(settingsPath)
	if err != nil {
		return fmt.Errorf("read Claude settings: %w", err)
	}
	overridesValue := config["skillOverrides"]
	if overridesValue != nil {
		if _, ok := overridesValue.(map[string]interface{}); !ok {
			return fmt.Errorf("Claude skillOverrides config must be an object")
		}
	}
	overrides := object(overridesValue)
	if mode == modeExplicit || mode == modeDisabled {
		value := "user-invocable-only"
		if mode == modeDisabled {
			value = "off"
		}
		if policy.ClaudeLastOverride == "" {
			if previous, exists := overrides[skill.Name]; exists {
				if previousText, ok := previous.(string); ok {
					policy.ClaudePrevious = previousText
					policy.ClaudeHadPrevious = true
				}
			}
		}
		overrides[skill.Name] = value
		policy.ClaudeLastOverride = value
	} else if policy.ClaudeLastOverride != "" && overrides[skill.Name] == policy.ClaudeLastOverride {
		if policy.ClaudeHadPrevious {
			overrides[skill.Name] = policy.ClaudePrevious
		} else {
			delete(overrides, skill.Name)
		}
		policy.ClaudeLastOverride = ""
		policy.ClaudePrevious = ""
		policy.ClaudeHadPrevious = false
	}
	if len(overrides) > 0 {
		config["skillOverrides"] = overrides
	} else {
		delete(config, "skillOverrides")
	}
	return writeJSONConfig(settingsPath, config)
}

func updateManagedMarkdown(path, marker, endMarker, block string, enabled bool) error {
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if !enabled {
			return nil
		}
		return writeAtomic(path, []byte(block+"\n"), 0o644)
	}
	if err != nil {
		return fmt.Errorf("read managed instructions: %w", err)
	}
	text := string(content)
	start := strings.Index(text, marker)
	if start >= 0 {
		endRelative := strings.Index(text[start:], endMarker)
		if endRelative >= 0 {
			end := start + endRelative + len(endMarker)
			text = strings.TrimRight(text[:start], "\n") + text[end:]
		}
	}
	if enabled {
		if strings.TrimSpace(text) != "" {
			text = strings.TrimRight(text, "\n") + "\n\n"
		}
		text += block + "\n"
	}
	if text == string(content) {
		return nil
	}
	return writeWithBackup(path, []byte(text), 0o644)
}
