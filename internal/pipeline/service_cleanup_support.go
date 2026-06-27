package pipeline

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	cxset "github.com/arcgolabs/collectionx/set"
	appEvent "github.com/lyonbrown4d/spack/internal/event"
	"github.com/samber/lo"
	"github.com/samber/oops"
)

type cleanupFile struct {
	path      string
	namespace string
	size      int64
	modTime   time.Time
	lastUsed  time.Time
}

type cleanupResult struct {
	scanned          int
	removed          int
	removedBytes     int64
	totalBytes       int64
	removedTTL       int
	removedSize      int
	removedTTLBytes  int64
	removedSizeBytes int64
}

func (s *Service) cleanupOnce(ctx context.Context) {
	result := s.cleanupArtifacts(ctx, time.Now())
	if s.metrics != nil {
		s.metrics.CleanupRunsTotal.Inc()
		if result.removedTTL > 0 {
			s.metrics.CleanupRemovedTotal.WithLabelValues("ttl").Add(float64(result.removedTTL))
			s.metrics.CleanupRemovedBytesTotal.WithLabelValues("ttl").Add(float64(result.removedTTLBytes))
		}
		if result.removedSize > 0 {
			s.metrics.CleanupRemovedTotal.WithLabelValues("size").Add(float64(result.removedSize))
			s.metrics.CleanupRemovedBytesTotal.WithLabelValues("size").Add(float64(result.removedSizeBytes))
		}
	}
	if result.removed > 0 {
		go s.catMetrics.SyncCatalog(s.catalog)
	}
	if result.removed > 0 {
		s.logger.Info("Pipeline cache cleanup completed",
			slog.Int("scanned", result.scanned),
			slog.Int("removed", result.removed),
			slog.Int("removed_ttl", result.removedTTL),
			slog.Int("removed_size", result.removedSize),
			slog.Int64("removed_bytes", result.removedBytes),
			slog.Int64("remaining_bytes", result.totalBytes),
		)
	}
}

func (s *Service) cleanupArtifacts(ctx context.Context, now time.Time) cleanupResult {
	if strings.TrimSpace(s.cfg.CacheDir) == "" {
		return cleanupResult{}
	}

	s.cleanupMu.Lock()
	defer s.cleanupMu.Unlock()

	files, err := collectCleanupFiles(s.cfg.CacheDir)
	if err != nil {
		s.logger.Error("Pipeline cache scan failed", slog.String("err", err.Error()))
		return cleanupResult{}
	}

	result := s.prepareCleanupResult(files)
	remaining := s.removeExpiredCleanupFiles(ctx, files, now, &result)
	s.enforceCleanupCacheLimit(ctx, remaining, &result)
	return result
}

func (s *Service) prepareCleanupResult(files []cleanupFile) cleanupResult {
	result := cleanupResult{scanned: len(files)}
	for index := range files {
		file := &files[index]
		file.namespace = cleanupNamespace(file.path, s.cfg.CacheDir)
		file.lastUsed = s.effectiveLastUsed(file.path, file.modTime)
		result.totalBytes += file.size
	}
	return result
}

func (s *Service) removeExpiredCleanupFiles(
	ctx context.Context,
	files []cleanupFile,
	now time.Time,
	result *cleanupResult,
) []cleanupFile {
	return lo.Filter(files, func(file cleanupFile, _ int) bool {
		if s.shouldRemoveExpiredFile(file, now) && s.removeCleanupFile(ctx, file, appEvent.VariantRemovalReasonTTL) {
			recordExpiredCleanupRemoval(result, file.size)
			return false
		}
		return true
	})
}

func (s *Service) shouldRemoveExpiredFile(file cleanupFile, now time.Time) bool {
	return s.artifactPolicy != nil && s.artifactPolicy.ShouldRemoveExpired(file.namespace, file.lastUsed, now)
}

