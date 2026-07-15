package artifact

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/samber/oops"
)

var (
	ErrUnsafePath     = errors.New("unsafe artifact path")
	ErrRootReplaced   = errors.New("artifact root was replaced")
	ErrRootSymlink    = errors.New("artifact root symlink not allowed")
	ErrSymlinkSegment = errors.New("artifact path symlink not allowed")

	errArtifactParentNotDirectory = errors.New("artifact parent is not a directory")
	errArtifactTargetDirectory    = errors.New("artifact target is a directory")
)

type Store interface {
	Root() string
	PathFor(assetPath, sourceHash, namespace, suffix string) (string, error)
	Write(path string, data []byte) error
}

type LocalStore struct {
	root string
	info fs.FileInfo
}

func newLocalStore(root string) (Store, error) {
	normalizedRoot, err := normalizeArtifactRoot(root)
	if err != nil {
		return nil, err
	}
	err = os.MkdirAll(normalizedRoot, 0o750)
	if err != nil {
		return nil, oops.In("artifact").Owner("store").With("root", normalizedRoot).Wrap(err)
	}
	info, err := os.Lstat(normalizedRoot)
	if err != nil {
		return nil, oops.In("artifact").Owner("store").With("root", normalizedRoot).Wrap(err)
	}
	if isSymlink(info) {
		return nil, oops.In("artifact").Owner("store").With("root", normalizedRoot).Wrap(fmt.Errorf("%w: %s", ErrRootSymlink, normalizedRoot))
	}
	if !info.IsDir() {
		return nil, oops.In("artifact").Owner("store").With("root", normalizedRoot).Wrap(fmt.Errorf("cache root is not a directory: %s", normalizedRoot))
	}
	rootDir, err := os.OpenRoot(normalizedRoot)
	if err != nil {
		return nil, oops.In("artifact").Owner("store").With("root", normalizedRoot).Wrap(err)
	}
	openedInfo, statErr := rootDir.Stat(".")
	closeErr := rootDir.Close()
	if statErr != nil {
		return nil, oops.In("artifact").Owner("store").With("root", normalizedRoot).Wrap(statErr)
	}
	if closeErr != nil {
		return nil, oops.In("artifact").Owner("store").With("root", normalizedRoot).Wrap(closeErr)
	}
	if !os.SameFile(info, openedInfo) {
		return nil, oops.In("artifact").Owner("store").With("root", normalizedRoot).Wrap(fmt.Errorf("%w: %s", ErrRootReplaced, normalizedRoot))
	}
	return &LocalStore{root: normalizedRoot, info: info}, nil
}

func normalizeArtifactRoot(root string) (string, error) {
	trimmed := strings.TrimSpace(root)
	if trimmed == "" {
		return "", oops.In("artifact").Owner("store").Wrap(errors.New("missing cache root"))
	}
	absolute, err := filepath.Abs(filepath.Clean(trimmed))
	if err != nil {
		return "", oops.In("artifact").Owner("store").With("root", trimmed).Wrap(err)
	}
	return absolute, nil
}

func (s *LocalStore) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}

func (s *LocalStore) PathFor(assetPath, sourceHash, namespace, suffix string) (string, error) {
	if s == nil {
		return "", oops.In("artifact").Owner("store").Wrap(errors.New("artifact store is nil"))
	}
	relativePath, err := artifactRelativePath(assetPath, sourceHash, namespace, suffix)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.root, filepath.FromSlash(relativePath)), nil
}

func (s *LocalStore) Write(artifactPath string, data []byte) error {
	relativePath, err := s.relativePath(artifactPath)
	if err != nil {
		return err
	}
	rootDir, err := s.openValidatedRoot()
	if err != nil {
		return err
	}
	defer discardRoot(rootDir)

	if err := ensureArtifactParent(rootDir, path.Dir(relativePath)); err != nil {
		return err
	}
	if err := rejectExistingArtifactSymlink(rootDir, relativePath); err != nil {
		return err
	}
	tmpPath := relativePath + ".tmp-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := rootDir.WriteFile(filepath.FromSlash(tmpPath), data, 0o600); err != nil {
		return fmt.Errorf("write artifact temp file: %w", err)
	}
	if err := rootDir.Rename(filepath.FromSlash(tmpPath), filepath.FromSlash(relativePath)); err != nil {
		if removeErr := rootDir.Remove(filepath.FromSlash(tmpPath)); removeErr != nil && !os.IsNotExist(removeErr) {
			return errors.Join(fmt.Errorf("rename artifact temp file: %w", err), fmt.Errorf("cleanup artifact temp file: %w", removeErr))
		}
		return fmt.Errorf("rename artifact temp file: %w", err)
	}
	return nil
}

