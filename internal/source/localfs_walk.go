package source

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/charlievieth/fastwalk"
	"github.com/samber/oops"
)

func (s *LocalFS) walkDirectory() ([]File, error) {
	files := cxlist.NewConcurrentListWithCapacity[File](128)
	conf := fastwalk.Config{
		Follow: false,
		Sort:   fastwalk.SortDirsFirst,
	}
	if err := fastwalk.Walk(&conf, s.root, func(walkPath string, entry fs.DirEntry, walkErr error) error {
		file, fileErr := buildFastwalkFile(s.root, walkPath, entry, walkErr)
		if fileErr != nil {
			return fileErr
		}
		files.Add(file)
		return nil
	}); err != nil {
		return nil, oops.Wrap(err)
	}
	if err := s.validateRoot(); err != nil {
		return nil, err
	}
	return files.Snapshot().Sort(func(left, right File) int {
		return strings.Compare(left.Path, right.Path)
	}).Values(), nil
}

func buildFastwalkFile(root, walkPath string, entry fs.DirEntry, walkErr error) (File, error) {
	if walkErr != nil {
		return File{}, oops.Wrap(walkErr)
	}
	relativePath, relErr := cleanFastwalkRelativePath(root, walkPath)
	if relErr != nil {
		return File{}, relErr
	}
	fullPath := filepath.Join(root, filepath.FromSlash(relativePath))
	if entry.Type()&fs.ModeSymlink != 0 {
		return File{}, oops.Owner("source").Wrap(fmt.Errorf("%w: %s", ErrSymlinkNotAllowed, fullPath))
	}

	info, err := entry.Info()
	if err != nil {
		return File{}, oops.Wrap(err)
	}

	return File{
		Path:     relativePath,
		FullPath: fullPath,
		Size:     info.Size(),
		IsDir:    entry.IsDir(),
		ModTime:  info.ModTime(),
	}, nil
}

func cleanFastwalkRelativePath(root, walkPath string) (string, error) {
	relativePath, err := filepath.Rel(root, walkPath)
	if err != nil {
		return "", oops.Wrap(err)
	}
	if relativePath == "." {
		return ".", nil
	}
	if filepath.IsAbs(relativePath) ||
		relativePath == ".." ||
		strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return "", oops.Owner("source").Wrap(fmt.Errorf("source path escaped root: %s", walkPath))
	}
	cleaned := filepath.ToSlash(filepath.Clean(relativePath))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", oops.Owner("source").Wrap(fmt.Errorf("source path escaped root: %s", walkPath))
	}
	return cleaned, nil
}
