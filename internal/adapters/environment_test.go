package adapters

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestFindCommandsSearchesNvmBinDirectories(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("nvm layout is Unix-only")
	}
	t.Setenv("PATH", "")
	home := t.TempDir()
	binary := filepath.Join(home, ".nvm", "versions", "node", "v22.22.1", "bin", "codex")
	if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// System directories such as /opt/homebrew/bin may add the developer's own copy
	// after it; the home-relative nvm directory must come first.
	if got := findCommands(home, []string{"codex"}); len(got) == 0 || got[0] != binary {
		t.Fatalf("findCommands() = %v, want %s first", got, binary)
	}
}

func TestCommandVersionRetriesFailuresAfterTTL(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script")
	}
	directory := t.TempDir()
	binary, counter := filepath.Join(directory, "tool"), filepath.Join(directory, "runs")
	script := "#!/bin/sh\necho run >> " + counter + "\nexit 1\n"
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	clock := time.Now()
	now = func() time.Time { return clock }
	t.Cleanup(func() { now = time.Now })
	runs := func() int {
		content, _ := os.ReadFile(counter)
		return strings.Count(string(content), "run")
	}

	cachedCommandVersion(binary)
	cachedCommandVersion(binary)
	if got := runs(); got != 1 {
		t.Fatalf("--version ran %d times within the TTL, want 1", got)
	}
	clock = clock.Add(versionFailureTTL + time.Second)
	cachedCommandVersion(binary)
	if got := runs(); got != 2 {
		t.Fatalf("--version ran %d times after the TTL, want 2", got)
	}
}

func TestNvmBinsAreOrderedByNumericVersion(t *testing.T) {
	bins := []string{"/n/v9.11.2/bin", "/n/v20.10.0/bin", "/n/v18.20.8/bin", "/n/v20.9.1/bin"}
	sortNewestNodeFirst(bins)
	want := []string{"/n/v20.10.0/bin", "/n/v20.9.1/bin", "/n/v18.20.8/bin", "/n/v9.11.2/bin"}
	for index := range want {
		if bins[index] != want[index] {
			t.Fatalf("order = %v, want %v", bins, want)
		}
	}
}

func TestCommandVersionFindsNodeNextToShim(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses shell scripts")
	}
	t.Setenv("PATH", "/usr/bin:/bin")
	bin := t.TempDir()
	// A fake `node` beside the shim, as in an nvm bin directory.
	if err := os.WriteFile(filepath.Join(bin, "fakenode"), []byte("#!/bin/sh\necho 1.18.20\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(bin, "opencode")
	if err := os.WriteFile(shim, []byte("#!/usr/bin/env fakenode\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := cachedCommandVersion(shim); got != "1.18.20\n" {
		t.Fatalf("cachedCommandVersion() = %q, want the version printed via the sibling interpreter", got)
	}
}
