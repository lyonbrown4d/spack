package pipeline_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/arcgolabs/eventx"
	"github.com/daiyuang/spack/internal/catalog"
	"github.com/daiyuang/spack/internal/config"
	appEvent "github.com/daiyuang/spack/internal/event"
	"github.com/daiyuang/spack/internal/pipeline"
)

func TestCleanupArtifactsRemovesExpiredVariants(t *testing.T) {
	root := t.TempDir()
	cat := catalog.NewInMemoryCatalog()
	addAssetForCleanupTest(t, cat, "app.js", "application/javascript", "hash-1")

	expiredPath := filepath.Join(root, "encoding", "hash-1", "app.js.br")
	writeArtifactForCleanupTest(t, expiredPath, []byte("expired"))
	setMTimeForCleanupTest(t, expiredPath, time.Now().Add(-2*time.Hour))

	upsertVariantForCleanupTest(t, cat, &catalog.Variant{
		ID:           "app.js|encoding=br",
		AssetPath:    "app.js",
		ArtifactPath: expiredPath,
		MediaType:    "application/javascript",
		SourceHash:   "hash-1",
		ETag:         "\"hash-1-br\"",
		Encoding:     "br",
	})

	svc := pipeline.NewServiceForTest(&config.Compression{
		CacheDir: root,
		MaxAge:   "1h",
	}, slog.New(slog.DiscardHandler), cat, 1)

	if removed := pipeline.CleanupRemovedForTest(svc, time.Now()); removed != 1 {
		t.Fatalf("expected one removed file, got %d", removed)
	}
	assertFileStateForCleanupTest(t, expiredPath, false)
	if cat.ListVariants("app.js").Len() != 0 {
		t.Fatalf("expected variant removed from catalog, got %#v", cat.ListVariants("app.js"))
	}
}

func TestCleanupArtifactsEnforcesMaxCacheBytes(t *testing.T) {
	root := t.TempDir()
	cat := catalog.NewInMemoryCatalog()
	addAssetForCleanupTest(t, cat, "bundle.js", "application/javascript", "hash-2")

	oldPath := filepath.Join(root, "encoding", "hash-2", "bundle.js.br")
	newPath := filepath.Join(root, "encoding", "hash-2", "bundle.js.gz")
	writeArtifactForCleanupTest(t, oldPath, []byte("0123456789abcdef"))
	writeArtifactForCleanupTest(t, newPath, []byte("0123456789abcdef"))
	setMTimeForCleanupTest(t, oldPath, time.Now().Add(-2*time.Hour))
	setMTimeForCleanupTest(t, newPath, time.Now().Add(-1*time.Hour))

	for _, variant := range []catalog.Variant{
		{
			ID:           "bundle.js|encoding=br",
			AssetPath:    "bundle.js",
			ArtifactPath: oldPath,
			MediaType:    "application/javascript",
			SourceHash:   "hash-2",
			ETag:         "\"hash-2-br\"",
			Encoding:     "br",
		},
		{
			ID:           "bundle.js|encoding=gzip",
			AssetPath:    "bundle.js",
			ArtifactPath: newPath,
			MediaType:    "application/javascript",
			SourceHash:   "hash-2",
			ETag:         "\"hash-2-gzip\"",
			Encoding:     "gzip",
		},
	} {
		upsertVariantForCleanupTest(t, cat, &variant)
	}

	svc := pipeline.NewServiceForTest(&config.Compression{
		CacheDir:      root,
		MaxCacheBytes: 16,
	}, slog.New(slog.DiscardHandler), cat, 1)

	if removed := pipeline.CleanupRemovedForTest(svc, time.Now()); removed != 1 {
		t.Fatalf("expected one removed file, got %d", removed)
	}
	assertFileStateForCleanupTest(t, oldPath, false)
	assertFileStateForCleanupTest(t, newPath, true)

	variants := cat.ListVariants("bundle.js")
	first, ok := variants.GetFirst()
	if !ok || variants.Len() != 1 || first.Encoding != "gzip" {
		t.Fatalf("expected only gzip variant retained, got %#v", variants)
	}
}

func TestCleanupArtifactsUsesNamespaceMaxAge(t *testing.T) {
	root := t.TempDir()
	cat := catalog.NewInMemoryCatalog()
	addAssetForCleanupTest(t, cat, "hero.png", "image/png", "hash-3")

	oldImage := filepath.Join(root, "image", "hash-3", "hero.png.fjpeg.jpg")
	writeArtifactForCleanupTest(t, oldImage, []byte("variant"))
	setMTimeForCleanupTest(t, oldImage, time.Now().Add(-2*time.Hour))

	upsertVariantForCleanupTest(t, cat, &catalog.Variant{
		ID:           "hero.png|format=jpeg",
		AssetPath:    "hero.png",
		ArtifactPath: oldImage,
		MediaType:    "image/jpeg",
		SourceHash:   "hash-3",
		ETag:         "\"hash-3-jpeg\"",
		Format:       "jpeg",
	})

	svc := pipeline.NewServiceForTest(&config.Compression{
		CacheDir:    root,
		MaxAge:      "24h",
		ImageMaxAge: "1h",
	}, slog.New(slog.DiscardHandler), cat, 1)

	if removed := pipeline.CleanupRemovedForTest(svc, time.Now()); removed != 1 {
		t.Fatalf("expected one removed file, got %d", removed)
	}
	assertFileStateForCleanupTest(t, oldImage, false)
}

