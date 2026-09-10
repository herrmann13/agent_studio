package application

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"agent-studio/internal/domain"
)

// npxInstallCommandPattern matches the `npx skills add <repo-url> --skill <name>` shorthand.
// The name after --skill is a registry slug, not necessarily the repository folder name
// (for example vercel-labs/agent-skills lists "vercel-react-best-practices" for the folder
// skills/react-best-practices), so it is resolved against the downloaded repository rather
// than assumed to be an exact path.
var npxInstallCommandPattern = regexp.MustCompile(`(?i)^npx\s+skills\s+add\s+(\S+)\s+--skill(?:=|\s+)(\S+)$`)

// splitInstallCommand extracts the repository URL and requested skill slug from an
// `npx skills add` command. It returns ok=false for a plain URL, which parseRepositoryURL
// then parses unchanged.
func splitInstallCommand(raw string) (repositoryURL, skillHint string, ok bool) {
	match := npxInstallCommandPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if match == nil {
		return "", "", false
	}
	repositoryURL = strings.Trim(strings.TrimSuffix(strings.TrimSpace(match[1]), "/"), `"'`)
	skillHint = strings.Trim(match[2], `"'`)
	return repositoryURL, skillHint, true
}

// InstallSkillFromURL prefers a shallow Git clone and falls back to a public archive.
func (s *DiscoveryService) InstallSkillFromURL(rawURL, targetScopeID string) (domain.SkillInstallResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	owner, repository, branch, subdirectory, host, skillHint, err := parseRepositoryURL(rawURL)
	if err != nil {
		return domain.SkillInstallResult{}, err
	}
	workspace, err := s.discover()
	if err != nil {
		return domain.SkillInstallResult{}, err
	}
	target, err := scopeByID(workspace.Scopes, targetScopeID)
	if err != nil {
		return domain.SkillInstallResult{}, err
	}
	if err := os.MkdirAll(filepath.Join(s.home, ".agent-studio"), 0o755); err != nil {
		return domain.SkillInstallResult{}, err
	}
	temporary, err := os.MkdirTemp(filepath.Join(s.home, ".agent-studio"), "skill-download-")
	if err != nil {
		return domain.SkillInstallResult{}, fmt.Errorf("create download directory: %w", err)
	}
	defer os.RemoveAll(temporary)

	method := "ZIP"
	if _, gitErr := exec.LookPath("git"); gitErr == nil && host != "" {
		if err := cloneRepository(temporary, host, owner, repository, branch); err == nil {
			method = "Git"
		} else if archiveErr := downloadRepositoryArchive(temporary, host, owner, repository, branch); archiveErr != nil {
			return domain.SkillInstallResult{}, fmt.Errorf("Git failed (%v); ZIP fallback failed (%w)", err, archiveErr)
		}
	} else if err := downloadRepositoryArchive(temporary, host, owner, repository, branch); err != nil {
		return domain.SkillInstallResult{}, err
	}

	sourceRoot, err := findSkillRoot(temporary, subdirectory, skillHint)
	if err != nil {
		return domain.SkillInstallResult{}, err
	}
	skillName := filepath.Base(sourceRoot)
	destination := filepath.Join(target.Root, skillName)
	if _, err := os.Stat(destination); err == nil {
		return domain.SkillInstallResult{}, fmt.Errorf("destination already contains %q", skillName)
	} else if !os.IsNotExist(err) {
		return domain.SkillInstallResult{}, err
	}
	if err := os.MkdirAll(target.Root, 0o755); err != nil {
		return domain.SkillInstallResult{}, fmt.Errorf("create skill directory: %w", err)
	}
	if err := copyDirectory(sourceRoot, destination); err != nil {
		return domain.SkillInstallResult{}, fmt.Errorf("install skill: %w", err)
	}
	if err := s.propagateSkillCopy(target, destination); err != nil {
		return domain.SkillInstallResult{}, err
	}
	updated, err := s.discover()
	if err != nil {
		return domain.SkillInstallResult{}, err
	}
	return domain.SkillInstallResult{Workspace: updated, Method: method}, nil
}

