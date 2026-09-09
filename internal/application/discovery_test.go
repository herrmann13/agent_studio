package application

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestDiscoverClassifiesSkillStatesByScope(t *testing.T) {
	home := t.TempDir()
	writeFixture(t, filepath.Join(home, ".config", "opencode", "opencode.json"), "{}")
	writeFixture(t, filepath.Join(home, ".claude", "settings.json"), "{}")
	writeFixture(t, filepath.Join(home, ".codex", "config.toml"), "model = \"test\"")
	writeFixture(t, filepath.Join(home, ".agents", "skills", "testing", "SKILL.md"), "---\nname: Testing\ndescription: Write focused tests.\n---\n")
	writeFixture(t, filepath.Join(home, ".codex", "skills", "testing", "SKILL.md"), "---\nname: Testing\ndescription: Write focused tests.\n---\n")

	result, err := NewDiscoveryService(home).Discover()
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(result.Agents) != 3 || len(result.ConfigFiles) != 3 {
		t.Fatalf("unexpected discovery result = %#v", result)
	}
	if len(result.Skills) != 2 {
		t.Fatalf("skills = %d, want 2", len(result.Skills))
	}
	for _, skill := range result.Skills {
		if !containsString(skill.States, "duplicated") {
			t.Errorf("skill states = %#v, want duplicated", skill.States)
		}
	}
}

