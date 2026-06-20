package source

import (
	"fmt"
	"io/fs"
	"os"

	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/samber/oops"
)

func (s *LocalFS) validateRoot() error {
	info, err := os.Lstat(s.root)
	if err != nil {
		return oops.Wrap(err)
	}
	if isSymlink(info) {
		return oops.Owner("source").Wrap(fmt.Errorf("%w: %s", ErrSymlinkNotAllowed, s.root))
	}
	if s.bundle != nil {
		return s.validateBundleRoot(info)
	}
	return s.validateDirectoryRoot(info)
}

func (s *LocalFS) validateBundleRoot(info fs.FileInfo) error {
	if !info.Mode().IsRegular() || !spackbundle.IsBundlePath(s.root) {
		return oops.Owner("source").Wrap(fmt.Errorf("assets root must be a .spack bundle: %s", s.root))
	}
	return s.validateStableRoot(info)
}

func (s *LocalFS) validateDirectoryRoot(info fs.FileInfo) error {
	if !info.IsDir() {
		return oops.Owner("source").Wrap(fmt.Errorf("assets root must be a directory: %s", s.root))
	}
	return s.validateStableRoot(info)
}

func (s *LocalFS) validateStableRoot(info fs.FileInfo) error {
	if s.rootInfo == nil || os.SameFile(s.rootInfo, info) {
		return nil
	}
	return oops.Owner("source").Wrap(fmt.Errorf("%w: %s", ErrRootReplaced, s.root))
}
