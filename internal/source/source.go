package source

import "time"

type File struct {
	Path       string
	FullPath   string
	Kind       string
	Size       int64
	IsDir      bool
	ModTime    time.Time
	MediaType  string
	SourceHash string
	ETag       string
	AssetPath  string
	Encoding   string
	Format     string
	Width      int
}

type ChangeEvent struct {
	Path     string
	FullPath string
	Op       string
}
