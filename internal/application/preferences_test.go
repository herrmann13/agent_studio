package application

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func agentLaneIDs(t *testing.T, service *DiscoveryService) []string {
	t.Helper()
	return scopeIDs(service, t)
}

func TestSetAgentOrderReordersLanes(t *testing.T) {
	home := t.TempDir()
	service := newTestService(home) // opencode (v1), claude, codex
	if _, err := service.SetAgentOrder([]string{"codex", "opencode", "claude"}); err != nil {
		t.Fatalf("SetAgentOrder() error = %v", err)
	}
	if got, want := agentLaneIDs(t, service), []string{"codex", "opencode", "claude"}; !slices.Equal(got, want) {
		t.Fatalf("lanes = %v, want %v", got, want)
	}
	result, _ := service.Discover()
	if result.Scopes[0].ID != "global" {
		t.Fatalf("Global should stay first, got %s", result.Scopes[0].ID)
	}
}

func TestAgentOrderSurvivesUninstall(t *testing.T) {
	home := t.TempDir()
	if _, err := newTestService(home).SetAgentOrder([]string{"claude", "codex", "opencode"}); err != nil {
		t.Fatal(err)
	}
	withoutCodex := NewDiscoveryServiceWithEnvironment(fakeEnvironment(home, map[string]string{"opencode": "1.18.20", "claude": "2.1.0"}))
	if _, err := withoutCodex.SetAgentOrder([]string{"opencode", "claude"}); err != nil {
		t.Fatal(err)
	}
	// Codex comes back: its saved place is kept after the agents that were reordered.
	if got, want := agentLaneIDs(t, newTestService(home)), []string{"opencode", "claude", "codex"}; !slices.Equal(got, want) {
		t.Fatalf("lanes = %v, want %v", got, want)
	}
}

func TestSetTrackedAgentsHidesAndRestoresLanes(t *testing.T) {
	home := t.TempDir()
	writeFixture(t, filepath.Join(home, ".codex", "skills", "testing", "SKILL.md"), "---\nname: testing\n---\n")
	service := newTestService(home)

	result, err := service.SetTrackedAgents([]string{"opencode", "claude"})
	if err != nil {
		t.Fatalf("SetTrackedAgents() error = %v", err)
	}
	if got := agentLaneIDs(t, service); slices.Contains(got, "codex") {
		t.Fatalf("hidden agent still has a lane: %v", got)
	}
	for _, agent := range result.Agents {
		if agent.Tracked == (agent.ID == "codex") {
			t.Fatalf("agent %s tracked = %v", agent.ID, agent.Tracked)
		}
	}
	var hiddenScope bool
	for _, scope := range result.Scopes {
		if scope.ID == "codex" {
			hiddenScope = scope.Hidden
		}
	}
	if !hiddenScope {
		t.Fatalf("hidden agent's scope should be reported with Hidden set")
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "skills", "testing", "SKILL.md")); err != nil {
		t.Fatalf("hiding an agent must not touch its files: %v", err)
	}

	if _, err := service.SetTrackedAgents([]string{"opencode", "claude", "codex"}); err != nil {
		t.Fatal(err)
	}
	if got := agentLaneIDs(t, service); !slices.Contains(got, "codex") {
		t.Fatalf("re-tracked agent has no lane: %v", got)
	}
}

func TestSetTrackedAgentsKeepsUninstalledAgentsHidden(t *testing.T) {
	home := t.TempDir()
	if _, err := newTestService(home).SetTrackedAgents([]string{"opencode", "claude"}); err != nil {
		t.Fatal(err)
	}
	withoutCodex := NewDiscoveryServiceWithEnvironment(fakeEnvironment(home, map[string]string{"opencode": "1.18.20", "claude": "2.1.0"}))
	if _, err := withoutCodex.SetTrackedAgents([]string{"opencode"}); err != nil {
		t.Fatal(err)
	}
	if got := agentLaneIDs(t, newTestService(home)); !slices.Equal(got, []string{"opencode"}) {
		t.Fatalf("lanes = %v, want only opencode (codex and claude stay hidden)", got)
	}
}

