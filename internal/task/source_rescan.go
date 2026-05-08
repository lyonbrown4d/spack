package task

import (
	"cmp"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/daiyuang/spack/internal/assetcache"
	"github.com/daiyuang/spack/internal/catalog"
	"github.com/daiyuang/spack/internal/source"
	"github.com/daiyuang/spack/internal/sourcecatalog"
	"github.com/go-co-op/gocron/v2"
	"github.com/samber/oops"
	"log/slog"
	"time"
)

const sourceRescanInterval = 5 * time.Minute

var errFullSourceRescanRequired = errors.New("source rescan full refresh required")

type SourceRescanReport struct {
	TotalBytes         int64
	Scanned            int
	Added              int
	Updated            int
	Removed            int
	RemovedVariants    int
	RemovedArtifacts   int
	CacheInvalidations int
}

type sourceRescanRun struct {
	ctx       context.Context
	scanner   sourcecatalog.Scanner
	cat       catalog.Catalog
	bodyCache *assetcache.Cache
	changes   *cxlist.List[source.ChangeEvent]
	report    SourceRescanReport
}

type sourceRescanSidecarChange struct {
	match sourcecatalog.SidecarMatch
	file  source.File
}

type sourceRescanDeletedSidecar struct {
	match sourcecatalog.SidecarMatch
	path  string
}

type sourceRescanChangeSet struct {
	deletedAssets         map[string]struct{}
	deletedSourceSidecars map[string]sourceRescanDeletedSidecar
	changedAssets         map[string]source.File
	changedSourceSidecars map[string]sourceRescanSidecarChange
	touchedAssetPaths     map[string]struct{}
}

func registerSourceRescanTask(ctx context.Context, scheduler gocron.Scheduler, runtime *sourceRescanRuntime) (bool, error) {
	if runtime == nil {
		return false, nil
	}

	job, err := scheduler.NewJob(
		gocron.DurationJob(sourceRescanInterval),
		gocron.NewTask(func() {
			runSourceRescan(ctx, runtime)
		}),
		gocron.WithName("source_rescan"),
	)
	if err != nil {
		return false, oops.In("task").Owner("source rescan").Wrap(err)
	}

	runtime.logger.Info("Task source rescan enabled",
		slog.String("id", job.ID().String()),
		slog.String("interval", sourceRescanInterval.String()),
	)
	return true, nil
}

func runSourceRescan(ctx context.Context, runtime *sourceRescanRuntime, changes ...source.ChangeEvent) {
	runtime.rescanMu.Lock()
	defer runtime.rescanMu.Unlock()

	startedAt := time.Now()
	report, err := syncSourceCatalog(ctx, runtime.scanner, runtime.catalog, runtime.bodyCache, changes...)
	recordTaskRunMetrics(ctx, runtime.obs, "source_rescan", startedAt, err)
	if err != nil {
		runtime.logger.Error("Task source rescan failed", slog.String("err", err.Error()))
		return
	}
	recordSourceRescanMetrics(ctx, runtime.obs, report)
	go runtime.catMetrics.SyncCatalog(runtime.catalog)
	go runtime.catMetrics.SetSourceBytes(report.TotalBytes)

	runtime.logger.Info("Task source rescan completed",
		slog.Int("scanned", report.Scanned),
		slog.Int("added", report.Added),
		slog.Int("updated", report.Updated),
		slog.Int("removed", report.Removed),
		slog.Int("removed_variants", report.RemovedVariants),
		slog.Int("removed_artifacts", report.RemovedArtifacts),
		slog.Int("cache_invalidations", report.CacheInvalidations),
		slog.Duration("duration", time.Since(startedAt)),
	)
}

func syncSourceCatalog(
	ctx context.Context,
	scanner sourcecatalog.Scanner,
	cat catalog.Catalog,
	bodyCache *assetcache.Cache,
	changes ...source.ChangeEvent,
) (SourceRescanReport, error) {
	run := &sourceRescanRun{
		ctx:       ctx,
		scanner:   scanner,
		cat:       cat,
		bodyCache: bodyCache,
	}
	if len(changes) > 0 {
		run.changes = cxlist.NewList(changes...)
	}

	return run.sync()
}

func (r *sourceRescanRun) sync() (SourceRescanReport, error) {
	if r.changes != nil && !r.changes.IsEmpty() {
		if err := r.syncIncrementally(); err != nil {
			if !errors.Is(err, errFullSourceRescanRequired) {
				return SourceRescanReport{}, err
			}
			r.report = SourceRescanReport{}
		} else {
			return r.report, nil
		}
	}
	return r.syncFull()
}

