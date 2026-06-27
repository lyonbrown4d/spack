package source

import (
	"fmt"
	"path/filepath"

	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/samber/oops"
)

type bundleSource struct {
	path    string
	root    string
	index   spackbundle.Index
	entries *cxmapping.Map[string, spackbundle.IndexFile]
}

func newBundleSource(bundlePath, extractedRoot string, index spackbundle.Index) (*bundleSource, error) {
	entries := cxmapping.NewMapWithCapacity[string, spackbundle.IndexFile](len(index.Files))
	for fileIndex := range index.Files {
		file := index.Files[fileIndex]
		filePath, ok := cleanRelativeAssetPath(file.Path)
		if !ok {
			return nil, fmt.Errorf("invalid bundle index path %q", file.Path)
		}
		file.Path = filePath
		entries.Set(filePath, file)
	}
	return &bundleSource{
		path:    bundlePath,
		root:    extractedRoot,
		index:   index,
		entries: entries,
	}, nil
}

func (s *LocalFS) walkBundle(walkFn func(File) error) error {
	for indexFile := range s.bundle.index.Files {
		entry := s.bundle.index.Files[indexFile]
		file, err := s.bundleFile(entry)
		if err != nil {
			return err
		}
		if err := walkFn(file); err != nil {
			return err
		}
	}
	return nil
}

func (s *LocalFS) findBundleFile(relativePath string) (File, bool, error) {
	entry, ok := s.bundle.entries.GetOption(relativePath).Get()
	if !ok {
		return File{}, false, nil
	}
	file, err := s.bundleFile(entry)
	if err != nil {
		return File{}, false, err
	}
	return file, true, nil
}

func (s *LocalFS) bundleFile(entry spackbundle.IndexFile) (File, error) {
	relativePath, ok := cleanRelativeAssetPath(entry.Path)
	if !ok {
		return File{}, oops.Errorf("invalid bundle index path %q", entry.Path)
	}
	return File{
		Path:       relativePath,
		FullPath:   filepath.Join(s.bundle.root, filepath.FromSlash(relativePath)),
		Kind:       entry.Kind,
		Size:       entry.Size,
		ModTime:    s.bundle.index.CreatedAt,
		MediaType:  entry.MediaType,
		SourceHash: entry.SourceHash,
		ETag:       entry.ETag,
		AssetPath:  entry.AssetPath,
		Encoding:   entry.Encoding,
		Format:     entry.Format,
		Width:      entry.Width,
	}, nil
}
