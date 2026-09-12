package pipeline

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/observabilityx"
	"github.com/lyonbrown4d/spack/internal/asyncx"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/contentcoding"
)

type assetErrorWarmStage struct {
	errors map[string]error
	mu     sync.Mutex
	runs   []string
}

func (s *assetErrorWarmStage) Name() string { return "asset-error" }

func (s *assetErrorWarmStage) Plan(asset *catalog.Asset, _ Request) *cxlist.List[Task] {
	return cxlist.NewList(Task{AssetPath: asset.Path})
}

func (s *assetErrorWarmStage) Execute(_ context.Context, _ Task, asset *catalog.Asset) (*catalog.Variant, error) {
	s.mu.Lock()
	s.runs = append(s.runs, asset.Path)
	s.mu.Unlock()
	return nil, s.errors[asset.Path]
}

func TestWarmAggregatesErrorsFromMultipleAssets(t *testing.T) {
	firstErr := errors.New("first asset failed")
	secondErr := errors.New("second asset failed")
	cat := catalog.NewInMemoryCatalog()
	upsertWarmAssets(t, cat, "first.js", "second.js")
	stage := &assetErrorWarmStage{errors: map[string]error{
		"first.js":  firstErr,
		"second.js": secondErr,
	}}
	svc := newWarmServiceForStages(cat, cxlist.NewList[Stage](stage))

	err := svc.Warm(t.Context())
	if !errors.Is(err, firstErr) || !errors.Is(err, secondErr) {
		t.Fatalf("expected both asset errors, got %v", err)
	}
}

func TestWarmExcludesSkippedVariantFromJoinedFailure(t *testing.T) {
	wantErr := errors.New("real failure")
	cat := catalog.NewInMemoryCatalog()
	upsertWarmAssets(t, cat, "skip.js", "fail.js")
	stage := &assetErrorWarmStage{errors: map[string]error{
		"skip.js": ErrVariantSkipped,
		"fail.js": wantErr,
	}}
	svc := newWarmServiceForStages(cat, cxlist.NewList[Stage](stage))

	err := svc.Warm(t.Context())
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected real failure, got %v", err)
	}
	if errors.Is(err, ErrVariantSkipped) {
		t.Fatalf("expected skipped variant to be excluded, got %v", err)
	}
}

func TestWarmPropagatesCompressionStoreWriteFailure(t *testing.T) {
	wantErr := errors.New("artifact write failed")
	root := t.TempDir()
	sourcePath := filepath.Join(root, "app.js")
	payload := bytes.Repeat([]byte("const value = 'compressible';\n"), 256)
	if err := os.WriteFile(sourcePath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	asset := &catalog.Asset{
		Path:       "app.js",
		FullPath:   sourcePath,
		Size:       int64(len(payload)),
		MediaType:  "application/javascript",
		SourceHash: "source-hash",
	}
	cat := catalog.NewInMemoryCatalog()
	if err := cat.UpsertAsset(asset); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Compression{
		Enable:    true,
		Mode:      config.CompressionModeWarmup,
		Encodings: "gzip",
		GzipLevel: 1,
	}
	stage := newCompressionStage(
		cfg,
		contentcoding.NewRegistry(contentcoding.Options{GzipLevel: cfg.GzipLevel}, cfg.NormalizedEncodings()),
		writeFailureArtifactStore{root: root, err: wantErr},
		cat,
		nil,
	)
	svc := newWarmServiceForConfigAndStages(cfg, cat, cxlist.NewList[Stage](stage))

	err := svc.Warm(t.Context())
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected store write failure, got %v", err)
	}
}

func upsertWarmAssets(t *testing.T, cat catalog.Catalog, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if err := cat.UpsertAsset(&catalog.Asset{
			Path:       path,
			FullPath:   path,
			MediaType:  "application/javascript",
			SourceHash: path + "-hash",
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func newWarmServiceForStages(cat catalog.Catalog, stages *cxlist.List[Stage]) *Service {
	return newWarmServiceForConfigAndStages(
		&config.Compression{Enable: true, Mode: config.CompressionModeWarmup},
		cat,
		stages,
	)
}

func newWarmServiceForConfigAndStages(
	cfg *config.Compression,
	cat catalog.Catalog,
	stages *cxlist.List[Stage],
) *Service {
	return newServiceState(
		cfg,
		slog.New(slog.DiscardHandler),
		cat,
		serviceDeps{
			stages:  stages,
			obs:     observabilityx.Nop(),
			workers: &asyncx.Settings{Size: 2},
		},
		0,
	)
}

type writeFailureArtifactStore struct {
	root string
	err  error
}

func (s writeFailureArtifactStore) Root() string { return s.root }

func (s writeFailureArtifactStore) PathFor(assetPath, _, namespace, suffix string) (string, error) {
	return filepath.Join(s.root, namespace, assetPath+suffix), nil
}

func (s writeFailureArtifactStore) Write(string, []byte) error { return s.err }
