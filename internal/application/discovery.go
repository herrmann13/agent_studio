package application

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"agent-studio/internal/adapters"
	"agent-studio/internal/adapters/claude"
	"agent-studio/internal/adapters/codex"
	"agent-studio/internal/adapters/opencode"
	"agent-studio/internal/domain"
)

// DiscoveryService inventories and manages local skills. All writes are explicit operations.
//
// Wails dispatches each JS-to-Go call on its own goroutine, so two near-simultaneous
// UI actions (a fast double-drop, or the update poller firing during a copy) would
// otherwise race on the same projects.json/skill-policies.json files and native
// configs. mu serializes every public operation so state changes stay consistent.
type DiscoveryService struct {
	home     string
	env      adapters.Environment
	adapters []adapters.Adapter
	mu       sync.Mutex
}

func NewDiscoveryService(home string) *DiscoveryService {
	return NewDiscoveryServiceWithEnvironment(adapters.SystemEnvironment(home))
}

// NewDiscoveryServiceWithEnvironment builds a service over a custom view of the
// machine, so tests control which agents appear installed.
func NewDiscoveryServiceWithEnvironment(env adapters.Environment) *DiscoveryService {
	return &DiscoveryService{home: env.Home, env: env, adapters: []adapters.Adapter{opencode.Adapter{}, claude.Adapter{}, codex.Adapter{}}}
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
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.discover()
}

// discover is the unlocked implementation. Every public, state-changing method below
// already holds s.mu for its whole operation, so they call discover() directly
// instead of Discover() to avoid deadlocking on a non-reentrant mutex.
func (s *DiscoveryService) discover() (domain.DiscoveryResult, error) {
	result := domain.DiscoveryResult{
		Agents:      []domain.Agent{},
		Skills:      []domain.Skill{},
		ConfigFiles: []domain.ConfigFile{},
		Scopes:      []domain.Scope{},
		Projects:    []domain.Project{},
		ScannedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	result.Projects = s.loadProjects()
	// Every supported agent reads `~/.agents/skills` natively except Claude Code,
	// which receives a marked copy (see skill_propagation.go).
	result.Scopes = append(result.Scopes, domain.Scope{
		ID: "global", Name: "Global (all agents)", Kind: "global", Root: s.globalSkillRoot(),
	})

	// Only installed agents are listed; an agent whose executable is missing
	// contributes no scope even if its config directory is still on disk.
	for _, adapter := range s.adapters {
		for _, installation := range adapter.Detect(s.env) {
			result.Agents = append(result.Agents, installation.Agent)
			if installation.Config != nil {
				result.ConfigFiles = append(result.ConfigFiles, *installation.Config)
			}
			result.Scopes = append(result.Scopes, installation.Scope)
		}
	}

	for _, project := range result.Projects {
		result.Scopes = append(result.Scopes, domain.Scope{
			ID: "project:" + project.ID, Name: project.Name + " (shared)", Kind: "project", Root: filepath.Join(project.Path, ".agents", "skills"),
		})
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
	s.mu.Lock()
	defer s.mu.Unlock()
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
			return s.discover()
		}
	}
	hash := sha256.Sum256([]byte(absPath))
	projects = append(projects, domain.Project{ID: fmt.Sprintf("%x", hash[:8]), Name: filepath.Base(absPath), Path: absPath})
	if err := s.saveProjects(projects); err != nil {
		return domain.DiscoveryResult{}, err
	}
	return s.discover()
}

// SetSkillInvocationMode persists a per-copy policy and synchronizes its agent.
func (s *DiscoveryService) SetSkillInvocationMode(skillPath, mode string) (domain.DiscoveryResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validInvocationMode(mode) {
		return domain.DiscoveryResult{}, fmt.Errorf("invalid skill invocation mode %q", mode)
	}
	workspace, err := s.discover()
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
	for _, provider := range s.providersForScope(scope) {
		if err := s.syncSkillPolicy(skill, scope, provider, mode, &policy); err != nil {
			return domain.DiscoveryResult{}, err
		}
	}
	policies[skill.Path] = policy
	if err := s.saveSkillPolicies(policies); err != nil {
		return domain.DiscoveryResult{}, err
	}
	return s.discover()
}

func (s *DiscoveryService) providersForScope(scope domain.Scope) []domain.Provider {
	if scope.Provider != "" {
		return []domain.Provider{scope.Provider}
	}
	if scope.Kind == "global" {
		// A global skill only configures the agents that are actually installed,
		// writing their user-level policy files (never a project's).
		var providers []domain.Provider
		for _, adapter := range s.adapters {
			if len(adapter.Detect(s.env)) > 0 {
				providers = append(providers, adapter.Provider())
			}
		}
		return providers
	}
	if scope.Kind == "project" {
		return []domain.Provider{domain.ProviderOpenCode, domain.ProviderClaude, domain.ProviderCodex}
	}
	return nil
}

// RemoveProject stops tracking a project without modifying its directory or skills.
func (s *DiscoveryService) RemoveProject(projectID string) (domain.DiscoveryResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	return s.discover()
}

