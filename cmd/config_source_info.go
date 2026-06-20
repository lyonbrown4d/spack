package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/lyonbrown4d/spack/internal/spackbundle"
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
		return "", nil, errors.New("assets.root is required")
	}
	resolved, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", nil, fmt.Errorf("resolve assets.root: %w", err)
	}
	info, err := os.Lstat(resolved)
	if err != nil {
		return "", nil, fmt.Errorf("stat assets.root: %w", err)
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return "", nil, fmt.Errorf("assets.root symlink is not allowed: %s", resolved)
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
	return "", nil, fmt.Errorf("assets.root must be an existing directory or readable .spack bundle: %s", root)
}

func effectiveBundleInfo(root string) (map[string]any, error) {
	index, err := spackbundle.ReadIndex(root)
	if err != nil {
		return nil, fmt.Errorf("read assets.root bundle index: %w", err)
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
	total := int64(0)
	for _, file := range index.Files {
		total += file.Size
	}
	return total
}

func redactedPath(path string, redact bool) string {
	if redact && strings.TrimSpace(path) != "" {
		return redactedPathValue
	}
	return path
}
