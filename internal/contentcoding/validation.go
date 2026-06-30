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

	switch strings.TrimSpace(strings.ToLower(encoding)) {
	case "gzip":
		return isValidGzipPayload(sample)
	case "zstd":
		return isValidZstdPayload(sample)
	case "br":
		return isValidBrotliPayload(sample)
	default:
		return false
	}
}

func isValidGzipPayload(sample []byte) bool {
	reader, err := gzip.NewReader(bytes.NewReader(sample))
	if err != nil {
		return false
	}
	return reader.Close() == nil
}

func isValidZstdPayload(sample []byte) bool {
	var header zstd.Header
	return header.Decode(sample) == nil
}

func isValidBrotliPayload(sample []byte) bool {
	reader := brotli.NewReader(bytes.NewReader(sample))
	buf := make([]byte, 1)
	_, err := reader.Read(buf)
	return err == nil || errors.Is(err, io.EOF)
}
