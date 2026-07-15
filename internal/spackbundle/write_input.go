package spackbundle

import (
	"cmp"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	cxset "github.com/arcgolabs/collectionx/set"
	"github.com/samber/oops"
)

func normalizedOutputPath(output string) (string, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return "", oops.In("spackbundle").Owner("write").Wrap(errors.New("bundle output path is required"))
	}
	absolute, err := filepath.Abs(filepath.Clean(output))
	if err != nil {
		return "", oops.Wrapf(err, "resolve bundle output path")
	}
	return absolute, nil
}

func normalizedRootPath(root string) (string, fs.FileInfo, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", nil, oops.In("spackbundle").Owner("write").Wrap(errors.New("bundle root path is required"))
	}
	absolute, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", nil, oops.Wrapf(err, "resolve bundle root path")
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return "", nil, oops.Wrapf(err, "stat bundle root")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", nil, oops.Errorf("bundle root must not be a symlink: %s", absolute)
	}
	if !info.IsDir() {
		return "", nil, oops.Errorf("bundle root must be a directory: %s", absolute)
	}
	return absolute, info, nil
}

func normalizeFiles(root string, rootInfo fs.FileInfo, files []File) ([]File, error) {
	normalized := make([]File, 0, len(files))
	seen := cxset.NewSetWithCapacity[string](len(files))
	for index := range files {
		file, err := normalizeFile(root, rootInfo, files[index], seen)
		if err != nil {
			return nil, err
		}
		normalized = append(normalized, file)
	}
	slices.SortFunc(normalized, func(left, right File) int {
		return cmp.Compare(left.Path, right.Path)
	})
	return normalized, nil
}

func normalizeFile(root string, rootInfo fs.FileInfo, file File, seen *cxset.Set[string]) (File, error) {
	cleanPath, err := cleanBundlePath(file.Path)
	if err != nil {
		return File{}, err
	}
	if seen.Contains(cleanPath) {
		return File{}, oops.Errorf("bundle path %q is duplicated", cleanPath)
	}
	fullPath, relativePath, info, err := statBundleFile(root, file)
	if err != nil {
		return File{}, err
	}
	file.Path = cleanPath
	file.FullPath = fullPath
	file.Size = info.Size()
	file.root = root
	file.rootInfo = rootInfo
	file.rootRelativePath = relativePath
	seen.Add(cleanPath)
	return file, nil
}

func statBundleFile(root string, file File) (string, string, os.FileInfo, error) {
	fullPath, err := filepath.Abs(filepath.Clean(file.FullPath))
	if err != nil {
		return "", "", nil, oops.Wrapf(err, "resolve bundle file %q", file.Path)
	}
	if file.AllowExternal {
		info, err := statExternalBundleFile(fullPath, file.Path)
		return fullPath, "", info, err
	}
	return statRootBundleFile(root, fullPath, file.Path)
}

func statExternalBundleFile(fullPath, bundlePath string) (os.FileInfo, error) {
	info, err := os.Lstat(fullPath)
	if err != nil {
		return nil, oops.Wrapf(err, "stat bundle file %q", bundlePath)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, oops.Errorf("bundle file %q is a symlink", bundlePath)
	}
	if info.IsDir() {
		return nil, oops.Errorf("bundle file %q is a directory", bundlePath)
	}
	return info, nil
}

func statRootBundleFile(root, fullPath, bundlePath string) (string, string, os.FileInfo, error) {
	relativePath, err := filepath.Rel(root, fullPath)
	if err != nil || isPathOutsideRoot(relativePath) {
		return "", "", nil, oops.Errorf("bundle file %q escapes root", bundlePath)
	}
	relativePath = filepath.ToSlash(relativePath)
	rootDir, err := os.OpenRoot(root)
	if err != nil {
		return "", "", nil, oops.Wrapf(err, "open bundle root")
	}
	defer discardBundleRoot(rootDir)
	info, err := lstatBundlePathWithinRoot(rootDir, root, relativePath)
	if err != nil {
		return "", "", nil, err
	}
	if info.IsDir() {
		return "", "", nil, oops.Errorf("bundle file %q is a directory", bundlePath)
	}
	return fullPath, relativePath, info, nil
}

func lstatBundlePathWithinRoot(rootDir *os.Root, root, relativePath string) (fs.FileInfo, error) {
	currentPath := ""
	var info fs.FileInfo
	for segment := range strings.SplitSeq(relativePath, "/") {
		if currentPath == "" {
			currentPath = segment
		} else {
			currentPath = path.Join(currentPath, segment)
		}
		var err error
		info, err = rootDir.Lstat(filepath.FromSlash(currentPath))
		if err != nil {
			return nil, oops.Wrapf(err, "stat bundle file %q", filepath.Join(root, filepath.FromSlash(currentPath)))
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, oops.Errorf("bundle file %q is a symlink", filepath.Join(root, filepath.FromSlash(currentPath)))
		}
	}
	return info, nil
}

func isPathOutsideRoot(relativePath string) bool {
	return relativePath == "." || filepath.IsAbs(relativePath) || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator))
}

func discardBundleRoot(rootDir *os.Root) {
	if rootDir == nil {
		return
	}
	if err := rootDir.Close(); err != nil {
		return
	}
}
