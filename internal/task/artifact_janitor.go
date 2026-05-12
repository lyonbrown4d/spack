package task

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxset "github.com/arcgolabs/collectionx/set"
	cxtree "github.com/arcgolabs/collectionx/tree"
	"github.com/go-co-op/gocron/v2"
	"github.com/lyonbrown4d/spack/internal/artifact"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/samber/oops"
)

const artifactJanitorInterval = 15 * time.Minute

type ArtifactJanitorReport struct {
	ScannedArtifacts   int
	RemovedOrphans     int
	RemovedDirectories int
	MissingVariants    int
	CacheInvalidations int
}

type artifactJanitorRun struct {
	ctx        context.Context
	root       string
	rootHandle *os.Root
	expected   *cxset.Set[string]
	cat        catalog.Catalog
	bodyCache  *assetcache.Cache
	report     *ArtifactJanitorReport
}

func registerArtifactJanitorTask(ctx context.Context, scheduler gocron.Scheduler, runtime *artifactJanitorRuntime) (bool, error) {
	if runtime == nil || runtime.store == nil || strings.TrimSpace(runtime.store.Root()) == "" {
		return false, nil
	}

	job, err := scheduler.NewJob(
		gocron.DurationJob(artifactJanitorInterval),
		gocron.NewTask(func() {
			runArtifactJanitor(ctx, runtime)
		}),
		gocron.WithName("artifact_janitor"),
	)
	if err != nil {
		return false, oops.In("task").Owner("artifact janitor").Wrap(err)
	}

	runtime.logger.Info("Task artifact janitor enabled",
		slog.String("id", job.ID().String()),
		slog.String("interval", artifactJanitorInterval.String()),
		slog.String("root", runtime.store.Root()),
	)
	return true, nil
}

func runArtifactJanitor(ctx context.Context, runtime *artifactJanitorRuntime) {
	startedAt := time.Now()
	report, err := syncArtifactCatalog(ctx, runtime.store, runtime.catalog, runtime.bodyCache)
	recordTaskRunMetrics(ctx, runtime.obs, "artifact_janitor", startedAt, err)
	if err != nil {
		runtime.logger.Error("Task artifact janitor failed", slog.String("err", err.Error()))
		return
	}
	recordArtifactJanitorMetrics(ctx, runtime.obs, report)
	runtime.catMetrics.SyncCatalog(runtime.catalog)

	if report.ScannedArtifacts == 0 && report.RemovedOrphans == 0 && report.MissingVariants == 0 && report.RemovedDirectories == 0 {
		return
	}

	runtime.logger.Info("Task artifact janitor completed",
		slog.Int("scanned_artifacts", report.ScannedArtifacts),
		slog.Int("removed_orphans", report.RemovedOrphans),
		slog.Int("missing_variants", report.MissingVariants),
		slog.Int("removed_directories", report.RemovedDirectories),
		slog.Int("cache_invalidations", report.CacheInvalidations),
		slog.Duration("duration", time.Since(startedAt)),
	)
}

func syncArtifactCatalog(
	ctx context.Context,
	store artifact.Store,
	cat catalog.Catalog,
	bodyCache *assetcache.Cache,
) (ArtifactJanitorReport, error) {
	root := strings.TrimSpace(store.Root())
	if root == "" {
		return ArtifactJanitorReport{}, nil
	}

	run := artifactJanitorRun{
		ctx:       ctx,
		root:      root,
		expected:  cxset.NewSet[string](),
		cat:       cat,
		bodyCache: bodyCache,
		report:    &ArtifactJanitorReport{},
	}
	if err := run.collectCatalogArtifactPaths(); err != nil {
		return ArtifactJanitorReport{}, err
	}
	if err := run.removeOrphanArtifacts(); err != nil {
		return ArtifactJanitorReport{}, err
	}
	run.report.RemovedDirectories += pruneEmptyArtifactDirs(root)
	return *run.report, nil
}

func (r *artifactJanitorRun) collectCatalogArtifactPaths() error {
	var collectErr error
	r.cat.AllVariants().Range(func(_ int, variant *catalog.Variant) bool {
		if err := r.contextErr(); err != nil {
			collectErr = err
			return false
		}
		if err := r.collectCatalogVariant(variant); err != nil {
			collectErr = err
			return false
		}
		return true
	})
	if collectErr != nil {
		return collectErr
	}
	return nil
}

