package application

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
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

// DiscoveryService inventories and manages local skills. All writes are explicit operations.
type DiscoveryService struct {
	home     string
	adapters []adapters.Adapter
}

func NewDiscoveryService(home string) *DiscoveryService {
	return &DiscoveryService{home: home, adapters: []adapters.Adapter{opencode.Adapter{}, claude.Adapter{}, codex.Adapter{}}}
}

func DefaultDiscoveryService() (*DiscoveryService, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}
	return NewDiscoveryService(home), nil
}

// Discover returns the complete, read-only workspace inventory.
func (s *DiscoveryService) Discover() (domain.DiscoveryResult, error) {
	result := domain.DiscoveryResult{
		Agents:      []domain.Agent{},
		Skills:      []domain.Skill{},
		ConfigFiles: []domain.ConfigFile{},
		Scopes:      []domain.Scope{},
		Projects:    []domain.Project{},
		ScannedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	result.Projects = s.loadProjects()
	result.Scopes = append(result.Scopes, domain.Scope{
		ID: "global", Name: "Global", Kind: "global", Root: filepath.Join(s.home, ".agents", "skills"),
	})

	for _, adapter := range s.adapters {
		agent, config := adapter.Detect(s.home)
		result.Agents = append(result.Agents, agent)
		if config != nil {
			result.ConfigFiles = append(result.ConfigFiles, *config)
		}
		roots := adapter.SkillRoots(s.home)
		if len(roots) == 0 {
			continue
		}
		result.Scopes = append(result.Scopes, domain.Scope{
			ID: string(agent.Provider), Name: agent.Name, Kind: "agent", Provider: agent.Provider, Root: roots[0],
		})
	}

	for _, project := range result.Projects {
		result.Scopes = append(result.Scopes, domain.Scope{
			ID: "project:" + project.ID, Name: project.Name + " (shared)", Kind: "project", Root: filepath.Join(project.Path, ".agents", "skills"),
		},
			domain.Scope{ID: "project:" + project.ID + ":claude", Name: project.Name + " (Claude)", Kind: "project", Provider: domain.ProviderClaude, Root: filepath.Join(project.Path, ".claude", "skills")},
			domain.Scope{ID: "project:" + project.ID + ":opencode", Name: project.Name + " (OpenCode)", Kind: "project", Provider: domain.ProviderOpenCode, Root: filepath.Join(project.Path, ".opencode", "skills")},
		)
	}

	for _, scope := range result.Scopes {
		skills, err := discoverSkills(scope)
		if err != nil {
			return domain.DiscoveryResult{}, err
		}
		result.Skills = append(result.Skills, skills...)
	}
	applySkillStates(result.Skills)
	if err := s.applySkillPolicies(result.Skills); err != nil {
		return domain.DiscoveryResult{}, err
	}
	sort.Slice(result.Skills, func(i, j int) bool { return result.Skills[i].Name < result.Skills[j].Name })
	return result, nil
}

// AddProject persists a selected project. Its generic .agents/skills directory is then tracked.
func (s *DiscoveryService) AddProject(path string) (domain.DiscoveryResult, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return domain.DiscoveryResult{}, fmt.Errorf("resolve project path: %w", err)
	}
	info, err := os.Stat(absPath)
	if err != nil || !info.IsDir() {
		return domain.DiscoveryResult{}, fmt.Errorf("project directory is unavailable: %s", absPath)
	}
	projects := s.loadProjects()
	for _, project := range projects {
		if project.Path == absPath {
			return s.Discover()
		}
	}
	hash := sha256.Sum256([]byte(absPath))
	projects = append(projects, domain.Project{ID: fmt.Sprintf("%x", hash[:8]), Name: filepath.Base(absPath), Path: absPath})
	if err := s.saveProjects(projects); err != nil {
		return domain.DiscoveryResult{}, err
	}
	return s.Discover()
}

