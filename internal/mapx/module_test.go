package mapx_test

import (
	"testing"

	"github.com/lyonbrown4d/spack/internal/mapx"
)

type mapperTestReport struct {
	AssetCount int    `json:"asset_count"`
	SourceType string `json:"source_type"`
}

func TestNewUsesJSONFallbackTags(t *testing.T) {
	instance := mapx.New()

	var report mapperTestReport
	if err := instance.MapInto(&report, map[string]any{
		"asset_count": 3,
		"source_type": "directory",
	}); err != nil {
		t.Fatal(err)
	}

	if report.AssetCount != 3 {
		t.Fatalf("expected asset_count 3, got %d", report.AssetCount)
	}
	if report.SourceType != "directory" {
		t.Fatalf("expected source_type directory, got %q", report.SourceType)
	}
}
