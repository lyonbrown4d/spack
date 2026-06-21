package source

import (
	"fmt"
	"strings"

	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/samber/oops"
)

type bundleSource struct {
	path    string
	index   spackbundle.Index
	entries map[string]spackbundle.IndexFile
}

func newBundleSource(root string) (*bundleSource, error) {
	index, err := spackbundle.ReadIndex(root)
	if err != nil {
		return nil, fmt.Errorf("read source bundle index: %w", err)
	}
	entries := make(map[string]spackbundle.IndexFile, len(index.Files))
	for indexFile := range index.Files {
		file := index.Files[indexFile]
		if strings.TrimSpace(file.Path) == "" {
			continue
		}
		entries[file.Path] = file
	}
	return &bundleSource{
		path:    root,
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
	entry, ok := s.bundle.entries[relativePath]
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
	reference, err := spackbundle.NewReference(s.bundle.path, entry.Path)
	if err != nil {
		return File{}, oops.Wrap(err)
	}
	return File{
		Path:       entry.Path,
		FullPath:   reference,
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