// CopySkill copies the complete skill directory to a selected scope. The source remains unchanged.
func (s *DiscoveryService) CopySkill(skillPath, targetScopeID string) (domain.DiscoveryResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace, err := s.discover()
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
	destinationDir := filepath.Join(target.Root, skillDirName(source.Path))
	if _, err := os.Stat(destinationDir); err == nil {
		return domain.DiscoveryResult{}, fmt.Errorf("destination already contains %q", source.Name)
	} else if !os.IsNotExist(err) {
		return domain.DiscoveryResult{}, fmt.Errorf("inspect destination: %w", err)
	}
	if err := s.checkGlobalCopyTargets(target, filepath.Base(destinationDir)); err != nil {
		return domain.DiscoveryResult{}, err
	}
	if isSingleFileSkill(source.Path) {
		// Other agents only understand `<name>/SKILL.md`, so a loose OpenCode v2
		// skill file becomes a regular skill folder at its destination.
		if err := copySingleFileSkill(source.Path, destinationDir); err != nil {
			return domain.DiscoveryResult{}, err
		}
	} else if err := copyDirectory(filepath.Dir(source.Path), destinationDir); err != nil {
		return domain.DiscoveryResult{}, err
	}
	if err := removeManagedCodexMetadata(destinationDir); err != nil {
		return domain.DiscoveryResult{}, err
	}
	if err := s.propagateSkillCopy(target, destinationDir); err != nil {
		return domain.DiscoveryResult{}, err
	}
	return s.discover()
}

// SyncGlobalSkills gives every installed agent that cannot read `~/.agents/skills`
// its marked copy of each Global skill, refreshing stale copies. It covers skills
// placed there outside Agent Studio and agents installed after a skill was added.
// A name already taken by an agent's own skill is skipped and reported.
func (s *DiscoveryService) SyncGlobalSkills() (domain.DiscoveryResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace, err := s.discover()
	if err != nil {
		return domain.DiscoveryResult{}, err
	}
	var skipped []string
	for _, skill := range workspace.Skills {
		if skill.ScopeID != "global" {
			continue
		}
		if err := s.propagateToGlobalAgents(filepath.Dir(skill.Path)); err != nil {
			skipped = append(skipped, err.Error())
		}
	}
	updated, err := s.discover()
	if err != nil {
		return domain.DiscoveryResult{}, err
	}
	if len(skipped) > 0 {
		return updated, fmt.Errorf("some global skills were not synced: %s", strings.Join(skipped, "; "))
	}
	return updated, nil
}

// DeleteSkill permanently removes the selected skill directory. The caller must confirm this action.
func (s *DiscoveryService) DeleteSkill(skillPath string) (domain.DiscoveryResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workspace, err := s.discover()
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
	// A loose OpenCode v2 skill file sits directly in the scope root, so only the
	// file itself may be removed -- never its parent directory.
	if isSingleFileSkill(skill.Path) {
		if err := os.Remove(skill.Path); err != nil {
			return domain.DiscoveryResult{}, fmt.Errorf("delete skill: %w", err)
		}
	} else if err := os.RemoveAll(filepath.Dir(skill.Path)); err != nil {
		return domain.DiscoveryResult{}, fmt.Errorf("delete skill: %w", err)
	}
	if err := s.removePropagatedCopiesForScope(scope, skillDirName(skill.Path)); err != nil {
		return domain.DiscoveryResult{}, err
	}
	return s.discover()
}

// propagateSkillCopy fans a project or global scope's canonical skill copy out to
// every agent that cannot read `.agents/skills` natively, so it is discoverable
// without depending on Agent Studio's own scope directory.
func (s *DiscoveryService) propagateSkillCopy(scope domain.Scope, skillDir string) error {
	if scope.Kind == "global" {
		return s.propagateToGlobalAgents(skillDir)
	}
	if scope.Kind != "project" {
		return nil
	}
	return s.propagateToProjectAgents(skillDir, projectRootFromScopeRoot(scope.Root))
}

func (s *DiscoveryService) removePropagatedCopiesForScope(scope domain.Scope, skillDirName string) error {
	if scope.Kind == "global" {
		return s.removeGlobalCopies(skillDirName)
	}
	if scope.Kind != "project" {
		return nil
	}
	return s.removePropagatedCopies(projectRootFromScopeRoot(scope.Root), skillDirName)
}

