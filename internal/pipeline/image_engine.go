package pipeline

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"

	"github.com/anthonynsimon/bild/transform"
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/media"
)

type imageEncodeOptions struct {
	JPEGQuality int
}

type imageGenerateRequest struct {
	SourcePath      string
	SourceMediaType string
	TargetFormat    string
	TargetWidth     int
	Encode          imageEncodeOptions
}

type imageGenerateResult struct {
	Payload     []byte
	Width       int
	SourceWidth int
	MediaType   string
	Extension   string
}

type imageEngine interface {
	Name() string
	SupportsSourceMediaType(mediaType string) bool
	SupportedTargetFormats() *cxlist.List[string]
	Generate(request imageGenerateRequest) (imageGenerateResult, error)
}

func newImageEngine() imageEngine {
	return builtinImageEngine{}
}

type builtinImageEngine struct{}

func (builtinImageEngine) Name() string {
	return "builtin"
}

func (builtinImageEngine) SupportsSourceMediaType(mediaType string) bool {
	switch media.ImageFormat(mediaType) {
	case "jpeg", "png":
		return true
	default:
		return false
	}
}

func (builtinImageEngine) SupportedTargetFormats() *cxlist.List[string] {
	return cxlist.NewList[string]("jpeg", "png")
}

func (builtinImageEngine) Generate(request imageGenerateRequest) (imageGenerateResult, error) {
	srcImage, sourceWidth, sourceHeight, err := loadBuiltinSourceImage(request.SourcePath)
	if err != nil {
		return imageGenerateResult{}, err
	}
	if sourceWidth <= 0 || sourceHeight <= 0 {
		return imageGenerateResult{}, ErrVariantSkipped
	}

	outputImage, outputWidth := resizeBuiltinImage(srcImage, sourceWidth, sourceHeight, request.TargetWidth)
	payload, ext, mediaType, err := encodeBuiltinImage(outputImage, request.TargetFormat, request.Encode)
	if err != nil {
		return imageGenerateResult{}, err
	}
	return imageGenerateResult{
		Payload:     payload,
		Width:       outputWidth,
		SourceWidth: sourceWidth,
		MediaType:   mediaType,
		Extension:   ext,
	}, nil
}

func loadBuiltinSourceImage(path string) (_ image.Image, _, _ int, err error) {
	// #nosec G304 -- path comes from scanned assets rooted under configured sources.
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("open source image: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close source image: %w", closeErr)
		}
	}()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("decode source image: %w", err)
	}

	bounds := img.Bounds()
	return img, bounds.Dx(), bounds.Dy(), nil
}

func resizeBuiltinImage(srcImage image.Image, srcWidth, srcHeight, targetWidth int) (image.Image, int) {
	if targetWidth <= 0 || targetWidth >= srcWidth {
		return srcImage, srcWidth
	}

	targetHeight := max(1, srcHeight*targetWidth/srcWidth)
	return transform.Resize(srcImage, targetWidth, targetHeight, transform.CatmullRom), targetWidth
}

func encodeBuiltinImage(img image.Image, format string, opts imageEncodeOptions) ([]byte, string, string, error) {
	descriptor, ok := media.LookupImageDescriptor(media.NormalizeImageFormat(format))
	if !ok {
		return nil, "", "", fmt.Errorf("unsupported image format: %s", format)
	}

	var buffer bytes.Buffer
	switch descriptor.Name {
	case "jpeg":
		if err := jpeg.Encode(&buffer, img, &jpeg.Options{Quality: clampJPEGQuality(opts.JPEGQuality)}); err != nil {
			return nil, "", "", fmt.Errorf("encode jpeg image: %w", err)
		}
	case "png":
		if err := png.Encode(&buffer, img); err != nil {
			return nil, "", "", fmt.Errorf("encode png image: %w", err)
		}
	default:
		return nil, "", "", fmt.Errorf("builtin engine does not support %s output", descriptor.Name)
	}

	return buffer.Bytes(), descriptor.Extension, descriptor.MediaType, nil
}

func clampJPEGQuality(quality int) int {
	if quality < 1 {
		return 1
	}
	if quality > 100 {
		return 100
	}
	return quality
}
