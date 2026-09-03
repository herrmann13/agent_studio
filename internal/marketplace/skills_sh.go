package marketplace

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"agent-studio/internal/domain"
)

const skillsShBaseURL = "https://skills.sh/api/v1"

type SkillsShProvider struct {
	client  *http.Client
	baseURL string
}

func NewSkillsShProvider() *SkillsShProvider {
	return &SkillsShProvider{client: &http.Client{}, baseURL: skillsShBaseURL}
}

func (p *SkillsShProvider) Search(query string, limit int) ([]domain.MarketplaceSkill, error) {
	if len(strings.TrimSpace(query)) < 2 {
		return nil, fmt.Errorf("search query must have at least 2 characters")
	}
	endpoint := p.baseURL + "/skills/search?q=" + url.QueryEscape(strings.TrimSpace(query)) + fmt.Sprintf("&limit=%d", clampLimit(limit))
	var response struct {
		Data []domain.MarketplaceSkill `json:"data"`
	}
	if err := p.get(endpoint, &response); err != nil {
		return nil, err
	}
	return response.Data, nil
}

func (p *SkillsShProvider) Details(id string) (domain.MarketplaceSkillDetails, error) {
	endpoint := p.baseURL + "/skills/" + escapedID(id)
	var details domain.MarketplaceSkillDetails
	if err := p.get(endpoint, &details); err != nil {
		return details, err
	}
	return details, nil
}

func (p *SkillsShProvider) Audits(id string) ([]domain.MarketplaceAudit, error) {
	endpoint := p.baseURL + "/skills/audit/" + escapedID(id)
	var response struct {
		Audits []domain.MarketplaceAudit `json:"audits"`
	}
	if err := p.get(endpoint, &response); err != nil {
		return nil, err
	}
	return response.Audits, nil
}

func (p *SkillsShProvider) get(endpoint string, output any) error {
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	if token := os.Getenv("SKILLS_SH_TOKEN"); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := p.client.Do(request)
	if err != nil {
		return fmt.Errorf("skills.sh request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if response.StatusCode == http.StatusUnauthorized {
			return fmt.Errorf("skills.sh requires authentication; configure SKILLS_SH_TOKEN")
		}
		return fmt.Errorf("skills.sh returned HTTP %d", response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(output); err != nil {
		return fmt.Errorf("decode skills.sh response: %w", err)
	}
	return nil
}

func escapedID(id string) string {
	parts := strings.Split(strings.Trim(id, "/"), "/")
	for index, part := range parts {
		parts[index] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func clampLimit(limit int) int {
	if limit < 1 {
		return 20
	}
	if limit > 200 {
		return 200
	}
	return limit
}
