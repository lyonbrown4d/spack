package contentcoding

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"strings"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

const ValidationSampleBytes int64 = 64 * 1024

func IsValidPayload(encoding string, sample []byte) bool {
	if len(sample) == 0 {
		return false
	}
	reader, closeReader, ok := payloadReader(encoding, sample)
	if !ok {
		return false
	}
	defer closeReader()
	return canReadPayloadPrefix(reader)
}

// DecodePayloadPrefix returns a decoded prefix from a compressed payload sample.
func DecodePayloadPrefix(encoding string, sample []byte, maxBytes int64) ([]byte, bool) {
	if len(sample) == 0 || maxBytes <= 0 {
		return nil, false
	}
	reader, closeReader, ok := payloadReader(encoding, sample)
	if !ok {
		return nil, false
	}
	defer closeReader()

	body, err := io.ReadAll(io.LimitReader(reader, maxBytes))
	if err != nil && len(body) == 0 {
		return nil, false
	}
	return body, len(body) > 0
}

func payloadReader(encoding string, sample []byte) (io.Reader, func(), bool) {
	switch normalizedEncoding(encoding) {
	case "gzip":
		reader, err := gzip.NewReader(bytes.NewReader(sample))
		if err != nil {
			return nil, func() {}, false
		}
		return reader, func() {
			if err := reader.Close(); err != nil {
				return
			}
		}, true
	case "zstd":
		reader, err := zstd.NewReader(bytes.NewReader(sample))
		if err != nil {
			return nil, func() {}, false
		}
		return reader, reader.Close, true
	case "br":
		return brotli.NewReader(bytes.NewReader(sample)), func() {}, true
	default:
		return nil, func() {}, false
	}
}

func canReadPayloadPrefix(reader io.Reader) bool {
	buf := make([]byte, 1)
	_, err := reader.Read(buf)
	return err == nil || errors.Is(err, io.EOF)
}

func normalizedEncoding(encoding string) string {
	return strings.TrimSpace(strings.ToLower(encoding))
}
