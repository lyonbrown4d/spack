package task

import (
	"context"
	"errors"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/go-co-op/gocron/v2"
	"github.com/lyonbrown4d/spack/internal/assetcache"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/sourcecatalog"
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
	publishCatalogChanged(ctx, runtime.bus, "source_rescan", runtime.logger)

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
	r.report.Scanned = changeSet.touchedAssetPaths.Len()
	return nil
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

func (r *sourceRescanRun) reconcileScannedAssets(
	scannedAssets *cxmapping.Map[string, *catalog.Asset],
	existingByPath *cxmapping.Map[string, *catalog.Asset],
) error {
	var syncErr error
	sortedMapEntries[*catalog.Asset](scannedAssets).Range(func(_ int, asset sortedMapEntry[*catalog.Asset]) bool {
		if err := r.syncScannedAsset(asset.key, asset.value, existingByPath); err != nil {
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
	sortedMapEntries[*catalog.Asset](existingByPath).Range(func(_ int, asset sortedMapEntry[*catalog.Asset]) bool {
		r.report.Removed++
		r.invalidateAssetAndVariants(asset.value.FullPath, r.cat.DeleteAsset(asset.key))
		return true
	})
}