func (r *sourceRescanRun) syncFull() (SourceRescanReport, error) {
	snapshot, err := r.collectScannedSnapshot()
	if err != nil {
		return SourceRescanReport{}, err
	}

	existingByPath := indexAssetsByPath(r.cat.AllAssets())
	if err := r.reconcileScannedAssets(snapshot.Assets, existingByPath); err != nil {
		return SourceRescanReport{}, err
	}
	r.reconcileRemovedAssets(existingByPath)
	if err := r.reconcileSourceSidecars(snapshot.Variants); err != nil {
		return SourceRescanReport{}, err
	}
	return r.report, nil
}

func (r *sourceRescanRun) syncIncrementally() error {
	changeSet, err := r.buildIncrementalChangeSet()
	if err != nil {
		return err
	}
	if changeSet == nil || changeSet.isEmpty() {
		return nil
	}

	if err := r.processDeletedAssets(changeSet.deletedAssets); err != nil {
		return err
	}
	r.processDeletedSourceSidecars(changeSet.deletedSourceSidecars)
	if err := r.processChangedAssets(changeSet.changedAssets); err != nil {
		return err
	}
	if err := r.processChangedSourceSidecars(changeSet.changedSourceSidecars); err != nil {
		return err
	}

	r.report.TotalBytes = totalAssetBytes(r.cat.AllAssets())
	r.report.Scanned = len(changeSet.touchedAssetPaths)
	return nil
}

func newSourceRescanChangeSet() *sourceRescanChangeSet {
	return &sourceRescanChangeSet{
		deletedAssets:         map[string]struct{}{},
		deletedSourceSidecars: map[string]sourceRescanDeletedSidecar{},
		changedAssets:         map[string]source.File{},
		changedSourceSidecars: map[string]sourceRescanSidecarChange{},
		touchedAssetPaths:     map[string]struct{}{},
	}
}

func (r *sourceRescanRun) buildIncrementalChangeSet() (*sourceRescanChangeSet, error) {
	changeSet := newSourceRescanChangeSet()

	var changeErr error
	r.changes.Range(func(_ int, change source.ChangeEvent) bool {
		normalizedPath := normalizeChangePath(strings.TrimSpace(change.Path))
		if normalizedPath == "" {
			return true
		}

		kind, err := classifyChange(change.Op)
		if err != nil {
			changeErr = err
			return false
		}
		if kind == sourceChangeFallback {
			changeErr = errFullSourceRescanRequired
			return false
		}
		if kind == sourceChangeUnknown {
			return true
		}

		match, isSidecar := r.scanner.MatchSidecarPath(normalizedPath)
		switch kind {
		case sourceChangeDelete:
			if isSidecar {
				changeSet.deletedSourceSidecars[normalizedPath] = sourceRescanDeletedSidecar{
					match: match,
					path:  normalizedPath,
				}
				if match.AssetPath != "" {
					changeSet.touchedAssetPaths[match.AssetPath] = struct{}{}
				}
			} else {
				changeSet.deletedAssets[normalizedPath] = struct{}{}
				changeSet.touchedAssetPaths[normalizedPath] = struct{}{}
			}
			delete(changeSet.changedAssets, normalizedPath)
			return true
		case sourceChangeWrite:
			file, found, findErr := r.scanner.FindFile(normalizedPath)
			if findErr != nil {
				changeErr = oops.In("task").Owner("source rescan").Wrap(findErr)
				return false
			}
			if !found {
				return true
			}

			changeSet.touchedAssetPaths[normalizedPath] = struct{}{}
			if isSidecar {
				if match.AssetPath != "" {
					changeSet.changedAssetMatchPath(match, normalizedPath, file)
				}
			} else {
				changeSet.changedAssets[normalizedPath] = file
			}
			return true
		default:
			return true
		}
	})

	if changeErr != nil {
		return nil, changeErr
	}

	changeSet.normalizeIncrementalTargets()
	return changeSet, nil
}

const (
	sourceChangeUnknown = iota
	sourceChangeWrite
	sourceChangeDelete
	sourceChangeFallback
)

func classifyChange(rawOp string) (int, error) {
	if strings.TrimSpace(rawOp) == "" {
		return sourceChangeUnknown, nil
	}

	op := strings.ToUpper(strings.TrimSpace(rawOp))
	if strings.Contains(op, "RENAME") || strings.Contains(op, "MOVE") {
		return sourceChangeFallback, nil
	}

	if strings.Contains(op, "CREATE") && strings.Contains(op, "REMOVE") {
		return sourceChangeFallback, nil
	}

	if strings.Contains(op, "CREATE") || strings.Contains(op, "WRITE") {
		return sourceChangeWrite, nil
	}

	if strings.Contains(op, "REMOVE") {
		return sourceChangeDelete, nil
	}

	return sourceChangeUnknown, nil
}

