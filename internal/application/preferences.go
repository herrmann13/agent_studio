package application

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/herrmann13/agent_studio/internal/adapters"
	"github.com/herrmann13/agent_studio/internal/domain"
)

// preferences holds how the workspace is laid out. Agent IDs are kept even while
// an agent is uninstalled, so its place and visibility survive a reinstall.
type preferences struct {
	AgentOrder   []string `json:"agentOrder,omitempty"`
	HiddenAgents []string `json:"hiddenAgents,omitempty"`
}

func (s *DiscoveryService) preferencesPath() string {
	return filepath.Join(s.home, ".agent-studio", "preferences.json")
}

// loadPreferences returns an error for an unreadable or invalid file, so callers
// never overwrite (and lose) settings the user may still repair by hand.
func (s *DiscoveryService) loadPreferences() (preferences, error) {
	var result preferences
	content, err := os.ReadFile(s.preferencesPath())
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return preferences{}, fmt.Errorf("read %s: %w", s.preferencesPath(), err)
	}
	if err := json.Unmarshal(content, &result); err != nil {
		return preferences{}, fmt.Errorf("%s is not valid JSON (%v); fix or delete it to change the agent layout", s.preferencesPath(), err)
	}
	return result, nil
}

func (s *DiscoveryService) savePreferences(value preferences) error {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(s.preferencesPath(), content, 0o600)
}

// sortByOrder stably sorts items by their ID's position in order; items whose ID
// is not in order keep their relative order after the others.
func sortByOrder[T any](items []T, order []string, id func(T) string) {
	position := make(map[string]int, len(order))
	for index, value := range order {
		if _, seen := position[value]; !seen {
			position[value] = index
		}
	}
	rank := func(item T) int {
		if index, ok := position[id(item)]; ok {
			return index
		}
		return len(order)
	}
	slices.SortStableFunc(items, func(a, b T) int { return rank(a) - rank(b) })
}

// detectInstallations lists installed agents in the user's saved order.
func (s *DiscoveryService) detectInstallations(order []string) []adapters.Installation {
	var installations []adapters.Installation
	for _, adapter := range s.adapters {
		installations = append(installations, adapter.Detect(s.env)...)
	}
	sortByOrder(installations, order, func(installation adapters.Installation) string { return installation.Agent.ID })
	return installations
}

// reorderVisible applies a new order of the given (visible) IDs to the full saved
// order, filling only the slots those IDs already occupied. Hidden and
// uninstalled agents keep their place; unknown IDs are appended.
func reorderVisible(full, visible []string) []string {
	result := slices.Clone(full)
	moved := map[string]bool{}
	for _, id := range visible {
		moved[id] = true
	}
	next := 0
	for index, id := range result {
		if moved[id] && next < len(visible) {
			result[index] = visible[next]
			next++
		}
	}
	for _, id := range visible[next:] {
		if !slices.Contains(result, id) {
			result = append(result, id)
		}
	}
	return result
}

// SetAgentOrder persists the order of the visible agent lanes.
func (s *DiscoveryService) SetAgentOrder(agentIDs []string) (domain.DiscoveryResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, err := s.loadPreferences()
	if err != nil {
		return domain.DiscoveryResult{}, err
	}
	installations := s.detectInstallations(value.AgentOrder)
	full := make([]string, 0, len(installations)+len(value.AgentOrder))
	for _, installation := range installations {
		full = append(full, installation.Agent.ID)
	}
	for _, id := range value.AgentOrder {
		if !slices.Contains(full, id) {
			full = append(full, id)
		}
	}
	value.AgentOrder = reorderVisible(full, agentIDs)
	if err := s.savePreferences(value); err != nil {
		return domain.DiscoveryResult{}, err
	}
	sortByOrder(installations, value.AgentOrder, func(installation adapters.Installation) string { return installation.Agent.ID })
	return s.discoverWith(installations, value, nil)
}

// SetTrackedAgents shows exactly the given installed agents. Tracking only
// affects what Agent Studio lists: hidden agents are still scanned (so conflict
// warnings stay accurate), Global skills still reach them, and no agent
// configuration is changed.
func (s *DiscoveryService) SetTrackedAgents(agentIDs []string) (domain.DiscoveryResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, err := s.loadPreferences()
	if err != nil {
		return domain.DiscoveryResult{}, err
	}
	installations := s.detectInstallations(value.AgentOrder)
	installed := map[string]bool{}
	for _, installation := range installations {
		installed[installation.Agent.ID] = true
	}
	for _, id := range agentIDs {
		if !installed[id] {
			return domain.DiscoveryResult{}, fmt.Errorf("agent %q is not installed", id)
		}
	}
	// Keep hiding agents that are uninstalled right now; recompute the rest.
	var hidden []string
	for _, id := range value.HiddenAgents {
		if !installed[id] {
			hidden = append(hidden, id)
		}
	}
	for id := range installed {
		if !slices.Contains(agentIDs, id) {
			hidden = append(hidden, id)
		}
	}
	slices.Sort(hidden)
	value.HiddenAgents = hidden
	if err := s.savePreferences(value); err != nil {
		return domain.DiscoveryResult{}, err
	}
	return s.discoverWith(installations, value, nil)
}

// ReorderProjects persists the order of tracked projects. Projects missing from
// projectIDs keep their relative order after the given ones.
func (s *DiscoveryService) ReorderProjects(projectIDs []string) (domain.DiscoveryResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	projects := s.loadProjects()
	sortByOrder(projects, projectIDs, func(project domain.Project) string { return project.ID })
	if err := s.saveProjects(projects); err != nil {
		return domain.DiscoveryResult{}, err
	}
	return s.discover()
}