// SetSkillInvocationMode persists a per-copy policy and synchronizes its agent.
func (s *DiscoveryService) SetSkillInvocationMode(skillPath, mode string) (domain.DiscoveryResult, error) {
	if !validInvocationMode(mode) {
		return domain.DiscoveryResult{}, fmt.Errorf("invalid skill invocation mode %q", mode)
	}
	workspace, err := s.Discover()
	if err != nil {
		return domain.DiscoveryResult{}, err
	}
	skill, err := s.skillFromWorkspace(workspace, skillPath)
	if err != nil {
		return domain.DiscoveryResult{}, err
	}
	scope, err := scopeByID(workspace.Scopes, skill.ScopeID)
	if err != nil {
		return domain.DiscoveryResult{}, err
	}
	policies := s.loadSkillPolicies()
	previous := policies[skill.Path]
	policy := previous
	policy.Mode = mode
	policies[skill.Path] = policy
	for _, provider := range providersForScope(scope) {
		if err := s.syncSkillPolicy(skill, scope, provider, mode, &policy); err != nil {
			return domain.DiscoveryResult{}, err
		}
	}
	policies[skill.Path] = policy
	if err := s.saveSkillPolicies(policies); err != nil {
		return domain.DiscoveryResult{}, err
	}
	return s.Discover()
}

func providersForScope(scope domain.Scope) []domain.Provider {
	if scope.Provider != "" {
		return []domain.Provider{scope.Provider}
	}
	if scope.Kind == "global" || scope.Kind == "project" {
		return []domain.Provider{domain.ProviderOpenCode, domain.ProviderCodex}
	}
	return nil
}

// RemoveProject stops tracking a project without modifying its directory or skills.
func (s *DiscoveryService) RemoveProject(projectID string) (domain.DiscoveryResult, error) {
	projects := s.loadProjects()
	filtered := make([]domain.Project, 0, len(projects))
	found := false
	for _, project := range projects {
		if project.ID == projectID {
			found = true
			continue
		}
		filtered = append(filtered, project)
	}
	if !found {
		return domain.DiscoveryResult{}, fmt.Errorf("project is not being tracked")
	}
	if err := s.saveProjects(filtered); err != nil {
		return domain.DiscoveryResult{}, err
	}
	return s.Discover()
}

// CopySkill copies the complete skill directory to a selected scope. The source remains unchanged.
func (s *DiscoveryService) CopySkill(skillPath, targetScopeID string) (domain.DiscoveryResult, error) {
	workspace, err := s.Discover()
	if err != nil {
		return domain.DiscoveryResult{}, err
	}
	source, err := s.skillFromWorkspace(workspace, skillPath)
	if err != nil {
		return domain.DiscoveryResult{}, err
	}
	target, err := scopeByID(workspace.Scopes, targetScopeID)
	if err != nil {
		return domain.DiscoveryResult{}, err
	}
	if source.ScopeID == target.ID {
		return domain.DiscoveryResult{}, fmt.Errorf("skill is already in this location")
	}
	sourceDir := filepath.Dir(source.Path)
	destinationDir := filepath.Join(target.Root, filepath.Base(sourceDir))
	if _, err := os.Stat(destinationDir); err == nil {
		return domain.DiscoveryResult{}, fmt.Errorf("destination already contains %q", source.Name)
	} else if !os.IsNotExist(err) {
		return domain.DiscoveryResult{}, fmt.Errorf("inspect destination: %w", err)
	}
	if err := copyDirectory(sourceDir, destinationDir); err != nil {
		return domain.DiscoveryResult{}, err
	}
	if err := removeManagedCodexMetadata(destinationDir); err != nil {
		return domain.DiscoveryResult{}, err
	}
	return s.Discover()
}

// DeleteSkill permanently removes the selected skill directory. The caller must confirm this action.
func (s *DiscoveryService) DeleteSkill(skillPath string) (domain.DiscoveryResult, error) {
	workspace, err := s.Discover()
	if err != nil {
		return domain.DiscoveryResult{}, err
	}
	skill, err := s.skillFromWorkspace(workspace, skillPath)
	if err != nil {
		return domain.DiscoveryResult{}, err
	}
	scope, err := scopeByID(workspace.Scopes, skill.ScopeID)
	if err != nil {
		return domain.DiscoveryResult{}, err
	}
	if err := s.removeSkillPolicy(skill, scope); err != nil {
		return domain.DiscoveryResult{}, err
	}
	sourceDir := filepath.Dir(skill.Path)
	if err := os.RemoveAll(sourceDir); err != nil {
		return domain.DiscoveryResult{}, fmt.Errorf("delete skill: %w", err)
	}
	return s.Discover()
}

