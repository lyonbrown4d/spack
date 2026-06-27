package spackbundle

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/samber/oops"
)

func normalizedBundlePath(bundlePath string) (string, error) {
	bundlePath = strings.TrimSpace(bundlePath)
	if bundlePath == "" {
		return "", oops.In("spackbundle").Owner("read").Wrap(errors.New("bundle path is required"))
	}
	absolute, err := filepath.Abs(filepath.Clean(bundlePath))
	if err != nil {
		return "", oops.Wrapf(err, "resolve bundle path")
	}
	return absolute, nil
}
