package adapters

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"sync"
	"time"
)

// Environment is how adapters observe the machine. It is injectable so discovery
// can be tested without depending on which agents the developer has installed.
type Environment struct {
	Home string
	// FindCommands returns every distinct executable found for the given names.
	FindCommands func(names ...string) []string
	// CommandVersion returns the raw `--version` output of an executable, or "".
	CommandVersion func(path string) string
}

// SystemEnvironment inspects the real machine.
func SystemEnvironment(home string) Environment {
	return Environment{
		Home:           home,
		FindCommands:   func(names ...string) []string { return findCommands(home, names) },
		CommandVersion: cachedCommandVersion,
	}
}

// FindCommand returns the first executable found for the given names, or "".
func (e Environment) FindCommand(names ...string) string {
	if e.FindCommands == nil {
		return ""
	}
	if paths := e.FindCommands(names...); len(paths) > 0 {
		return paths[0]
	}
	return ""
}

// commandDirectories lists common install locations searched in addition to PATH.
// A desktop app launched from Finder or a .desktop file does not inherit the login
// shell's PATH, so LookPath alone misses agents installed under the user's home.
func commandDirectories(home string) []string {
	if runtime.GOOS == "windows" {
		return []string{
			filepath.Join(home, ".local", "bin"),
			filepath.Join(home, ".opencode", "bin"),
			filepath.Join(home, ".bun", "bin"),
			filepath.Join(home, "AppData", "Roaming", "npm"),
		}
	}
	return []string{
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, ".opencode", "bin"),
		filepath.Join(home, ".claude", "local"),
		filepath.Join(home, ".bun", "bin"),
		filepath.Join(home, ".npm-global", "bin"),
		filepath.Join(home, ".volta", "bin"),
		filepath.Join(home, "go", "bin"),
		"/opt/homebrew/bin",
		"/usr/local/bin",
		"/usr/bin",
	}
}

func findCommands(home string, names []string) []string {
	var result []string
	seen := map[string]bool{}
	add := func(path string) {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			resolved = path
		}
		if seen[resolved] {
			return
		}
		seen[resolved] = true
		result = append(result, path)
	}
	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			add(path)
		}
		for _, directory := range commandDirectories(home) {
			for _, candidate := range executableNames(name) {
				path := filepath.Join(directory, candidate)
				if isExecutable(path) {
					add(path)
				}
			}
		}
	}
	return result
}

func executableNames(name string) []string {
	if runtime.GOOS == "windows" {
		return []string{name + ".exe", name + ".cmd"}
	}
	return []string{name}
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return runtime.GOOS == "windows" || info.Mode().Perm()&0o111 != 0
}

type versionCacheKey struct {
	path    string
	size    int64
	modTime time.Time
}

var (
	versionCacheMu sync.Mutex
	versionCache   = map[versionCacheKey]string{}
)

// cachedCommandVersion runs `<path> --version` once per binary build. Discovery
// runs after every UI action, so the result is reused until the file changes.
func cachedCommandVersion(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	key := versionCacheKey{path: path, size: info.Size(), modTime: info.ModTime()}
	versionCacheMu.Lock()
	cached, ok := versionCache[key]
	versionCacheMu.Unlock()
	if ok {
		return cached
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, path, "--version").Output()
	version := ""
	if err == nil {
		version = string(output)
	}
	versionCacheMu.Lock()
	versionCache[key] = version
	versionCacheMu.Unlock()
	return version
}

var semverPattern = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

// ParseVersion extracts "X.Y.Z" and its major number from `--version` output.
func ParseVersion(output string) (string, int, bool) {
	match := semverPattern.FindStringSubmatch(output)
	if match == nil {
		return "", 0, false
	}
	major, err := strconv.Atoi(match[1])
	if err != nil {
		return "", 0, false
	}
	return match[0], major, true
}