func TestCleanupArtifactsKeepsHotVariant(t *testing.T) {
	root := t.TempDir()
	cat := catalog.NewInMemoryCatalog()
	addAssetForCleanupTest(t, cat, "bundle.js", "application/javascript", "hash-4")

	oldPath := filepath.Join(root, "encoding", "hash-4", "bundle.js.br")
	writeArtifactForCleanupTest(t, oldPath, []byte("compressed"))
	setMTimeForCleanupTest(t, oldPath, time.Now().Add(-3*time.Hour))

	upsertVariantForCleanupTest(t, cat, &catalog.Variant{
		ID:           "bundle.js|encoding=br",
		AssetPath:    "bundle.js",
		ArtifactPath: oldPath,
		MediaType:    "application/javascript",
		SourceHash:   "hash-4",
		ETag:         "\"hash-4-br\"",
		Encoding:     "br",
	})

	svc := pipeline.NewServiceForTest(&config.Compression{
		CacheDir: root,
		MaxAge:   "1h",
	}, slog.New(slog.DiscardHandler), cat, 1)
	svc.MarkVariantHit(oldPath)

	if removed := pipeline.CleanupRemovedForTest(svc, time.Now()); removed != 0 {
		t.Fatalf("expected no removed files for hot variant, got %d", removed)
	}
	assertFileStateForCleanupTest(t, oldPath, true)
}

func TestVariantServedEventKeepsHotVariant(t *testing.T) {
	root := t.TempDir()
	cat := catalog.NewInMemoryCatalog()
	addAssetForCleanupTest(t, cat, "bundle.js", "application/javascript", "hash-5")

	oldPath := filepath.Join(root, "encoding", "hash-5", "bundle.js.br")
	writeArtifactForCleanupTest(t, oldPath, []byte("compressed"))
	setMTimeForCleanupTest(t, oldPath, time.Now().Add(-3*time.Hour))

	upsertVariantForCleanupTest(t, cat, &catalog.Variant{
		ID:           "bundle.js|encoding=br",
		AssetPath:    "bundle.js",
		ArtifactPath: oldPath,
		MediaType:    "application/javascript",
		SourceHash:   "hash-5",
		ETag:         "\"hash-5-br\"",
		Encoding:     "br",
	})

	bus := eventx.New()
	svc := pipeline.NewServiceWithBusForTest(&config.Compression{
		CacheDir: root,
		MaxAge:   "1h",
	}, slog.New(slog.DiscardHandler), cat, bus, 1)
	if err := pipeline.SubscribeVariantServedForTest(svc); err != nil {
		t.Fatal(err)
	}

	if err := bus.Publish(context.Background(), appEvent.VariantServed{
		AssetPath:    "bundle.js",
		ArtifactPath: oldPath,
		ServedAt:     time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	if removed := pipeline.CleanupRemovedForTest(svc, time.Now()); removed != 0 {
		t.Fatalf("expected no removed files for recently served variant, got %d", removed)
	}
	assertFileStateForCleanupTest(t, oldPath, true)
}

func TestCleanupArtifactsRebuildsNamespaceBucketFromScratch(t *testing.T) {
	root := t.TempDir()
	cat := catalog.NewInMemoryCatalog()
	addAssetForCleanupTest(t, cat, "bundle.js", "application/javascript", "hash-8")

	oldPath := filepath.Join(root, "encoding", "hash-8", "bundle.js.br")
	writeArtifactForCleanupTest(t, oldPath, []byte("0123456789abcdef"))
	setMTimeForCleanupTest(t, oldPath, time.Now().Add(-2*time.Hour))

	upsertVariantForCleanupTest(t, cat, &catalog.Variant{
		ID:           "bundle.js|encoding=br",
		AssetPath:    "bundle.js",
		ArtifactPath: oldPath,
		MediaType:    "application/javascript",
		SourceHash:   "hash-8",
		ETag:         "\"hash-8-br\"",
		Encoding:     "br",
	})

	if err := os.Remove(oldPath); err != nil {
		t.Fatalf("prepare cleanup miss path, err=%v", err)
	}

	// Ensure cache policy still handles namespace buckets with missing files gracefully.
	svc := pipeline.NewServiceForTest(&config.Compression{
		CacheDir:      root,
		MaxCacheBytes: 1,
	}, slog.New(slog.DiscardHandler), cat, 1)

	if removed := pipeline.CleanupRemovedForTest(svc, time.Now()); removed != 0 {
		t.Fatalf("expected no removed files after deleting artifact first, got %d", removed)
	}
}
