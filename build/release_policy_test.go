package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvaluateReleasePolicy(t *testing.T) {
	t.Parallel()
	longPrerelease := "v2.1.0-" + strings.Repeat("a", maxDockerTagLength)
	tests := []struct {
		name           string
		current        string
		tags           []string
		wantPrerelease bool
		wantFloating   bool
		wantHighest    string
		wantErr        bool
	}{
		{name: "latest stable advances floating tags", current: "v2.1.0", tags: []string{"v1.9.0", "v2.0.9", "v2.1.0-rc.1", "not-semver"}, wantFloating: true, wantHighest: "v2.0.9"},
		{name: "older stable cannot roll floating tags back", current: "v2.0.9", tags: []string{"v2.0.9", "v2.1.0", "v3.0.0-rc.1"}, wantHighest: "v2.1.0"},
		{name: "prerelease only publishes fixed tags", current: "v3.0.0-rc.1", tags: []string{"v2.9.0", "v3.0.0-rc.1"}, wantPrerelease: true, wantHighest: "v2.9.0"},
		{name: "newer prerelease does not block stable floating tags", current: "v2.1.0", tags: []string{"v2.0.9", "v3.0.0-rc.1"}, wantFloating: true, wantHighest: "v2.0.9"},
		{name: "no repository tags allows first stable", current: "v1.0.0", wantFloating: true},
		{name: "current tag in repository tags may publish floating", current: "v2.1.0", tags: []string{"v2.0.9", "v2.1.0"}, wantFloating: true, wantHighest: "v2.1.0"},
		{name: "empty tag is rejected", wantErr: true},
		{name: "build metadata is rejected", current: "v2.1.0+build.7", tags: []string{"v2.0.9"}, wantErr: true},
		{name: "docker tag longer than limit is rejected", current: longPrerelease, wantErr: true},
		{name: "invalid docker tag character is rejected", current: "v2.1.0/amd64", wantErr: true},
		{name: "short version is rejected", current: "v2.1", wantErr: true},
		{name: "numeric prerelease with leading zero is rejected", current: "v2.1.0-01", wantErr: true},
		{name: "empty prerelease identifier is rejected", current: "v2.1.0-alpha..1", wantErr: true},
		{name: "missing v prefix is rejected", current: "2.1.0", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := evaluateReleasePolicy(test.current, test.tags)
			if test.wantErr {
				if err == nil {
					t.Fatal("evaluateReleasePolicy() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("evaluateReleasePolicy() error = %v", err)
			}
			if got.Prerelease != test.wantPrerelease {
				t.Errorf("Prerelease = %t, want %t", got.Prerelease, test.wantPrerelease)
			}
			if got.PublishFloating != test.wantFloating {
				t.Errorf("PublishFloating = %t, want %t", got.PublishFloating, test.wantFloating)
			}
			if got.HighestStable != test.wantHighest {
				t.Errorf("HighestStable = %q, want %q", got.HighestStable, test.wantHighest)
			}
		})
	}
}
func TestWriteReleasePolicy(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "github-output")
	t.Setenv(releasePolicyOutputEnv, outputPath)
	t.Setenv("GITHUB_OUTPUT", "")
	result := releasePolicyResult{CurrentTag: "v2.1.0", HighestStable: "v2.0.9", PublishFloating: true}
	if err := writeReleasePolicy(result); err != nil {
		t.Fatalf("writeReleasePolicy() error = %v", err)
	}
	output, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read policy output: %v", err)
	}
	for _, expected := range []string{"release-tag=v2.1.0", "prerelease=false", "publish-floating=true", "highest-stable=v2.0.9"} {
		if !strings.Contains(string(output), expected+"\n") {
			t.Errorf("output %q does not contain %q", output, expected)
		}
	}
}
