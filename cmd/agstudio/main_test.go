package main

import "testing"

func TestCurrentVersionUsesConfiguredVersion(t *testing.T) {
	originalVersion := version
	version = "v0.1.0"
	t.Cleanup(func() { version = originalVersion })

	if got := currentVersion(); got != "v0.1.0" {
		t.Errorf("currentVersion() = %q, want %q", got, "v0.1.0")
	}
}
