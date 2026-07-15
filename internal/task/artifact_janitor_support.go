package task

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/samber/oops"
)

func openArtifactRoot(root string) (*os.Root, error) {
	rootHandle, err := openValidatedDirectoryRoot(root, "artifact janitor")
	if err == nil {
		return rootHandle, nil
	}
	if os.IsNotExist(err) {
		return nil, os.ErrNotExist
	}
	return nil, oops.In("task").Owner("artifact janitor").Wrap(err)
}

func (r *artifactJanitorRun) visitArtifactPath(relativePath string, entry fs.DirEntry, walkErr error) error {
	if walkErr != nil {
		return walkErr
	}
	if ctxErr := r.contextErr(); ctxErr != nil {
		return ctxErr
	}
	if entry.IsDir() {
		return nil
	}

	artifactPath := filepath.Join(r.root, filepath.FromSlash(relativePath))
	if r.report != nil {
		r.report.ScannedArtifacts++
	}
	if r.expected != nil && r.expected.Contains(artifactPath) {
		return nil
	}
	return r.removeOrphanArtifact(relativePath, artifactPath)
}

func artifactExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func variantArtifactPath(variant *catalog.Variant) string {
	if variant == nil {
		return ""
	}
	return strings.TrimSpace(variant.ArtifactPath)
}

func (r *artifactJanitorRun) contextErr() error {
	if r == nil || r.ctx == nil {
		return nil
	}
	if err := r.ctx.Err(); err != nil {
		return oops.In("task").Owner("artifact janitor").Wrap(err)
	}
	return nil
}

func closeRoot(root *os.Root) error {
	if root == nil {
		return nil
	}
	if err := root.Close(); err != nil {
		return oops.In("task").Owner("artifact janitor").Wrap(err)
	}
	return nil
}
