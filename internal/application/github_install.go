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
	"strings"
	"time"

	"agent-studio/internal/domain"
)

// InstallSkillFromURL prefers a shallow Git clone and falls back to a public archive.
func (s *DiscoveryService) InstallSkillFromURL(rawURL, targetScopeID string) (domain.SkillInstallResult, error) {
	owner, repository, branch, subdirectory, host, err := parseRepositoryURL(rawURL)
	if err != nil {
		return domain.SkillInstallResult{}, err
	}
	workspace, err := s.Discover()
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

	sourceRoot, err := findSkillRoot(temporary, subdirectory)
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
	updated, err := s.Discover()
	if err != nil {
		return domain.SkillInstallResult{}, err
	}
	return domain.SkillInstallResult{Workspace: updated, Method: method}, nil
}

func parseRepositoryURL(rawURL string) (owner, repository, branch, subdirectory, host string, err error) {
	parsed, parseErr := url.Parse(strings.TrimSpace(rawURL))
	if parseErr != nil || parsed.Scheme != "https" {
		return "", "", "", "", "", fmt.Errorf("only public HTTPS repository URLs are supported")
	}
	host = strings.ToLower(parsed.Host)
	if host != "github.com" && host != "gitlab.com" && host != "bitbucket.org" {
		return "", "", "", "", "", fmt.Errorf("unsupported public repository host: %s", host)
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", "", "", "", fmt.Errorf("repository URL must include owner and repository")
	}
	owner, repository, branch = parts[0], strings.TrimSuffix(parts[1], ".git"), "main"
	if len(parts) >= 4 && parts[2] == "tree" {
		branch = parts[3]
		if len(parts) > 4 {
			subdirectory = strings.Join(parts[4:], "/")
		}
	}
	if len(parts) >= 4 && parts[2] != "tree" {
		return "", "", "", "", "", fmt.Errorf("unsupported repository URL format")
	}
	return owner, repository, branch, subdirectory, host, nil
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

func findSkillRoot(root, subdirectory string) (string, error) {
	searchRoot := root
	if subdirectory != "" {
		clean := filepath.Clean(filepath.FromSlash(subdirectory))
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("repository skill path is unsafe")
		}
		matches, _ := filepath.Glob(filepath.Join(root, "*", clean))
		if len(matches) != 1 {
			return "", fmt.Errorf("skill directory was not found in repository")
		}
		searchRoot = matches[0]
	}
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
