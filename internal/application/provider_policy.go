package application

import (
	"agent-studio/internal/domain"
)

func (s *DiscoveryService) syncSkillPolicy(skill domain.Skill, scope domain.Scope, provider domain.Provider, mode string, policy *skillPolicy) error {
	switch provider {
	case domain.ProviderOpenCode:
		return s.syncOpenCodePolicy(skill, mode, policy)
	case domain.ProviderClaude:
		return s.syncClaudePolicy(skill, scope, mode, policy)
	case domain.ProviderCodex:
		return s.syncCodexPolicy(skill, scope, mode)
	default:
		return nil
	}
}
