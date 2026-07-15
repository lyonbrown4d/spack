// Package pipeline plans and executes derived asset generation.
package pipeline

import (
	"errors"
	"fmt"
	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	cxset "github.com/arcgolabs/collectionx/set"
	"github.com/lyonbrown4d/spack/internal/artifact"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/contentcoding"
	contentcodingspec "github.com/lyonbrown4d/spack/internal/contentcoding/spec"
	"github.com/lyonbrown4d/spack/internal/media"
	"github.com/samber/oops"
	"os"
	"time"
)

type compressionStage struct {
	cfg        *config.Compression
	store      artifact.Store
	catalog    catalog.Catalog
	strategies contentcoding.Registry
}

func newCompressionStage(
	cfg *config.Compression,
	registry contentcoding.Registry,
	store artifact.Store,
	cat catalog.Catalog,
) *compressionStage {
	return &compressionStage{
		cfg:        cfg,
		store:      store,
		catalog:    cat,
		strategies: registry,
	}
}

func (s *compressionStage) Name() string {
	return "compression"
}

func (s *compressionStage) Plan(asset *catalog.Asset, request Request) *cxlist.List[Task] {
	if !s.cfg.PipelineEnabled() || !isCompressible(asset) {
		return nil
	}

	supportedEncodings := s.strategies.Names()
	encodings := filterConfiguredEncodings(normalizeEncodings(request.PreferredEncodings), supportedEncodings)
	if encodings == nil || encodings.IsEmpty() {
		encodings = supportedEncodings
	}

	return cxlist.FilterMapList[string, Task](encodings, func(_ int, encoding string) (Task, bool) {
		variant, ok := s.catalog.FindEncodingVariant(asset.Path, encoding)
		if ok && hasEncodingVariant(variant, asset.SourceHash, encoding) {
			return Task{}, false
		}
		return Task{
			AssetPath: asset.Path,
			Encoding:  encoding,
		}, true
	})
}

func (s *compressionStage) Execute(task Task, asset *catalog.Asset) (*catalog.Variant, error) {
	stageErr := oops.In("pipeline").Owner("compression stage").
		With("asset_path", asset.Path).
		With("encoding", task.Encoding)
	if asset.SourceHash == "" {
		return nil, stageErr.Wrap(errors.New("asset missing source hash"))
	}

	raw, err := os.ReadFile(asset.FullPath)
	if err != nil {
		return nil, stageErr.Wrap(err)
	}
	if int64(len(raw)) < s.cfg.MinSize {
		return nil, ErrVariantSkipped
	}

	compressed, suffix, err := s.compress(raw, task.Encoding)
	if err != nil {
		return nil, stageErr.Wrap(err)
	}
	if len(compressed) >= len(raw) {
		return nil, ErrVariantSkipped
	}

	targetPath, err := s.store.PathFor(asset.Path, asset.SourceHash, "encoding", suffix)
	if err != nil {
		return nil, stageErr.Wrap(err)
	}
	if err := s.store.Write(targetPath, compressed); err != nil {
		return nil, stageErr.With("artifact_path", targetPath).Wrap(err)
	}

	return &catalog.Variant{
		ID:           asset.Path + suffix,
		AssetPath:    asset.Path,
		ArtifactPath: targetPath,
		Size:         int64(len(compressed)),
		MediaType:    asset.MediaType,
		SourceHash:   asset.SourceHash,
		ETag:         fmt.Sprintf("\"%s-%s\"", asset.SourceHash, task.Encoding),
		Encoding:     task.Encoding,
		Metadata: catalog.MetadataWithModTime(cxmapping.NewMapFrom(map[string]string{
			"stage": "compression",
		}), time.Now()),
	}, nil
}

func (s *compressionStage) compress(raw []byte, encoding string) ([]byte, string, error) {
	strategy, ok := s.strategies.Lookup(encoding)
	if !ok {
		return nil, "", oops.In("pipeline").Owner("compression strategy").With("encoding", encoding).
			Wrap(errors.New("unsupported compression encoding"))
	}

	compressed, err := strategy.Compress(raw)
	if err != nil {
		return nil, "", oops.In("pipeline").Owner("compression strategy").With("encoding", encoding).Wrap(err)
	}
	return compressed, strategy.Suffix(), nil
}

func hasEncodingVariant(variant *catalog.Variant, sourceHash, encoding string) bool {
	if variant == nil || variant.Encoding != encoding {
		return false
	}
	if sourceHash != "" && variant.SourceHash != "" && variant.SourceHash != sourceHash {
		return false
	}
	if variant.ArtifactPath == "" {
		return false
	}
	_, err := os.Stat(variant.ArtifactPath)
	return err == nil
}

func isCompressible(asset *catalog.Asset) bool {
	return media.IsCompressibleMediaType(asset.MediaType)
}

func normalizeEncodings(encodings *cxlist.List[string]) *cxlist.List[string] {
	if encodings == nil {
		return nil
	}
	return contentcodingspec.NormalizeNames(encodings)
}

func filterConfiguredEncodings(encodings, supported *cxlist.List[string]) *cxlist.List[string] {
	if encodings == nil || supported == nil {
		return nil
	}
	supportedSet := cxset.NewOrderedSet[string](supported.Values()...)
	return encodings.Where(func(_ int, encoding string) bool {
		return supportedSet.Contains(encoding)
	})
}
