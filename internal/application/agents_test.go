package application

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/herrmann13/agent_studio/internal/adapters"
)

// fakeEnvironment reports the given executables as installed, mapped to the
// `--version` output each one prints.
func fakeEnvironment(home string, commands map[string]string) adapters.Environment {
	return adapters.Environment{
		Home: home,
		FindCommands: func(names ...string) []string {
			var paths []string
			for _, name := range names {
				if _, ok := commands[name]; ok {
					paths = append(paths, "/fake/bin/"+name)
				}
			}
			return paths
		},
		CommandVersion: func(path string) string { return commands[filepath.Base(path)] },
	}
}

// newTestService treats Claude Code, Codex, and OpenCode v1 as installed.
func newTestService(home string) *DiscoveryService {
	return NewDiscoveryServiceWithEnvironment(fakeEnvironment(home, map[string]string{
		"opencode": "1.18.20", "claude": "2.1.0 (Claude Code)", "codex": "codex-cli 0.153.0",
	}))
}

func scopeIDs(service *DiscoveryService, t *testing.T) []string {
	t.Helper()
	result, err := service.Discover()
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	var ids []string
	for _, scope := range result.Scopes {
		if scope.Kind == "agent" && !scope.Hidden {
			ids = append(ids, scope.ID)
		}
	}
	return ids
}

func TestDiscoverHidesAgentsThatAreNotInstalled(t *testing.T) {
	home := t.TempDir()
	// Configs alone do not make an agent installed.
	writeFixture(t, filepath.Join(home, ".config", "opencode", "opencode.json"), "{}")
	writeFixture(t, filepath.Join(home, ".codex", "config.toml"), "")
	service := NewDiscoveryServiceWithEnvironment(fakeEnvironment(home, map[string]string{"claude": "2.1.0"}))

	result, err := service.Discover()
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(result.Agents) != 1 || result.Agents[0].ID != "claude" {
		t.Fatalf("agents = %#v, want only claude", result.Agents)
	}
	if ids := scopeIDs(service, t); len(ids) != 1 || ids[0] != "claude" {
		t.Fatalf("scopes = %v, want [claude]", ids)
	}
}

func TestDiscoverSeparatesOpenCodeVersions(t *testing.T) {
	home := t.TempDir()
	writeFixture(t, filepath.Join(home, ".config", "opencode", "skills", "testing", "SKILL.md"), "---\nname: testing\n---\n")
	tests := []struct {
		name     string
		commands map[string]string
		want     []string
	}{
		{name: "v1 only", commands: map[string]string{"opencode": "1.18.20"}, want: []string{"opencode"}},
		{name: "v2 only", commands: map[string]string{"opencode": "opencode v2.1.0", "opencode2": "opencode v2.1.0"}, want: []string{"opencode-v2"}},
		{name: "both", commands: map[string]string{"opencode": "opencode v2.1.0", "opencode2": "1.18.20"}, want: []string{"opencode", "opencode-v2"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := NewDiscoveryServiceWithEnvironment(fakeEnvironment(home, test.commands))
			result, err := service.Discover()
			if err != nil {
				t.Fatalf("Discover() error = %v", err)
			}
			ids := scopeIDs(service, t)
			if len(ids) != len(test.want) {
				t.Fatalf("scopes = %v, want %v", ids, test.want)
			}
			for index := range ids {
				if ids[index] != test.want[index] {
					t.Fatalf("scopes = %v, want %v", ids, test.want)
				}
			}
			if len(result.ConfigFiles) > 1 {
				t.Fatalf("shared OpenCode config reported %d times", len(result.ConfigFiles))
			}
			// The shared skill appears in every OpenCode lane but is a single copy.
			if len(result.Skills) != len(test.want) {
				t.Fatalf("skills = %d, want %d", len(result.Skills), len(test.want))
			}
			for _, skill := range result.Skills {
				if containsString(skill.States, "duplicated") || containsString(skill.States, "conflict") {
					t.Fatalf("shared skill flagged as %v", skill.States)
				}
			}
		})
	}
}

