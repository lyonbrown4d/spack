package source

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/samber/oops"
)

func (s *LocalFS) Watch(ctx context.Context) (<-chan ChangeEvent, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, oops.Wrap(err)
	}
	if err := s.addWatchDirs(watcher); err != nil {
		s.closeWatcher(watcher)
		return nil, err
	}

	changes := make(chan ChangeEvent, 1)
	go s.watchLoop(ctx, watcher, changes)
	return changes, nil
}

func (s *LocalFS) addWatchDirs(watcher *fsnotify.Watcher) error {
	if err := s.validateRoot(); err != nil {
		return err
	}
	if err := filepath.WalkDir(s.root, func(fullPath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return oops.Owner("source").Wrap(fmt.Errorf("%w: %s", ErrSymlinkNotAllowed, fullPath))
		}
		if !entry.IsDir() {
			return nil
		}
		return watcher.Add(fullPath)
	}); err != nil {
		return oops.Wrap(err)
	}
	return nil
}

func (s *LocalFS) watchLoop(ctx context.Context, watcher *fsnotify.Watcher, changes chan<- ChangeEvent) {
	defer close(changes)
	defer s.closeWatcher(watcher)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			s.handleWatchEvent(watcher, changes, event)
		case err, ok := <-watcher.Errors:
			if !s.handleWatchError(err, ok) {
				return
			}
		}
	}
}

func (s *LocalFS) closeWatcher(watcher *fsnotify.Watcher) {
	if err := watcher.Close(); err != nil && s.logger != nil {
		s.logger.Debug("Close source watcher failed", slog.String("err", err.Error()))
	}
}

func (s *LocalFS) handleWatchError(err error, ok bool) bool {
	if !ok {
		return false
	}
	if err != nil && s.logger != nil {
		s.logger.Warn("Source watcher error", slog.String("err", err.Error()))
	}
	return true
}

func (s *LocalFS) handleWatchEvent(watcher *fsnotify.Watcher, changes chan<- ChangeEvent, event fsnotify.Event) {
	if event.Op.Has(fsnotify.Create) {
		s.addCreatedWatchDir(watcher, event.Name)
	}
	if !isContentWatchEvent(event) {
		return
	}

	change, ok := s.changeEvent(event)
	if !ok {
		return
	}
	select {
	case changes <- change:
	default:
	}
}

func (s *LocalFS) addCreatedWatchDir(watcher *fsnotify.Watcher, fullPath string) {
	if !isPathWithinRoot(s.root, fullPath) {
		return
	}
	info, err := os.Lstat(fullPath)
	if err != nil || isSymlink(info) || !info.IsDir() {
		return
	}
	if err := watcher.Add(fullPath); err != nil && s.logger != nil {
		s.logger.Warn("Add source watch directory failed",
			slog.String("path", fullPath),
			slog.String("err", err.Error()),
		)
	}
}

func (s *LocalFS) changeEvent(event fsnotify.Event) (ChangeEvent, bool) {
	rel, err := filepath.Rel(s.root, event.Name)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ChangeEvent{}, false
	}
	return ChangeEvent{
		Path:     filepath.ToSlash(rel),
		FullPath: event.Name,
		Op:       event.Op.String(),
	}, true
}

func isContentWatchEvent(event fsnotify.Event) bool {
	return event.Op.Has(fsnotify.Create) ||
		event.Op.Has(fsnotify.Write) ||
		event.Op.Has(fsnotify.Remove) ||
		event.Op.Has(fsnotify.Rename)
}
