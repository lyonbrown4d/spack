package sourcecatalog

import (
	"context"
	"path/filepath"
	"strings"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	cxprefix "github.com/arcgolabs/collectionx/prefix"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/contentcoding"
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/samber/oops"
)

const (
	SourceSidecarStage     = "source_sidecar"
	BundleCompressionStage = "compression"
	BundleImageStage       = "image"
)

type Snapshot struct {
	Assets     *cxmapping.Map[string, *catalog.Asset]
	Variants   *cxmapping.Map[string, *catalog.Variant]
	TotalBytes int64
}

type Scanner struct {
	src           *source.LocalFS
	matchers      *cxlist.List[sidecarMatcher]
	matcherTrie   *cxprefix.Trie[sidecarMatcher]
	pathFilter    assetPathFilter
	sidecarPolicy sidecarTrustPolicy
}

type SidecarMatcher struct {
	Encoding string
	Suffix   string
}

type SidecarMatch struct {
	AssetPath string
	Encoding  string
	Suffix    string
}

func NewScanner(src *source.LocalFS, registry contentcoding.Registry) Scanner {
	return newScanner(src, registry, assetPathFilter{}, trustSourceSidecars)
}

func NewScannerWithAssets(src *source.LocalFS, registry contentcoding.Registry, assets *config.Assets) Scanner {
	return newScanner(src, registry, newAssetPathFilter(assets), trustSourceSidecars)
}

func NewCompilerScannerWithAssets(src *source.LocalFS, registry contentcoding.Registry, assets *config.Assets) Scanner {
	return newScanner(src, registry, newAssetPathFilter(assets), strictBundleSidecars)
}

func newScanner(
	src *source.LocalFS,
	registry contentcoding.Registry,
	pathFilter assetPathFilter,
	sidecarPolicy sidecarTrustPolicy,
) Scanner {
	matchers := buildSidecarMatchers(registry)
	return Scanner{
		src:           src,
		matchers:      matchers,
		matcherTrie:   buildSidecarMatcherTrie(matchers),
		pathFilter:    pathFilter,
		sidecarPolicy: sidecarPolicy,
	}
}

func (s Scanner) Scan(ctx context.Context) (Snapshot, error) {
	return s.ScanWithCatalog(ctx, nil)
}

func (s Scanner) FindFile(path string) (source.File, bool, error) {
	return s.findFileByPath(path)
}

func (s Scanner) MatchSidecarPath(path string) (SidecarMatch, bool) {
	normalized := normalizeSourcePath(path)
	if normalized == "" {
		return SidecarMatch{}, false
	}

	matcher, ok := matchSidecarWithTrie(normalized, s.matcherTrie).Get()
	if !ok {
		return SidecarMatch{}, false
	}

	assetPath := strings.TrimSuffix(normalized, matcher.suffix)
	if assetPath == "" || assetPath == normalized {
		return SidecarMatch{}, false
	}
	return SidecarMatch{
		AssetPath: assetPath,
		Encoding:  matcher.encoding,
		Suffix:    matcher.suffix,
	}, true
}

func (s Scanner) SidecarMatchers() *cxlist.List[SidecarMatcher] {
	return cxlist.MapList(s.matchers, func(_ int, matcher sidecarMatcher) SidecarMatcher {
		return SidecarMatcher{
			Encoding: matcher.encoding,
			Suffix:   matcher.suffix,
		}
	})
}

func (s Scanner) SourceStats() source.Stats {
	if s.src == nil {
		return source.Stats{Mode: source.SourceModeUnknown}
	}
	return s.src.Stats()
}

func (s Scanner) Watch(ctx context.Context) (<-chan source.ChangeEvent, error) {
	changes, err := s.src.Watch(ctx)
	if err != nil {
		return nil, oops.In("sourcecatalog").Owner("watch").Wrap(err)
	}
	return changes, nil
}

func (s Scanner) ScanWithCatalog(ctx context.Context, cat catalog.Catalog) (Snapshot, error) {
	ctx = normalizeScanContext(ctx)
	if err := s.pathFilter.Err(); err != nil {
		return Snapshot{}, oops.In("sourcecatalog").Owner("scan filter").Wrap(err)
	}

	filesByPath, totalBytes, err := s.collectSourceFiles(ctx)
	if err != nil {
		return Snapshot{}, err
	}

	existing := buildExistingScanState(cat)
	sidecars := recognizeSidecars(filesByPath, s.matchers)
	assets, err := buildAssets(ctx, filesByPath, sidecars, existing.assets)
	if err != nil {
		return Snapshot{}, err
	}
	variants, err := buildSidecarVariants(ctx, sidecars, assets, existing.sidecars, s.sidecarPolicy)
	if err != nil {
		return Snapshot{}, err
	}
	buildExplicitBundleVariants(filesByPath, assets).Range(func(key string, variant *catalog.Variant) bool {
		variants.Set(key, variant)
		return true
	})

	return Snapshot{
		Assets:     assets,
		Variants:   variants,
		TotalBytes: totalBytes,
	}, nil
}

func (s Scanner) collectSourceFiles(ctx context.Context) (*cxmapping.Map[string, source.File], int64, error) {
	scanErr := oops.In("sourcecatalog").Owner("scan")
	filesByPath := cxmapping.NewMap[string, source.File]()
	totalBytes := int64(0)

	if err := s.src.Walk(func(file source.File) error {
		if err := scanContextErr(ctx); err != nil {
			return err
		}
		if file.IsDir {
			return nil
		}
		allowed, filterErr := s.pathFilter.Allow(file.Path)
		if filterErr != nil {
			return filterErr
		}
		if !allowed {
			return nil
		}
		filesByPath.Set(file.Path, file)
		totalBytes += file.Size
		return nil
	}); err != nil {
		return nil, 0, scanErr.Wrap(err)
	}

	return filesByPath, totalBytes, nil
}

func (s Scanner) findFileByPath(path string) (source.File, bool, error) {
	normalized := normalizeSourcePath(path)
	if normalized == "" {
		return source.File{}, false, nil
	}
	allowed, filterErr := s.pathFilter.Allow(normalized)
	if filterErr != nil {
		return source.File{}, false, oops.In("sourcecatalog").Owner("source filter").Wrap(filterErr)
	}
	if !allowed {
		return source.File{}, false, nil
	}

	file, found, err := s.src.FindFile(normalized)
	if err != nil {
		return source.File{}, false, oops.In("sourcecatalog").Owner("source lookup").Wrap(err)
	}
	return file, found, nil
}

func normalizeSourcePath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	return strings.TrimPrefix(filepath.ToSlash(filepath.Clean(trimmed)), "/")
}

func normalizeScanContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func scanContextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return oops.In("sourcecatalog").Owner("scan context").Wrap(err)
	}
	return nil
}
