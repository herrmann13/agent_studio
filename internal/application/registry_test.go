package application

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func registryTestService(t *testing.T, handler http.HandlerFunc) *DiscoveryService {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	service := newTestService(t.TempDir())
	service.registryURL = server.URL
	return service
}

func TestSearchRegistrySkillsMapsResults(t *testing.T) {
	service := registryTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/search" || r.URL.Query().Get("q") != "grill me" || r.URL.Query().Get("limit") == "" {
			t.Errorf("unexpected request %s", r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"skills":[
			{"id":"mattpocock/skills/grill-me","source":"mattpocock/skills","skillId":"grill-me","name":"grill-me","installs":1200},
			{"id":"bad","source":"../../etc","skillId":"x","name":"x","installs":1},
			{"id":"bad2","source":"owner/repo","skillId":"a b","name":"y","installs":1}
		]}`))
	})

	results, err := service.SearchRegistrySkills("  grill me ")
	if err != nil {
		t.Fatalf("SearchRegistrySkills() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %#v, want only the valid entry", results)
	}
	got := results[0]
	if got.Source != "mattpocock/skills" || got.SkillID != "grill-me" || got.Installs != 1200 || !strings.HasSuffix(got.URL, "/mattpocock/skills/grill-me") {
		t.Fatalf("result = %#v", got)
	}
}

func TestSearchRegistrySkillsSkipsShortQueries(t *testing.T) {
	service := registryTestService(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no request expected, got %s", r.URL)
	})
	results, err := service.SearchRegistrySkills(" a ")
	if err != nil || len(results) != 0 {
		t.Fatalf("SearchRegistrySkills() = %v, %v; want empty", results, err)
	}
}

func TestSearchRegistrySkillsReportsAPIErrors(t *testing.T) {
	service := registryTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":"Rate limit exceeded"}`))
	})
	if _, err := service.SearchRegistrySkills("react"); err == nil || !strings.Contains(err.Error(), "Rate limit exceeded") {
		t.Fatalf("SearchRegistrySkills() error = %v, want the API message", err)
	}
}

func TestInstallRegistrySkillRejectsUnsafeIdentifiers(t *testing.T) {
	service := newTestService(t.TempDir())
	for _, test := range []struct{ source, skill string }{
		{"../etc", "x"},
		{"owner/repo/extra", "x"},
		{"owner/repo", "x --skill y"},
		{"https://evil.example/owner/repo", "x"},
	} {
		if _, err := service.InstallRegistrySkill(test.source, test.skill, "global"); err == nil {
			t.Errorf("InstallRegistrySkill(%q, %q) succeeded, want refusal", test.source, test.skill)
		}
	}
}

func TestSearchRegistrySkillsCapsResponseSize(t *testing.T) {
	service := registryTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"skills":[{"id":"x","source":"o/r","skillId":"s","name":"` + strings.Repeat("a", registryMaxResponseBytes) + `"}]}`))
	})
	if _, err := service.SearchRegistrySkills("react"); err == nil {
		t.Fatalf("SearchRegistrySkills() accepted a response over the size limit")
	}
}
