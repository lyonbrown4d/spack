package artifact

import (
	"path"
	"path/filepath"
	"strings"

	spackpkg "github.com/lyonbrown4d/spack/pkg"
	"github.com/samber/oops"
)

func artifactRelativePath(assetPath, sourceHash, namespace, suffix string) (string, error) {
	cleanAssetPath, err := cleanArtifactAssetPath(assetPath)
	if err != nil {
		return "", err
	}
	namespace, err = cleanArtifactSegment(namespace, "namespace")
	if err != nil {
		return "", err
	}
	sourceHash, err = cleanArtifactSegment(sourceHash, "source hash")
	if err != nil {
		return "", err
	}
	if err := validateArtifactSuffix(suffix); err != nil {
		return "", err
	}
	return path.Join(namespace, sourceHash, cleanAssetPath+suffix), nil
}

func cleanArtifactAssetPath(raw string) (string, error) {
	trimmed := strings.Trim(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return "index", nil
	}
	if strings.ContainsRune(trimmed, '\x00') || strings.ContainsRune(trimmed, '\\') || filepath.IsAbs(trimmed) || path.IsAbs(trimmed) {
		return "", oops.In("artifact").Owner("store").With("asset_path", raw).Wrap(ErrUnsafePath)
	}
	cleaned := path.Clean(trimmed)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || spackpkg.HasUnsafePortablePathSegment(cleaned) {
		return "", oops.In("artifact").Owner("store").With("asset_path", raw).Wrap(ErrUnsafePath)
	}
	return cleaned, nil
}

func cleanArtifactSegment(raw, label string) (string, error) {
	segment := strings.TrimSpace(raw)
	if segment == "" || strings.ContainsAny(segment, "/\\") || strings.ContainsRune(segment, '\x00') || spackpkg.IsUnsafePortablePathSegment(segment) {
		return "", oops.In("artifact").Owner("store").With(label, raw).Wrap(ErrUnsafePath)
	}
	return segment, nil
}

func validateArtifactSuffix(suffix string) error {
	if strings.ContainsAny(suffix, "/\\") || strings.ContainsRune(suffix, '\x00') || spackpkg.HasUnsafePortablePathSegment(suffix) {
		return oops.In("artifact").Owner("store").With("suffix", suffix).Wrap(ErrUnsafePath)
	}
	return nil
}

func unsafeArtifactFilesystemPath(relativePath string) bool {
	return relativePath == "." || filepath.IsAbs(relativePath) || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator))
}

func unsafeArtifactSlashPath(relativePath string) bool {
	return relativePath == "." || relativePath == ".." || strings.HasPrefix(relativePath, "../") || spackpkg.HasUnsafePortablePathSegment(relativePath)
}