func parseRepositoryURL(rawURL string) (owner, repository, branch, subdirectory, host, skillHint string, err error) {
	input := strings.TrimSpace(rawURL)
	if repositoryURL, hint, ok := splitInstallCommand(input); ok {
		input, skillHint = repositoryURL, hint
	}
	parsed, parseErr := url.Parse(input)
	if parseErr != nil || parsed.Scheme != "https" {
		return "", "", "", "", "", "", fmt.Errorf("only public HTTPS repository URLs are supported")
	}
	host = strings.ToLower(parsed.Host)
	if host != "github.com" && host != "gitlab.com" && host != "bitbucket.org" {
		return "", "", "", "", "", "", fmt.Errorf("unsupported public repository host: %s", host)
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", "", "", "", "", fmt.Errorf("repository URL must include owner and repository")
	}
	owner, repository, branch = parts[0], strings.TrimSuffix(parts[1], ".git"), "main"
	if len(parts) >= 4 && parts[2] == "tree" {
		branch = parts[3]
		if len(parts) > 4 {
			subdirectory = strings.Join(parts[4:], "/")
		}
	}
	if len(parts) >= 4 && parts[2] != "tree" {
		return "", "", "", "", "", "", fmt.Errorf("unsupported repository URL format")
	}
	if subdirectory != "" {
		skillHint = ""
	}
	return owner, repository, branch, subdirectory, host, skillHint, nil
}

func cloneRepository(destination, host, owner, repository, branch string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	repositoryURL := fmt.Sprintf("https://%s/%s/%s.git", host, owner, repository)
	command := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "--no-tags", "--branch", branch, repositoryURL, destination)
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if output, err := command.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("git clone timed out")
		}
		return fmt.Errorf("git clone: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func downloadRepositoryArchive(destination, host, owner, repository, branch string) error {
	var archiveURL string
	switch host {
	case "github.com":
		archiveURL = fmt.Sprintf("https://codeload.github.com/%s/%s/zip/refs/heads/%s", owner, repository, url.PathEscape(branch))
	case "gitlab.com":
		archiveURL = fmt.Sprintf("https://gitlab.com/%s/%s/-/archive/%s/%s-%s.zip", owner, repository, url.PathEscape(branch), repository, url.PathEscape(branch))
	case "bitbucket.org":
		archiveURL = fmt.Sprintf("https://bitbucket.org/%s/%s/get/%s.zip", owner, repository, url.PathEscape(branch))
	default:
		return fmt.Errorf("no ZIP fallback for host %s", host)
	}
	request, err := http.NewRequest(http.MethodGet, archiveURL, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/zip")
	client := &http.Client{Timeout: 2 * time.Minute}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("download repository ZIP: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("repository ZIP returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 200<<20))
	if err != nil {
		return fmt.Errorf("read repository ZIP: %w", err)
	}
	return extractPublicArchive(data, destination)
}

func extractPublicArchive(data []byte, destination string) error {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("read repository ZIP: %w", err)
	}
	for _, entry := range archive.File {
		relative := filepath.Clean(filepath.FromSlash(entry.Name))
		if filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("archive contains unsafe path: %q", entry.Name)
		}
		target := filepath.Join(destination, relative)
		if entry.FileInfo().Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("archive contains unsupported symlink: %q", entry.Name)
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		input, err := entry.Open()
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		input.Close()
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

// findSkillRoot locates the downloaded skill directory. With an explicit subdirectory (from a
// /tree/<branch>/<path> URL) it resolves that path directly. With an npx `--skill <hint>` slug and
// no explicit path, the hint is a registry name that does not always match the repository's folder
// name (see splitInstallCommand), so it first tries the conventional skills/<hint> layout and falls
// back to searching the whole repository for a skill whose SKILL.md name or folder matches the hint.
func findSkillRoot(root, subdirectory, skillHint string) (string, error) {
	if subdirectory != "" {
		searchRoot, err := resolveSubdirectory(root, subdirectory)
		if err != nil {
			return "", err
		}
		return singleSkillIn(searchRoot)
	}
	if skillHint != "" {
		if conventional, err := resolveSubdirectory(root, "skills/"+skillHint); err == nil {
			if match, matchErr := singleSkillIn(conventional); matchErr == nil {
				return match, nil
			}
		}
		if match, ok := findSkillBySlug(root, skillHint); ok {
			return match, nil
		}
		return "", fmt.Errorf("could not find a skill named %q in this repository; open the repository and copy the exact skill folder URL instead", skillHint)
	}
	return singleSkillIn(root)
}

