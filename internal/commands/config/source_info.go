package configcmd

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/samber/lo"
	"github.com/samber/oops"
)

const redactedPathValue = "REDACTED"

func effectiveSourceInfo(root string, redact bool) (map[string]any, error) {
	resolved, info, err := statEffectiveSourceRoot(root)
	if err != nil {
		return nil, err
	}

	sourceType, bundle, err := effectiveSourceTypeAndBundle(resolved, info)
	if err != nil {
		return nil, err
	}

	out := map[string]any{
		"root":          redactedPath(root, redact),
		"root_resolved": redactedPath(resolved, redact),
		"type":          sourceType,
	}
	if bundle != nil {
		out["bundle"] = bundle
	}
	return out, nil
}

func statEffectiveSourceRoot(root string) (string, fs.FileInfo, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", nil, oops.In("config").Owner("source info").Wrap(errors.New("assets.root is required"))
	}
	resolved, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", nil, oops.Wrapf(err, "resolve assets.root")
	}
	info, err := os.Lstat(resolved)
	if err != nil {
		return "", nil, oops.Wrapf(err, "stat assets.root")
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return "", nil, oops.Errorf("assets.root symlink is not allowed: %s", resolved)
	}
	return resolved, info, nil
}

func effectiveSourceTypeAndBundle(root string, info fs.FileInfo) (string, map[string]any, error) {
	if info.IsDir() {
		return "directory", nil, nil
	}
	if info.Mode().IsRegular() && spackbundle.IsBundlePath(root) {
		bundle, err := effectiveBundleInfo(root)
		if err != nil {
			return "", nil, err
		}
		return "bundle", bundle, nil
	}
	return "", nil, oops.Errorf("assets.root must be an existing directory or readable .spack bundle: %s", root)
}

func effectiveBundleInfo(root string) (map[string]any, error) {
	reader, err := spackbundle.OpenReader(root)
	if err != nil {
		return nil, oops.Wrapf(err, "open assets.root bundle")
	}
	index, err := reader.Index()
	closeErr := reader.Close()
	if err != nil {
		return nil, oops.Wrapf(err, "read assets.root bundle index")
	}
	if closeErr != nil {
		return nil, oops.Wrapf(closeErr, "close assets.root bundle")
	}
	return map[string]any{
		"format_version": index.APIVersion,
		"index_kind":     index.Kind,
		"created_at":     index.CreatedAt,
		"file_count":     len(index.Files),
		"total_bytes":    bundleIndexTotalBytes(index),
	}, nil
}

func bundleIndexTotalBytes(index spackbundle.Index) int64 {
	return lo.SumBy(index.Files, func(file spackbundle.IndexFile) int64 {
		return file.Size
	})
}

func redactedPath(path string, redact bool) string {
	if redact && strings.TrimSpace(path) != "" {
		return redactedPathValue
	}
	return path
}
