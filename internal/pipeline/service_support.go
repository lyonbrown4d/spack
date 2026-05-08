package pipeline

import (
	"cmp"
	"context"
	"fmt"

	cxlist "github.com/arcgolabs/collectionx/list"
	appEvent "github.com/daiyuang/spack/internal/event"
	"github.com/samber/lo"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func requestKey(request Request) string {
	assetPath := strings.TrimSpace(request.AssetPath)
	encodings := normalizeRequestStrings(request.PreferredEncodings)
	formats := normalizeRequestStrings(request.PreferredFormats)
	widths := normalizeRequestInts(request.PreferredWidths)
	return buildRequestKey(assetPath, encodings, formats, widths)
}

func normalizeRequestStrings(values *cxlist.List[string]) *cxlist.List[string] {
	if values == nil || values.IsEmpty() {
		return nil
	}

	normalized := cxlist.MapList[string, string](values, func(_ int, value string) string {
		return strings.ToLower(strings.TrimSpace(value))
	}).Where(func(_ int, value string) bool {
		return value != ""
	})
	normalized = cxlist.NewList[string](lo.Uniq[string](normalized.Values())...)
	if normalized.IsEmpty() {
		return nil
	}
	return normalized.Sort(strings.Compare)
}

func normalizeRequestInts(values *cxlist.List[int]) *cxlist.List[int] {
	if values == nil || values.IsEmpty() {
		return nil
	}

	normalized := values.Where(func(_ int, value int) bool {
		return value > 0
	})
	normalized = cxlist.NewList[int](lo.Uniq[int](normalized.Values())...)
	if normalized.IsEmpty() {
		return nil
	}
	return normalized.Sort(cmp.Compare[int])
}

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

func (s *Service) cleanupLoop(ctx context.Context, interval time.Duration) {
	defer close(s.cleanupDone)
	s.cleanupOnce(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.cleanupStop:
			return
		case <-ticker.C:
			s.cleanupOnce(ctx)
		}
	}
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

func (s *Service) enforceCleanupCacheLimit(ctx context.Context, files []cleanupFile, result *cleanupResult) {
	maxCacheBytes := int64(0)
	if s.artifactPolicy != nil {
		maxCacheBytes = s.artifactPolicy.MaxCacheBytes()
	}
	if maxCacheBytes <= 0 || result.totalBytes <= maxCacheBytes {
		return
	}

	pq, err := cxlist.NewPriorityQueue(func(left, right cleanupFile) bool {
		return left.lastUsed.Before(right.lastUsed)
	}, files...)
	if err != nil {
		return
	}

	for {
		if result.totalBytes <= maxCacheBytes {
			return
		}
		file, ok := pq.Pop()
		if !ok {
			return
		}
		if !s.removeCleanupFile(ctx, file, appEvent.VariantRemovalReasonSize) {
			continue
		}
		recordSizeCleanupRemoval(result, file.size)
	}
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
			return fmt.Errorf("read cleanup file info: %w", err)
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
		return nil, fmt.Errorf("walk cleanup directory: %w", err)
	}
	return files.Values(), nil
}
