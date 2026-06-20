package spackbundle

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const referencePrefix = "spackbundle:"

// Reference identifies a file inside a SPACK bundle.
type Reference struct {
	BundlePath string
	FilePath   string
}

// NewReference builds an opaque internal path for a bundled file.
func NewReference(bundlePath, filePath string) (string, error) {
	bundlePath, err := normalizedBundlePath(bundlePath)
	if err != nil {
		return "", err
	}
	filePath, err = cleanBundlePath(filePath)
	if err != nil {
		return "", err
	}
	return referencePrefix + encodeReferencePart(bundlePath) + "." + encodeReferencePart(filePath), nil
}

// IsReference reports whether path is a SPACK bundle internal reference.
func IsReference(path string) bool {
	return strings.HasPrefix(strings.TrimSpace(path), referencePrefix)
}

// ParseReference parses a SPACK bundle internal reference.
func ParseReference(path string) (Reference, error) {
	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, referencePrefix) {
		return Reference{}, errors.New("not a spack bundle reference")
	}
	body := strings.TrimPrefix(path, referencePrefix)
	left, right, ok := strings.Cut(body, ".")
	if !ok || left == "" || right == "" {
		return Reference{}, errors.New("invalid spack bundle reference")
	}
	bundlePath, err := decodeReferencePart(left)
	if err != nil {
		return Reference{}, fmt.Errorf("decode bundle reference path: %w", err)
	}
	filePath, err := decodeReferencePart(right)
	if err != nil {
		return Reference{}, fmt.Errorf("decode bundle reference file: %w", err)
	}
	filePath, err = cleanBundlePath(filePath)
	if err != nil {
		return Reference{}, err
	}
	return Reference{
		BundlePath: bundlePath,
		FilePath:   filePath,
	}, nil
}

// ReadReference reads a file addressed by a SPACK bundle internal reference.
func ReadReference(path string) ([]byte, error) {
	ref, err := ParseReference(path)
	if err != nil {
		return nil, err
	}
	return ReadFile(ref.BundlePath, ref.FilePath)
}

func encodeReferencePart(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func decodeReferencePart(value string) (string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return "", fmt.Errorf("decode reference part: %w", err)
	}
	return string(decoded), nil
}
