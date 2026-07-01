package contentcoding_test

import (
	"bytes"
	"compress/gzip"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	"github.com/lyonbrown4d/spack/internal/contentcoding"
)

func TestIsValidPayloadBoundaries(t *testing.T) {
	validGzip := gzipPayloadForTest(t, []byte("console.log('ok')"))
	validBrotli := brotliPayloadForTest(t, []byte("body"))
	validZstd := zstdPayloadForTest(t, []byte("body"))

	cases := []struct {
		name     string
		encoding string
		payload  []byte
		want     bool
	}{
		{name: "empty payload", encoding: "gzip", payload: nil, want: false},
		{name: "unknown encoding", encoding: "deflate", payload: validGzip, want: false},
		{name: "zlib is not gzip", encoding: "gzip", payload: []byte{0x78, 0xda, 0x01, 0x02}, want: false},
		{name: "truncated gzip header", encoding: "gzip", payload: []byte{0x1f, 0x8b}, want: false},
		{name: "valid gzip", encoding: "gzip", payload: validGzip, want: true},
		{name: "valid gzip normalized", encoding: " GZip ", payload: validGzip, want: true},
		{name: "valid brotli", encoding: "br", payload: validBrotli, want: true},
		{name: "valid zstd", encoding: "zstd", payload: validZstd, want: true},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := contentcoding.IsValidPayload(tt.encoding, tt.payload); got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestDecodePayloadPrefixBoundaries(t *testing.T) {
	payload := gzipPayloadForTest(t, []byte("abcdef"))
	decoded, ok := contentcoding.DecodePayloadPrefix("gzip", payload, 3)
	if !ok {
		t.Fatal("expected gzip payload to decode")
	}
	if string(decoded) != "abc" {
		t.Fatalf("expected prefix abc, got %q", decoded)
	}

	if decoded, ok := contentcoding.DecodePayloadPrefix("gzip", payload, 0); ok || decoded != nil {
		t.Fatalf("expected zero max bytes to fail, got %#v", decoded)
	}
	if decoded, ok := contentcoding.DecodePayloadPrefix("gzip", []byte{0x78, 0xda}, 8); ok || decoded != nil {
		t.Fatalf("expected invalid gzip to fail, got %#v", decoded)
	}
	if decoded, ok := contentcoding.DecodePayloadPrefix("unknown", payload, 8); ok || decoded != nil {
		t.Fatalf("expected unknown encoding to fail, got %#v", decoded)
	}
}

func gzipPayloadForTest(t *testing.T, body []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	writer := gzip.NewWriter(&out)
	if _, err := writer.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func brotliPayloadForTest(t *testing.T, body []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	writer := brotli.NewWriter(&out)
	if _, err := writer.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func zstdPayloadForTest(t *testing.T, body []byte) []byte {
	t.Helper()
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded := encoder.EncodeAll(body, nil)
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}
	return encoded
}
