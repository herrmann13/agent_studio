package update

import "testing"

func TestSemanticVersionComparison(t *testing.T) {
	current, err := parseSemanticVersion("v0.1.9")
	if err != nil {
		t.Fatal(err)
	}
	latest, err := parseSemanticVersion("0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if !latest.newerThan(current) || current.newerThan(latest) {
		t.Fatal("incorrect semantic version comparison")
	}
	prerelease, err := parseSemanticVersion("v0.2.0-rc.2")
	if err != nil {
		t.Fatal(err)
	}
	previousPrerelease, err := parseSemanticVersion("v0.2.0-rc.1")
	if err != nil {
		t.Fatal(err)
	}
	stable, err := parseSemanticVersion("v0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if !prerelease.newerThan(previousPrerelease) || !stable.newerThan(prerelease) || prerelease.newerThan(stable) {
		t.Fatal("incorrect prerelease comparison")
	}
	for _, value := range []string{"1", "1.2", "1.2.3-rc..1", "1.a.3"} {
		if _, err := parseSemanticVersion(value); err == nil {
			t.Fatalf("accepted invalid version %q", value)
		}
	}
}

func TestAssetForPlatform(t *testing.T) {
	release := Release{Assets: []Asset{
		{Name: "agent-studio-v0.2.0-linux-amd64.deb"},
		{Name: "agent-studio-v0.2.0-macos-arm64.dmg"},
	}}
	asset, err := assetForPlatform(release, "linux", "amd64")
	if err != nil || asset.Name != "agent-studio-v0.2.0-linux-amd64.deb" {
		t.Fatalf("incorrect Linux asset: %#v, %v", asset, err)
	}
	asset, err = assetForPlatform(release, "darwin", "arm64")
	if err != nil || asset.Name != "agent-studio-v0.2.0-macos-arm64.dmg" {
		t.Fatalf("incorrect macOS asset: %#v, %v", asset, err)
	}
	if _, err := assetForPlatform(release, "darwin", "amd64"); err == nil {
		t.Fatal("accepted missing asset")
	}
}

func TestChecksumForAsset(t *testing.T) {
	checksum := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	got, err := checksumForAsset(checksum+"  agent-studio-v0.2.0-linux-amd64.deb\n", "agent-studio-v0.2.0-linux-amd64.deb")
	if err != nil || got != checksum {
		t.Fatalf("incorrect checksum: %q, %v", got, err)
	}
	if _, err := checksumForAsset("", "missing.deb"); err == nil {
		t.Fatal("accepted missing checksum")
	}
}
