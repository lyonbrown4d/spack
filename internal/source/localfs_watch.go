package source

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/samber/oops"
)

func (s *LocalFS) Watch(ctx context.Context) (<-chan ChangeEvent, error) {
	if s.bundle != nil {
		return nil, oops.Owner("source").Wrap(errors.New("source bundle extraction cannot be watched"))
	}
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
	return s.Walk(func(file File) error {
		if !file.IsDir {
			return nil
		}
		if err := watcher.Add(file.FullPath); err != nil {
			return oops.Wrap(err)
		}
		return nil
	})
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
	relativePath, ok := s.relativeWatchPath(fullPath)
	if !ok || relativePath == "." {
		return
	}
	rootDir, err := s.openRoot()
	if err != nil {
		return
	}
	defer closeRoot(rootDir)
	info, err := lstatPathWithinRoot(rootDir, s.root, relativePath)
	if err != nil || !info.IsDir() {
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
	relativePath, ok := s.relativeWatchPath(event.Name)
	if !ok {
		return ChangeEvent{}, false
	}
	return ChangeEvent{
		Path:     relativePath,
		FullPath: event.Name,
		Op:       event.Op.String(),
	}, true
}

func (s *LocalFS) relativeWatchPath(fullPath string) (string, bool) {
	rel, err := filepath.Rel(s.root, fullPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func isContentWatchEvent(event fsnotify.Event) bool {
	return event.Op.Has(fsnotify.Create) ||
		event.Op.Has(fsnotify.Write) ||
		event.Op.Has(fsnotify.Remove) ||
		event.Op.Has(fsnotify.Rename)
}
