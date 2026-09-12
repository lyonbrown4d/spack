package pipeline

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/observabilityx"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/samber/oops"
)

type availableImageStage struct {
	*imageStage
}

func (availableImageStage) ValidateWarmup() error {
	return nil
}

type partialImageArtifactStore struct {
	root      string
	writeErr  error
	writes    int
	firstPath string
}

func (s *partialImageArtifactStore) Root() string {
	return s.root
}

func (s *partialImageArtifactStore) PathFor(assetPath, sourceHash, namespace, suffix string) (string, error) {
	return filepath.Join(s.root, namespace, sourceHash, filepath.Clean(assetPath)+suffix), nil
}

func (s *partialImageArtifactStore) Write(path string, data []byte) error {
	s.writes++
	if s.writes > 1 {
		return oops.Wrapf(s.writeErr, "write partial image artifact")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return oops.Wrapf(err, "create partial image artifact directory")
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return oops.Wrapf(err, "write partial image artifact")
	}
	s.firstPath = path
	return nil
}

type imageOnlyLifecycleFixture struct {
	svc      *Service
	store    *partialImageArtifactStore
	catalog  catalog.Catalog
	asset    *catalog.Asset
	cacheDir string
	writeErr error
}

func newImageOnlyLifecycleFixture(t *testing.T) *imageOnlyLifecycleFixture {
	t.Helper()
	root := t.TempDir()
	cacheDir := filepath.Join(root, "cache")
	asset := imageBatchAssetForTest(t, root, 10_000)
	cat := catalog.NewInMemoryCatalog()
	upsertImageBatchAssetForTest(t, cat, asset)
	writeErr := errors.New("second image artifact write failed")
	store := &partialImageArtifactStore{root: cacheDir, writeErr: writeErr}
	imageCfg := &config.Image{
		Enable:            true,
		Widths:            "640,320",
		Formats:           "jpeg",
		JPEGQuality:       70,
		MaxOutputVariants: 2,
	}
	stage := availableImageStage{imageStage: newImageStage(
		imageCfg,
		&recordingImageEngine{results: cxlist.NewList(
			imageBatchResultForTest(640, []byte("first-image-artifact")),
			imageBatchResultForTest(320, []byte("second-image-artifact")),
		)},
		store,
		cat,
		nil,
	)}
	cfg := &config.Compression{
		Enable:       false,
		Mode:         config.CompressionModeOff,
		CacheDir:     cacheDir,
		CleanupEvery: "1h",
		ImageMaxAge:  "1h",
	}
	return &imageOnlyLifecycleFixture{
		svc: newServiceState(
			cfg,
			slog.New(slog.DiscardHandler),
			cat,
			serviceDeps{
				stages: cxlist.NewList[Stage](stage),
				obs:    observabilityx.Nop(),
			},
			0,
		),
		store:    store,
		catalog:  cat,
		asset:    asset,
		cacheDir: cacheDir,
		writeErr: writeErr,
	}
}

func (f *imageOnlyLifecycleFixture) start(t *testing.T) {
	t.Helper()
	if err := f.svc.start(t.Context(), 1, 0); err != nil {
		t.Fatalf("start image-only pipeline: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := f.svc.stop(stopCtx); err != nil {
			t.Errorf("stop image-only pipeline: %v", err)
		}
	})
	if f.svc.cleanup == nil {
		t.Fatal("expected image-only pipeline cleanup lifecycle to start")
	}
	if info, err := os.Stat(f.cacheDir); err != nil || !info.IsDir() {
		t.Fatalf("expected image-only cache directory to be initialized: %v", err)
	}
}

func (f *imageOnlyLifecycleFixture) stopBackgroundCleanup(t *testing.T) {
	t.Helper()
	stopCtx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := f.svc.stopCleanup(stopCtx); err != nil {
		t.Fatalf("stop background cleanup: %v", err)
	}
}
func (f *imageOnlyLifecycleFixture) requirePartialWarmFailure(t *testing.T) {
	t.Helper()
	if err := f.svc.Warm(t.Context()); !errors.Is(err, f.writeErr) {
		t.Fatalf("expected partial image write failure, got %v", err)
	}
	if f.store.firstPath == "" {
		t.Fatal("expected first image artifact to be written")
	}
	if variants := f.catalog.ListVariants(f.asset.Path); !variants.IsEmpty() {
		t.Fatalf("expected partial batch artifacts to remain outside catalog, got %v", variants.Values())
	}
}

func (f *imageOnlyLifecycleFixture) ageOrphan(t *testing.T) {
	t.Helper()
	oldTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(f.store.firstPath, oldTime, oldTime); err != nil {
		t.Fatalf("age orphaned image artifact: %v", err)
	}
}

func (f *imageOnlyLifecycleFixture) requireOrphanCleanup(t *testing.T) {
	t.Helper()
	result := f.svc.cleanupArtifacts(t.Context(), time.Now())
	if result.removed != 1 {
		t.Fatalf("expected one orphaned image artifact removed, got %d", result.removed)
	}
	if _, err := os.Stat(f.store.firstPath); !os.IsNotExist(err) {
		t.Fatalf("expected orphaned image artifact to be removed, got %v", err)
	}
}

func TestImageOnlyLifecycleCleansOrphanedPartialBatchArtifact(t *testing.T) {
	fixture := newImageOnlyLifecycleFixture(t)
	fixture.start(t)
	fixture.stopBackgroundCleanup(t)
	fixture.requirePartialWarmFailure(t)
	fixture.ageOrphan(t)
	fixture.requireOrphanCleanup(t)
}

func TestImageOnlyLifecycleDoesNotStartLazyCompressionWorkers(t *testing.T) {
	fixture := newImageOnlyLifecycleFixture(t)
	fixture.svc.cfg.Enable = false
	fixture.svc.cfg.Mode = config.CompressionModeLazy
	fixture.start(t)

	if fixture.svc.lazyWorkerPool != nil {
		t.Fatal("expected image-only lifecycle not to start lazy compression workers")
	}
	if fixture.svc.cleanup == nil {
		t.Fatal("expected image-only lifecycle to retain cleanup")
	}
}
