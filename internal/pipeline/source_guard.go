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
	guard, err := pipelineSourceGuard(src, fullPath)
	if err != nil {
		return nil, err
	}
	body, err := guard.ReadFile(fullPath)
	if err != nil {
		return nil, oops.Wrapf(err, "read guarded pipeline source")
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
	guard, err := pipelineSourceGuard(src, fullPath)
	if err != nil {
		return 0, err
	}
	file, info, err := guard.OpenFile(fullPath)
	if err != nil {
		return 0, oops.Wrapf(err, "open guarded pipeline source")
	}
	if closeErr := file.Close(); closeErr != nil {
		return 0, oops.Wrapf(closeErr, "close guarded pipeline source")
	}
	if info == nil {
		return 0, oops.In("pipeline").Owner("source guard").Wrap(errors.New("pipeline source info is nil"))
	}
	return info.Size(), nil
}

func pipelineSourceGuard(src *source.LocalFS, fullPath string) (*source.LocalRootGuard, error) {
	if src != nil {
		guard, ok, err := src.RootGuard()
		if err != nil {
			return nil, oops.Wrapf(err, "create local source root guard")
		}
		if ok && guard != nil {
			return guard, nil
		}
	}
	root := filepath.Dir(strings.TrimSpace(fullPath))
	guard, ok, err := source.NewLocalRootGuard(root)
	if err != nil {
		return nil, oops.Wrapf(err, "create fallback source root guard")
	}
	if !ok || guard == nil {
		return nil, oops.In("pipeline").Owner("source guard").Wrap(errors.New("local source root guard is required"))
	}
	return guard, nil
}
