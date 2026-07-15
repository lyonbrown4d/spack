package source

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"
)

// LocalRootGuard binds final file opens to the configured local source root.
// It rejects symlinks, root replacement, and paths outside the original root.
type LocalRootGuard struct {
	root string
	info fs.FileInfo
}

func NewLocalRootGuard(root string) (*LocalRootGuard, bool, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, false, nil
	}
	absolute, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return nil, false, oops.Wrapf(err, "resolve local source root")
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return nil, false, oops.Wrapf(err, "stat local source root")
	}
	if isSymlink(info) {
		return nil, false, oops.Owner("source").Wrap(fmt.Errorf("%w: %s", ErrSymlinkNotAllowed, absolute))
	}
	if !info.IsDir() {
		return nil, false, nil
	}
	return &LocalRootGuard{root: absolute, info: info}, true, nil
}

func (g *LocalRootGuard) Root() string {
	if g == nil {
		return ""
	}
	return g.root
}

func (g *LocalRootGuard) ReadFile(fullPath string) ([]byte, error) {
	file, _, err := g.OpenFile(fullPath)
	if err != nil {
		return nil, err
	}
	body, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil {
		return nil, oops.Wrap(readErr)
	}
	if closeErr != nil {
		return nil, oops.Wrap(closeErr)
	}
	return body, nil
}

func (g *LocalRootGuard) OpenFile(fullPath string) (*os.File, fs.FileInfo, error) {
	if g == nil {
		return nil, nil, oops.Owner("source").Wrap(errors.New("local source root guard is nil"))
	}
	relativePath, err := g.relativePath(fullPath)
	if err != nil {
		return nil, nil, err
	}
	rootDir, err := g.openValidatedRoot()
	if err != nil {
		return nil, nil, err
	}
	defer closeRoot(rootDir)
	return g.openStableFile(rootDir, relativePath, fullPath)
}

func (g *LocalRootGuard) openValidatedRoot() (*os.Root, error) {
	rootDir, err := os.OpenRoot(g.root)
	if err != nil {
		return nil, oops.Wrap(err)
	}
	if err := g.validateCurrentRoot(rootDir); err != nil {
		closeRoot(rootDir)
		return nil, err
	}
	return rootDir, nil
}

func (g *LocalRootGuard) openStableFile(rootDir *os.Root, relativePath, fullPath string) (*os.File, fs.FileInfo, error) {
	info, err := g.lstatRegularFile(rootDir, relativePath, fullPath)
	if err != nil {
		return nil, nil, err
	}
	file, err := rootDir.Open(filepath.FromSlash(relativePath))
	if err != nil {
		return nil, nil, oops.Wrap(err)
	}
	openedInfo, err := file.Stat()
	if err != nil {
		discardClose(file)
		return nil, nil, oops.Wrap(err)
	}
	if openedInfo.IsDir() {
		discardClose(file)
		return nil, nil, oops.Owner("source").Wrap(fmt.Errorf("source path is a directory: %s", fullPath))
	}
	if !os.SameFile(info, openedInfo) {
		discardClose(file)
		return nil, nil, oops.Owner("source").Wrap(fmt.Errorf("%w: %s", ErrRootReplaced, fullPath))
	}
	return file, openedInfo, nil
}

func (g *LocalRootGuard) lstatRegularFile(rootDir *os.Root, relativePath, fullPath string) (fs.FileInfo, error) {
	info, err := lstatPathWithinRoot(rootDir, g.root, relativePath)
	if err != nil {
		return nil, oops.Wrap(err)
	}
	if info.IsDir() {
		return nil, oops.Owner("source").Wrap(fmt.Errorf("source path is a directory: %s", fullPath))
	}
	return info, nil
}

func (g *LocalRootGuard) validateCurrentRoot(rootDir *os.Root) error {
	openedInfo, err := rootDir.Stat(".")
	if err != nil {
		return oops.Wrap(err)
	}
	currentInfo, err := os.Lstat(g.root)
	if err != nil {
		return oops.Wrap(err)
	}
	if err := validateOpenedDirectoryRoot(g.root, openedInfo, currentInfo); err != nil {
		return err
	}
	if !os.SameFile(g.info, currentInfo) {
		return oops.Owner("source").Wrap(fmt.Errorf("%w: %s", ErrRootReplaced, g.root))
	}
	return nil
}

func (g *LocalRootGuard) relativePath(fullPath string) (string, error) {
	absolute, err := filepath.Abs(filepath.Clean(strings.TrimSpace(fullPath)))
	if err != nil {
		return "", oops.Wrapf(err, "resolve source path")
	}
	relativePath, err := filepath.Rel(g.root, absolute)
	if err != nil {
		return "", oops.Wrapf(err, "resolve source relative path")
	}
	if relativePath == "." || filepath.IsAbs(relativePath) || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return "", oops.Owner("source").Wrap(fmt.Errorf("source path escaped root: %s", fullPath))
	}
	cleaned, ok := cleanRelativeAssetPath(filepath.ToSlash(relativePath))
	if !ok {
		return "", oops.Owner("source").Wrap(fmt.Errorf("invalid source path: %s", fullPath))
	}
	return cleaned, nil
}

func discardClose(file *os.File) {
	if file == nil {
		return
	}
	if err := file.Close(); err != nil {
		return
	}
}
