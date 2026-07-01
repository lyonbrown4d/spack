package pkg_test

import (
	"encoding/base64"
	"testing"

	"github.com/lyonbrown4d/spack/internal/constant"
	"github.com/lyonbrown4d/spack/pkg"
)

func TestHasMatchingMagicBoundaries(t *testing.T) {
	png := decodeBase64ForMimeTest(t, "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMB/atX+9kAAAAASUVORK5CYII=")
	jpeg := []byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43, 0x00}
	webp := []byte("RIFF\x10\x00\x00\x00WEBPVP8 ")

	cases := []struct {
		name string
		path string
		body []byte
		want bool
	}{
		{name: "png matches png", path: "hero.png", body: png, want: true},
		{name: "png rejects text", path: "hero.png", body: []byte("not a png"), want: false},
		{name: "jpg accepts jpeg magic", path: "hero.jpg", body: jpeg, want: true},
		{name: "jpeg rejects png", path: "hero.jpeg", body: png, want: false},
		{name: "webp matches webp", path: "hero.webp", body: webp, want: true},
		{name: "non magic extension passes", path: "app.js", body: []byte("not magic checked"), want: true},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := pkg.HasMatchingMagic(tt.path, tt.body); got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestRequiresMagicValidationBoundaries(t *testing.T) {
	for _, path := range []string{"hero.png", "hero.jpg", "hero.jpeg", "hero.webp", "hero.avif", "hero.gif"} {
		if !pkg.RequiresMagicValidation(path) {
			t.Fatalf("expected %s to require magic validation", path)
		}
	}
	for _, path := range []string{"app.js", "style.css", "index.html", "data.json", "font.woff2"} {
		if pkg.RequiresMagicValidation(path) {
			t.Fatalf("expected %s to skip magic validation", path)
		}
	}
}

func TestDetectMIMEBytesBoundaries(t *testing.T) {
	png := decodeBase64ForMimeTest(t, "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMB/atX+9kAAAAASUVORK5CYII=")
	if got := pkg.DetectMIMEBytes(png); got != constant.Png {
		t.Fatalf("expected PNG MIME, got %q", got)
	}
	if got := pkg.DetectMIMEBytes(nil); got != constant.OctetStream {
		t.Fatalf("expected octet-stream for empty body, got %q", got)
	}
}

func decodeBase64ForMimeTest(t *testing.T, value string) []byte {
	t.Helper()
	body, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
