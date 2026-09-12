//go:build windows

package source

import (
	"errors"
	"os"

	"github.com/samber/oops"
)

func prepareLocalFSRoot(rootDir **os.Root) error {
	if rootDir == nil || *rootDir == nil {
		return nil
	}
	if err := (*rootDir).Close(); err != nil {
		return oops.Wrap(err)
	}
	*rootDir = nil
	return nil
}

func (r *localFSResourceState) useRoot(use func(*os.Root) error) (err error) {
	rootDir, err := os.OpenRoot(r.root)
	if err != nil {
		return oops.Wrap(err)
	}
	defer func() {
		if closeErr := rootDir.Close(); closeErr != nil {
			err = errors.Join(err, oops.Wrap(closeErr))
		}
	}()

	if err := validateCurrentLocalFSRoot(rootDir, r.root, r.rootInfo); err != nil {
		return err
	}
	return use(rootDir)
}

func (r *localFSResourceState) closeRoot() error {
	return nil
}
