package application

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/herrmann13/agent_studio/internal/domain"
)

const (
	defaultRegistryURL  = "https://skills.sh"
	registryResultLimit = 20
	// registryMinimumQuery mirrors the skills.sh API, which rejects shorter queries.
	registryMinimumQuery = 2
	// registryMaxResponseBytes caps the response read; 20 results are a few KB.
	registryMaxResponseBytes = 1 << 20
)

// registrySourcePattern accepts the `owner/repository` GitHub slug skills.sh
// reports as a skill's source; anything else is refused before building a URL.
var (
	registrySourcePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	registrySkillPattern  = regexp.MustCompile(`^[A-Za-z0-9_.:-]+$`)
)

type registrySearchResponse struct {
	Skills []struct {
		ID       string `json:"id"`
		Source   string `json:"source"`
		SkillID  string `json:"skillId"`
		Name     string `json:"name"`
		Installs int    `json:"installs"`
	} `json:"skills"`
	Error string `json:"error"`
}

// SearchRegistrySkills looks a skill up in the public skills.sh directory, the
// same index `npx skills find` uses. It does not touch local state, so it does
// not take the service lock and can run while another operation is in progress.
func (s *DiscoveryService) SearchRegistrySkills(query string) ([]domain.RegistrySkill, error) {
	query = strings.TrimSpace(query)
	if len([]rune(query)) < registryMinimumQuery {
		return []domain.RegistrySkill{}, nil
	}
	endpoint := strings.TrimSuffix(s.registryURL, "/") + "/api/search?" + url.Values{
		"q":     {query},
		"limit": {fmt.Sprint(registryResultLimit)},
	}.Encode()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "agent-studio")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("search skills.sh: %w", err)
	}
	defer response.Body.Close()
	var payload registrySearchResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, registryMaxResponseBytes)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("search skills.sh: unexpected response (HTTP %d)", response.StatusCode)
	}
	if response.StatusCode != http.StatusOK {
		if payload.Error != "" {
			return nil, fmt.Errorf("search skills.sh: %s", payload.Error)
		}
		return nil, fmt.Errorf("search skills.sh: HTTP %d", response.StatusCode)
	}
	results := make([]domain.RegistrySkill, 0, len(payload.Skills))
	for _, skill := range payload.Skills {
		if !registrySourcePattern.MatchString(skill.Source) || !registrySkillPattern.MatchString(skill.SkillID) {
			continue
		}
		name := skill.Name
		if name == "" {
			name = skill.SkillID
		}
		results = append(results, domain.RegistrySkill{
			ID:       skill.Source + "/" + skill.SkillID,
			Name:     name,
			Source:   skill.Source,
			SkillID:  skill.SkillID,
			Installs: skill.Installs,
			URL:      strings.TrimSuffix(s.registryURL, "/") + "/" + skill.Source + "/" + skill.SkillID,
		})
	}
	return results, nil
}

// InstallRegistrySkill installs a skills.sh result into a scope. It is exactly
// `npx skills add https://github.com/<source> --skill <skillId>`, so the skill is
// fetched from its GitHub repository the same way as a pasted install command.
func (s *DiscoveryService) InstallRegistrySkill(source, skillID, targetScopeID string) (domain.SkillInstallResult, error) {
	if !registrySourcePattern.MatchString(source) {
		return domain.SkillInstallResult{}, fmt.Errorf("invalid skills.sh source %q", source)
	}
	if !registrySkillPattern.MatchString(skillID) {
		return domain.SkillInstallResult{}, fmt.Errorf("invalid skills.sh skill %q", skillID)
	}
	return s.InstallSkillFromURL("npx skills add https://github.com/"+source+" --skill "+skillID, targetScopeID)
}
