package application

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"agent-studio/internal/adapters"
	"agent-studio/internal/adapters/claude"
	"agent-studio/internal/adapters/codex"
	"agent-studio/internal/adapters/opencode"
	"agent-studio/internal/domain"
)

// DiscoveryService inventories supported local agents and skills without writing files.
type DiscoveryService struct {
	home     string
	adapters []adapters.Adapter
}

func NewDiscoveryService(home string) *DiscoveryService {
	return &DiscoveryService{
		home: home,
		adapters: []adapters.Adapter{
			opencode.Adapter{},
			claude.Adapter{},
			codex.Adapter{},
		},
	}
}

func DefaultDiscoveryService() (*DiscoveryService, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}
	return NewDiscoveryService(home), nil
}

func (s *DiscoveryService) Discover() (domain.DiscoveryResult, error) {
	result := domain.DiscoveryResult{ScannedAt: time.Now().UTC().Format(time.RFC3339)}
	skillsByPath := make(map[string]*domain.Skill)

	for _, adapter := range s.adapters {
		agent, config := adapter.Detect(s.home)
		result.Agents = append(result.Agents, agent)
		if config != nil {
			result.ConfigFiles = append(result.ConfigFiles, *config)
		}

		for _, root := range adapter.SkillRoots(s.home) {
			if err := discoverSkills(root, agent.Provider, skillsByPath); err != nil {
				return domain.DiscoveryResult{}, err
			}
		}
	}

	for _, skill := range skillsByPath {
		sort.Slice(skill.Sources, func(i, j int) bool { return skill.Sources[i] < skill.Sources[j] })
		result.Skills = append(result.Skills, *skill)
	}
	sort.Slice(result.Skills, func(i, j int) bool { return result.Skills[i].Name < result.Skills[j].Name })
	return result, nil
}

func discoverSkills(root string, provider domain.Provider, skillsByPath map[string]*domain.Skill) error {
	info, err := os.Stat(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect skill directory %q: %w", root, err)
	}
	if !info.IsDir() {
		return nil
	}

	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "SKILL.md" {
			return nil
		}

		absolutePath, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		skill, exists := skillsByPath[absolutePath]
		if !exists {
			parsedSkill, err := parseSkill(absolutePath)
			if err != nil {
				return err
			}
			skill = &parsedSkill
			skillsByPath[absolutePath] = skill
		}
		if !containsProvider(skill.Sources, provider) {
			skill.Sources = append(skill.Sources, provider)
		}
		return nil
	})
}

func parseSkill(path string) (domain.Skill, error) {
	file, err := os.Open(path)
	if err != nil {
		return domain.Skill{}, fmt.Errorf("open skill %q: %w", path, err)
	}
	defer file.Close()

	name := filepath.Base(filepath.Dir(path))
	description := "No description provided."
	var firstContent string
	metadataNameFound := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "name:") {
			name = cleanMetadataValue(strings.TrimPrefix(line, "name:"))
			metadataNameFound = true
			continue
		}
		if strings.HasPrefix(line, "description:") {
			description = cleanMetadataValue(strings.TrimPrefix(line, "description:"))
			continue
		}
		if strings.HasPrefix(line, "# ") && !metadataNameFound {
			name = strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
		if firstContent == "" && line != "" && !strings.HasPrefix(line, "---") && !strings.Contains(line, ":") && !strings.HasPrefix(line, "#") {
			firstContent = line
		}
	}
	if err := scanner.Err(); err != nil {
		return domain.Skill{}, fmt.Errorf("read skill %q: %w", path, err)
	}
	if description == "No description provided." && firstContent != "" {
		description = firstContent
	}

	hash := sha256.Sum256([]byte(path))
	return domain.Skill{ID: fmt.Sprintf("%x", hash[:8]), Name: name, Description: description, Path: path}, nil
}

func cleanMetadataValue(value string) string {
	return strings.Trim(strings.TrimSpace(value), "\"'")
}

func containsProvider(providers []domain.Provider, target domain.Provider) bool {
	for _, provider := range providers {
		if provider == target {
			return true
		}
	}
	return false
}
