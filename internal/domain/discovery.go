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
}

// ConfigFile describes an existing, safe-to-display configuration file.
type ConfigFile struct {
	Provider Provider `json:"provider"`
	Path     string   `json:"path"`
	Scope    string   `json:"scope"`
}

// Skill is prompt content found in a known local skill directory.
type Skill struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Path        string     `json:"path"`
	Sources     []Provider `json:"sources"`
}

// DiscoveryResult is the read-only inventory returned to the desktop UI.
type DiscoveryResult struct {
	Agents      []Agent      `json:"agents"`
	Skills      []Skill      `json:"skills"`
	ConfigFiles []ConfigFile `json:"configFiles"`
	ScannedAt   string       `json:"scannedAt"`
}