// resolveSubdirectory finds subdirectory under root, accounting for the two possible downloaded
// layouts: git checks out directly into root, while archive downloads nest a single repository
// directory (for example, agents-main).
func resolveSubdirectory(root, subdirectory string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(subdirectory))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("repository skill path is unsafe")
	}

	candidates := []string{filepath.Join(root, clean)}
	entries, readErr := os.ReadDir(root)
	if readErr != nil {
		return "", fmt.Errorf("inspect downloaded repository: %w", readErr)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			candidates = append(candidates, filepath.Join(root, entry.Name(), clean))
		}
	}

	matches := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		info, statErr := os.Stat(candidate)
		if statErr == nil && info.IsDir() {
			matches = append(matches, candidate)
		}
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("skill directory was not found in repository")
	}
	return matches[0], nil
}

// singleSkillIn walks searchRoot and returns the directory of the one SKILL.md found there.
func singleSkillIn(searchRoot string) (string, error) {
	var roots []string
	err := filepath.WalkDir(searchRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && entry.Name() == "SKILL.md" {
			roots = append(roots, filepath.Dir(path))
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(roots) != 1 {
		return "", fmt.Errorf("URL must identify exactly one skill containing SKILL.md")
	}
	return roots[0], nil
}

// findSkillBySlug searches the whole downloaded repository for a skill matching hint, trying
// progressively looser tiers (exact SKILL.md name, exact folder name, then hint carrying an extra
// prefix such as an owner or product name, e.g. "vercel-react-best-practices" for a
// react-best-practices folder). It only returns a match when exactly one candidate satisfies a tier.
func findSkillBySlug(root, hint string) (string, bool) {
	type candidate struct {
		dir, dirName, skillName string
	}
	var candidates []candidate
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if entry.IsDir() || entry.Name() != "SKILL.md" {
			return nil
		}
		dir := filepath.Dir(path)
		name := ""
		if skill, err := parseSkill(path); err == nil {
			name = skill.Name
		}
		candidates = append(candidates, candidate{dir: dir, dirName: filepath.Base(dir), skillName: name})
		return nil
	})

	normalizedHint := normalizeSkillSlug(hint)
	tiers := []func(candidate) bool{
		func(c candidate) bool { return normalizeSkillSlug(c.skillName) == normalizedHint },
		func(c candidate) bool { return normalizeSkillSlug(c.dirName) == normalizedHint },
		func(c candidate) bool {
			normalizedDir, normalizedName := normalizeSkillSlug(c.dirName), normalizeSkillSlug(c.skillName)
			return (normalizedDir != "" && strings.HasSuffix(normalizedHint, "-"+normalizedDir)) ||
				(normalizedName != "" && strings.HasSuffix(normalizedHint, "-"+normalizedName))
		},
	}
	for _, matches := range tiers {
		var found []candidate
		for _, c := range candidates {
			if matches(c) {
				found = append(found, c)
			}
		}
		if len(found) == 1 {
			return found[0].dir, true
		}
	}
	return "", false
}

// normalizeSkillSlug lower-cases value and collapses any run of non-alphanumeric characters into a
// single hyphen, so names like "React Best Practices" and "react-best-practices" compare equal.
func normalizeSkillSlug(value string) string {
	var builder strings.Builder
	lastWasDash := true // suppresses a leading hyphen
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastWasDash = false
			continue
		}
		if !lastWasDash {
			builder.WriteByte('-')
			lastWasDash = true
		}
	}
	return strings.TrimRight(builder.String(), "-")
}
