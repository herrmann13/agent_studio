package adapters

import "agent-studio/internal/domain"

// Installation is one locally installed agent, together with the skill
// directory Agent Studio shows as that agent's scope.
type Installation struct {
	Agent  domain.Agent
	Config *domain.ConfigFile
	Scope  domain.Scope
}

// Adapter owns the filesystem conventions of one supported agent.
type Adapter interface {
	// Detect returns the installed variants of the agent. An agent whose
	// executable cannot be found returns none and is hidden from the workspace.
	Detect(env Environment) []Installation
	// UserSkillRoot is the agent's own user-level skill directory, whether or not
	// the agent is installed.
	UserSkillRoot(home string) string
	ProjectSkillRoot(projectPath string) string
	Provider() domain.Provider
}