func TestSetTrackedAgentsRejectsUnknownAgent(t *testing.T) {
	if _, err := newTestService(t.TempDir()).SetTrackedAgents([]string{"opencode-v2"}); err == nil {
		t.Fatalf("SetTrackedAgents() accepted an agent that is not installed")
	}
}

func TestReorderProjects(t *testing.T) {
	home := t.TempDir()
	service := newTestService(home)
	var ids []string
	for _, name := range []string{"alpha", "beta", "gamma"} {
		path := filepath.Join(home, name)
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		result, err := service.AddProject(path)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, result.Projects[len(result.Projects)-1].ID)
	}
	result, err := service.ReorderProjects([]string{ids[2], ids[0]})
	if err != nil {
		t.Fatalf("ReorderProjects() error = %v", err)
	}
	var names []string
	for _, project := range result.Projects {
		names = append(names, project.Name)
	}
	if want := []string{"gamma", "alpha", "beta"}; !slices.Equal(names, want) {
		t.Fatalf("projects = %v, want %v", names, want)
	}
	var lanes []string
	for _, scope := range result.Scopes {
		if scope.Kind == "project" {
			lanes = append(lanes, scope.Name)
		}
	}
	if lanes[0] != "gamma (shared)" {
		t.Fatalf("project lanes = %v, want gamma first", lanes)
	}
}

func TestHiddenAgentStillCountsForConflicts(t *testing.T) {
	home := t.TempDir()
	writeFixture(t, filepath.Join(home, ".codex", "skills", "testing", "SKILL.md"), "---\nname: testing\n---\nCodex version.\n")
	writeFixture(t, filepath.Join(home, ".claude", "skills", "testing", "SKILL.md"), "---\nname: testing\n---\nClaude version.\n")
	result, err := newTestService(home).SetTrackedAgents([]string{"opencode", "claude"})
	if err != nil {
		t.Fatal(err)
	}
	for _, skill := range result.Skills {
		if skill.ScopeID == "claude" && !slices.Contains(skill.States, "conflict") {
			t.Fatalf("Claude copy states = %v, want conflict with the hidden Codex copy", skill.States)
		}
	}
}

func TestSetAgentOrderKeepsHiddenAgentsInPlace(t *testing.T) {
	home := t.TempDir()
	service := newTestService(home)
	if _, err := service.SetAgentOrder([]string{"codex", "claude", "opencode"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetTrackedAgents([]string{"claude", "opencode"}); err != nil {
		t.Fatal(err)
	}
	// Only the visible lanes are sent, as the GUI does.
	if _, err := service.SetAgentOrder([]string{"opencode", "claude"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetTrackedAgents([]string{"claude", "opencode", "codex"}); err != nil {
		t.Fatal(err)
	}
	if got, want := agentLaneIDs(t, service), []string{"codex", "opencode", "claude"}; !slices.Equal(got, want) {
		t.Fatalf("lanes = %v, want %v (hidden codex keeps its first place)", got, want)
	}
}

func TestInvalidPreferencesAreReportedAndNeverOverwritten(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".agent-studio", "preferences.json")
	writeFixture(t, path, `{"hiddenAgents": ["codex"`)
	service := newTestService(home)

	result, err := service.Discover()
	if err != nil {
		t.Fatalf("Discover() error = %v, want the default layout with a warning", err)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "preferences.json") {
		t.Fatalf("warnings = %v", result.Warnings)
	}
	if got := agentLaneIDs(t, service); len(got) != 3 {
		t.Fatalf("lanes = %v, want every agent shown by default", got)
	}
	if _, err := service.SetAgentOrder([]string{"codex"}); err == nil {
		t.Fatalf("SetAgentOrder() overwrote an invalid preferences file")
	}
	if content, _ := os.ReadFile(path); string(content) != `{"hiddenAgents": ["codex"` {
		t.Fatalf("preferences file was modified: %s", content)
	}
}

func TestReorderVisible(t *testing.T) {
	got := reorderVisible([]string{"a", "hidden", "b", "c"}, []string{"c", "a", "new"})
	if want := []string{"c", "hidden", "b", "a", "new"}; !slices.Equal(got, want) {
		t.Fatalf("reorderVisible() = %v, want %v", got, want)
	}
}
