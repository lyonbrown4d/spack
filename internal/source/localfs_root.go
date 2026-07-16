package source

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"
)

// NewLocalDirectory creates a local source rooted at an existing directory.
// It is intended for trusted local artifact directories outside the configured
// asset source, such as generated compression caches.
func NewLocalDirectory(root string) (*LocalFS, bool, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, false, nil
	}
	absolute, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return nil, false, oops.Wrapf(err, "resolve local directory")
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return nil, false, oops.Wrapf(err, "stat local directory")
	}
	if !info.IsDir() && !isSymlink(info) {
		return nil, false, nil
	}
	resolved, err := resolveLocalFSDirectoryRoot(absolute)
	if err != nil {
		return nil, false, err
	}
	return &LocalFS{
		root:     resolved.root,
		rootInfo: resolved.info,
		logger:   slog.New(slog.DiscardHandler),
	}, true, nil
}

// Root returns the resolved local filesystem root used for reads.
func (s *LocalFS) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}
