package sourcecatalog

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/daiyuang/spack/internal/catalog"
	"github.com/daiyuang/spack/internal/contentcoding"
	"github.com/daiyuang/spack/internal/source"
	"github.com/samber/oops"
)

const SourceSidecarStage = "source_sidecar"

type Snapshot struct {
	Assets     *cxmapping.Map[string, *catalog.Asset]
	Variants   *cxmapping.Map[string, *catalog.Variant]
	TotalBytes int64
}

type Scanner struct {
	src      source.Source
	matchers *cxlist.List[sidecarMatcher]
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

func NewScanner(src source.Source, registry contentcoding.Registry) Scanner {
	return Scanner{
		src:      src,
		matchers: buildSidecarMatchers(registry),
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

	match := SidecarMatch{}
	found := false
	s.matchers.Range(func(_ int, matcher sidecarMatcher) bool {
		if !strings.HasSuffix(normalized, matcher.suffix) {
			return true
		}
		assetPath := strings.TrimSuffix(normalized, matcher.suffix)
		if assetPath == "" || assetPath == normalized {
			return true
		}
		match = SidecarMatch{
			AssetPath: assetPath,
			Encoding:  matcher.encoding,
			Suffix:    matcher.suffix,
		}
		found = true
		return false
	})
	return match, found
}

func (s Scanner) SidecarMatchers() *cxlist.List[SidecarMatcher] {
	return cxlist.MapList(s.matchers, func(_ int, matcher sidecarMatcher) SidecarMatcher {
		return SidecarMatcher{
			Encoding: matcher.encoding,
			Suffix:   matcher.suffix,
		}
	})
}

func (s Scanner) Watch(ctx context.Context) (<-chan source.ChangeEvent, error) {
	watcher, ok := s.src.(source.Watcher)
	if !ok {
		return nil, source.ErrWatchUnsupported
	}
	changes, err := watcher.Watch(ctx)
	if err != nil {
		return nil, oops.In("sourcecatalog").Owner("watch").Wrap(err)
	}
	return changes, nil
}

func (s Scanner) ScanWithCatalog(ctx context.Context, cat catalog.Catalog) (Snapshot, error) {
	ctx = normalizeScanContext(ctx)

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
	variants, err := buildSidecarVariants(ctx, sidecars, assets, existing.sidecars)
	if err != nil {
		return Snapshot{}, err
	}

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
	if finder, ok := s.src.(source.FileFinder); ok {
		file, found, err := finder.FindFile(normalized)
		if err != nil {
			return source.File{}, false, err
		}
		if found {
			return file, true, nil
		}
	}

	if normalized == "" {
		return source.File{}, false, nil
	}

	lookupErr := errors.New("sourcecatalog lookup complete")
	var foundFile source.File
	var found bool
	normalizedFull := filepath.Clean(path)

	if err := s.src.Walk(func(file source.File) error {
		if file.Path == normalized || file.FullPath == normalizedFull {
			foundFile = file
			found = true
			return lookupErr
		}
		return nil
	}); err != nil && !errors.Is(err, lookupErr) {
		return source.File{}, false, oops.In("sourcecatalog").Owner("source lookup").Wrap(err)
	}
	return foundFile, found, nil
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