func TestOpenCodeV2IdentifiesSkillsLikeTheCLI(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, ".config", "opencode")
	writeFixture(t, filepath.Join(config, "skills", "folder", "SKILL.md"), "---\nname: folder\n---\n")
	writeFixture(t, filepath.Join(config, "skills", "folder", "references", "SKILL.md"), "---\nname: not-a-skill\n---\n")
	writeFixture(t, filepath.Join(config, "skills", "loose.md"), "---\ndescription: Loose file.\n---\nBody.\n")
	writeFixture(t, filepath.Join(config, "skills", "group", "nested", "SKILL.md"), "---\nname: nested\n---\n")
	writeFixture(t, filepath.Join(config, "skills", "group", "notes.md"), "not a skill below the root")
	writeFixture(t, filepath.Join(config, "skills", ".hidden", "SKILL.md"), "---\nname: hidden\n---\n")
	writeFixture(t, filepath.Join(config, "skill", "singular", "SKILL.md"), "---\nname: singular\n---\n")
	service := NewDiscoveryServiceWithEnvironment(fakeEnvironment(home, map[string]string{"opencode": "opencode v2.1.0"}))

	result, err := service.Discover()
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	names := map[string]string{}
	for _, skill := range result.Skills {
		names[skill.Name] = skill.Path
	}
	for _, want := range []string{"folder", "loose", "nested", "singular"} {
		if _, ok := names[want]; !ok {
			t.Errorf("skill %q not discovered; got %v", want, names)
		}
	}
	if len(names) != 4 {
		t.Fatalf("skills = %v, want exactly 4", names)
	}

	// Deleting a loose skill removes only that file, never the skills root.
	loose := names["loose"]
	if _, err := service.DeleteSkill(loose); err != nil {
		t.Fatalf("DeleteSkill() error = %v", err)
	}
	if _, err := os.Stat(loose); !os.IsNotExist(err) {
		t.Fatalf("loose skill still exists: %v", err)
	}
	if _, err := os.Stat(names["folder"]); err != nil {
		t.Fatalf("sibling skill was removed: %v", err)
	}
}

func TestCopyLooseOpenCodeV2SkillCreatesSkillFolder(t *testing.T) {
	home := t.TempDir()
	loose := filepath.Join(home, ".config", "opencode", "skills", "loose.md")
	writeFixture(t, loose, "---\ndescription: Loose file.\n---\nBody.\n")
	service := NewDiscoveryServiceWithEnvironment(fakeEnvironment(home, map[string]string{"opencode": "opencode v2.1.0", "claude": "2.1.0"}))

	if _, err := service.CopySkill(loose, "claude"); err != nil {
		t.Fatalf("CopySkill() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "loose", "SKILL.md")); err != nil {
		t.Fatalf("copied skill folder missing: %v", err)
	}
}

func TestUnreadableSkillPathsDoNotBreakDiscovery(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, ".config", "opencode")
	writeFixture(t, filepath.Join(config, "skills", "readable", "SKILL.md"), "---\nname: readable\n---\n")
	// `skill` exists as a file instead of a directory.
	writeFixture(t, filepath.Join(config, "skill"), "not a directory")
	locked := filepath.Join(config, "skills", "locked")
	writeFixture(t, filepath.Join(locked, "inner", "SKILL.md"), "---\nname: inner\n---\n")
	claudeLocked := filepath.Join(home, ".claude", "skills", "locked")
	writeFixture(t, filepath.Join(claudeLocked, "inner", "SKILL.md"), "---\nname: inner\n---\n")
	writeFixture(t, filepath.Join(home, ".claude", "skills", "visible", "SKILL.md"), "---\nname: visible\n---\n")
	for _, directory := range []string{locked, claudeLocked} {
		if err := os.Chmod(directory, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(directory, 0o755) })
	}
	service := NewDiscoveryServiceWithEnvironment(fakeEnvironment(home, map[string]string{"opencode": "opencode v2.1.0", "claude": "2.1.0"}))

	result, err := service.Discover()
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	names := map[string]bool{}
	for _, skill := range result.Skills {
		names[skill.Name] = true
	}
	if !names["readable"] || !names["visible"] {
		t.Fatalf("readable skills missing: %v", names)
	}
}
