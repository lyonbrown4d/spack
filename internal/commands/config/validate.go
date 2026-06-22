package configcmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/lyonbrown4d/spack/internal/spackbundle"
)

func validateConfiguredAssetsRoot(root string) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return errors.New("assets.root is required")
	}

	info, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("stat assets.root: %w", err)
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("assets.root symlink is not allowed: %s", root)
	}
	if info.IsDir() {
		return nil
	}
	if info.Mode().IsRegular() && spackbundle.IsBundlePath(root) {
		return validateBundleAssetsRoot(root)
	}
	return fmt.Errorf("assets.root must be an existing directory or readable .spack bundle: %s", root)
}

func validateBundleAssetsRoot(root string) error {
	reader, err := spackbundle.OpenReader(root)
	if err != nil {
		return fmt.Errorf("read assets.root bundle index: %w", err)
	}
	_, indexErr := reader.Index()
	closeErr := reader.Close()
	if indexErr != nil {
		return fmt.Errorf("read assets.root bundle index: %w", indexErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close assets.root bundle: %w", closeErr)
	}
	return nil
}
