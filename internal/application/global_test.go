package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGlobalSkillReachesEveryInstalledAgent(t *testing.T) {
	home := t.TempDir()
	service := newTestService(home)
	source := filepath.Join(home, ".codex", "skills", "testing", "SKILL.md")
	writeFixture(t, source, "---\nname: testing\ndescription: Write focused tests.\n---\nBody.\n")

	result, err := service.CopySkill(source, "global")
	if err != nil {
		t.Fatalf("CopySkill() error = %v", err)
	}
	// Codex and OpenCode read ~/.agents/skills natively; Claude Code gets a marked copy.
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "testing", "SKILL.md")); err != nil {
		t.Fatalf("global copy missing: %v", err)
	}
	claudeCopy := filepath.Join(home, ".claude", "skills", "testing")
	if !isGlobalCopy(claudeCopy) {
		t.Fatalf("Claude copy missing or unmarked")
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "opencode", "skills", "testing")); !os.IsNotExist(err) {
		t.Fatalf("OpenCode should not receive a copy: %v", err)
	}
	// The marked Claude copy is listed once, under Global, not in Claude's lane.
	for _, skill := range result.Skills {
		if skill.ScopeID == "claude" {
			t.Fatalf("managed Claude copy listed in Claude lane: %s", skill.Path)
		}
	}

	globalSkill := filepath.Join(home, ".agents", "skills", "testing", "SKILL.md")
	if _, err := service.DeleteSkill(globalSkill); err != nil {
		t.Fatalf("DeleteSkill() error = %v", err)
	}
	if _, err := os.Stat(claudeCopy); !os.IsNotExist(err) {
		t.Fatalf("Claude copy should be removed with the global skill: %v", err)
	}
}

func TestGlobalSkillSkipsClaudeWhenNotInstalled(t *testing.T) {
	home := t.TempDir()
	service := NewDiscoveryServiceWithEnvironment(fakeEnvironment(home, map[string]string{"codex": "0.162.0"}))
	source := filepath.Join(home, ".codex", "skills", "testing", "SKILL.md")
	writeFixture(t, source, "---\nname: testing\n---\n")

	if _, err := service.CopySkill(source, "global"); err != nil {
		t.Fatalf("CopySkill() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
		t.Fatalf("~/.claude should not be created when Claude Code is not installed: %v", err)
	}
}

func TestGlobalSkillDoesNotOverwriteClaudeOwnSkill(t *testing.T) {
	home := t.TempDir()
	service := newTestService(home)
	own := filepath.Join(home, ".claude", "skills", "testing", "SKILL.md")
	writeFixture(t, own, "---\nname: testing\n---\nClaude's own.\n")
	source := filepath.Join(home, ".codex", "skills", "testing", "SKILL.md")
	writeFixture(t, source, "---\nname: testing\n---\nCodex.\n")

	_, err := service.CopySkill(source, "global")
	if err == nil || !strings.Contains(err.Error(), "already contains its own") {
		t.Fatalf("CopySkill() error = %v, want refusal", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "testing")); !os.IsNotExist(err) {
		t.Fatalf("nothing should be written on refusal: %v", err)
	}
	if content, _ := os.ReadFile(own); !strings.Contains(string(content), "Claude's own.") {
		t.Fatalf("Claude's own skill was modified: %s", content)
	}
}

func TestGlobalSkillPolicyConfiguresOnlyInstalledAgents(t *testing.T) {
	home := t.TempDir()
	service := NewDiscoveryServiceWithEnvironment(fakeEnvironment(home, map[string]string{"opencode": "opencode v2.1.0"}))
	skillPath := filepath.Join(home, ".agents", "skills", "testing", "SKILL.md")
	writeFixture(t, skillPath, "---\nname: testing\n---\n")

	if _, err := service.SetSkillInvocationMode(skillPath, modeDisabled); err != nil {
		t.Fatalf("SetSkillInvocationMode() error = %v", err)
	}
	config, err := os.ReadFile(filepath.Join(home, ".config", "opencode", "opencode.json"))
	if err != nil || !strings.Contains(string(config), `"testing": "deny"`) {
		t.Fatalf("OpenCode config = %s, %v", config, err)
	}
	for _, path := range []string{filepath.Join(home, ".claude", "settings.json"), filepath.Join(home, ".codex", "config.toml")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s written for an agent that is not installed", path)
		}
	}
}

func TestSyncGlobalSkillsCopiesSkillsPlacedOutsideAgentStudio(t *testing.T) {
	home := t.TempDir()
	writeFixture(t, filepath.Join(home, ".agents", "skills", "placed", "SKILL.md"), "---\nname: placed\n---\n")
	writeFixture(t, filepath.Join(home, ".agents", "skills", "taken", "SKILL.md"), "---\nname: taken\n---\n")
	writeFixture(t, filepath.Join(home, ".claude", "skills", "taken", "SKILL.md"), "---\nname: taken\n---\nClaude's own.\n")
	service := newTestService(home)

	_, err := service.SyncGlobalSkills()
	if err == nil || !strings.Contains(err.Error(), "taken") {
		t.Fatalf("SyncGlobalSkills() error = %v, want conflict on taken", err)
	}
	if !isGlobalCopy(filepath.Join(home, ".claude", "skills", "placed")) {
		t.Fatalf("placed skill was not synced to Claude Code")
	}
	if isGlobalCopy(filepath.Join(home, ".claude", "skills", "taken")) {
		t.Fatalf("Claude's own skill was overwritten")
	}
}