func (s *DiscoveryService) removeSkillPolicy(skill domain.Skill, scope domain.Scope) error {
	policies := s.loadSkillPolicies()
	policy, exists := policies[skill.Path]
	if !exists {
		return nil
	}
	for _, provider := range s.providersForScope(scope) {
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
	var paths []string
	var err error
	if scope.SkillLayout == domain.SkillLayoutOpenCodeV2 {
		paths, err = scanOpenCodeV2Skills(scope.Root)
	} else {
		paths, err = scanSkillFolders(scope.Root)
	}
	if err != nil {
		return nil, err
	}
	skills := make([]domain.Skill, 0, len(paths))
	for _, path := range paths {
		if !isSingleFileSkill(path) && isGlobalCopy(filepath.Dir(path)) {
			continue
		}
		skill, err := parseSkill(path)
		if err != nil {
			return nil, err
		}
		skill.ScopeID = scope.ID
		skill.States = []string{"available", "associated"}
		skills = append(skills, skill)
	}
	return skills, nil
}

// scanSkillFolders finds every `SKILL.md` below root.
func scanSkillFolders(root string) ([]string, error) {
	info, err := os.Stat(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect skill directory %q: %w", root, err)
	}
	if !info.IsDir() {
		return nil, nil
	}
	var paths []string
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && entry.Name() == "SKILL.md" {
			paths = append(paths, path)
		}
		return nil
	})
	return paths, err
}

// openCodeV2MaxDepth mirrors OpenCode v2's skill scanner, which stops descending
// six directories below a skill root.
const openCodeV2MaxDepth = 6

// scanOpenCodeV2Skills reproduces how OpenCode v2 identifies skills (confirmed
// against the 2.1.0 CLI's `GET /api/skill`): it reads both `skills` and its
// sibling `skill` directory; a folder containing `SKILL.md` is one skill and is not
// descended into; a loose `<name>.md` directly in a root is also a skill; hidden
// entries are ignored; and symlinks are followed.
func scanOpenCodeV2Skills(root string) ([]string, error) {
	var paths []string
	for _, base := range []string{root, filepath.Join(filepath.Dir(root), "skill")} {
		type pending struct {
			directory string
			depth     int
		}
		queue := []pending{{directory: base}}
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			entries, err := os.ReadDir(current.directory)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("inspect skill directory %q: %w", current.directory, err)
			}
			var files, directories []string
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".") || entry.Name() == "node_modules" {
					continue
				}
				path := filepath.Join(current.directory, entry.Name())
				info, err := os.Stat(path)
				if err != nil {
					continue
				}
				if info.IsDir() {
					directories = append(directories, path)
				} else if info.Mode().IsRegular() {
					files = append(files, path)
				}
			}
			if skillFile := filepath.Join(current.directory, "SKILL.md"); slices.Contains(files, skillFile) {
				paths = append(paths, skillFile)
				continue
			}
			if current.depth == 0 {
				for _, file := range files {
					if strings.HasSuffix(file, ".md") {
						paths = append(paths, file)
					}
				}
			}
			if current.depth < openCodeV2MaxDepth {
				for _, directory := range directories {
					queue = append(queue, pending{directory: directory, depth: current.depth + 1})
				}
			}
		}
	}
	sort.Strings(paths)
	return paths, nil
}

// isSingleFileSkill reports an OpenCode v2 skill stored as a loose `<name>.md`.
func isSingleFileSkill(path string) bool {
	return filepath.Base(path) != "SKILL.md"
}

// skillDirName is the folder name a skill occupies, or would occupy once copied.
func skillDirName(path string) string {
	if isSingleFileSkill(path) {
		return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	return filepath.Base(filepath.Dir(path))
}

func copySingleFileSkill(source, destinationDir string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	content, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destinationDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(destinationDir, "SKILL.md"), content, info.Mode().Perm())
}

func parseSkill(path string) (domain.Skill, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return domain.Skill{}, fmt.Errorf("read skill %q: %w", path, err)
	}
	name := skillDirName(path)
	description := "No description provided."
	var firstContent string
	metadataNameFound := false
	inFrontmatter := false
	frontmatterClosed := false
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for lineNumber := 0; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "---" && !frontmatterClosed {
			if lineNumber == 0 {
				inFrontmatter = true
			} else if inFrontmatter {
				inFrontmatter = false
				frontmatterClosed = true
			}
			continue
		}
		if inFrontmatter {
			if strings.HasPrefix(line, "name:") {
				name, metadataNameFound = cleanMetadataValue(strings.TrimPrefix(line, "name:")), true
			} else if strings.HasPrefix(line, "description:") {
				description = cleanMetadataValue(strings.TrimPrefix(line, "description:"))
			}
			continue
		}
		if strings.HasPrefix(line, "# ") && !metadataNameFound {
			name = strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
		if firstContent == "" && line != "" && !strings.Contains(line, ":") && !strings.HasPrefix(line, "#") {
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
		// Agents that share a skill directory (OpenCode v1 and v2) list the same
		// file in each of their scopes; that is one copy, not a duplicate.
		paths := make(map[string]bool)
		for _, index := range indices {
			paths[skills[index].Path] = true
		}
		if len(paths) < 2 {
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
	uniqueProjects := make([]domain.Project, 0, len(projects))
	seenPaths := make(map[string]struct{}, len(projects))
	for _, project := range projects {
		if project.ID == "" || project.Name == "" || project.Path == "" {
			continue
		}
		if _, seen := seenPaths[project.Path]; seen {
			continue
		}
		seenPaths[project.Path] = struct{}{}
		uniqueProjects = append(uniqueProjects, project)
	}
	return uniqueProjects
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
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("skill contains unsupported symlink: %q", path)
		}
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
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
