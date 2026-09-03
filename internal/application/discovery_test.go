package application

import (
	"os"
	"path/filepath"
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

	result, err := service.CopySkill(source, "codex")
	if err != nil {
		t.Fatalf("CopySkill() error = %v", err)
	}
	destination := filepath.Join(home, ".codex", "skills", "testing", "SKILL.md")
	if _, err := os.Stat(destination); err != nil {
		t.Fatalf("copied skill is missing: %v", err)
	}
	if len(result.Skills) != 2 {
		t.Fatalf("skills = %d, want 2", len(result.Skills))
	}

	if _, err := service.DeleteSkill(source); err != nil {
		t.Fatalf("DeleteSkill() error = %v", err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("deleted global skill still exists: %v", err)
	}
	if _, err := os.Stat(destination); err != nil {
		t.Fatalf("agent skill was deleted: %v", err)
	}
	backups, err := filepath.Glob(filepath.Join(home, ".agent-studio", "backups", "*", "testing", "SKILL.md"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("backup was not created: %v", err)
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
