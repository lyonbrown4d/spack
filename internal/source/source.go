package source

import "time"

type File struct {
	Path       string
	FullPath   string
	Size       int64
	IsDir      bool
	ModTime    time.Time
	MediaType  string
	SourceHash string
	ETag       string
}

type ChangeEvent struct {
	Path     string
	FullPath string
	Op       string
}
