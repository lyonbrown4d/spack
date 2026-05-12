package pkg

import (
	"mime"
	"os"
	"path"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	"github.com/lyonbrown4d/spack/internal/constant"
)

func DetectMIME(filePath string) constant.MimeType {
	if detected, ok := detectMIMEByExtension(filePath); ok {
		return detected
	}
	if detected, ok := detectMIMEByContent(filePath); ok {
		return detected
	}
	if detected, ok := detectMIMEByStdlib(filePath); ok {
		return detected
	}
	return constant.OctetStream
}

func detectMIMEByExtension(filePath string) (constant.MimeType, bool) {
	switch strings.ToLower(path.Ext(filePath)) {
	case ".js":
		return constant.ApplicationJavascript, true
	case ".css":
		return constant.CSS, true
	case ".html", ".htm":
		return constant.HTML, true
	case ".json":
		return constant.JSON, true
	case ".svg":
		return constant.Svg, true
	default:
		return "", false
	}
}

func detectMIMEByContent(filePath string) (constant.MimeType, bool) {
	// #nosec G304 -- MIME detection is performed on local asset files only.
	f, err := os.Open(filePath)
	if err != nil {
		return "", false
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			return
		}
	}()

	mtype, err := mimetype.DetectReader(f)
	if err != nil || mtype == nil {
		return "", false
	}

	return detectKnownBinaryMIME(normalizeMIME(mtype.String()))
}

func detectKnownBinaryMIME(raw string) (constant.MimeType, bool) {
	detected := constant.MimeType(raw)
	if detected == constant.Png || detected == constant.Jpeg || detected == constant.Svg {
		return detected, true
	}
	return "", false
}

func detectMIMEByStdlib(filePath string) (constant.MimeType, bool) {
	normalized := normalizeMIME(mime.TypeByExtension(strings.ToLower(path.Ext(filePath))))
	if normalized == "" {
		return "", false
	}
	return constant.MimeType(normalized), true
}

func normalizeMIME(raw string) string {
	if idx := strings.Index(raw, ";"); idx > 0 {
		return raw[:idx]
	}
	return raw
}
