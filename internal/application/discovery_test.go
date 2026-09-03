package application

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverFindsConfiguredAgentsAndSharedSkills(t *testing.T) {
	home := t.TempDir()
	writeFixture(t, filepath.Join(home, ".config", "opencode", "opencode.json"), "{}")
	writeFixture(t, filepath.Join(home, ".claude", "settings.json"), "{}")
	writeFixture(t, filepath.Join(home, ".codex", "config.toml"), "model = \"test\"")
	writeFixture(t, filepath.Join(home, ".agents", "skills", "testing", "SKILL.md"), "---\nname: Testing\ndescription: Write focused tests.\n---\n")

	result, err := NewDiscoveryService(home).Discover()
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(result.Agents) != 3 {
		t.Fatalf("agents = %d, want 3", len(result.Agents))
	}
	for _, agent := range result.Agents {
		if agent.Status != "configured" {
			t.Errorf("%s status = %q, want configured", agent.Provider, agent.Status)
		}
	}
	if len(result.ConfigFiles) != 3 {
		t.Errorf("config files = %d, want 3", len(result.ConfigFiles))
	}
	if len(result.Skills) != 1 {
		t.Fatalf("skills = %d, want 1", len(result.Skills))
	}
	skill := result.Skills[0]
	if skill.Name != "Testing" || skill.Description != "Write focused tests." {
		t.Errorf("skill = %#v", skill)
	}
	if len(skill.Sources) != 3 {
		t.Errorf("skill sources = %#v, want all providers", skill.Sources)
	}
}

func TestDiscoverHandlesMissingDirectories(t *testing.T) {
	result, err := NewDiscoveryService(t.TempDir()).Discover()
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(result.Agents) != 3 || len(result.Skills) != 0 || len(result.ConfigFiles) != 0 {
		t.Errorf("unexpected result = %#v", result)
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
