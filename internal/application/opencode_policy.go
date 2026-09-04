package application

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-studio/internal/domain"
)

const (
	modeAlways    = "always"
	modeAutomatic = "automatic"
	modeExplicit  = "explicit"
	modeDisabled  = "disabled"
)

type skillPolicy struct {
	Mode               string `json:"mode"`
	LastPermission     string `json:"lastPermission,omitempty"`
	PreviousPermission string `json:"previousPermission,omitempty"`
	HadPrevious        bool   `json:"hadPreviousPermission,omitempty"`
	ClaudeLastOverride string `json:"claudeLastOverride,omitempty"`
	ClaudePrevious     string `json:"claudePreviousOverride,omitempty"`
	ClaudeHadPrevious  bool   `json:"claudeHadPreviousOverride,omitempty"`
}

func validInvocationMode(mode string) bool {
	return mode == modeAlways || mode == modeAutomatic || mode == modeExplicit || mode == modeDisabled
}

func (s *DiscoveryService) policiesPath() string {
	return filepath.Join(s.home, ".agent-studio", "skill-policies.json")
}

func (s *DiscoveryService) loadSkillPolicies() map[string]skillPolicy {
	content, err := os.ReadFile(s.policiesPath())
	if err != nil {
		return map[string]skillPolicy{}
	}
	var policies map[string]skillPolicy
	if json.Unmarshal(content, &policies) != nil || policies == nil {
		return map[string]skillPolicy{}
	}
	return policies
}

func (s *DiscoveryService) saveSkillPolicies(policies map[string]skillPolicy) error {
	content, err := json.MarshalIndent(policies, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.policiesPath()), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.policiesPath(), content, 0o600)
}

func (s *DiscoveryService) applySkillPolicies(skills []domain.Skill) error {
	policies := s.loadSkillPolicies()
	for index := range skills {
		if policy, ok := policies[skills[index].Path]; ok && validInvocationMode(policy.Mode) {
			skills[index].InvocationMode = policy.Mode
		}
	}
	return nil
}

func (s *DiscoveryService) syncOpenCodePolicy(skill domain.Skill, mode string, policy *skillPolicy) error {
	configPath := s.openCodeConfigPath(skill)
	config, err := readJSONConfig(configPath)
	if err != nil {
		return err
	}

	if mode == modeAlways {
		generatedPath := filepath.Join(s.home, ".agent-studio", "generated", "opencode", "instructions", skill.ID+".md")
		content, readErr := os.ReadFile(skill.Path)
		if readErr != nil {
			return fmt.Errorf("read skill instructions: %w", readErr)
		}
		generated := []byte("# Agent Studio managed instruction: " + skill.Name + "\n\n" + string(content))
		if err := writeAtomic(generatedPath, generated, 0o644); err != nil {
			return err
		}
		instructions := stringSlice(config["instructions"])
		instructions = removeString(instructions, generatedPath)
		instructions = append(instructions, generatedPath)
		config["instructions"] = instructions
	} else {
		generatedPath := filepath.Join(s.home, ".agent-studio", "generated", "opencode", "instructions", skill.ID+".md")
		instructions := removeString(stringSlice(config["instructions"]), generatedPath)
		if len(instructions) == 0 {
			delete(config, "instructions")
		} else {
			config["instructions"] = instructions
		}
	}
	if err := syncOpenCodeCommand(s, skill, mode); err != nil {
		return err
	}

	permissionValue := config["permission"]
	if permissionValue != nil {
		if _, ok := permissionValue.(map[string]interface{}); !ok {
			return fmt.Errorf("OpenCode permission config must be an object to configure individual skills")
		}
	}
	permission := object(permissionValue)
	skillPermissions := object(permission["skill"])
	if mode == modeExplicit || mode == modeDisabled {
		value := "ask"
		if mode == modeDisabled {
			value = "deny"
		}
		if policy.LastPermission == "" {
			if previous, exists := skillPermissions[skill.Name]; exists {
				if previousText, ok := previous.(string); ok {
					policy.PreviousPermission = previousText
					policy.HadPrevious = true
				}
			}
		}
		skillPermissions[skill.Name] = value
		policy.LastPermission = value
	} else if policy.LastPermission != "" && skillPermissions[skill.Name] == policy.LastPermission {
		if policy.HadPrevious {
			skillPermissions[skill.Name] = policy.PreviousPermission
		} else {
			delete(skillPermissions, skill.Name)
		}
		policy.LastPermission = ""
		policy.PreviousPermission = ""
		policy.HadPrevious = false
	}
	if len(skillPermissions) > 0 {
		permission["skill"] = skillPermissions
	} else {
		delete(permission, "skill")
	}
	if len(permission) > 0 {
		config["permission"] = permission
	} else {
		delete(config, "permission")
	}

	return writeJSONConfig(configPath, config)
}