func (s *Service) enforceCleanupCacheLimitByNamespace(ctx context.Context, files []cleanupFile, result *cleanupResult) []cleanupFile {
	filesByNamespace := cxmapping.NewMultiMapWithCapacity[string, cleanupFile](len(files))
	for _, file := range files {
		filesByNamespace.Put(file.namespace, file)
	}

	remaining := cxlist.NewList[cleanupFile]()
	filesByNamespace.Range(func(namespace string, namespaceFiles []cleanupFile) bool {
		limit := s.artifactPolicy.MaxCacheBytesForNamespace(namespace)
		remainingFiles := s.evictCleanupFilesBySize(ctx, namespaceFiles, limit, appEvent.VariantRemovalReasonSize, result)
		if len(remainingFiles) > 0 {
			remaining.Add(remainingFiles...)
		}
		return true
	})
	return remaining.Values()
}

func (s *Service) enforceCleanupCacheLimitByBytes(ctx context.Context, files []cleanupFile, result *cleanupResult, maxCacheBytes int64) {
	_ = s.evictCleanupFilesBySize(ctx, files, maxCacheBytes, appEvent.VariantRemovalReasonSize, result)
}

func (s *Service) evictCleanupFilesBySize(
	ctx context.Context,
	files []cleanupFile,
	maxCacheBytes int64,
	reason appEvent.VariantRemovalReason,
	result *cleanupResult,
) []cleanupFile {
	if maxCacheBytes <= 0 || len(files) == 0 {
		return files
	}

	totalBytes := lo.SumBy(files, func(file cleanupFile) int64 {
		return file.size
	})
	if totalBytes <= maxCacheBytes {
		return files
	}

	pq, err := cxlist.NewPriorityQueue(func(left, right cleanupFile) bool {
		return left.lastUsed.Before(right.lastUsed)
	}, files...)
	if err != nil {
		return files
	}

	removed := cxset.NewSet[string]()
	for totalBytes > maxCacheBytes {
		file, ok := pq.Pop()
		if !ok {
			return files
		}
		if !s.removeCleanupFile(ctx, file, reason) {
			continue
		}
		recordSizeCleanupRemoval(result, file.size)
		totalBytes -= file.size
		removed.Add(file.path)
	}
	if removed.IsEmpty() {
		return files
	}
	return lo.Reject(files, func(file cleanupFile, _ int) bool {
		return removed.Contains(file.path)
	})
}

func (s *Service) enforceCleanupCacheLimit(ctx context.Context, files []cleanupFile, result *cleanupResult) {
	files = s.enforceCleanupCacheLimitByNamespace(ctx, files, result)
	s.enforceCleanupCacheLimitByBytes(ctx, files, result, s.artifactPolicy.MaxCacheBytes())
}

func (s *Service) removeCleanupFile(ctx context.Context, file cleanupFile, reason appEvent.VariantRemovalReason) bool {
	if err := os.Remove(file.path); err != nil {
		if !os.IsNotExist(err) {
			s.logger.Debug("Pipeline cache cleanup remove failed",
				slog.String("path", file.path),
				slog.String("err", err.Error()),
			)
			return false
		}
	}
	s.catalog.DeleteVariantByArtifactPath(file.path)
	s.clearVariantHit(file.path)
	s.publishVariantRemoved(ctx, file.path, reason)
	return true
}

func cleanupNamespace(path, root string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return ""
	}
	namespace, _, _ := strings.Cut(filepath.ToSlash(relative), "/")
	return strings.TrimSpace(namespace)
}

func (s *Service) effectiveLastUsed(path string, modTime time.Time) time.Time {
	lastHit, ok := s.variantHits.Get(path)
	return lo.Ternary(ok && lastHit.After(modTime), lastHit, modTime)
}

func (s *Service) clearVariantHit(path string) {
	s.variantHits.Delete(path)
}

func collectCleanupFiles(root string) ([]cleanupFile, error) {
	files := cxlist.NewList[cleanupFile]()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return oops.Wrapf(err, "read cleanup file info")
		}
		files.Add(cleanupFile{
			path:    path,
			size:    info.Size(),
			modTime: info.ModTime(),
		})
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, oops.Wrapf(err, "walk cleanup directory")
	}
	return files.Values(), nil
}