func (s *DiscoveryService) removeSkillPolicy(skill domain.Skill, scope domain.Scope) error {
	policies := s.loadSkillPolicies()
	policy, exists := policies[skill.Path]
	if !exists {
		return nil
	}
	for _, provider := range providersForScope(scope) {
		if err := s.syncSkillPolicy(skill, scope, provider, modeAutomatic, &policy); err != nil {
			return err
		}
	}
	delete(policies, skill.Path)
	return s.saveSkillPolicies(policies)
}

func (s *DiscoveryService) skillFromWorkspace(workspace domain.DiscoveryResult, skillPath string) (domain.Skill, error) {
	absPath, err := filepath.Abs(skillPath)
	if err != nil {
		return domain.Skill{}, err
	}
	for _, skill := range workspace.Skills {
		if skill.Path == absPath {
			return skill, nil
		}
	}
	return domain.Skill{}, fmt.Errorf("skill is no longer available")
}

func discoverSkills(scope domain.Scope) ([]domain.Skill, error) {
	info, err := os.Stat(scope.Root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect skill directory %q: %w", scope.Root, err)
	}
	if !info.IsDir() {
		return nil, nil
	}
	var skills []domain.Skill
	err = filepath.WalkDir(scope.Root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "SKILL.md" {
			return nil
		}
		skill, err := parseSkill(path)
		if err != nil {
			return err
		}
		skill.ScopeID = scope.ID
		skill.States = []string{"available"}
		if scope.Kind != "global" {
			skill.States = append(skill.States, "associated")
		}
		skills = append(skills, skill)
		return nil
	})
	return skills, err
}

func parseSkill(path string) (domain.Skill, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return domain.Skill{}, fmt.Errorf("read skill %q: %w", path, err)
	}
	name := filepath.Base(filepath.Dir(path))
	description := "No description provided."
	var firstContent string
	metadataNameFound := false
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "name:") {
			name, metadataNameFound = cleanMetadataValue(strings.TrimPrefix(line, "name:")), true
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
	if description == "No description provided." && firstContent != "" {
		description = firstContent
	}
	pathHash := sha256.Sum256([]byte(path))
	contentHash := sha256.Sum256(content)
	return domain.Skill{ID: fmt.Sprintf("%x", pathHash[:8]), Name: name, Description: description, Path: path, ContentHash: fmt.Sprintf("%x", contentHash[:]), InvocationMode: "automatic"}, nil
}

func applySkillStates(skills []domain.Skill) {
	byName := make(map[string][]int)
	for index, skill := range skills {
		byName[strings.ToLower(skill.Name)] = append(byName[strings.ToLower(skill.Name)], index)
	}
	for _, indices := range byName {
		if len(indices) < 2 {
			continue
		}
		hashes := make(map[string]bool)
		for _, index := range indices {
			hashes[skills[index].ContentHash] = true
		}
		state := "duplicated"
		if len(hashes) > 1 {
			state = "conflict"
		}
		for _, index := range indices {
			skills[index].States = append(skills[index].States, state)
		}
	}
}

func scopeByID(scopes []domain.Scope, id string) (domain.Scope, error) {
	for _, scope := range scopes {
		if scope.ID == id {
			return scope, nil
		}
	}
	return domain.Scope{}, fmt.Errorf("destination scope is unavailable")
}

func (s *DiscoveryService) projectsPath() string {
	return filepath.Join(s.home, ".agent-studio", "projects.json")
}

func (s *DiscoveryService) loadProjects() []domain.Project {
	content, err := os.ReadFile(s.projectsPath())
	if err != nil {
		return nil
	}
	var projects []domain.Project
	if json.Unmarshal(content, &projects) != nil {
		return nil
	}
	return projects
}

func (s *DiscoveryService) saveProjects(projects []domain.Project) error {
	content, err := json.MarshalIndent(projects, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.projectsPath()), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.projectsPath(), content, 0o600)
}

func copyDirectory(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relativePath, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relativePath)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(output, input)
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

func cleanMetadataValue(value string) string { return strings.Trim(strings.TrimSpace(value), "\"'") }
