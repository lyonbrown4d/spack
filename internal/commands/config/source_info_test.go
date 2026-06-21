package configcmd_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	configcmd "github.com/lyonbrown4d/spack/internal/commands/config"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
)

func TestEffectiveSourceInfoReportsBundleMetadata(t *testing.T) {
	root := t.TempDir()
	assetPath := filepath.Join(root, "index.html")
	writeInspectTestFile(t, assetPath, []byte("<h1>ok</h1>"))
	bundlePath := filepath.Join(t.TempDir(), "app.spack")
	createdAt := time.Unix(1_725_000_111, 0).UTC()
	if _, err := spackbundle.Write(context.Background(), spackbundle.WriteOptions{
		Output: bundlePath,
		Root:   root,
		Now: func() time.Time {
			return createdAt
		},
		Files: []spackbundle.File{
			{Path: "index.html", FullPath: assetPath, Kind: "asset", MediaType: "text/html"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	sourceInfo, err := configcmd.EffectiveSourceInfoForTest(bundlePath, false)
	if err != nil {
		t.Fatal(err)
	}
	assertSourceInfoValue(t, sourceInfo, "type", "bundle")
	bundle := assertSourceInfoMap(t, sourceInfo, "bundle")
	assertSourceInfoValue(t, bundle, "format_version", spackbundle.FormatVersion)
	assertSourceInfoValue(t, bundle, "file_count", 1)
	assertSourceInfoValue(t, bundle, "created_at", createdAt)
}

func TestEffectiveSourceInfoRedactsPaths(t *testing.T) {
	sourceInfo, err := configcmd.EffectiveSourceInfoForTest(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	assertSourceInfoValue(t, sourceInfo, "root", "REDACTED")
	assertSourceInfoValue(t, sourceInfo, "root_resolved", "REDACTED")
}

func assertSourceInfoMap(t *testing.T, sourceInfo map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := sourceInfo[key].(map[string]any)
	if !ok {
		t.Fatalf("expected %s map, got %#v", key, sourceInfo[key])
	}
	return value
}

func assertSourceInfoValue(t *testing.T, sourceInfo map[string]any, key string, want any) {
	t.Helper()
	if got := sourceInfo[key]; got != want {
		t.Fatalf("expected %s %#v, got %#v", key, want, got)
	}
}
