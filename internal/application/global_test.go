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

func TestGlobalSkillAcceptsNpxSkillsSymlinkInClaude(t *testing.T) {
	home := t.TempDir()
	globalDir := filepath.Join(home, ".agents", "skills", "linked")
	writeFixture(t, filepath.Join(globalDir, "SKILL.md"), "---\nname: linked\n---\n")
	link := filepath.Join(home, ".claude", "skills", "linked")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(globalDir, link); err != nil {
		t.Fatal(err)
	}
	service := newTestService(home)

	if _, err := service.SyncGlobalSkills(); err != nil {
		t.Fatalf("SyncGlobalSkills() error = %v, want the symlink accepted", err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink should be left in place: %v", err)
	}

	if _, err := service.DeleteSkill(filepath.Join(globalDir, "SKILL.md")); err != nil {
		t.Fatalf("DeleteSkill() error = %v", err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("symlink to the deleted global skill should be removed: %v", err)
	}
}

func TestGlobalSkillRefusesSymlinkToAnotherSkill(t *testing.T) {
	home := t.TempDir()
	writeFixture(t, filepath.Join(home, ".agents", "skills", "testing", "SKILL.md"), "---\nname: testing\n---\n")
	other := filepath.Join(home, "elsewhere", "testing")
	writeFixture(t, filepath.Join(other, "SKILL.md"), "---\nname: testing\n---\nOther.\n")
	if err := os.MkdirAll(filepath.Join(home, ".claude", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, filepath.Join(home, ".claude", "skills", "testing")); err != nil {
		t.Fatal(err)
	}

	if _, err := newTestService(home).SyncGlobalSkills(); err == nil || !strings.Contains(err.Error(), "already contains its own") {
		t.Fatalf("SyncGlobalSkills() error = %v, want refusal", err)
	}
	if content, _ := os.ReadFile(filepath.Join(other, "SKILL.md")); !strings.Contains(string(content), "Other.") {
		t.Fatalf("linked skill was modified: %s", content)
	}
}

func TestSyncGlobalSkillsRemovesOrphanedCopies(t *testing.T) {
	home := t.TempDir()
	service := newTestService(home)
	globalSkill := filepath.Join(home, ".agents", "skills", "gone", "SKILL.md")
	writeFixture(t, globalSkill, "---\nname: gone\n---\n")
	if _, err := service.SyncGlobalSkills(); err != nil {
		t.Fatalf("SyncGlobalSkills() error = %v", err)
	}
	claudeCopy := filepath.Join(home, ".claude", "skills", "gone")
	if !isGlobalCopy(claudeCopy) {
		t.Fatalf("Claude copy was not created")
	}
	ownSkill := filepath.Join(home, ".claude", "skills", "own", "SKILL.md")
	writeFixture(t, ownSkill, "---\nname: own\n---\n")

	// Removed outside Agent Studio, e.g. by `npx skills remove`.
	if err := os.RemoveAll(filepath.Dir(globalSkill)); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SyncGlobalSkills(); err != nil {
		t.Fatalf("SyncGlobalSkills() error = %v", err)
	}
	if _, err := os.Stat(claudeCopy); !os.IsNotExist(err) {
		t.Fatalf("orphaned Claude copy should be removed: %v", err)
	}
	if _, err := os.Stat(ownSkill); err != nil {
		t.Fatalf("Claude's own skill must be kept: %v", err)
	}
}

func TestNestedGlobalSkillCopyIsNotTreatedAsOrphan(t *testing.T) {
	home := t.TempDir()
	service := newTestService(home)
	nested := filepath.Join(home, ".agents", "skills", "team", "foo", "SKILL.md")
	writeFixture(t, nested, "---\nname: foo\n---\n")
	if _, err := service.SyncGlobalSkills(); err != nil {
		t.Fatalf("SyncGlobalSkills() error = %v", err)
	}
	claudeCopy := filepath.Join(home, ".claude", "skills", "foo")
	if !isGlobalCopy(claudeCopy) {
		t.Fatalf("nested global skill was not copied to Claude Code")
	}
	if err := service.removeOrphanedGlobalCopies(); err != nil {
		t.Fatal(err)
	}
	if !isGlobalCopy(claudeCopy) {
		t.Fatalf("copy of a nested global skill was removed as an orphan")
	}

	if _, err := service.DeleteSkill(nested); err != nil {
		t.Fatalf("DeleteSkill() error = %v", err)
	}
	if _, err := os.Stat(claudeCopy); !os.IsNotExist(err) {
		t.Fatalf("copy of the deleted nested skill should be removed: %v", err)
	}
}

func TestSyncGlobalSkillsRemovesDanglingLinksIntoGlobal(t *testing.T) {
	home := t.TempDir()
	globalDir := filepath.Join(home, ".agents", "skills", "linked")
	writeFixture(t, filepath.Join(globalDir, "SKILL.md"), "---\nname: linked\n---\n")
	claudeRoot := filepath.Join(home, ".claude", "skills")
	if err := os.MkdirAll(claudeRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(globalDir, filepath.Join(claudeRoot, "linked")); err != nil {
		t.Fatal(err)
	}
	// A link the user made to somewhere else must survive even when dangling.
	if err := os.Symlink(filepath.Join(home, "elsewhere", "mine"), filepath.Join(claudeRoot, "mine")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(globalDir); err != nil { // removed outside Agent Studio
		t.Fatal(err)
	}

	if _, err := newTestService(home).SyncGlobalSkills(); err != nil {
		t.Fatalf("SyncGlobalSkills() error = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(claudeRoot, "linked")); !os.IsNotExist(err) {
		t.Fatalf("dangling link into ~/.agents/skills should be removed: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(claudeRoot, "mine")); err != nil {
		t.Fatalf("unrelated link was removed: %v", err)
	}
}
