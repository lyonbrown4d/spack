package server

import (
	"path/filepath"
	"strings"

	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
)

func catalogFileSourceRoot(cat catalog.Catalog) string {
	root := ""
	cat.AllAssets().Range(func(_ int, asset *catalog.Asset) bool {
		if asset != nil {
			root = commonCatalogFileRoot(root, asset.FullPath)
		}
		return true
	})
	cat.AllVariants().Range(func(_ int, variant *catalog.Variant) bool {
		if variant != nil {
			root = commonCatalogFileRoot(root, variant.ArtifactPath)
		}
		return true
	})
	return root
}

func commonCatalogFileRoot(root, filePath string) string {
	if strings.TrimSpace(filePath) == "" || spackbundle.IsReference(filePath) {
		return root
	}
	absolute, err := filepath.Abs(filepath.Clean(filePath))
	if err != nil {
		return root
	}
	dir := filepath.Dir(absolute)
	if root == "" {
		return dir
	}
	return commonDirectoryRoot(root, dir)
}

func commonDirectoryRoot(left, right string) string {
	candidate := filepath.Clean(left)
	for {
		if pathWithinDirectory(candidate, right) {
			return candidate
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return parent
		}
		candidate = parent
	}
}

func pathWithinDirectory(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return relative == "." || (!filepath.IsAbs(relative) && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}
