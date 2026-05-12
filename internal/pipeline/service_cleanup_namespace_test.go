package pipeline_test

import (
	"testing"
	"time"

	"github.com/lyonbrown4d/spack/internal/pipeline"
)

func TestCleanupArtifactsEnforcesNamespaceMaxCacheBytes(t *testing.T) {
	root := t.TempDir()
	catalog, artifacts := setupNamespaceMaxCacheBytesCase(t, root, time.Now())
	svc := newNamespaceCleanupService(t, root, catalog)

	if removed := pipeline.CleanupRemovedForTest(svc, time.Now()); removed != 2 {
		t.Fatalf("expected two removed files, got %d", removed)
	}
	assertFileStateForCleanupTest(t, artifacts.encodingOld, false)
	assertFileStateForCleanupTest(t, artifacts.encodingNew, true)
	assertFileStateForCleanupTest(t, artifacts.imageOld, false)
	assertFileStateForCleanupTest(t, artifacts.imageNew, true)

	if catalog.ListVariants("bundle.js").Len() != 1 {
		t.Fatalf("expected one encoding variant retained, got %#v", catalog.ListVariants("bundle.js"))
	}
	if catalog.ListVariants("hero.png").Len() != 1 {
		t.Fatalf("expected one image variant retained, got %#v", catalog.ListVariants("hero.png"))
	}
}
