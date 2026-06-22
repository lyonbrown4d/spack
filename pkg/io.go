// Package pkg contains small shared helpers used across packages.
package pkg

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"

	"github.com/samber/oops"
)

func HashFile(path string) (string, error) {
	// #nosec G304 -- paths come from the scanned local asset tree.
	file, err := os.Open(path)
	if err != nil {
		return "", oops.Wrapf(err, "open file for hashing")
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			return
		}
	}()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", oops.Wrapf(err, "copy file into hasher")
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
