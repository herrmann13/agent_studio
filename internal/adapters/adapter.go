package adapters

import "agent-studio/internal/domain"

// Adapter owns the filesystem conventions of one supported agent.
type Adapter interface {
	Detect(home string) (domain.Agent, *domain.ConfigFile)
	SkillRoots(home string) []string
}