func TestCopyAndDeleteSkill(t *testing.T) {
	home := t.TempDir()
	service := NewDiscoveryService(home)
	source := filepath.Join(home, ".agents", "skills", "testing", "SKILL.md")
	writeFixture(t, source, "# Testing\n")
	writeFixture(t, filepath.Join(filepath.Dir(source), "agents", "openai.yaml"), "# Agent Studio managed invocation policy\npolicy:\n  allow_implicit_invocation: false\n")

	result, err := service.CopySkill(source, "codex")
	if err != nil {
		t.Fatalf("CopySkill() error = %v", err)
	}
	destination := filepath.Join(home, ".codex", "skills", "testing", "SKILL.md")
	if _, err := os.Stat(destination); err != nil {
		t.Fatalf("copied skill is missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(destination), "agents", "openai.yaml")); !os.IsNotExist(err) {
		t.Fatalf("managed Codex metadata leaked into copied skill: %v", err)
	}
	if len(result.Skills) != 2 {
		t.Fatalf("skills = %d, want 2", len(result.Skills))
	}

	// Global is now the canonical source that fans out to every agent's native
	// skill root (mirroring how a project's `.agents/skills` already fans out to
	// its agents), so deleting the global copy also removes the copies it owns
	// in each agent scope, including this one manually placed under "codex".
	if _, err := service.DeleteSkill(source); err != nil {
		t.Fatalf("DeleteSkill() error = %v", err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("deleted global skill still exists: %v", err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("propagated agent skill was not removed: %v", err)
	}
}

func TestCopyToGlobalPropagatesToEveryAgent(t *testing.T) {
	home := t.TempDir()
	service := NewDiscoveryService(home)
	source := filepath.Join(home, ".config", "opencode", "skills", "testing", "SKILL.md")
	writeFixture(t, source, "---\nname: testing\ndescription: Write focused tests.\n---\n")

	if _, err := service.CopySkill(source, "global"); err != nil {
		t.Fatalf("CopySkill() error = %v", err)
	}
	for _, nativeSkillPath := range []string{
		filepath.Join(home, ".claude", "skills", "testing", "SKILL.md"),
		filepath.Join(home, ".codex", "skills", "testing", "SKILL.md"),
		filepath.Join(home, ".config", "opencode", "skills", "testing", "SKILL.md"),
	} {
		if _, err := os.Stat(nativeSkillPath); err != nil {
			t.Fatalf("skill was not propagated to %s: %v", nativeSkillPath, err)
		}
	}

	result, err := service.DeleteSkill(filepath.Join(home, ".agents", "skills", "testing", "SKILL.md"))
	if err != nil {
		t.Fatalf("DeleteSkill() error = %v", err)
	}
	if len(result.Skills) != 0 {
		t.Fatalf("skills = %d, want 0 after removing the global copy and its propagated copies", len(result.Skills))
	}
}

func TestSkillInvocationModesSyncOpenCodeWithoutOverwritingUserCommand(t *testing.T) {
	home := t.TempDir()
	skillPath := filepath.Join(home, ".agents", "skills", "testing", "SKILL.md")
	writeFixture(t, skillPath, "---\nname: testing\ndescription: Write focused tests.\n---\nFollow the test workflow.\n")
	writeFixture(t, filepath.Join(home, ".config", "opencode", "opencode.json"), `{"permission":{"skill":{"testing":"deny"}}}`)
	service := NewDiscoveryService(home)

	result, err := service.SetSkillInvocationMode(skillPath, "explicit")
	if err != nil {
		t.Fatalf("SetSkillInvocationMode(explicit) error = %v", err)
	}
	if result.Skills[0].InvocationMode != "explicit" {
		t.Fatalf("mode = %q, want explicit", result.Skills[0].InvocationMode)
	}
	commandPath := filepath.Join(home, ".config", "opencode", "commands", "testing.md")
	if _, err := os.Stat(commandPath); err != nil {
		t.Fatalf("generated command is missing: %v", err)
	}

	if _, err := service.SetSkillInvocationMode(skillPath, "always"); err != nil {
		t.Fatalf("SetSkillInvocationMode(always) error = %v", err)
	}
	config, err := os.ReadFile(filepath.Join(home, ".config", "opencode", "opencode.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(config), `"testing": "deny"`) {
		t.Fatalf("original permission was not restored: %s", config)
	}

	userCommand := "---\ndescription: User command\n---\nDo not replace me.\n"
	if err := os.WriteFile(commandPath, []byte(userCommand), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetSkillInvocationMode(skillPath, "explicit"); err == nil {
		t.Fatal("expected existing user command to be protected")
	}
}

func TestSkillInvocationModesSyncClaude(t *testing.T) {
	home := t.TempDir()
	skillPath := filepath.Join(home, ".claude", "skills", "review", "SKILL.md")
	writeFixture(t, skillPath, "---\nname: review\ndescription: Review code.\n---\nReview the change.\n")
	writeFixture(t, filepath.Join(home, ".claude", "settings.json"), `{"permissions":{"allow":[]}}`)
	service := NewDiscoveryService(home)

	if _, err := service.SetSkillInvocationMode(skillPath, "explicit"); err != nil {
		t.Fatalf("Claude explicit mode error = %v", err)
	}
	settings, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(settings), `"review": "user-invocable-only"`) {
		t.Fatalf("Claude explicit override missing: %s", settings)
	}

	if _, err := service.SetSkillInvocationMode(skillPath, "always"); err != nil {
		t.Fatalf("Claude always mode error = %v", err)
	}
	instructions, err := os.ReadFile(filepath.Join(home, ".claude", "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(instructions), "BEGIN AGENT STUDIO SKILL") {
		t.Fatalf("Claude managed instructions missing: %s", instructions)
	}
}

func TestSkillInvocationModesSyncCodex(t *testing.T) {
	home := t.TempDir()
	skillPath := filepath.Join(home, ".codex", "skills", "review", "SKILL.md")
	writeFixture(t, skillPath, "---\nname: review\ndescription: Review code.\n---\nReview the change.\n")
	writeFixture(t, filepath.Join(home, ".codex", "config.toml"), "model = \"test\"\n")
	service := NewDiscoveryService(home)

	if _, err := service.SetSkillInvocationMode(skillPath, "explicit"); err != nil {
		t.Fatalf("Codex explicit mode error = %v", err)
	}
	metadata, err := os.ReadFile(filepath.Join(home, ".codex", "skills", "review", "agents", "openai.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(metadata), "allow_implicit_invocation: false") {
		t.Fatalf("Codex explicit policy missing: %s", metadata)
	}

	if _, err := service.SetSkillInvocationMode(skillPath, "disabled"); err != nil {
		t.Fatalf("Codex disabled mode error = %v", err)
	}
	config, err := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(config), "enabled = false") {
		t.Fatalf("Codex disabled policy missing: %s", config)
	}
}

func TestAddProjectTracksProjectSkillDirectory(t *testing.T) {
	home := t.TempDir()
	project := filepath.Join(home, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := NewDiscoveryService(home).AddProject(project)
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}
	if len(result.Projects) != 1 || len(result.Scopes) != 5 {
		t.Fatalf("unexpected workspace = %#v", result)
	}
}

func TestCopySkillToProjectPropagatesToAllAgents(t *testing.T) {
	home := t.TempDir()
	project := filepath.Join(home, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	service := NewDiscoveryService(home)

	result, err := service.AddProject(project)
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}
	var projectScopeID string
	for _, scope := range result.Scopes {
		if scope.Kind == "project" {
			projectScopeID = scope.ID
		}
	}
	if projectScopeID == "" {
		t.Fatalf("project scope not found = %#v", result.Scopes)
	}

	source := filepath.Join(home, ".agents", "skills", "testing", "SKILL.md")
	writeFixture(t, source, "---\nname: Testing\ndescription: Write focused tests.\n---\nBody.\n")

	if _, err := service.CopySkill(source, projectScopeID); err != nil {
		t.Fatalf("CopySkill() error = %v", err)
	}

	for _, agentDir := range []string{".claude", ".codex", ".opencode"} {
		propagated := filepath.Join(project, agentDir, "skills", "testing", "SKILL.md")
		if _, err := os.Stat(propagated); err != nil {
			t.Fatalf("propagated skill missing in %s: %v", agentDir, err)
		}
	}

	codexMetadata := filepath.Join(project, ".codex", "skills", "testing", "agents", "openai.yaml")
	metadata, err := os.ReadFile(codexMetadata)
	if err != nil {
		t.Fatalf("Codex metadata missing: %v", err)
	}
	if !strings.Contains(string(metadata), "name: Testing") {
		t.Fatalf("Codex metadata name missing: %s", metadata)
	}
}

func TestDeleteProjectSkillRemovesPropagatedCopies(t *testing.T) {
	home := t.TempDir()
	project := filepath.Join(home, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	service := NewDiscoveryService(home)

	result, err := service.AddProject(project)
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}
	var projectScopeID string
	for _, scope := range result.Scopes {
		if scope.Kind == "project" {
			projectScopeID = scope.ID
		}
	}

	source := filepath.Join(home, ".agents", "skills", "testing", "SKILL.md")
	writeFixture(t, source, "---\nname: Testing\ndescription: Write focused tests.\n---\nBody.\n")
	if _, err := service.CopySkill(source, projectScopeID); err != nil {
		t.Fatalf("CopySkill() error = %v", err)
	}

	projectSkill := filepath.Join(project, ".agents", "skills", "testing", "SKILL.md")
	if _, err := service.DeleteSkill(projectSkill); err != nil {
		t.Fatalf("DeleteSkill() error = %v", err)
	}

	for _, agentDir := range []string{".claude", ".codex", ".opencode"} {
		propagated := filepath.Join(project, agentDir, "skills", "testing", "SKILL.md")
		if _, err := os.Stat(propagated); !os.IsNotExist(err) {
			t.Fatalf("propagated skill not removed in %s", agentDir)
		}
	}
}

func TestGlobalSkillPublishesToAllAgents(t *testing.T) {
	home := t.TempDir()
	skillPath := filepath.Join(home, ".agents", "skills", "testing", "SKILL.md")
	writeFixture(t, skillPath, "---\nname: testing\ndescription: Write focused tests.\n---\nFollow the test workflow.\n")
	writeFixture(t, filepath.Join(home, ".config", "opencode", "opencode.json"), "{}")
	writeFixture(t, filepath.Join(home, ".claude", "settings.json"), "{}")
	writeFixture(t, filepath.Join(home, ".codex", "config.toml"), "model = \"test\"\n")
	service := NewDiscoveryService(home)

	if _, err := service.SetSkillInvocationMode(skillPath, "always"); err != nil {
		t.Fatalf("SetSkillInvocationMode(always) error = %v", err)
	}

	opencodeConfig, err := os.ReadFile(filepath.Join(home, ".config", "opencode", "opencode.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(opencodeConfig), "instructions") {
		t.Fatalf("OpenCode instructions missing: %s", opencodeConfig)
	}

	claudeMD, err := os.ReadFile(filepath.Join(home, ".claude", "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(claudeMD), "BEGIN AGENT STUDIO SKILL") {
		t.Fatalf("Claude managed instructions missing: %s", claudeMD)
	}

	codexMD, err := os.ReadFile(filepath.Join(home, ".codex", "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(codexMD), "Agent Studio managed skill policy") {
		t.Fatalf("Codex managed instructions missing: %s", codexMD)
	}
}

func TestProjectSkillPublishesToClaudeInProject(t *testing.T) {
	home := t.TempDir()
	project := filepath.Join(home, "project")
	skillPath := filepath.Join(project, ".agents", "skills", "testing", "SKILL.md")
	writeFixture(t, skillPath, "---\nname: testing\ndescription: Write focused tests.\n---\nFollow the test workflow.\n")
	service := NewDiscoveryService(home)

	result, err := service.AddProject(project)
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}
	if _, err := service.SetSkillInvocationMode(skillPath, "always"); err != nil {
		t.Fatalf("SetSkillInvocationMode(always) error = %v", err)
	}

	projectClaudeMD, err := os.ReadFile(filepath.Join(project, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(projectClaudeMD), "BEGIN AGENT STUDIO SKILL") {
		t.Fatalf("project Claude managed instructions missing: %s", projectClaudeMD)
	}

	if _, err := os.ReadFile(filepath.Join(home, ".claude", "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatalf("global Claude instructions should not be written for a project skill")
	}

	_ = result
}

func TestRemoveProjectStopsTrackingWithoutDeletingProjectFiles(t *testing.T) {
	home := t.TempDir()
	project := filepath.Join(home, "project")
	skill := filepath.Join(project, ".agents", "skills", "testing", "SKILL.md")
	writeFixture(t, skill, "# Testing\n")

	service := NewDiscoveryService(home)
	result, err := service.AddProject(project)
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}
	if _, err := service.RemoveProject(result.Projects[0].ID); err != nil {
		t.Fatalf("RemoveProject() error = %v", err)
	}
	updated, err := service.Discover()
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(updated.Projects) != 0 {
		t.Fatalf("projects = %d, want 0", len(updated.Projects))
	}
	if _, err := os.Stat(skill); err != nil {
		t.Fatalf("project skill was deleted: %v", err)
	}
}

func TestLoadProjectsIgnoresInvalidAndDuplicateEntries(t *testing.T) {
	home := t.TempDir()
	projectsPath := filepath.Join(home, ".agent-studio", "projects.json")
	writeFixture(t, projectsPath, `[
  {"id":"one","name":"Project","path":"/projects/one"},
  {"id":"duplicate","name":"Duplicate","path":"/projects/one"},
  {"id":"","name":"Missing ID","path":"/projects/two"},
  {"id":"three","name":"Project three","path":"/projects/three"}
]`)

	projects := NewDiscoveryService(home).loadProjects()
	if len(projects) != 2 {
		t.Fatalf("projects = %#v, want only valid unique entries", projects)
	}
	if projects[0].ID != "one" || projects[1].ID != "three" {
		t.Fatalf("projects = %#v, want first valid entry for each path", projects)
	}
}

func TestParseRepositoryURL(t *testing.T) {
	tests := []struct {
		name, input, owner, repository, branch, directory string
	}{
		{"repository", "https://github.com/acme/skills", "acme", "skills", "main", ""},
		{"skill folder", "https://github.com/acme/skills/tree/develop/packages/testing", "acme", "skills", "develop", "packages/testing"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			owner, repository, branch, directory, host, err := parseRepositoryURL(test.input)
			if err != nil {
				t.Fatalf("parseRepositoryURL() error = %v", err)
			}
			if owner != test.owner || repository != test.repository || branch != test.branch || directory != test.directory || host != "github.com" {
				t.Errorf("got %q/%q/%q/%q/%q", owner, repository, branch, directory, host)
			}
		})
	}
}

func TestParseRepositoryURLRejectsInvalidURLs(t *testing.T) {
	for _, input := range []string{"http://github.com/acme/skills", "https://unknown.com/acme/skills", "https://github.com/acme"} {
		if _, _, _, _, _, err := parseRepositoryURL(input); err == nil {
			t.Errorf("parseRepositoryURL(%q) accepted invalid URL", input)
		}
	}
}

func TestFindSkillRootSupportsGitAndZIPLayouts(t *testing.T) {
	const skillPath = "plugins/ui-design/skills/responsive-design"

	tests := []struct {
		name       string
		repository string
	}{
		{name: "git checkout", repository: ""},
		{name: "zip archive", repository: "agents-main"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			base := root
			if test.repository != "" {
				base = filepath.Join(root, test.repository)
			}
			expected := filepath.Join(base, filepath.FromSlash(skillPath))
			writeFixture(t, filepath.Join(expected, "SKILL.md"), "# Responsive design\n")

			actual, err := findSkillRoot(root, skillPath)
			if err != nil {
				t.Fatalf("findSkillRoot() error = %v", err)
			}
			if actual != expected {
				t.Errorf("findSkillRoot() = %q, want %q", actual, expected)
			}
		})
	}
}

func TestParseSkillIgnoresMetadataLooksLikeLinesInTheBody(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SKILL.md")
	writeFixture(t, path, "---\nname: Testing\ndescription: Write focused tests.\n---\n"+
		"# Testing\n\nExample config:\n\nname: not-the-skill-name\ndescription: not the real description\n")

	skill, err := parseSkill(path)
	if err != nil {
		t.Fatalf("parseSkill() error = %v", err)
	}
	if skill.Name != "Testing" {
		t.Errorf("Name = %q, want %q (body text must not override frontmatter)", skill.Name, "Testing")
	}
	if skill.Description != "Write focused tests." {
		t.Errorf("Description = %q, want %q (body text must not override frontmatter)", skill.Description, "Write focused tests.")
	}
}

func TestCopyDirectoryPreservesExecutablePermissions(t *testing.T) {
	source := t.TempDir()
	scriptPath := filepath.Join(source, "scripts", "run.sh")
	writeFixture(t, scriptPath, "#!/bin/sh\necho hi\n")
	if err := os.Chmod(scriptPath, 0o755); err != nil {
		t.Fatalf("chmod fixture: %v", err)
	}

	destination := filepath.Join(t.TempDir(), "copy")
	if err := copyDirectory(source, destination); err != nil {
		t.Fatalf("copyDirectory() error = %v", err)
	}
	info, err := os.Stat(filepath.Join(destination, "scripts", "run.sh"))
	if err != nil {
		t.Fatalf("copied script is missing: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("copied script lost its executable bit: mode = %v", info.Mode())
	}
}

// TestConcurrentAddProjectDoesNotLoseWrites guards against the read-modify-write race on
// projects.json: Wails dispatches each JS call on its own goroutine, so without
// DiscoveryService's mutex, concurrent AddProject calls can each read the same
// projects.json, append their own entry, and overwrite each other on save.
func TestConcurrentAddProjectDoesNotLoseWrites(t *testing.T) {
	home := t.TempDir()
	service := NewDiscoveryService(home)
	const projectCount = 20

	var wg sync.WaitGroup
	for index := 0; index < projectCount; index++ {
		projectPath := filepath.Join(t.TempDir(), fmt.Sprintf("project-%d", index))
		if err := os.MkdirAll(projectPath, 0o755); err != nil {
			t.Fatalf("create project directory: %v", err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := service.AddProject(projectPath); err != nil {
				t.Errorf("AddProject() error = %v", err)
			}
		}()
	}
	wg.Wait()

	result, err := service.Discover()
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(result.Projects) != projectCount {
		t.Fatalf("projects = %d, want %d (concurrent AddProject calls lost writes)", len(result.Projects), projectCount)
	}
}

func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