func (s *DiscoveryService) openCodeConfigPath(skill domain.Skill) string {
	if strings.HasPrefix(skill.ScopeID, "project:") {
		return filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(skill.Path)))), "opencode.json")
	}
	return filepath.Join(s.home, ".config", "opencode", "opencode.json")
}

func syncOpenCodeCommand(s *DiscoveryService, skill domain.Skill, mode string) error {
	commandRoot := filepath.Join(s.home, ".config", "opencode", "commands")
	if strings.HasPrefix(skill.ScopeID, "project:") {
		commandRoot = filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(skill.Path)))), ".opencode", "commands")
	}
	commandPath := filepath.Join(commandRoot, skill.Name+".md")
	if mode != modeExplicit {
		if content, err := os.ReadFile(commandPath); err == nil && strings.Contains(string(content), "<!-- Agent Studio managed skill command -->") {
			if err := os.Remove(commandPath); err != nil {
				return fmt.Errorf("remove generated OpenCode command: %w", err)
			}
		}
		return nil
	}
	if existing, err := os.ReadFile(commandPath); err == nil && !strings.Contains(string(existing), "<!-- Agent Studio managed skill command -->") {
		return fmt.Errorf("OpenCode command %q already exists and will not be overwritten", skill.Name)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect OpenCode command: %w", err)
	}
	content := fmt.Sprintf("<!-- Agent Studio managed skill command -->\n---\ndescription: Explicitly use the %s skill\n---\nUse the OpenCode skill named `%s` for the request below.\n\n$ARGUMENTS\n", skill.Name, skill.Name)
	return writeAtomic(commandPath, []byte(content), 0o644)
}

func readJSONConfig(path string) (map[string]interface{}, error) {
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]interface{}{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read OpenCode config: %w", err)
	}
	var config map[string]interface{}
	if err := json.Unmarshal(content, &config); err != nil {
		return nil, fmt.Errorf("OpenCode config is not valid JSON (JSONC is not supported yet): %w", err)
	}
	if config == nil {
		config = map[string]interface{}{}
	}
	return config, nil
}

func writeJSONConfig(path string, config map[string]interface{}) error {
	content, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encode OpenCode config: %w", err)
	}
	return writeWithBackup(path, append(content, '\n'), 0o600)
}

func writeWithBackup(path string, content []byte, mode os.FileMode) error {
	if existing, readErr := os.ReadFile(path); readErr == nil {
		backup := filepath.Join(filepath.Dir(path), ".agent-studio-backups", filepath.Base(path)+"."+time.Now().UTC().Format("20060102T150405.000000000Z")+".bak")
		if err := writeAtomic(backup, existing, 0o600); err != nil {
			return fmt.Errorf("backup %s: %w", path, err)
		}
	} else if !os.IsNotExist(readErr) {
		return fmt.Errorf("inspect %s: %w", path, readErr)
	}
	return writeAtomic(path, content, mode)
}

func writeAtomic(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".agent-studio-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, path)
}

func object(value interface{}) map[string]interface{} {
	if value == nil {
		return map[string]interface{}{}
	}
	if result, ok := value.(map[string]interface{}); ok {
		return result
	}
	return map[string]interface{}{}
}

func stringSlice(value interface{}) []string {
	items, ok := value.([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func removeString(values []string, target string) []string {
	result := values[:0]
	for _, value := range values {
		if value != target {
			result = append(result, value)
		}
	}
	return result
}
