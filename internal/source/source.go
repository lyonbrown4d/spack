package source

import (
	"context"
	"errors"
	"time"
)

var ErrWatchUnsupported = errors.New("source watch unsupported")

type File struct {
	Path     string
	FullPath string
	Size     int64
	IsDir    bool
	ModTime  time.Time
}

type Source interface {
	Walk(func(File) error) error
}

type FileFinder interface {
	FindFile(path string) (File, bool, error)
}

type ChangeEvent struct {
	Path     string
	FullPath string
	Op       string
}

type Watcher interface {
	Watch(context.Context) (<-chan ChangeEvent, error)
}