func (changes *sourceRescanChangeSet) normalizeIncrementalTargets() {
	for assetPath := range changes.deletedAssets {
		delete(changes.changedAssets, assetPath)
	}

	for sidecarPath, sidecarChange := range changes.changedSourceSidecars {
		if _, deleted := changes.deletedAssets[sidecarChange.match.AssetPath]; deleted {
			delete(changes.changedSourceSidecars, sidecarPath)
			continue
		}
		changes.touchedAssetPaths[sidecarChange.match.AssetPath] = struct{}{}
	}
}

func (changes *sourceRescanChangeSet) isEmpty() bool {
	return len(changes.deletedAssets) == 0 &&
		len(changes.deletedSourceSidecars) == 0 &&
		len(changes.changedAssets) == 0 &&
		len(changes.changedSourceSidecars) == 0
}

func (changes *sourceRescanChangeSet) changedAssetMatchPath(match sourcecatalog.SidecarMatch, sidecarPath string, file source.File) {
	if match.AssetPath == "" {
		return
	}
	changes.changedSourceSidecars[sidecarPath] = sourceRescanSidecarChange{
		match: match,
		file:  file,
	}
	changes.touchedAssetPaths[match.AssetPath] = struct{}{}
}

func (r *sourceRescanRun) processDeletedAssets(deletedAssets map[string]struct{}) error {
	for assetPath := range deletedAssets {
		existing, ok := r.cat.FindAsset(assetPath)
		if !ok {
			continue
		}
		r.report.Removed++
		r.invalidateAssetAndVariants(existing.FullPath, r.cat.DeleteAsset(assetPath))
	}
	return nil
}

func (r *sourceRescanRun) processDeletedSourceSidecars(deletedSourceSidecars map[string]sourceRescanDeletedSidecar) {
	for sidecarPath := range deletedSourceSidecars {
		change, ok := deletedSourceSidecars[sidecarPath]
		if !ok {
			continue
		}
		r.removeSidecarVariants(change.match)
	}
}

func (r *sourceRescanRun) processChangedAssets(changedAssets map[string]source.File) error {
	var processErr error
	sortedChangedPaths(changedAssets).Range(func(_ int, path string) bool {
		if processErr != nil {
			return false
		}

		next, buildErr := sourcecatalog.BuildAsset(changedAssets[path])
		if buildErr != nil {
			processErr = oops.In("task").Owner("source rescan").With("asset_path", path).Wrap(buildErr)
			return false
		}

		existing, found := r.cat.FindAsset(path)
		if found {
			if !assetChanged(existing, next) {
				return true
			}
			r.report.Updated++
			r.invalidateAssetAndVariants(existing.FullPath, r.cat.DeleteVariants(path))
		} else {
			r.report.Added++
		}

		if err := r.cat.UpsertAsset(next); err != nil {
			processErr = oops.In("task").Owner("source rescan").With("asset_path", path).Wrap(err)
			return false
		}
		if err := r.rebuildSourceSidecarsForAsset(next); err != nil {
			processErr = oops.In("task").Owner("source rescan").With("asset_path", path).Wrap(err)
			return false
		}
		return true
	})
	return processErr
}

func (r *sourceRescanRun) rebuildSourceSidecarsForAsset(asset *catalog.Asset) error {
	if asset == nil {
		return nil
	}

	var processErr error
	r.scanner.SidecarMatchers().Range(func(_ int, matcher sourcecatalog.SidecarMatcher) bool {
		if processErr != nil {
			return false
		}

		sidecarPath := asset.Path + matcher.Suffix
		file, found, findErr := r.scanner.FindFile(sidecarPath)
		if findErr != nil || !found {
			return true
		}
		variant, buildErr := sourcecatalog.BuildSourceSidecarVariant(file, sourcecatalog.SidecarMatch{
			AssetPath: asset.Path,
			Encoding:  matcher.Encoding,
			Suffix:    matcher.Suffix,
		}, asset)
		if buildErr != nil {
			processErr = oops.In("task").Owner("source rescan").With("asset_path", asset.Path).With("sidecar_path", sidecarPath).Wrap(buildErr)
			return false
		}
		if err := r.cat.UpsertVariant(variant); err != nil {
			processErr = oops.In("task").Owner("source rescan").With("asset_path", asset.Path).With("sidecar_path", sidecarPath).Wrap(err)
			return false
		}
		return true
	})
	if processErr != nil {
		return processErr
	}
	return nil
}

