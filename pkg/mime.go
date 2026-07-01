package pkg

import (
	"mime"
	"os"
	"path"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	"github.com/lyonbrown4d/spack/internal/constant"
)

var extensionMIMEs = map[string]constant.MimeType{
	".js":   constant.ApplicationJavascript,
	".mjs":  constant.ApplicationJavascript,
	".cjs":  constant.ApplicationJavascript,
	".css":  constant.CSS,
	".html": constant.HTML,
	".htm":  constant.HTML,
	".json": constant.JSON,
	".svg":  constant.Svg,
	".png":  constant.Png,
	".jpg":  constant.Jpeg,
	".jpeg": constant.Jpeg,
	".webp": constant.MimeType("image/webp"),
	".avif": constant.MimeType("image/avif"),
	".gif":  constant.MimeType("image/gif"),
	".wasm": constant.Wasm,
}

var binaryMagicMIMEs = map[string]constant.MimeType{
	string(constant.Png):  constant.Png,
	string(constant.Jpeg): constant.Jpeg,
	string(constant.Jpg):  constant.Jpg,
	string(constant.Svg):  constant.Svg,
	"image/webp":          constant.MimeType("image/webp"),
	"image/avif":          constant.MimeType("image/avif"),
	"image/gif":           constant.MimeType("image/gif"),
}

var magicValidatedExtensionMIMEs = map[string]constant.MimeType{
	".png":  constant.Png,
	".jpg":  constant.Jpeg,
	".jpeg": constant.Jpeg,
	".webp": constant.MimeType("image/webp"),
	".avif": constant.MimeType("image/avif"),
	".gif":  constant.MimeType("image/gif"),
}

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

// HasMatchingMagic reports whether content bytes match the trusted binary extension of filePath.
func HasMatchingMagic(filePath string, body []byte) bool {
	expected, ok := expectedMagicMIME(filePath)
	if !ok {
		return true
	}
	return mimeCompatible(expected, DetectMIMEBytes(body))
}

// RequiresMagicValidation reports whether filePath has a binary extension with a stable magic signature.
func RequiresMagicValidation(filePath string) bool {
	_, ok := expectedMagicMIME(filePath)
	return ok
}

// DetectMIMEBytes detects a MIME type from raw bytes without trusting a file extension.
func DetectMIMEBytes(body []byte) constant.MimeType {
	if len(body) == 0 {
		return constant.OctetStream
	}
	mtype := mimetype.Detect(body)
	if mtype == nil {
		return constant.OctetStream
	}
	normalized := normalizeMIME(mtype.String())
	if detected, ok := detectKnownBinaryMIME(normalized); ok {
		return detected
	}
	if normalized == "" {
		return constant.OctetStream
	}
	return constant.MimeType(normalized)
}

func detectMIMEByExtension(filePath string) (constant.MimeType, bool) {
	detected, ok := extensionMIMEs[strings.ToLower(path.Ext(filePath))]
	return detected, ok
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
	detected, ok := binaryMagicMIMEs[raw]
	return detected, ok
}

func detectMIMEByStdlib(filePath string) (constant.MimeType, bool) {
	normalized := normalizeMIME(mime.TypeByExtension(strings.ToLower(path.Ext(filePath))))
	if normalized == "" {
		return "", false
	}
	return constant.MimeType(normalized), true
}

func expectedMagicMIME(filePath string) (constant.MimeType, bool) {
	expected, ok := magicValidatedExtensionMIMEs[strings.ToLower(path.Ext(filePath))]
	return expected, ok
}

func mimeCompatible(expected, detected constant.MimeType) bool {
	if expected == detected {
		return true
	}
	return (expected == constant.Jpeg || expected == constant.Jpg) && (detected == constant.Jpeg || detected == constant.Jpg)
}

func normalizeMIME(raw string) string {
	if idx := strings.Index(raw, ";"); idx > 0 {
		return raw[:idx]
	}
	return raw
}
