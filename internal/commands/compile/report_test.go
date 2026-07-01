package compilecmd_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/lyonbrown4d/spack/internal/catalog"
	compilecmd "github.com/lyonbrown4d/spack/internal/commands/compile"
	"github.com/lyonbrown4d/spack/internal/compiler"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
)

func TestWriteCompileReportIncludesCatalogAndBundleStats(t *testing.T) {
	root := t.TempDir()
	bundlePath := writeReportTestBundle(t, root)
	cat := reportTestCatalog(t, root)
	cfg := reportTestConfig()
	report := buildReportForTest(t, &cfg, cat, bundlePath)
	assertReportCounts(t, report)
	assertReportFileWritten(t, root, report)
}

func writeReportTestBundle(t *testing.T, root string) string {
	t.Helper()
	bundlePath := filepath.Join(root, "app.spack")
	if err := os.WriteFile(bundlePath, []byte("bundle"), 0o600); err != nil {
		t.Fatal(err)
	}
	return bundlePath
}

func reportTestCatalog(t *testing.T, root string) catalog.Catalog {
	t.Helper()
	cat := catalog.NewCatalog()
	asset := &catalog.Asset{Path: "index.html", FullPath: filepath.Join(root, "index.html"), Size: 12}
	if err := cat.UpsertAsset(asset); err != nil {
		t.Fatal(err)
	}
	variant := &catalog.Variant{
		ID:           "index.html.gz",
		AssetPath:    "index.html",
		ArtifactPath: filepath.Join(root, "index.html.gz"),
		Metadata: cxmapping.NewMapFrom(map[string]string{
			"stage": sourcecatalog.SourceSidecarStage,
		}),
	}
	if err := cat.UpsertVariant(variant); err != nil {
		t.Fatal(err)
	}
	return cat
}

func reportTestConfig() config.Config {
	cfg := config.DefaultConfig()
	cfg.Compression.Enable = true
	cfg.Compression.Mode = config.CompressionModeWarmup
	return cfg
}

func buildReportForTest(t *testing.T, cfg *config.Config, cat catalog.Catalog, bundlePath string) compilecmd.CompileReportForTest {
	t.Helper()
	report, err := compilecmd.BuildCompileReportForTest(compiler.Runtime{Config: cfg, Catalog: cat}, spackbundle.WriteSummary{
		Output: bundlePath,
		Files:  1,
		Bytes:  12,
	}, 1500*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func assertReportCounts(t *testing.T, report compilecmd.CompileReportForTest) {
	t.Helper()
	if report.Assets != 1 || report.Variants != 1 || report.SourceSidecars != 1 {
		t.Fatalf("unexpected catalog counts: %#v", report)
	}
	if report.SourceBytes != 12 || report.BundleBytes != int64(len("bundle")) {
		t.Fatalf("unexpected byte counts: %#v", report)
	}
	if report.DurationMillis != 1500 {
		t.Fatalf("expected duration 1500ms, got %d", report.DurationMillis)
	}
	if !report.CompressionEnabled || report.CompressionMode != config.CompressionModeWarmup {
		t.Fatalf("unexpected compression fields: %#v", report)
	}
}

func assertReportFileWritten(t *testing.T, root string, report compilecmd.CompileReportForTest) {
	t.Helper()
	reportPath := filepath.Join(root, "compile-report.json")
	if err := compilecmd.WriteCompileReportForTest(reportPath, report); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("expected non-empty compile report")
	}
}
