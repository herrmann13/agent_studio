package domain

type MarketplaceSkill struct {
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Source      string `json:"source"`
	Installs    int    `json:"installs"`
	SourceType  string `json:"sourceType"`
	InstallURL  string `json:"installUrl"`
	URL         string `json:"url"`
	IsDuplicate bool   `json:"isDuplicate,omitempty"`
}

type MarketplaceSearchResult struct {
	Skills []MarketplaceSkill `json:"skills"`
	Query  string             `json:"query,omitempty"`
}

type MarketplaceFile struct {
	Path     string `json:"path"`
	Contents string `json:"contents"`
}

type MarketplaceDetails struct {
	ID       string            `json:"id"`
	Source   string            `json:"source"`
	Slug     string            `json:"slug"`
	Installs int               `json:"installs"`
	Hash     string            `json:"hash"`
	Files    []MarketplaceFile `json:"files"`
}

type MarketplaceAudit struct {
	Provider  string `json:"provider"`
	Status    string `json:"status"`
	Summary   string `json:"summary"`
	RiskLevel string `json:"riskLevel"`
}

type MarketplaceSkillDetails struct {
	MarketplaceDetails
	Audits []MarketplaceAudit `json:"audits,omitempty"`
}

type SkillInstallResult struct {
	Workspace DiscoveryResult `json:"workspace"`
	Method    string          `json:"method"`
}
