package pipeline

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
	"github.com/samber/oops"
)

type pipelineLocalDirectoryFactory func(string) (*source.LocalFS, bool, error)

func readPipelineSourceFile(src *source.LocalFS, fullPath string) ([]byte, error) {
	return readPipelineSourceFileWithFactory(src, fullPath, source.NewLocalDirectory)
}

func readPipelineSourceFileWithFactory(
	src *source.LocalFS,
	fullPath string,
	newLocalDirectory pipelineLocalDirectoryFactory,
) (body []byte, err error) {
	if spackbundle.IsReference(fullPath) {
		body, err = spackbundle.ReadReference(fullPath)
		if err != nil {
			return nil, oops.Wrapf(err, "read bundle source asset")
		}
		return body, nil
	}
	files, owned, err := pipelineFileSource(src, fullPath, newLocalDirectory)
	if err != nil {
		return nil, err
	}
	defer func() {
		if !owned {
			return
		}
		err = joinPipelineSourceCleanupError(err, files.Cleanup())
	}()

	body, err = files.ReadFile(fullPath)
	if err != nil {
		return nil, oops.Wrapf(err, "read pipeline source")
	}
	return body, nil
}

func validatePipelineSourceFile(src *source.LocalFS, fullPath string) (int64, error) {
	return validatePipelineSourceFileWithFactory(src, fullPath, source.NewLocalDirectory)
}

func validatePipelineSourceFileWithFactory(
	src *source.LocalFS,
	fullPath string,
	newLocalDirectory pipelineLocalDirectoryFactory,
) (size int64, err error) {
	if spackbundle.IsReference(fullPath) {
		var body []byte
		body, err = spackbundle.ReadReference(fullPath)
		if err != nil {
			return 0, oops.Wrapf(err, "read bundle source asset")
		}
		return int64(len(body)), nil
	}
	files, owned, err := pipelineFileSource(src, fullPath, newLocalDirectory)
	if err != nil {
		return 0, err
	}
	defer func() {
		if !owned {
			return
		}
		err = joinPipelineSourceCleanupError(err, files.Cleanup())
	}()

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

func pipelineFileSource(
	src *source.LocalFS,
	fullPath string,
	newLocalDirectory pipelineLocalDirectoryFactory,
) (*source.LocalFS, bool, error) {
	if src != nil {
		return src, false, nil
	}
	root := filepath.Dir(strings.TrimSpace(fullPath))
	files, ok, err := newLocalDirectory(root)
	if err != nil {
		return nil, false, oops.Wrapf(err, "create fallback pipeline source")
	}
	if !ok || files == nil {
		return nil, false, oops.In("pipeline").Owner("source").Wrap(errors.New("local file source is required"))
	}
	return files, true, nil
}

func joinPipelineSourceCleanupError(operationErr, cleanupErr error) error {
	if cleanupErr == nil {
		return operationErr
	}
	wrappedCleanupErr := oops.Wrapf(cleanupErr, "cleanup fallback pipeline source")
	if operationErr == nil {
		return wrappedCleanupErr
	}
	return errors.Join(operationErr, wrappedCleanupErr)
}