func (r *sourceRescanRun) processChangedSourceSidecars(changedSidecars map[string]sourceRescanSidecarChange) error {
	var processErr error
	sortedChangedPaths(changedSidecars).Range(func(_ int, sidecarPath string) bool {
		if processErr != nil {
			return false
		}

		change := changedSidecars[sidecarPath]
		asset, ok := r.cat.FindAsset(change.match.AssetPath)
		if !ok || asset == nil {
			return true
		}

		variant, buildErr := sourcecatalog.BuildSourceSidecarVariant(change.file, change.match, asset)
		if buildErr != nil {
			processErr = oops.In("task").Owner("source rescan").With("asset_path", change.match.AssetPath).Wrap(buildErr)
			return false
		}
		if err := r.cat.UpsertVariant(variant); err != nil {
			processErr = oops.In("task").Owner("source rescan").With("asset_path", change.match.AssetPath).Wrap(err)
			return false
		}
		return true
	})
	return processErr
}

func sortedChangedPaths[K any](values map[string]K) *cxlist.List[string] {
	paths := cxlist.NewList[string]()
	for path := range values {
		paths.Add(path)
	}
	return paths.Sort(func(left, right string) int {
		return cmp.Compare(left, right)
	})
}

func (r *sourceRescanRun) collectScannedSnapshot() (sourcecatalog.Snapshot, error) {
	snapshotErr := oops.In("task").Owner("source rescan")
	snapshot, err := r.scanner.ScanWithCatalog(r.ctx, r.cat)
	if err != nil {
		return sourcecatalog.Snapshot{}, snapshotErr.Wrap(err)
	}
	r.report.TotalBytes = snapshot.TotalBytes
	r.report.Scanned = snapshot.Assets.Len()
	return snapshot, nil
}

func indexAssetsByPath(assets *cxlist.List[*catalog.Asset]) *cxmapping.Map[string, *catalog.Asset] {
	return cxmapping.AssociateList[*catalog.Asset, string, *catalog.Asset](assets, func(_ int, asset *catalog.Asset) (string, *catalog.Asset) {
		return asset.Path, asset
	})
}

func (r *sourceRescanRun) reconcileScannedAssets(
	scannedAssets *cxmapping.Map[string, *catalog.Asset],
	existingByPath *cxmapping.Map[string, *catalog.Asset],
) error {
	var syncErr error
	sortedMapKeys[*catalog.Asset](scannedAssets).Range(func(_ int, assetPath string) bool {
		asset, _ := scannedAssets.Get(assetPath)
		if err := r.syncScannedAsset(assetPath, asset, existingByPath); err != nil {
			syncErr = err
			return false
		}
		return true
	})
	if syncErr != nil {
		return syncErr
	}
	return nil
}

func (r *sourceRescanRun) syncScannedAsset(
	assetPath string,
	asset *catalog.Asset,
	existingByPath *cxmapping.Map[string, *catalog.Asset],
) error {
	existing, found := existingByPath.Get(assetPath)
	if found {
		existingByPath.Delete(assetPath)
	}

	if !found {
		r.report.Added++
	} else if assetChanged(existing, asset) {
		r.report.Updated++
		r.invalidateAssetAndVariants(existing.FullPath, r.cat.DeleteVariants(assetPath))
	}

	if err := r.cat.UpsertAsset(asset); err != nil {
		return oops.In("task").Owner("source rescan").With("asset_path", asset.Path).Wrap(err)
	}
	return nil
}

func (r *sourceRescanRun) reconcileRemovedAssets(
	existingByPath *cxmapping.Map[string, *catalog.Asset],
) {
	cxlist.NewList[*catalog.Asset](existingByPath.Values()...).Sort(func(left, right *catalog.Asset) int {
		return cmp.Compare(left.Path, right.Path)
	}).Range(func(_ int, asset *catalog.Asset) bool {
		r.report.Removed++
		r.invalidateAssetAndVariants(asset.FullPath, r.cat.DeleteAsset(asset.Path))
		return true
	})
}

