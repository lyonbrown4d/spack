package pipeline

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/samber/oops"
)

func readPipelineSourceFile(src *source.LocalFS, fullPath string) ([]byte, error) {
	if spackbundle.IsReference(fullPath) {
		body, err := spackbundle.ReadReference(fullPath)
		if err != nil {
			return nil, oops.Wrapf(err, "read bundle source asset")
		}
		return body, nil
	}
	files, err := pipelineFileSource(src, fullPath)
	if err != nil {
		return nil, err
	}
	body, err := files.ReadFile(fullPath)
	if err != nil {
		return nil, oops.Wrapf(err, "read pipeline source")
	}
	return body, nil
}

func validatePipelineSourceFile(src *source.LocalFS, fullPath string) (int64, error) {
	if spackbundle.IsReference(fullPath) {
		body, err := spackbundle.ReadReference(fullPath)
		if err != nil {
			return 0, oops.Wrapf(err, "read bundle source asset")
		}
		return int64(len(body)), nil
	}
	files, err := pipelineFileSource(src, fullPath)
	if err != nil {
		return 0, err
	}
	file, info, err := files.OpenFile(fullPath)
	if err != nil {
		return 0, oops.Wrapf(err, "open pipeline source")
	}
	if closeErr := file.Close(); closeErr != nil {
		return 0, oops.Wrapf(closeErr, "close pipeline source")
	}
	if info == nil {
		return 0, oops.In("pipeline").Owner("source").Wrap(errors.New("pipeline source info is nil"))
	}
	return info.Size(), nil
}

func pipelineFileSource(src *source.LocalFS, fullPath string) (*source.LocalFS, error) {
	if src != nil && src.Root() != "" {
		return src, nil
	}
	root := filepath.Dir(strings.TrimSpace(fullPath))
	files, ok, err := source.NewLocalDirectory(root)
	if err != nil {
		return nil, oops.Wrapf(err, "create fallback pipeline source")
	}
	if !ok || files == nil {
		return nil, oops.In("pipeline").Owner("source").Wrap(errors.New("local file source is required"))
	}
	return files, nil
}
