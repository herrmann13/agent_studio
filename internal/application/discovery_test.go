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

	if _, err := service.DeleteSkill(destination); err != nil {
		t.Fatalf("DeleteSkill() error = %v", err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("deleted skill still exists: %v", err)
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