func (r *sourceRescanRun) reconcileSourceSidecars(
	scannedVariants *cxmapping.Map[string, *catalog.Variant],
) error {
	existingByID := r.indexSourceSidecarVariants()
	var syncErr error
	sortedMapKeys[*catalog.Variant](scannedVariants).Range(func(_ int, variantID string) bool {
		variant, _ := scannedVariants.Get(variantID)
		if err := r.cat.UpsertVariant(variant); err != nil {
			syncErr = oops.In("task").Owner("source rescan").With("variant_id", variantID).With("asset_path", variant.AssetPath).Wrap(err)
			return false
		}
		existingByID.Delete(variantID)
		return true
	})
	if syncErr != nil {
		return syncErr
	}

	cxlist.NewList[*catalog.Variant](existingByID.Values()...).Sort(func(left, right *catalog.Variant) int {
		return cmp.Compare(left.ID, right.ID)
	}).Range(func(_ int, variant *catalog.Variant) bool {
		if !r.cat.DeleteVariantByArtifactPath(variant.ArtifactPath) {
			return true
		}
		r.report.RemovedVariants++
		r.report.CacheInvalidations += invalidateAssetCache(r.bodyCache, variant.ArtifactPath)
		return true
	})
	return nil
}

func (r *sourceRescanRun) indexSourceSidecarVariants() *cxmapping.Map[string, *catalog.Variant] {
	variantsByID := cxmapping.NewMap[string, *catalog.Variant]()
	r.cat.ListVariantsByStage(sourcecatalog.SourceSidecarStage).Range(func(_ int, variant *catalog.Variant) bool {
		if sourcecatalog.IsSourceSidecarVariant(variant) {
			variantsByID.Set(variant.ID, variant)
		}
		return true
	})
	return variantsByID
}

func (r *sourceRescanRun) invalidateAssetAndVariants(
	assetPath string,
	variants *cxlist.List[*catalog.Variant],
) {
	r.report.CacheInvalidations += invalidateAssetCache(r.bodyCache, assetPath)
	r.report.RemovedVariants += r.removeAssetVariants(variants)
}

func (r *sourceRescanRun) removeAssetVariants(variants *cxlist.List[*catalog.Variant]) int {
	removed := 0
	variants.Range(func(_ int, variant *catalog.Variant) bool {
		removed++
		r.report.CacheInvalidations += invalidateAssetCache(r.bodyCache, variant.ArtifactPath)
		r.report.RemovedArtifacts += removeVariantArtifact(variant)
		return true
	})
	return removed
}

func (r *sourceRescanRun) removeSidecarVariants(match sourcecatalog.SidecarMatch) {
	if match.AssetPath == "" {
		return
	}

	variants := r.cat.ListVariants(match.AssetPath).Where(func(_ int, variant *catalog.Variant) bool {
		if variant == nil {
			return false
		}
		return variant.Encoding == match.Encoding && variant.ID == match.AssetPath+match.Suffix
	})
	if variants.IsEmpty() {
		return
	}

	variants.Range(func(_ int, variant *catalog.Variant) bool {
		if variant == nil {
			return true
		}
		if !r.cat.DeleteVariantByArtifactPath(variant.ArtifactPath) {
			return true
		}
		r.report.RemovedVariants++
		r.report.CacheInvalidations += invalidateAssetCache(r.bodyCache, variant.ArtifactPath)
		return true
	})
}

func sortedMapKeys[T any](values *cxmapping.Map[string, T]) *cxlist.List[string] {
	return cxlist.NewList[string](values.Keys()...).Sort(cmp.Compare[string])
}

func totalAssetBytes(assets *cxlist.List[*catalog.Asset]) int64 {
	var total int64
	assets.Range(func(_ int, asset *catalog.Asset) bool {
		if asset != nil {
			total += asset.Size
		}
		return true
	})
	return total
}

func removeVariantArtifact(variant *catalog.Variant) int {
	if sourcecatalog.IsSourceSidecarVariant(variant) {
		return 0
	}
	return removeArtifactFile(variant.ArtifactPath)
}

func invalidateAssetCache(bodyCache *assetcache.Cache, path string) int {
	if bodyCache != nil && bodyCache.Delete(path) {
		return 1
	}
	return 0
}

func removeArtifactFile(path string) int {
	if path == "" {
		return 0
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return 0
	}
	return 1
}

func normalizeChangePath(rawPath string) string {
	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" {
		return ""
	}
	return strings.TrimPrefix(filepath.ToSlash(filepath.Clean(trimmed)), "/")
}

func assetChanged(existing, next *catalog.Asset) bool {
	if existing == nil || next == nil {
		return existing != next
	}
	return existing.FullPath != next.FullPath ||
		existing.Size != next.Size ||
		existing.MediaType != next.MediaType ||
		existing.SourceHash != next.SourceHash ||
		existing.ETag != next.ETag
}
