package task

import (
	"strings"

	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/samber/oops"
)

func (r *sourceRescanRun) buildIncrementalChangeSet() (*sourceRescanChangeSet, error) {
	changeSet := newSourceRescanChangeSet()

	var buildErr error
	r.changes.Range(func(_ int, change source.ChangeEvent) bool {
		if change.FullRescan {
			buildErr = errFullSourceRescanRequired
			return false
		}
		if err := r.addChangeToIncrementalSet(changeSet, strings.TrimSpace(change.Path), change.Op); err != nil {
			buildErr = err
			return false
		}
		return true
	})
	if buildErr != nil {
		return nil, buildErr
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

func classifyChange(rawOp string) int {
	op := strings.TrimSpace(strings.ToUpper(rawOp))
	switch {
	case op == "":
		return sourceChangeUnknown
	case strings.Contains(op, "RENAME") || strings.Contains(op, "MOVE"):
		return sourceChangeFallback
	case strings.Contains(op, "CREATE") && strings.Contains(op, "REMOVE"):
		return sourceChangeFallback
	case strings.Contains(op, "CREATE") || strings.Contains(op, "WRITE"):
		return sourceChangeWrite
	case strings.Contains(op, "REMOVE"):
		return sourceChangeDelete
	default:
		return sourceChangeUnknown
	}
}

func (r *sourceRescanRun) addChangeToIncrementalSet(changeSet *sourceRescanChangeSet, rawPath, rawOp string) error {
	normalizedPath := normalizeChangePath(rawPath)
	if normalizedPath == "" {
		return nil
	}

	switch classifyChange(rawOp) {
	case sourceChangeUnknown:
		return nil
	case sourceChangeFallback:
		return errFullSourceRescanRequired
	case sourceChangeDelete:
		r.recordAssetDelete(changeSet, normalizedPath)
	case sourceChangeWrite:
		return r.recordAssetWrite(changeSet, normalizedPath)
	default:
		return nil
	}
	return nil
}

func (r *sourceRescanRun) recordAssetDelete(changeSet *sourceRescanChangeSet, normalizedPath string) {
	match, isSidecar := r.scanner.MatchSidecarPath(normalizedPath)
	if !isSidecar {
		changeSet.deletedAssets.Add(normalizedPath)
		changeSet.touchedAssetPaths.Add(normalizedPath)
		changeSet.changedAssets.Delete(normalizedPath)
		return
	}
	changeSet.deletedSourceSidecars.Set(normalizedPath, sourceRescanDeletedSidecar{
		match: match,
		path:  normalizedPath,
	})
	if match.AssetPath != "" {
		changeSet.touchedAssetPaths.Add(match.AssetPath)
	}
}

func (r *sourceRescanRun) recordAssetWrite(changeSet *sourceRescanChangeSet, normalizedPath string) error {
	match, isSidecar := r.scanner.MatchSidecarPath(normalizedPath)
	if isSidecar {
		if _, exists := changeSet.changedSourceSidecars.Get(normalizedPath); exists {
			return nil
		}
	} else if _, exists := changeSet.changedAssets.Get(normalizedPath); exists {
		return nil
	}

	file, found, findErr := r.scanner.FindFile(normalizedPath)
	if findErr != nil {
		return oops.In("task").Owner("source rescan").Wrap(findErr)
	}
	if !found {
		return nil
	}

	changeSet.touchedAssetPaths.Add(normalizedPath)
	if isSidecar {
		changeSet.changedAssetMatchPath(match, normalizedPath, file)
		return nil
	}
	changeSet.changedAssets.Set(normalizedPath, file)
	return nil
}
