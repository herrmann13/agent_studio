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
	// Tracked is false when the user hid the agent; it then has no scope.
	Tracked bool `json:"tracked"`
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
	// Hidden marks the scope of an agent the user stopped tracking. It is still
	// scanned; interfaces leave it out of their lanes and destination lists.
	Hidden bool `json:"hidden,omitempty"`
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
	// Warnings are problems that did not stop discovery, such as an invalid
	// preferences file.
	Warnings []string `json:"warnings,omitempty"`
}

// RegistrySkill is a search result from the public skills.sh directory.
type RegistrySkill struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Source   string `json:"source"`
	SkillID  string `json:"skillId"`
	Installs int    `json:"installs"`
	URL      string `json:"url"`
}

// SkillInstallResult reports how a skill installed from a repository URL was fetched.
type SkillInstallResult struct {
	Workspace DiscoveryResult `json:"workspace"`
	Method    string          `json:"method"`
}