func (r *artifactJanitorRun) collectCatalogVariant(variant *catalog.Variant) error {
	artifactPath := variantArtifactPath(variant)
	if artifactPath == "" {
		return nil
	}
	if artifactExists(artifactPath) {
		r.expected.Add(artifactPath)
		return nil
	}
	if _, statErr := os.Stat(artifactPath); statErr != nil && !os.IsNotExist(statErr) {
		return oops.In("task").Owner("artifact janitor").With("artifact_path", artifactPath).Wrap(statErr)
	}
	if r.cat.DeleteVariantByArtifactPath(artifactPath) {
		r.report.MissingVariants++
		r.report.CacheInvalidations += invalidateAssetCache(r.bodyCache, artifactPath)
	}
	return nil
}

func (r *artifactJanitorRun) removeOrphanArtifacts() error {
	rootHandle, err := openArtifactRoot(r.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if rootHandle == nil {
		return nil
	}

	r.rootHandle = rootHandle
	walkErr := r.walkOrphanArtifacts()
	closeErr := closeRoot(rootHandle)
	r.rootHandle = nil
	if walkErr != nil {
		return walkErr
	}
	return closeErr
}

func (r *artifactJanitorRun) removeOrphanArtifact(relativePath, artifactPath string) error {
	if removeErr := r.rootHandle.Remove(filepath.ToSlash(relativePath)); removeErr != nil && !os.IsNotExist(removeErr) {
		return oops.In("task").Owner("artifact janitor").With("artifact_path", artifactPath).Wrap(removeErr)
	}
	if r.report != nil {
		r.report.RemovedOrphans++
		r.report.CacheInvalidations += invalidateAssetCache(r.bodyCache, artifactPath)
	}
	r.cat.DeleteVariantByArtifactPath(artifactPath)
	return nil
}

func (r *artifactJanitorRun) walkOrphanArtifacts() error {
	walkErr := fs.WalkDir(r.rootHandle.FS(), ".", func(relativePath string, entry fs.DirEntry, err error) error {
		return r.visitArtifactPath(relativePath, entry, err)
	})
	if walkErr == nil || os.IsNotExist(walkErr) {
		return nil
	}
	return oops.In("task").Owner("artifact janitor").Wrap(walkErr)
}

func pruneEmptyArtifactDirs(root string) int {
	cleanRoot := strings.TrimSpace(filepath.Clean(root))
	if cleanRoot == "" {
		return 0
	}

	entries, directories := buildArtifactDirEntries(cleanRoot)
	if len(entries) <= 1 || directories == nil {
		return 0
	}

	artifactTree, treeErr := cxtree.Build(entries)
	if treeErr != nil {
		return sortAndRemoveDirectories(directories)
	}

	return removeArtifactDirsByTree(artifactTree, cleanRoot)
}

func buildArtifactDirEntries(cleanRoot string) ([]cxtree.Entry[string, struct{}], *cxlist.List[string]) {
	directories := cxlist.NewList[string]()
	entries := []cxtree.Entry[string, struct{}]{cxtree.RootEntry(cleanRoot, struct{}{})}
	if walkErr := filepath.WalkDir(cleanRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		cleanPath := filepath.Clean(path)
		directories.Add(cleanPath)
		if cleanPath == cleanRoot {
			return nil
		}
		entries = append(entries, cxtree.ChildEntry(cleanPath, filepath.Clean(filepath.Dir(cleanPath)), struct{}{}))
		return nil
	}); walkErr != nil {
		return nil, nil
	}
	return entries, directories
}

func removeArtifactDirsByTree(artifactTree *cxtree.Tree[string, struct{}], root string) int {
	nodes := artifactTree.Descendants(root)
	if len(nodes) == 0 {
		return 0
	}

	removed := 0
	for _, node := range slices.Backward(nodes) {
		if node == nil || node.ID() == root {
			continue
		}
		if os.Remove(node.ID()) == nil {
			removed++
		}
	}
	return removed
}

func sortAndRemoveDirectories(directories *cxlist.List[string]) int {
	if directories.IsEmpty() {
		return 0
	}

	removed := 0
	directories.Sort(func(left, right string) int {
		return len(filepath.Clean(right)) - len(filepath.Clean(left))
	}).Range(func(_ int, path string) bool {
		if err := os.Remove(path); err == nil {
			removed++
		}
		return true
	})
	return removed
}
