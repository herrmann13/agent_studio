package domain

// Provider identifies a supported terminal coding agent.
type Provider string

const (
	ProviderOpenCode Provider = "opencode"
	ProviderClaude   Provider = "claude"
	ProviderCodex    Provider = "codex"
)

// Agent is a locally discovered terminal agent. Discovery never changes it.
type Agent struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Provider    Provider `json:"provider"`
	Status      string   `json:"status"`
	ConfigPath  string   `json:"configPath"`
	CommandPath string   `json:"commandPath,omitempty"`
	Version     string   `json:"version,omitempty"`
}

// SkillLayoutOpenCodeV2 marks a scope whose skills follow OpenCode v2 discovery:
// the root and its sibling `skill` directory are both read, `<name>/SKILL.md`
// folders may be nested, and a loose `<name>.md` directly in a root is a skill too.
const SkillLayoutOpenCodeV2 = "opencode-v2"

// ConfigFile describes an existing, safe-to-display configuration file.
type ConfigFile struct {
	Provider Provider `json:"provider"`
	Path     string   `json:"path"`
	Scope    string   `json:"scope"`
}

// Skill is prompt content found in a known local skill directory.
type Skill struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Path           string   `json:"path"`
	ScopeID        string   `json:"scopeId"`
	States         []string `json:"states"`
	ContentHash    string   `json:"contentHash"`
	InvocationMode string   `json:"invocationMode"`
}

// Scope is a physical skill directory managed by the workspace.
type Scope struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Kind        string   `json:"kind"`
	Provider    Provider `json:"provider,omitempty"`
	Root        string   `json:"root"`
	SkillLayout string   `json:"skillLayout,omitempty"`
}

// Project is a user-selected directory whose project skill location is tracked.
type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// DiscoveryResult is the read-only inventory returned to the desktop UI.
type DiscoveryResult struct {
	Agents      []Agent      `json:"agents"`
	Skills      []Skill      `json:"skills"`
	ConfigFiles []ConfigFile `json:"configFiles"`
	Scopes      []Scope      `json:"scopes"`
	Projects    []Project    `json:"projects"`
	ScannedAt   string       `json:"scannedAt"`
}

// SkillInstallResult reports how a skill installed from a repository URL was fetched.
type SkillInstallResult struct {
	Workspace DiscoveryResult `json:"workspace"`
	Method    string          `json:"method"`
}
