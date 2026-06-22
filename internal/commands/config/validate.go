package configcmd

import (
	"errors"
	"io/fs"
	"os"
	"strings"

	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/samber/oops"
)

func validateConfiguredAssetsRoot(root string) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return oops.In("config").Owner("validate").Wrap(errors.New("assets.root is required"))
	}

	info, err := os.Lstat(root)
	if err != nil {
		return oops.Wrapf(err, "stat assets.root")
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return oops.Errorf("assets.root symlink is not allowed: %s", root)
	}
	if info.IsDir() {
		return nil
	}
	if info.Mode().IsRegular() && spackbundle.IsBundlePath(root) {
		return validateBundleAssetsRoot(root)
	}
	return oops.Errorf("assets.root must be an existing directory or readable .spack bundle: %s", root)
}

func validateBundleAssetsRoot(root string) error {
	reader, err := spackbundle.OpenReader(root)
	if err != nil {
		return oops.Wrapf(err, "read assets.root bundle index")
	}
	_, indexErr := reader.Index()
	closeErr := reader.Close()
	if indexErr != nil {
		return oops.Wrapf(indexErr, "read assets.root bundle index")
	}
	if closeErr != nil {
		return oops.Wrapf(closeErr, "close assets.root bundle")
	}
	return nil
}
