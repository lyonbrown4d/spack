package source

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/samber/oops"
)

type Type string

const (
	TypeDirectory Type = "directory"
	TypeBundle    Type = "bundle"
)

type Resolved struct {
	ConfiguredRoot string
	Root           string
	Type           Type
	Info           fs.FileInfo
	Bundle         *BundleMetadata
}

type BundleMetadata struct {
	FormatVersion       string
	IndexKind           string
	CreatedAt           time.Time
	FileCount           int
	AssetCount          int
	SourceSidecarCount  int
	CompressedFileCount int
	TotalBytes          int64
}

type Resolver struct{}

func NewResolver() *Resolver {
	return &Resolver{}
}

func (r *Resolver) Resolve(root string) (Resolved, error) {
	configuredRoot := strings.TrimSpace(root)
	if configuredRoot == "" {
		return Resolved{}, oops.Owner("source").Wrap(errors.New("assets root is required"))
	}

	resolvedRoot, err := filepath.Abs(filepath.Clean(configuredRoot))
	if err != nil {
		return Resolved{}, oops.Wrapf(err, "resolve assets.root")
	}

	info, err := os.Lstat(resolvedRoot)
	if err != nil {
		return Resolved{}, oops.Wrapf(err, "stat assets.root")
	}
	if isSymlink(info) {
		return Resolved{}, oops.Owner("source").Wrap(fmt.Errorf("%w: %s", ErrSymlinkNotAllowed, resolvedRoot))
	}
	if info.IsDir() {
		return Resolved{
			ConfiguredRoot: configuredRoot,
			Root:           resolvedRoot,
			Type:           TypeDirectory,
			Info:           info,
		}, nil
	}
	if info.Mode().IsRegular() && spackbundle.IsBundlePath(resolvedRoot) {
		bundle, err := resolveBundleMetadata(resolvedRoot)
		if err != nil {
			return Resolved{}, err
		}
		return Resolved{
			ConfiguredRoot: configuredRoot,
			Root:           resolvedRoot,
			Type:           TypeBundle,
			Info:           info,
			Bundle:         &bundle,
		}, nil
	}
	return Resolved{}, oops.Owner("source").Wrap(fmt.Errorf("assets root must be an existing directory or readable .spack bundle: %s", resolvedRoot))
}

func resolveBundleMetadata(root string) (BundleMetadata, error) {
	reader, err := spackbundle.OpenReader(root)
	if err != nil {
		return BundleMetadata{}, oops.Owner("source").Wrap(oops.Wrapf(err, "read assets.root bundle index"))
	}
	index, err := reader.Index()
	closeErr := reader.Close()
	if err != nil {
		return BundleMetadata{}, oops.Owner("source").Wrap(oops.Wrapf(err, "read assets.root bundle index"))
	}
	if closeErr != nil {
		return BundleMetadata{}, oops.Owner("source").Wrap(oops.Wrapf(closeErr, "close assets.root bundle"))
	}
	return summarizeBundleMetadata(index), nil
}

func summarizeBundleMetadata(index spackbundle.Index) BundleMetadata {
	summary := BundleMetadata{
		FormatVersion: index.APIVersion,
		IndexKind:     index.Kind,
		CreatedAt:     index.CreatedAt,
		FileCount:     len(index.Files),
	}
	for fileIndex := range index.Files {
		file := index.Files[fileIndex]
		summary.TotalBytes += file.Size
		if file.Kind == "asset" || file.Kind == "" {
			summary.AssetCount++
		}
		if file.Kind == "source_sidecar" {
			summary.SourceSidecarCount++
		}
		if strings.TrimSpace(file.Encoding) != "" {
			summary.CompressedFileCount++
		}
	}
	return summary
}
