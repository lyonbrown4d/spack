//go:build !windows

package source

import (
	"io/fs"
	"os"
)

func prepareLocalFSRoot(_ **os.Root) error {
	return nil
}

func (r *localFSResourceState) useRoot(use func(*os.Root) error) error {
	if r.rootDir == nil {
		return fs.ErrClosed
	}
	return use(r.rootDir)
}

func (r *localFSResourceState) closeRoot() error {
	if r.rootDir == nil {
		return nil
	}
	return r.rootDir.Close()
}
