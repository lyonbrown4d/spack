// Package spackbundle reads and writes SPACK AOT asset bundles.
package spackbundle

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"
)

const (
	// IndexPath is the reserved path of the binary bundle index inside a SPACK archive.
	IndexPath = ".spack/index.bin"
	// FormatVersion is the current SPACK bundle format version.
	FormatVersion = "spack.io/aot/v1alpha1"
)

var indexMagic = []byte("SPACKIDX\x00")

// Index describes the files embedded in a SPACK bundle.
type Index struct {
	APIVersion string      `json:"apiVersion"`
	Kind       string      `json:"kind"`
	CreatedAt  time.Time   `json:"created_at"`
	Files      []IndexFile `json:"files"`
}

// IndexFile describes one file embedded in a SPACK bundle.
type IndexFile struct {
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	Size       int64  `json:"size"`
	MediaType  string `json:"media_type,omitempty"`
	SourceHash string `json:"source_hash,omitempty"`
	ETag       string `json:"etag,omitempty"`
	AssetPath  string `json:"asset_path,omitempty"`
	Encoding   string `json:"encoding,omitempty"`
	Format     string `json:"format,omitempty"`
	Width      int    `json:"width,omitempty"`
}

func marshalIndex(index Index) ([]byte, error) {
	index.APIVersion = FormatVersion
	index.Kind = "BundleIndex"
	payload, err := json.Marshal(index)
	if err != nil {
		return nil, fmt.Errorf("marshal bundle index: %w", err)
	}
	size, err := checkedUint32(len(payload))
	if err != nil {
		return nil, fmt.Errorf("bundle index is too large: %d bytes", len(payload))
	}

	body := make([]byte, 0, len(indexMagic)+4+len(payload))
	body = append(body, indexMagic...)
	body = binary.BigEndian.AppendUint32(body, size)
	body = append(body, payload...)
	return body, nil
}

func unmarshalIndex(body []byte) (Index, error) {
	if !bytes.HasPrefix(body, indexMagic) {
		return Index{}, errors.New("bundle index magic mismatch")
	}
	if len(body) < len(indexMagic)+4 {
		return Index{}, errors.New("bundle index is truncated")
	}
	size := binary.BigEndian.Uint32(body[len(indexMagic) : len(indexMagic)+4])
	payload := body[len(indexMagic)+4:]
	if uint64(len(payload)) != uint64(size) {
		return Index{}, errors.New("bundle index size mismatch")
	}

	var index Index
	if err := json.Unmarshal(payload, &index); err != nil {
		return Index{}, fmt.Errorf("decode bundle index: %w", err)
	}
	if index.APIVersion != FormatVersion {
		return Index{}, fmt.Errorf("unsupported bundle index version %q", index.APIVersion)
	}
	if index.Kind != "BundleIndex" {
		return Index{}, fmt.Errorf("unsupported bundle index kind %q", index.Kind)
	}
	return index, nil
}

func checkedUint32(value int) (uint32, error) {
	if value < 0 || value > math.MaxUint32 {
		return 0, errors.New("value exceeds uint32 range")
	}
	return uint32(value), nil
}
