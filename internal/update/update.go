package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	githubAPIURL  = "https://api.github.com/repos/herrmann13/agent_studio/releases/latest"
	updateTimeout = 20 * time.Second
)

type Release struct {
	TagName string  `json:"tag_name"`
	HTMLURL string  `json:"html_url"`
	Body    string  `json:"body"`
	Assets  []Asset `json:"assets"`
}

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// Info is the update state presented to the desktop interface.
type Info struct {
	CurrentVersion  string `json:"currentVersion"`
	LatestVersion   string `json:"latestVersion"`
	ReleaseNotes    string `json:"releaseNotes"`
	ReleaseURL      string `json:"releaseURL"`
	UpdateAvailable bool   `json:"updateAvailable"`
}

type semanticVersion struct {
	major      int
	minor      int
	patch      int
	prerelease string
}

var semanticVersionPattern = regexp.MustCompile(`^v?([0-9]+)\.([0-9]+)\.([0-9]+)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)

func Check(ctx context.Context, currentVersion string) (Info, error) {
	current, err := parseSemanticVersion(currentVersion)
	if err != nil {
		return Info{}, fmt.Errorf("invalid installed version %q: %w", currentVersion, err)
	}
	release, err := fetchLatestRelease(ctx, currentVersion)
	if err != nil {
		return Info{}, err
	}
	latest, err := parseSemanticVersion(release.TagName)
	if err != nil {
		return Info{}, fmt.Errorf("GitHub release has an invalid version: %w", err)
	}
	return Info{
		CurrentVersion:  currentVersion,
		LatestVersion:   release.TagName,
		ReleaseNotes:    release.Body,
		ReleaseURL:      release.HTMLURL,
		UpdateAvailable: latest.newerThan(current),
	}, nil
}

// DownloadAndInstall fetches the requested latest release, validates its checksum, and starts its platform installer.
func DownloadAndInstall(ctx context.Context, currentVersion, tagName string) error {
	release, err := fetchLatestRelease(ctx, currentVersion)
	if err != nil {
		return err
	}
	if release.TagName != tagName {
		return fmt.Errorf("the available release changed; check for updates again")
	}
	asset, err := assetForPlatform(release, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	path, err := downloadAsset(ctx, currentVersion, asset)
	if err != nil {
		return err
	}
	keepInstaller := false
	defer func() {
		if !keepInstaller {
			_ = os.Remove(path)
		}
	}()
	if err := verifyAssetChecksum(ctx, currentVersion, release, asset, path); err != nil {
		return err
	}
	if err := install(path); err != nil {
		return err
	}
	// The background macOS installer mounts the DMG after this process exits.
	keepInstaller = runtime.GOOS == "darwin"
	return nil
}

func fetchLatestRelease(ctx context.Context, currentVersion string) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubAPIURL, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Agent-Studio/"+currentVersion)
	resp, err := (&http.Client{Timeout: updateTimeout}).Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("could not check for updates: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("GitHub returned %s while checking for updates", resp.Status)
	}
	var release Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2*1024*1024)).Decode(&release); err != nil {
		return Release{}, fmt.Errorf("invalid update response: %w", err)
	}
	return release, nil
}

func parseSemanticVersion(raw string) (semanticVersion, error) {
	raw = strings.TrimPrefix(strings.TrimSpace(raw), "v")
	parts := semanticVersionPattern.FindStringSubmatch(raw)
	if parts == nil {
		return semanticVersion{}, fmt.Errorf("%q is not a semantic version", raw)
	}
	major, err := strconv.Atoi(parts[1])
	if err != nil {
		return semanticVersion{}, err
	}
	minor, err := strconv.Atoi(parts[2])
	if err != nil {
		return semanticVersion{}, err
	}
	patch, err := strconv.Atoi(parts[3])
	if err != nil {
		return semanticVersion{}, err
	}
	return semanticVersion{major: major, minor: minor, patch: patch, prerelease: parts[4]}, nil
}

func (v semanticVersion) newerThan(other semanticVersion) bool {
	if v.major != other.major {
		return v.major > other.major
	}
	if v.minor != other.minor {
		return v.minor > other.minor
	}
	if v.patch != other.patch {
		return v.patch > other.patch
	}
	if v.prerelease == "" {
		return other.prerelease != ""
	}
	if other.prerelease == "" {
		return false
	}
	return comparePrerelease(v.prerelease, other.prerelease) > 0
}

func comparePrerelease(left, right string) int {
	leftParts := strings.Split(left, ".")
	rightParts := strings.Split(right, ".")
	for index := 0; index < len(leftParts) && index < len(rightParts); index++ {
		if leftParts[index] == rightParts[index] {
			continue
		}
		leftNumber, leftIsNumber := prereleaseNumber(leftParts[index])
		rightNumber, rightIsNumber := prereleaseNumber(rightParts[index])
		switch {
		case leftIsNumber && rightIsNumber:
			if leftNumber < rightNumber {
				return -1
			}
			return 1
		case leftIsNumber:
			return -1
		case rightIsNumber:
			return 1
		case leftParts[index] < rightParts[index]:
			return -1
		default:
			return 1
		}
	}
	if len(leftParts) < len(rightParts) {
		return -1
	}
	if len(leftParts) > len(rightParts) {
		return 1
	}
	return 0
}

func prereleaseNumber(value string) (int, bool) {
	if value == "" {
		return 0, false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return 0, false
		}
	}
	number, err := strconv.Atoi(value)
	return number, err == nil
}

func assetForPlatform(release Release, goos, goarch string) (Asset, error) {
	var suffix string
	switch goos {
	case "darwin":
		suffix = "-macos-" + goarch + ".dmg"
	case "linux":
		suffix = "-linux-" + goarch + ".deb"
	default:
		return Asset{}, fmt.Errorf("updates are not supported on %s", goos)
	}
	for _, asset := range release.Assets {
		if strings.HasSuffix(asset.Name, suffix) {
			return asset, nil
		}
	}
	return Asset{}, fmt.Errorf("the release has no installer for %s/%s", goos, goarch)
}

func downloadAsset(ctx context.Context, currentVersion string, asset Asset) (string, error) {
	path, err := download(ctx, currentVersion, asset, "agent-studio-update-*")
	if err != nil {
		return "", fmt.Errorf("could not download %s: %w", asset.Name, err)
	}
	return path, nil
}

func verifyAssetChecksum(ctx context.Context, currentVersion string, release Release, asset Asset, path string) error {
	var checksumAsset Asset
	for _, candidate := range release.Assets {
		if candidate.Name == "SHA256SUMS" {
			checksumAsset = candidate
			break
		}
	}
	if checksumAsset.Name == "" {
		return fmt.Errorf("the release has no SHA256SUMS file")
	}
	checksumPath, err := download(ctx, currentVersion, checksumAsset, "agent-studio-checksums-*")
	if err != nil {
		return fmt.Errorf("could not download checksums: %w", err)
	}
	defer os.Remove(checksumPath)
	checksums, err := os.ReadFile(checksumPath)
	if err != nil {
		return err
	}
	expected, err := checksumForAsset(string(checksums), asset.Name)
	if err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	if !strings.EqualFold(expected, hex.EncodeToString(hash.Sum(nil))) {
		return fmt.Errorf("checksum verification failed for %s", asset.Name)
	}
	return nil
}

func download(ctx context.Context, currentVersion string, asset Asset, pattern string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.BrowserDownloadURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Agent-Studio/"+currentVersion)
	resp, err := (&http.Client{Timeout: 5 * updateTimeout}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub returned %s", resp.Status)
	}
	file, err := os.CreateTemp("", pattern+filepath.Ext(asset.Name))
	if err != nil {
		return "", err
	}
	path := file.Name()
	if _, err := io.Copy(file, io.LimitReader(resp.Body, 1024*1024*1024)); err != nil {
		file.Close()
		os.Remove(path)
		return "", err
	}
	if err := file.Close(); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

func checksumForAsset(content, name string) (string, error) {
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[1] != name {
			continue
		}
		if len(fields[0]) == sha256.Size*2 {
			if _, err := hex.DecodeString(fields[0]); err == nil {
				return fields[0], nil
			}
		}
	}
	return "", fmt.Errorf("checksum not found for %s", name)
}
