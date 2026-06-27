package spackbundle

import (
	"archive/tar"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/klauspost/compress/zstd"
	"github.com/samber/oops"
)

type bundleStream struct {
	file      *os.File
	decoder   *zstd.Decoder
	tarReader *tar.Reader
}

func openBundleStream(bundlePath string) (*bundleStream, error) {
	absolute, err := normalizedBundlePath(bundlePath)
	if err != nil {
		return nil, err
	}
	file, err := openBundleFile(absolute)
	if err != nil {
		return nil, oops.Wrapf(err, "open bundle")
	}
	magic := make([]byte, len(bundleMagic))
	if _, readErr := io.ReadFull(file, magic); readErr != nil {
		discardError(file.Close())
		return nil, oops.Wrapf(readErr, "read bundle magic")
	}
	if string(magic) != bundleMagic {
		discardError(file.Close())
		return nil, oops.In("spackbundle").Owner("read").Wrap(errors.New("bundle magic mismatch"))
	}
	decoder, err := zstd.NewReader(file)
	if err != nil {
		discardError(file.Close())
		return nil, oops.Wrapf(err, "create bundle zstd reader")
	}
	return &bundleStream{
		file:      file,
		decoder:   decoder,
		tarReader: tar.NewReader(decoder),
	}, nil
}

func openBundleFile(absolute string) (*os.File, error) {
	dir, name := filepath.Split(absolute)
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, oops.Wrapf(err, "open bundle directory")
	}
	defer func() {
		discardError(root.Close())
	}()
	file, err := root.Open(name)
	if err != nil {
		return nil, oops.Wrapf(err, "open bundle file")
	}
	return file, nil
}

func (s *bundleStream) Close() error {
	if s == nil {
		return nil
	}
	if s.decoder != nil {
		s.decoder.Close()
	}
	if s.file == nil {
		return nil
	}
	if err := s.file.Close(); err != nil {
		return oops.Wrapf(err, "close bundle")
	}
	return nil
}

func checkBundleMagic(bundlePath string) error {
	stream, err := openBundleStream(bundlePath)
	if err != nil {
		return err
	}
	return stream.Close()
}