func (s *LocalStore) relativePath(fullPath string) (string, error) {
	if s == nil {
		return "", oops.In("artifact").Owner("store").Wrap(errors.New("artifact store is nil"))
	}
	absolute, err := filepath.Abs(filepath.Clean(strings.TrimSpace(fullPath)))
	if err != nil {
		return "", oops.In("artifact").Owner("store").With("path", fullPath).Wrap(err)
	}
	relativePath, err := filepath.Rel(s.root, absolute)
	if err != nil {
		return "", oops.In("artifact").Owner("store").With("path", fullPath).Wrap(err)
	}
	if unsafeArtifactFilesystemPath(relativePath) {
		return "", oops.In("artifact").Owner("store").With("path", fullPath).Wrap(ErrUnsafePath)
	}
	cleaned := filepath.ToSlash(relativePath)
	if unsafeArtifactSlashPath(cleaned) {
		return "", oops.In("artifact").Owner("store").With("path", fullPath).Wrap(ErrUnsafePath)
	}
	return cleaned, nil
}

func (s *LocalStore) openValidatedRoot() (*os.Root, error) {
	rootDir, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, oops.In("artifact").Owner("store").With("root", s.root).Wrap(err)
	}
	openedInfo, err := rootDir.Stat(".")
	if err != nil {
		discardRoot(rootDir)
		return nil, oops.In("artifact").Owner("store").With("root", s.root).Wrap(err)
	}
	currentInfo, err := os.Lstat(s.root)
	if err != nil {
		discardRoot(rootDir)
		return nil, oops.In("artifact").Owner("store").With("root", s.root).Wrap(err)
	}
	if isSymlink(currentInfo) || !currentInfo.IsDir() || !os.SameFile(openedInfo, currentInfo) || !os.SameFile(s.info, currentInfo) {
		discardRoot(rootDir)
		return nil, oops.In("artifact").Owner("store").With("root", s.root).Wrap(fmt.Errorf("%w: %s", ErrRootReplaced, s.root))
	}
	return rootDir, nil
}

func ensureArtifactParent(rootDir *os.Root, relativeDir string) error {
	if relativeDir == "." || relativeDir == "" {
		return nil
	}
	current := ""
	for segment := range strings.SplitSeq(relativeDir, "/") {
		current = nextArtifactParentPath(current, segment)
		if current == "" {
			continue
		}
		if err := ensureArtifactParentSegment(rootDir, current); err != nil {
			return err
		}
	}
	return nil
}

func nextArtifactParentPath(current, segment string) string {
	if segment == "" {
		return current
	}
	return path.Join(current, segment)
}

func ensureArtifactParentSegment(rootDir *os.Root, current string) error {
	info, err := rootDir.Lstat(filepath.FromSlash(current))
	switch {
	case err == nil:
		return validateExistingArtifactParent(info, current)
	case os.IsNotExist(err):
		return createArtifactParent(rootDir, current)
	default:
		return oops.In("artifact").Owner("store").With("path", current).Wrap(err)
	}
}

func validateExistingArtifactParent(info fs.FileInfo, current string) error {
	switch {
	case isSymlink(info):
		return oops.In("artifact").Owner("store").With("path", current).Wrap(ErrSymlinkSegment)
	case !info.IsDir():
		return oops.In("artifact").Owner("store").With("path", current).Wrap(errArtifactParentNotDirectory)
	default:
		return nil
	}
}

func createArtifactParent(rootDir *os.Root, current string) error {
	err := rootDir.Mkdir(filepath.FromSlash(current), 0o750)
	if err == nil || os.IsExist(err) {
		return nil
	}
	return oops.In("artifact").Owner("store").With("path", current).Wrap(err)
}

func rejectExistingArtifactSymlink(rootDir *os.Root, relativePath string) error {
	info, err := rootDir.Lstat(filepath.FromSlash(relativePath))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return oops.In("artifact").Owner("store").With("path", relativePath).Wrap(err)
	}
	if isSymlink(info) {
		return oops.In("artifact").Owner("store").With("path", relativePath).Wrap(ErrSymlinkSegment)
	}
	if info.IsDir() {
		return oops.In("artifact").Owner("store").With("path", relativePath).Wrap(errArtifactTargetDirectory)
	}
	return nil
}

func isSymlink(info fs.FileInfo) bool {
	return info.Mode()&os.ModeSymlink != 0
}

func discardRoot(rootDir *os.Root) {
	if rootDir == nil {
		return
	}
	if err := rootDir.Close(); err != nil {
		return
	}
}
